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
	fetchMaxWait        = time.Minute
	resubscribeDelay    = time.Second
)

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
	sub := s.sub
	s.lock.Unlock()
	if sub != nil {
		return sub, nil
	}
	s.logger.Trace("creating a new subscription")
	sub, err := s.newsub()
	if err != nil {
		return nil, err
	}
	s.lock.Lock()
	s.sub = sub
	s.lock.Unlock()
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
				// no messages arrived before the fetch wait expired
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

func isConsumerNameAlreadyExistsError(err error) bool {
	return strings.Contains(err.Error(), "consumer name already in use")
}

func diffConfig(a nats.ConsumerConfig, b nats.ConsumerConfig) (string, bool) {
	if a.AckPolicy != b.AckPolicy {
		return fmt.Sprintf("ack policy: %v != %v", a.AckPolicy, b.AckPolicy), false
	}
	if a.DeliverPolicy != b.DeliverPolicy {
		return fmt.Sprintf("deliver policy: %v != %v", a.DeliverPolicy, b.DeliverPolicy), false
	}
	if a.Description != b.Description {
		return fmt.Sprintf("description: %v != %v", a.Description, b.Description), false
	}
	if a.Durable != b.Durable {
		return fmt.Sprintf("durable: %v != %v", a.Durable, b.Durable), false
	}
	if a.FilterSubject != b.FilterSubject {
		return fmt.Sprintf("filter subject: %v != %v", a.FilterSubject, b.FilterSubject), false
	}
	if a.MaxAckPending != b.MaxAckPending {
		return fmt.Sprintf("max ack pending: %v != %v", a.MaxAckPending, b.MaxAckPending), false
	}
	if a.MaxDeliver != b.MaxDeliver {
		return fmt.Sprintf("max deliver: %v != %v", a.MaxDeliver, b.MaxDeliver), false
	}
	if a.Name != b.Name {
		return fmt.Sprintf("name: %v != %v", a.Name, b.Name), false
	}
	if a.Replicas != b.Replicas {
		return fmt.Sprintf("replicas: %v != %v", a.Replicas, b.Replicas), false
	}
	if a.MaxRequestBatch != b.MaxRequestBatch {
		return fmt.Sprintf("max_fetch: %v != %v", a.MaxRequestBatch, b.MaxRequestBatch), false
	}
	if a.AckWait != b.AckWait {
		return fmt.Sprintf("ack_wait: %v != %v", a.AckWait, b.AckWait), false
	}
	return "", true
}
