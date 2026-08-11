package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/shopmonkeyus/go-common/compress"
	"github.com/shopmonkeyus/go-common/logger"
	gstring "github.com/shopmonkeyus/go-common/string"
	"github.com/vmihailenco/msgpack/v5"
)

const (
	maxDeliveryAttempts = 10
	// fetchMaxWait bounds how long a fetch waits for messages. It also bounds
	// how long the subscriber can go without noticing that its consumer was
	// deleted, since pull requests sent after the deletion are never answered
	fetchMaxWait     = 10 * time.Second
	resubscribeDelay = time.Second
)

// errSubscriberClosed is returned by subscription() when the subscriber was
// closed while a new subscription was being created
var errSubscriberClosed = errors.New("subscriber closed")

type Handler func(ctx context.Context, payload []byte, msg *nats.Msg) error

// Subscriber represents a nats subscriber
type Subscriber interface {
	// Close the subscriber and stop delivery
	Close() error
}

// inflight tracks the message currently being handled so the extender can
// keep extending its ack deadline while the handler runs
type inflight struct {
	msg     *nats.Msg
	msgid   string
	seq     uint64
	started time.Time
}

type subscriber struct {
	logger         logger.Logger
	newsub         func() (*nats.Subscription, error)
	handler        Handler
	ctx            context.Context
	cancel         context.CancelFunc
	extendInterval time.Duration
	maxfetch       int
	disableLog     bool
	wg             sync.WaitGroup

	lock     sync.Mutex // guards shutdown and sub
	shutdown bool
	sub      *nats.Subscription

	ackLock  sync.Mutex // guards inflight
	inflight *inflight
}

type subscriberOpts struct {
	ctx            context.Context
	logger         logger.Logger
	newsub         func() (*nats.Subscription, error)
	handler        Handler
	extendInterval time.Duration
	maxfetch       int
	disableLog     bool
}

var _ Subscriber = (*subscriber)(nil)

func newSubscriber(opts subscriberOpts) *subscriber {
	ctx, cancel := context.WithCancel(opts.ctx)
	if opts.extendInterval <= 0 {
		opts.extendInterval = time.Second * 28
	}
	if opts.maxfetch <= 0 {
		opts.maxfetch = 1
	}
	s := &subscriber{
		logger:         opts.logger,
		newsub:         opts.newsub,
		handler:        opts.handler,
		ctx:            ctx,
		cancel:         cancel,
		extendInterval: opts.extendInterval,
		maxfetch:       opts.maxfetch,
		disableLog:     opts.disableLog,
	}
	if sub, err := opts.newsub(); err == nil {
		s.sub = sub
	}
	s.wg.Add(2)
	go s.extender()
	go s.run()
	return s
}

// Close will shutdown subscriptions and wait for the subscriber to be shutdown
func (s *subscriber) Close() error {
	s.logger.Debug("subscriber closing")
	s.lock.Lock()
	s.shutdown = true
	s.lock.Unlock()
	s.cancel()      // stop the extender and cancel the handler context
	s.unsubscribe() // stop delivery and wake up a blocked fetch
	s.wg.Wait()     // wait for the run loop to nack pending messages if any
	s.logger.Debug("subscriber closed")
	return nil
}

func (s *subscriber) isShutdown() bool {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.shutdown
}

// unsubscribe drops the current subscription (if any) so the run loop will
// create a fresh one on its next iteration
func (s *subscriber) unsubscribe() {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.sub != nil {
		s.sub.Unsubscribe()
		s.sub = nil
	}
}

// subscription returns the current subscription, creating a new one if needed
func (s *subscriber) subscription() (*nats.Subscription, error) {
	s.lock.Lock()
	sub, shutdown := s.sub, s.shutdown
	s.lock.Unlock()
	if shutdown {
		return nil, errSubscriberClosed
	}
	if sub != nil {
		return sub, nil
	}
	s.logger.Trace("creating a new subscription")
	sub, err := s.newsub()
	if err != nil {
		return nil, err
	}
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.shutdown {
		// Close() ran its unsubscribe while we were creating this subscription,
		// so drop it here instead of leaving Close blocked on a dangling fetch
		sub.Unsubscribe()
		return nil, errSubscriberClosed
	}
	s.sub = sub
	return sub, nil
}

// sleep pauses for d or until the subscriber is closed, whichever comes first
func (s *subscriber) sleep(d time.Duration) {
	select {
	case <-s.ctx.Done():
	case <-time.After(d):
	}
}

// run fetches batches of messages and dispatches them to the handler until
// the subscriber is closed, recreating the subscription whenever it becomes
// unusable (leadership change, consumer deleted, connection loss, etc)
func (s *subscriber) run() {
	defer s.wg.Done()
	defer s.unsubscribe()
	for !s.isShutdown() {
		sub, err := s.subscription()
		if err != nil {
			if errors.Is(err, errSubscriberClosed) {
				return
			}
			// timeouts and connection errors are expected while nats is
			// reconnecting so don't log them
			if !errors.Is(err, nats.ErrTimeout) && !errors.Is(err, nats.ErrConnectionClosed) {
				s.logger.Error("error creating new subscription: %s", err)
			}
			s.sleep(resubscribeDelay)
			continue
		}
		msgs, err := sub.Fetch(s.maxfetch, nats.MaxWait(fetchMaxWait))
		if err != nil {
			switch {
			case s.isShutdown() || errors.Is(err, context.Canceled):
				return
			case errors.Is(err, nats.ErrTimeout) || errors.Is(err, context.DeadlineExceeded):
				// no messages arrived before the fetch wait expired. the server
				// only notifies fetches that were pending when the consumer was
				// deleted, so verify the consumer is still alive before fetching
				// again or we would keep fetching from a dead consumer forever
				if _, cerr := sub.ConsumerInfo(); cerr != nil {
					if !errors.Is(cerr, nats.ErrTimeout) && !errors.Is(cerr, nats.ErrConnectionClosed) {
						s.logger.Error("consumer is unavailable (%s), recreating the subscription", cerr)
					}
					s.unsubscribe()
					s.sleep(resubscribeDelay)
				}
			default:
				s.logger.Error("fetch failed (%s), recreating the subscription", err)
				s.unsubscribe()
				s.sleep(resubscribeDelay)
			}
			continue
		}
		for _, msg := range msgs {
			s.process(msg)
		}
	}
}

// process delivers a single message to the handler and acks/nacks it based on
// the handler result
func (s *subscriber) process(msg *nats.Msg) {
	// nack any message fetched after a shutdown started so it is redelivered
	if s.isShutdown() {
		msg.Nak()
		return
	}
	msgid := GetMsgIdFromHeader(msg)
	if msgid == "" {
		msgid = gstring.SHA256(msg.Data)
	}
	md, _ := msg.Metadata()
	logDetail := fmt.Sprintf("sub: %s, msgId: %s, consumerSeq: %v, streamSeq: %v, attempt: %d", msg.Subject, msgid, md.Sequence.Consumer, md.Sequence.Stream, md.NumDelivered)
	if md.NumDelivered > maxDeliveryAttempts {
		s.logger.Warn("terminating %s", logDetail)
		msg.Term() // no longer allow it to be reprocessed
		return
	}
	if !s.disableLog {
		s.logger.Debug("processing %s", logDetail)
	}
	data, err := decodePayload(msg)
	if err != nil {
		s.logger.Error("error uncompressing %s. err: %s", logDetail, err)
		msg.AckSync()
		return
	}

	// track the inflight message so the extender keeps its ack deadline alive
	// while the handler runs
	s.setInflight(&inflight{msg: msg, msgid: msgid, seq: md.Sequence.Consumer, started: time.Now()})
	err = s.handler(s.ctx, data, msg)
	s.setInflight(nil)

	if err == nil || strings.Contains(err.Error(), "message was already acknowledged") {
		return
	}
	if errors.Is(err, context.Canceled) {
		s.logger.Warn("nack %s [canceled]", logDetail)
		msg.Nak()
	} else {
		s.logger.Error("error handling %s. err: %s", logDetail, err)
		msg.AckSync()
	}
}

// decodePayload returns the message payload as JSON, decoding it based on the
// content encoding header
func decodePayload(msg *nats.Msg) ([]byte, error) {
	switch GetContentEncodingFromHeader(msg) {
	case "gzip/json":
		return compress.Gunzip(msg.Data)
	case "msgpack":
		var o any
		if err := msgpack.Unmarshal(msg.Data, &o); err != nil {
			return nil, err
		}
		return json.Marshal(o)
	default:
		return msg.Data, nil
	}
}

func (s *subscriber) setInflight(m *inflight) {
	s.ackLock.Lock()
	s.inflight = m
	s.ackLock.Unlock()
}

// extender extends the ack deadline of the inflight message on an interval so
// that long-running handlers don't get their message redelivered
func (s *subscriber) extender() {
	defer s.wg.Done()
	t := time.NewTicker(s.extendInterval)
	defer t.Stop()
	for {
		select {
		case <-s.ctx.Done():
			s.nackInflight()
			return
		case <-t.C:
			s.extendInflight()
		}
	}
}

func (s *subscriber) nackInflight() {
	s.ackLock.Lock()
	defer s.ackLock.Unlock()
	if m := s.inflight; m != nil {
		s.logger.Info("nack message %s (%v/%d) [canceled]", m.msg.Subject, m.msgid, m.seq)
		m.msg.Nak()
		s.inflight = nil
	}
}

func (s *subscriber) extendInflight() {
	s.ackLock.Lock()
	defer s.ackLock.Unlock()
	m := s.inflight
	if m == nil {
		return
	}
	if !s.disableLog {
		s.logger.Debug("extending %s ack timeout (%s/%d) running %v", m.msg.Subject, m.msgid, m.seq, time.Since(m.started))
	}
	if err := m.msg.InProgress(); err != nil {
		s.logger.Error("error extending in progress %s (%s/%d): %v", m.msg.Subject, m.msgid, m.seq, err)
	}
}

// durablePullOpts describes a subscriber bound to a stream's durable pull
// consumer
type durablePullOpts struct {
	ctx        context.Context
	logger     logger.Logger
	js         nats.JetStreamContext
	stream     string
	consumer   *nats.ConsumerConfig
	subOpts    []nats.SubOpt
	handler    Handler
	maxfetch   int
	disableLog bool
	logPrefix  string
}

// newDurablePullSubscriber ensures the stream's durable consumer exists and
// returns a subscriber that pulls from it. The consumer is recreated whenever
// the subscription is re-established in case it went away (e.g. deleted during
// a leadership change), so the pull subscription always binds to an existing
// consumer instead of creating one it would later delete.
func newDurablePullSubscriber(opts durablePullOpts) (Subscriber, error) {
	if err := ensureConsumer(opts.logger, opts.js, opts.stream, opts.consumer); err != nil {
		return nil, err
	}
	return newSubscriber(subscriberOpts{
		ctx:    opts.ctx,
		logger: opts.logger.WithPrefix(opts.logPrefix),
		newsub: func() (*nats.Subscription, error) {
			if err := ensureConsumer(opts.logger, opts.js, opts.stream, opts.consumer); err != nil {
				return nil, err
			}
			return opts.js.PullSubscribe(opts.consumer.FilterSubject, opts.consumer.Durable, opts.subOpts...)
		},
		handler:    opts.handler,
		maxfetch:   opts.maxfetch,
		disableLog: opts.disableLog,
	}), nil
}

// deliverSubOpt returns the subscription option matching policy, or fallback
// for policies that have no direct SubOpt equivalent
func deliverSubOpt(policy nats.DeliverPolicy, fallback nats.SubOpt) nats.SubOpt {
	switch policy {
	case nats.DeliverAllPolicy:
		return nats.DeliverAll()
	case nats.DeliverLastPolicy:
		return nats.DeliverLast()
	case nats.DeliverLastPerSubjectPolicy:
		return nats.DeliverLastPerSubject()
	case nats.DeliverNewPolicy:
		return nats.DeliverNew()
	default:
		return fallback
	}
}

// ensureConsumer creates the stream's durable consumer if it doesn't exist and
// updates it if its configuration doesn't match the expected one. Consumers
// must be created here rather than implicitly by PullSubscribe: the nats
// client deletes durable consumers it created itself on Unsubscribe, which
// would tear down a consumer shared with other subscribers.
func ensureConsumer(log logger.Logger, js nats.JetStreamContext, stream string, cconfig *nats.ConsumerConfig) error {
	ci, _ := js.ConsumerInfo(stream, cconfig.Durable)
	if ci == nil {
		if _, err := js.AddConsumer(stream, cconfig); err != nil && !errors.Is(err, nats.ErrConsumerNameAlreadyInUse) {
			return err
		}
		return nil
	}
	if msg, ok := diffConfig(ci.Config, *cconfig); !ok {
		log.Warn("consumer %s for stream %s has a configuration mismatch (%s) and must be updated", cconfig.Durable, stream, msg)
		if _, err := js.UpdateConsumer(stream, cconfig); err != nil {
			return err
		}
	}
	return nil
}

func diffConfig(a nats.ConsumerConfig, b nats.ConsumerConfig) (string, bool) {
	fields := []struct {
		name string
		a, b any
	}{
		{"ack policy", a.AckPolicy, b.AckPolicy},
		{"deliver policy", a.DeliverPolicy, b.DeliverPolicy},
		{"description", a.Description, b.Description},
		{"durable", a.Durable, b.Durable},
		{"filter subject", a.FilterSubject, b.FilterSubject},
		{"max ack pending", a.MaxAckPending, b.MaxAckPending},
		{"max deliver", a.MaxDeliver, b.MaxDeliver},
		{"name", a.Name, b.Name},
		{"replicas", a.Replicas, b.Replicas},
		{"max_fetch", a.MaxRequestBatch, b.MaxRequestBatch},
		{"ack_wait", a.AckWait, b.AckWait},
	}
	for _, f := range fields {
		if f.a != f.b {
			return fmt.Sprintf("%s: %v != %v", f.name, f.a, f.b), false
		}
	}
	return "", true
}
