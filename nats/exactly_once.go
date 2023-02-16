package nats

import (
	"strings"
	"sync"

	"github.com/nats-io/nats.go"
	"github.com/shopmonkeyus/go-common/compress"
	"github.com/shopmonkeyus/go-common/logger"
)

type ExactlyOnceHandler func(payload []byte, msgid string) error

type ExactlyOnceSubscriber struct {
	logger   logger.Logger
	sub      *nats.Subscription
	handler  ExactlyOnceHandler
	shutdown bool
	lock     *sync.Mutex
	wg       *sync.WaitGroup
}

// Close will shutdown subscriptions and wait for the subscriber to be shutdown
func (s *ExactlyOnceSubscriber) Close() {
	s.sub.Drain()
	s.lock.Lock()
	s.shutdown = true
	s.lock.Unlock()
	s.wg.Wait()
}

func (s *ExactlyOnceSubscriber) run() {
	s.wg.Add(1)
	defer s.wg.Done()
	for {
		s.lock.Lock()
		shutdown := s.shutdown
		s.lock.Unlock()
		if shutdown {
			return
		}
		msgs, err := s.sub.Fetch(1)
		if err != nil {
			s.lock.Lock()
			shutdown := s.shutdown
			s.lock.Unlock()
			if shutdown {
				return
			}
			if strings.Contains(err.Error(), "timeout") {
				continue
			}
			s.logger.Error("subscription fetch error: %s", err)
			continue
		}
		for _, msg := range msgs {
			encoding := msg.Header.Get("content-encoding")
			gzipped := encoding == "gzip/json"
			msgid := msg.Header.Get("Nats-Msg-Id")
			var err error
			data := msg.Data
			if gzipped {
				data, err = compress.Gunzip(data)
			}
			if err != nil {
				s.logger.Error("error uncompressing message: %s", err)
				msg.AckSync()
				continue
			}
			if err := s.handler(data, msgid); err != nil {
				s.logger.Error("error handling message: %s", err)
				msg.AckSync()
				continue
			}
			if err := msg.AckSync(); err != nil {
				s.logger.Error("error calling ack for message: %s. %s", msgid, err)
			}
		}
	}
}

// NewExactlyOnceConsumer will create (or reuse) an exactly once durable consumer
func NewExactlyOnceConsumer(logger logger.Logger, js nats.JetStreamContext, stream string, durable string, description string, subject string, handler ExactlyOnceHandler) (*ExactlyOnceSubscriber, error) {
	_, err := js.AddConsumer(stream, &nats.ConsumerConfig{
		Durable:       durable,
		Description:   description,
		FilterSubject: subject,
		AckPolicy:     nats.AckExplicitPolicy,
		MaxAckPending: 1,
		MaxDeliver:    1,
	})
	if err != nil {
		return nil, err
	}
	sub, err := js.PullSubscribe(subject,
		"",
		nats.Durable(durable),
		nats.MaxDeliver(1),
		nats.MaxAckPending(1),
		nats.AckExplicit(),
		nats.Description(description),
	)
	if err != nil {
		return nil, err
	}
	eos := &ExactlyOnceSubscriber{
		logger:   logger,
		sub:      sub,
		handler:  handler,
		shutdown: false,
		lock:     &sync.Mutex{},
		wg:       &sync.WaitGroup{},
	}
	go eos.run()
	return eos, nil
}
