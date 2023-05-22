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
	logger   logger.Logger
	sub      *nats.Subscription
	handler  Handler
	shutdown bool
	lock     sync.Mutex
	wg       sync.WaitGroup
	ctx      context.Context
	cancel   context.CancelFunc
}

type subscriberOpts struct {
	ctx     context.Context
	logger  logger.Logger
	sub     *nats.Subscription
	handler Handler
}

var _ Subscriber = (*subscriber)(nil)

func newSubscriber(opts subscriberOpts) *subscriber {
	_ctx, cancel := context.WithCancel(opts.ctx)
	sub := &subscriber{
		logger:  opts.logger,
		sub:     opts.sub,
		handler: opts.handler,
		ctx:     _ctx,
		cancel:  cancel,
	}
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

func (s *subscriber) run() {
	s.wg.Add(1)
	defer s.wg.Done()
	var ackLock sync.Mutex
	for {
		s.lock.Lock()
		shutdown := s.shutdown
		s.lock.Unlock()
		if shutdown {
			return
		}
		c, cf := context.WithTimeout(s.ctx, time.Minute)
		msgs, err := s.sub.Fetch(1, nats.Context(c))
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
			errMsg := err.Error()
			if strings.Contains(errMsg, "timeout") || strings.Contains(errMsg, "deadline exceeded") || strings.Contains(errMsg, "connection closed") {
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
				s.logger.Info("terminating msg: %v (%s/%v) after %d delivery attempts", msg.Subject, msgid, md.Sequence.Consumer, md.NumDelivered)
				msg.Term() // no longer allow it to be reprocessed
				continue
			}
			s.logger.Info("processing msg: %v (%s/%v), delivery: %d", msg.Subject, msgid, md.Sequence.Consumer, md.NumDelivered)
			encoding := msg.Header.Get("content-encoding")
			// FIXME:msgpack
			gzipped := encoding == "gzip/json"
			started := time.Now()
			var err error
			data := msg.Data
			if gzipped {
				data, err = compress.Gunzip(data)
			}
			if err != nil {
				s.logger.Error("error uncompressing message (%s/%d). %s", msgid, md.Sequence.Consumer, err)
				msg.AckSync()
				continue
			}
			// while we're still running, let the server know if we're in progress
			ctx, done := context.WithCancel(s.ctx)
			var ok bool
			go func() {
				for {
					select {
					case <-ctx.Done():
						ackLock.Lock()
						_ok := ok
						ackLock.Unlock()
						if !_ok {
							s.logger.Info("nack message: (%v/%d) [canceled]", msgid, md.Sequence.Consumer)
							msg.Nak()
						}
						done()
						return
					case <-time.After(time.Second * 28):
						s.logger.Debug("extending ack timeout (%s/%d) running %v", msgid, md.Sequence.Consumer, time.Since(started))
						msg.InProgress()
					}
				}
			}()
			// we need to block waiting for the handler to finish but we do it in its
			// own go routine so that we can more easily cancel
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := s.handler(ctx, data, msg); err != nil {
					ackLock.Lock()
					ok = true
					ackLock.Unlock()
					if errors.Is(err, context.Canceled) {
						s.logger.Info("nack message: (%v/%d) [canceled]", msgid, md.Sequence.Consumer)
						msg.Nak()
					} else {
						s.logger.Error("error handling message (%s/%d). %s", msgid, md.Sequence.Consumer, err)
						msg.AckSync()
					}
					done()
					return
				}
				ackLock.Lock()
				ok = true
				ackLock.Unlock()
			}()
			wg.Wait()
			done()
		}
	}
}
