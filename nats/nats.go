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
	fetchMaxWait        = 10 * time.Second
	resubscribeDelay    = time.Second
)

type Handler func(ctx context.Context, payload []byte, msg *nats.Msg) error

// Subscriber represents a nats subscriber
type Subscriber interface {
	// Close the subscriber and stop delivery
	Close() error
}

type subscriber struct {
	logger          logger.Logger
	newsub          func() (*nats.Subscription, error)
	sub             *nats.Subscription
	handler         Handler
	shutdown        bool
	lock            sync.Mutex
	wg              sync.WaitGroup
	ctx             context.Context
	cancel          context.CancelFunc
	inflight        *nats.Msg
	inflightSeq     uint64
	inflightMsgid   string
	inflightStarted *time.Time
	ackLock         sync.Mutex
	extendInterval  time.Duration
	maxfetch        int
	disableLog      bool
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
	_ctx, cancel := context.WithCancel(opts.ctx)
	if opts.extendInterval <= 0 {
		opts.extendInterval = time.Second * 28
	}
	if opts.maxfetch <= 0 {
		opts.maxfetch = 1
	}
	sub := &subscriber{
		logger:         opts.logger,
		newsub:         opts.newsub,
		handler:        opts.handler,
		ctx:            _ctx,
		cancel:         cancel,
		extendInterval: opts.extendInterval,
		maxfetch:       opts.maxfetch,
		disableLog:     opts.disableLog,
	}
	if s, err := opts.newsub(); err == nil {
		sub.sub = s
	}
	sub.wg.Add(2)
	go sub.extender()
	go sub.run()
	return sub
}

// Close will shutdown subscriptions and wait for the subscriber to be shutdown
func (s *subscriber) Close() error {
	s.logger.Debug("subscriber closing")
	s.lock.Lock()
	s.shutdown = true
	sub := s.sub
	s.sub = nil
	s.lock.Unlock()

	s.cancel() // signal sleep and extender to wake up/stop
	if sub != nil {
		sub.Unsubscribe() // wake up blocking fetch and stop delivery
	}
	s.wg.Wait()
	s.logger.Debug("subscriber closed")
	return nil
}

func (s *subscriber) sleep(d time.Duration) {
	select {
	case <-s.ctx.Done():
	case <-time.After(d):
	}
}

func (s *subscriber) extender() {
	defer s.wg.Done()
	t := time.NewTicker(s.extendInterval)
	defer t.Stop()
	for {
		select {
		case <-s.ctx.Done():
			s.ackLock.Lock()
			if s.inflight != nil {
				s.logger.Info("nack message %s (%v/%d) [canceled]", s.inflight.Subject, s.inflightMsgid, s.inflightSeq)
				s.inflight.Nak()
				s.inflight = nil
				s.inflightStarted = nil
				s.inflightMsgid = ""
				s.inflightSeq = 0
			}
			s.ackLock.Unlock()
			return
		case <-t.C:
			s.ackLock.Lock()
			if s.inflight != nil {
				if !s.disableLog {
					var running time.Duration
					if s.inflightStarted != nil {
						running = time.Since(*s.inflightStarted)
					}
					s.logger.Debug("extending %s ack timeout (%s/%d) running %v", s.inflight.Subject, s.inflightMsgid, s.inflightSeq, running)
				}
				if err := s.inflight.InProgress(); err != nil {
					s.logger.Error("error extending in progress %s (%s/%d): %v", s.inflight.Subject, s.inflightMsgid, s.inflightSeq, err)
				}
			}
			s.ackLock.Unlock()
		}
	}
}

func (s *subscriber) run() {
	defer s.wg.Done()
	for {
		s.lock.Lock()
		if s.shutdown {
			s.lock.Unlock()
			return
		}
		sub := s.sub
		s.lock.Unlock()

		if sub == nil {
			s.logger.Trace("need to create a new subscription")
			newSub, err := s.newsub()
			if err != nil {
				if !errors.Is(err, nats.ErrTimeout) && !errors.Is(err, nats.ErrConnectionClosed) && !errors.Is(err, nats.ErrDisconnected) {
					s.logger.Error("error creating new subscription: %s", err)
				}
				s.sleep(resubscribeDelay)
				continue
			}
			s.lock.Lock()
			if s.shutdown {
				s.lock.Unlock()
				newSub.Unsubscribe()
				return
			}
			s.sub = newSub
			sub = newSub
			s.lock.Unlock()
		}

		msgs, err := sub.Fetch(s.maxfetch, nats.MaxWait(fetchMaxWait))
		if err != nil {
			s.lock.Lock()
			shutdown := s.shutdown
			s.lock.Unlock()
			if shutdown || errors.Is(err, context.Canceled) {
				return
			}
			if errors.Is(err, nats.ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
				continue
			}

			if !errors.Is(err, nats.ErrConnectionClosed) && !errors.Is(err, nats.ErrDisconnected) && !errors.Is(err, nats.ErrFetchDisconnected) {
				s.logger.Error("subscription fetch error: %s", err)
			}

			s.lock.Lock()
			if s.sub != nil {
				s.sub.Unsubscribe()
				s.sub = nil
			}
			s.lock.Unlock()

			s.sleep(resubscribeDelay)
			continue
		}

		for _, msg := range msgs {
			s.lock.Lock()
			if s.shutdown {
				s.lock.Unlock()
				msg.Nak()
				continue
			}
			s.lock.Unlock()

			s.process(msg)
		}
	}
}

func (s *subscriber) process(msg *nats.Msg) {
	msgid := GetMsgIdFromHeader(msg)
	if msgid == "" {
		msgid = gstring.SHA256(msg.Data)
	}
	md, _ := msg.Metadata()
	sharedLogData := fmt.Sprintf("sub: %s, msgId: %s, consumerSeq: %v, streamSeq: %v, attempt: %d", msg.Subject, msgid, md.Sequence.Consumer, md.Sequence.Stream, md.NumDelivered)
	if md.NumDelivered > maxDeliveryAttempts {
		s.logger.Warn("terminating %s", sharedLogData)
		msg.Term()
		return
	}
	if !s.disableLog {
		s.logger.Debug("processing %s", sharedLogData)
	}
	encoding := GetContentEncodingFromHeader(msg)
	gzipped := encoding == "gzip/json"
	msgpacked := encoding == "msgpack"
	started := time.Now()
	var err error
	data := msg.Data
	if gzipped {
		data, err = compress.Gunzip(data)
	} else if msgpacked {
		var o any
		err = msgpack.Unmarshal(data, &o)
		if err == nil {
			data, err = json.Marshal(o)
		}
	}
	if err != nil {
		s.logger.Error("error uncompressing %s. err: %s", sharedLogData, err)
		msg.AckSync()
		return
	}

	s.ackLock.Lock()
	s.inflight = msg
	s.inflightMsgid = msgid
	s.inflightSeq = md.Sequence.Consumer
	s.inflightStarted = &started
	s.ackLock.Unlock()

	err = s.handler(s.ctx, data, msg)

	s.ackLock.Lock()
	s.inflight = nil
	s.inflightMsgid = ""
	s.inflightSeq = 0
	s.inflightStarted = nil
	s.ackLock.Unlock()

	if err != nil && !strings.Contains(err.Error(), "message was already acknowledged") {
		if errors.Is(err, context.Canceled) {
			s.logger.Warn("nack %s [canceled]", sharedLogData)
			msg.Nak()
		} else {
			s.logger.Error("error handling %s. err: %s", sharedLogData, err)
			msg.AckSync()
		}
	}
}

func isConsumerNameAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, nats.ErrConsumerNameAlreadyInUse) || strings.Contains(err.Error(), "consumer name already in use")
}

func ensureConsumer(log logger.Logger, js nats.JetStreamContext, stream string, cconfig *nats.ConsumerConfig) error {
	ci, _ := js.ConsumerInfo(stream, cconfig.Durable)
	if ci == nil {
		if _, err := js.AddConsumer(stream, cconfig); err != nil && !isConsumerNameAlreadyExistsError(err) {
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
