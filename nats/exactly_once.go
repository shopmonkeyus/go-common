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

type ExactlyOnceHandler func(ctx context.Context, payload []byte, msg *nats.Msg) error

type ExactlyOnceSubscriber struct {
	logger   logger.Logger
	sub      *nats.Subscription
	handler  ExactlyOnceHandler
	shutdown bool
	lock     *sync.Mutex
	wg       *sync.WaitGroup
	ctx      context.Context
	cancel   context.CancelFunc
}

// Close will shutdown subscriptions and wait for the subscriber to be shutdown
func (s *ExactlyOnceSubscriber) Close() {
	s.logger.Debug("subscriber closing")
	s.lock.Lock()
	s.shutdown = true
	s.lock.Unlock()
	s.cancel()
	s.sub.Unsubscribe()
	s.sub.Drain()
	s.wg.Wait()
	s.logger.Debug("subscriber closed")
}

func (s *ExactlyOnceSubscriber) run() {
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

type ExactlyOnceConsumerConfig struct {
	Context             context.Context
	Logger              logger.Logger
	JetStream           nats.JetStreamContext
	StreamName          string
	DurableName         string
	ConsumerDescription string
	FilterSubject       string
	Handler             ExactlyOnceHandler
	DeliverPolicy       nats.DeliverPolicy
	Deliver             nats.SubOpt
	MaxAckPending       int
}

// NewExactlyOnceConsumer will create (or reuse) an exactly once durable consumer
func NewExactlyOnceConsumerWithConfig(config ExactlyOnceConsumerConfig) (*ExactlyOnceSubscriber, error) {
	maxAckPending := 1
	if config.MaxAckPending > 0 {
		maxAckPending = config.MaxAckPending
	}
	deliver := config.Deliver
	if deliver == nil {
		deliver = nats.DeliverNew()
	}
	deliverPolicy := config.DeliverPolicy
	_, err := config.JetStream.AddConsumer(config.StreamName, &nats.ConsumerConfig{
		Durable:       config.DurableName,
		Description:   config.ConsumerDescription,
		FilterSubject: config.FilterSubject,
		AckPolicy:     nats.AckExplicitPolicy,
		MaxAckPending: maxAckPending,
		DeliverPolicy: deliverPolicy,
	})
	if err != nil {
		return nil, err
	}
	sub, err := config.JetStream.PullSubscribe(
		config.FilterSubject,
		config.DurableName,
		nats.MaxAckPending(maxAckPending),
		nats.ManualAck(),
		nats.AckExplicit(),
		nats.Description(config.ConsumerDescription),
		deliver,
	)
	if err != nil {
		return nil, err
	}
	_ctx, cancel := context.WithCancel(config.Context)
	eos := &ExactlyOnceSubscriber{
		logger:   config.Logger,
		sub:      sub,
		handler:  config.Handler,
		shutdown: false,
		lock:     &sync.Mutex{},
		wg:       &sync.WaitGroup{},
		ctx:      _ctx,
		cancel:   cancel,
	}
	go eos.run()
	return eos, nil
}

// NewExactlyOnceConsumer will create (or reuse) an exactly once durable consumer
func NewExactlyOnceConsumer(ctx context.Context, logger logger.Logger, js nats.JetStreamContext, stream string, durable string, description string, subject string, handler ExactlyOnceHandler) (*ExactlyOnceSubscriber, error) {
	return NewExactlyOnceConsumerWithConfig(ExactlyOnceConsumerConfig{
		Context:             ctx,
		Logger:              logger,
		JetStream:           js,
		StreamName:          stream,
		DurableName:         durable,
		ConsumerDescription: description,
		FilterSubject:       subject,
		Handler:             handler,
		DeliverPolicy:       nats.DeliverNewPolicy,
		Deliver:             nats.DeliverNew(),
	})
}
