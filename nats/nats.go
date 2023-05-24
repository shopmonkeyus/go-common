package nats

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/shopmonkeyus/go-common/compress"
	"github.com/shopmonkeyus/go-common/logger"
	gstring "github.com/shopmonkeyus/go-common/string"
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
}

type subscriberOpts struct {
	ctx            context.Context
	logger         logger.Logger
	sub            *nats.Subscription
	handler        Handler
	extendInterval time.Duration
	maxfetch       int
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
		sub:            opts.sub,
		handler:        opts.handler,
		ctx:            _ctx,
		cancel:         cancel,
		extendInterval: opts.extendInterval,
		maxfetch:       opts.maxfetch,
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
	s.cancel()
	s.sub.Unsubscribe()
	s.sub.Drain()
	s.wg.Wait()
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
				s.logger.Debug("extending %s ack timeout (%s/%d) running %v", s.inflight.Subject, s.inflightMsgid, s.inflightSeq, time.Since(*s.inflightStarted))
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
		s.lock.Unlock()
		if shutdown {
			return
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
			errMsg := err.Error()
			if strings.Contains(errMsg, "timeout") || strings.Contains(errMsg, "connection closed") {
				continue
			}
			s.logger.Error("subscription fetch error: %s", errMsg)
			time.Sleep(time.Second)
			continue
		}
		for _, msg := range msgs {
			msgid := msg.Header.Get("Nats-Msg-Id")
			if msgid == "" {
				msgid = gstring.SHA256(msg.Data)
			}
			md, _ := msg.Metadata()
			if md.NumDelivered > maxDeliveryAttempts {
				s.logger.Warn("terminating msg: %v (%s/%v) after %d delivery attempts", msg.Subject, msgid, md.Sequence.Consumer, md.NumDelivered)
				msg.Term() // no longer allow it to be reprocessed
				continue
			}
			s.logger.Debug("processing message: %v (%s/%v), delivery: %d", msg.Subject, msgid, md.Sequence.Consumer, md.NumDelivered)
			encoding := msg.Header.Get("content-encoding")
			gzipped := encoding == "gzip/json"
			started := time.Now()
			var err error
			data := msg.Data
			if gzipped {
				data, err = compress.Gunzip(data)
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
			if err != nil {
				if errors.Is(err, context.Canceled) {
					s.logger.Warn("nack message: (%v/%d) [canceled]", msgid, md.Sequence.Consumer)
					msg.Nak()
				} else {
					s.logger.Error("error handling message (%s/%d). %s", msgid, md.Sequence.Consumer, err)
					msg.AckSync()
				}
			}
		}
	}
}
