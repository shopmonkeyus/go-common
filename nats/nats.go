package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/shopmonkeyus/go-common/compress"
	"github.com/shopmonkeyus/go-common/logger"
	gstring "github.com/shopmonkeyus/go-common/string"
	"github.com/vmihailenco/msgpack/v5"
)

const maxDeliveryAttempts = 10

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
	if opts.extendInterval.Nanoseconds() == 0 {
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
	s, err := opts.newsub()
	if err == nil {
		sub.sub = s
	}
	go sub.extender()
	go sub.run()
	return sub
}

// Close will shutdown subscriptions and wait for the subscriber to be shutdown
func (s *subscriber) Close() error {
	s.logger.Debug("subscriber closing")
	s.lock.Lock()
	s.shutdown = true
	s.lock.Unlock()
	s.cancel()          // signal a blocking fetch to wake up
	s.sub.Unsubscribe() // unsubscribe so we don't get more messages
	s.wg.Wait()         // wait for us to nack all pending messages if any
	s.sub.Drain()       // close up shop
	s.logger.Debug("subscriber closed")
	return nil
}

func (s *subscriber) extender() {
	s.wg.Add(1)
	t := time.NewTicker(s.extendInterval)
	defer func() {
		t.Stop()
		s.wg.Done()
	}()
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
					s.logger.Debug("extending %s ack timeout (%s/%d) running %v", s.inflight.Subject, s.inflightMsgid, s.inflightSeq, time.Since(*s.inflightStarted))
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
	s.wg.Add(1)
	defer s.wg.Done()
	for {
		s.lock.Lock()
		shutdown := s.shutdown
		hassub := s.sub != nil
		s.lock.Unlock()
		if shutdown {
			return
		}
		if !hassub {
			s.logger.Trace("need to create a new subscription")
			sub, err := s.newsub()
			if err != nil {
				if errors.Is(err, nats.ErrTimeout) || errors.Is(err, nats.ErrConnectionClosed) {
					time.Sleep(time.Second)
					continue
				}
				s.logger.Error("error creating new subscription: %s", err)
				time.Sleep(time.Second)
				continue
			}
			s.lock.Lock()
			s.sub = sub
			s.lock.Unlock()
		}
		c, cf := context.WithTimeout(s.ctx, time.Minute)
		msgs, err := s.sub.Fetch(s.maxfetch, nats.Context(c))
		cf()
		if err != nil {
			s.lock.Lock()
			shutdown := s.shutdown
			s.lock.Unlock()
			if shutdown {
				return
			}
			// check to see if cancelled
			if errors.Is(err, context.Canceled) {
				return
			}
			// this is normal and we should continue to fetch more messages
			if errors.Is(err, context.DeadlineExceeded) {
				time.Sleep(time.Microsecond * 10)
				continue
			}
			if errors.Is(err, nats.ErrConnectionClosed) || errors.Is(err, nats.ErrDisconnected) {
				s.logger.Error("restarting to reconnect to nats...👋: %s", err)
				// lost outer nats connection so restart
				// otherwise we loop forever trying to reconnect
				os.Exit(1)
			}

			if errors.Is(err, nats.ErrTimeout) {
				time.Sleep(time.Microsecond * 10)
				continue
			}
			s.logger.Error("subscription fetch error: %s", err)
			time.Sleep(time.Second)
			continue
		}
		for _, msg := range msgs {
			// check through each message we process to make sure we're not in a shutdown
			// and if so, nack the message to allow another
			s.lock.Lock()
			if s.shutdown {
				msg.Nak()
				s.lock.Unlock()
				continue // keep going so that we nack all the messages
			}
			s.lock.Unlock()
			msgid := msg.Header.Get(nats.MsgIdHdr)
			if msgid == "" {
				msgid = gstring.SHA256(msg.Data)
			}
			md, _ := msg.Metadata()
			if md.NumDelivered > maxDeliveryAttempts {
				s.logger.Warn("terminating msg: %v (%s/%v) after %d delivery attempts", msg.Subject, msgid, md.Sequence.Consumer, md.NumDelivered)
				msg.Term() // no longer allow it to be reprocessed
				continue
			}
			if !s.disableLog {
				s.logger.Debug("processing message: %v (%s/%v), delivery: %d", msg.Subject, msgid, md.Sequence.Consumer, md.NumDelivered)
			}
			encoding := msg.Header.Get("content-encoding")
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
				s.logger.Error("error uncompressing message: %v (%s/%d). %s", msg.Subject, msgid, md.Sequence.Consumer, err)
				msg.AckSync()
				continue
			}

			// record our inflight message
			s.ackLock.Lock()
			s.inflight = msg
			s.inflightMsgid = msgid
			s.inflightSeq = md.Sequence.Consumer
			s.inflightStarted = &started
			s.ackLock.Unlock()

			// run our callback handler
			err = s.handler(s.ctx, data, msg)

			// make sure we untrack the inflight state so that the extender knows we're idle
			s.ackLock.Lock()
			s.inflight = nil
			s.inflightMsgid = ""
			s.inflightSeq = 0
			s.inflightStarted = nil
			s.ackLock.Unlock()

			// now do cleanup
			if err != nil && !strings.Contains(err.Error(), "message was already acknowledged") {
				if errors.Is(err, context.Canceled) {
					s.logger.Warn("nack message %s: (%v/%d) [canceled]", msg.Subject, msgid, md.Sequence.Consumer)
					msg.Nak()
				} else {
					s.logger.Error("error handling message %s: (%s/%d). %s", msg.Subject, msgid, md.Sequence.Consumer, err)
					msg.AckSync()
				}
			}
		}
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
	return "", true
}
