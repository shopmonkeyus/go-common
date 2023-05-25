package nats

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/shopmonkeyus/go-common/logger"
)

type queueConsumerConfig struct {
	Context             context.Context
	Logger              logger.Logger
	JetStream           nats.JetStreamContext
	StreamName          string
	DurableName         string
	ConsumerDescription string
	FilterSubject       string
	Handler             Handler
	DeliverPolicy       nats.DeliverPolicy
	Deliver             nats.SubOpt
	MaxAckPending       int
	MaxDeliver          int
	Replicas            int
	DisableSubLogging   bool
}

type QueueOptsFunc func(config *queueConsumerConfig) error

func defaultQueueConfig(logger logger.Logger, js nats.JetStreamContext, stream string, durable string, subject string, handler Handler) queueConsumerConfig {
	return queueConsumerConfig{
		Context:             context.Background(),
		Logger:              logger,
		JetStream:           js,
		StreamName:          stream,
		DurableName:         durable,
		ConsumerDescription: fmt.Sprintf("queue consumer for %s", stream),
		FilterSubject:       subject,
		Handler:             handler,
		DeliverPolicy:       nats.DeliverNewPolicy,
		Deliver:             nats.DeliverNew(),
		MaxDeliver:          1,
		MaxAckPending:       1000,
		Replicas:            3,
	}
}

// WithQueueDisableSubscriberLogging to turn off extra trace logging in the subscriber
func WithQueueDisableSubscriberLogging() QueueOptsFunc {
	return func(config *queueConsumerConfig) error {
		config.DisableSubLogging = true
		return nil
	}
}

// WithQueueReplicas set the number of replicas
func WithQueueReplicas(replicas int) QueueOptsFunc {
	return func(config *queueConsumerConfig) error {
		config.Replicas = replicas
		return nil
	}
}

// WithQueueMaxDeliver set the maximum deliver value
func WithQueueMaxDeliver(max int) QueueOptsFunc {
	return func(config *queueConsumerConfig) error {
		config.MaxDeliver = max
		return nil
	}
}

// WithQueueMaxAckPending set the maximum ack pending value
func WithQueueMaxAckPending(max int) QueueOptsFunc {
	return func(config *queueConsumerConfig) error {
		config.MaxAckPending = max
		return nil
	}
}

// WithQueueDelivery set the internal context
func WithQueueDelivery(policy nats.DeliverPolicy) QueueOptsFunc {
	return func(config *queueConsumerConfig) error {
		switch policy {
		case nats.DeliverAllPolicy:
			config.Deliver = nats.DeliverAll()
		case nats.DeliverLastPolicy:
			config.Deliver = nats.DeliverLast()
		case nats.DeliverLastPerSubjectPolicy:
			config.Deliver = nats.DeliverLastPerSubject()
		case nats.DeliverNewPolicy:
			config.Deliver = nats.DeliverNew()
		}
		config.DeliverPolicy = policy
		return nil
	}
}

// WithQueueContext set the internal context
func WithQueueContext(context context.Context) QueueOptsFunc {
	return func(config *queueConsumerConfig) error {
		config.Context = context
		return nil
	}
}

// WithQueueConsumerDescription set the consumer description
func WithQueueConsumerDescription(description string) QueueOptsFunc {
	return func(config *queueConsumerConfig) error {
		config.ConsumerDescription = description
		return nil
	}
}

func newQueueConsumerWithConfig(config queueConsumerConfig) (Subscriber, error) {
	ci, _ := config.JetStream.ConsumerInfo(config.StreamName, config.DurableName)
	cconfig := &nats.ConsumerConfig{
		Durable:       config.DurableName,
		Description:   config.ConsumerDescription,
		FilterSubject: config.FilterSubject,
		AckPolicy:     nats.AckExplicitPolicy,
		MaxAckPending: config.MaxAckPending,
		DeliverPolicy: config.DeliverPolicy,
		MaxDeliver:    config.MaxDeliver,
		Replicas:      config.Replicas,
	}
	if ci != nil {
		if !diffConfig(ci.Config, *cconfig) {
			config.Logger.Warn("consumer %s for stream %s has a configuration mismatch and must be updated", config.DurableName, config.StreamName)
			if _, err := config.JetStream.UpdateConsumer(config.StreamName, cconfig); err != nil {
				return nil, err
			}
		}
	}
	if ci == nil {
		if _, err := config.JetStream.AddConsumer(config.StreamName, cconfig); err != nil && !isConsumerNameAlreadyExistsError(err) {
			return nil, err
		}
	}
	sub, err := config.JetStream.PullSubscribe(
		config.FilterSubject,
		config.DurableName,
		nats.MaxAckPending(config.MaxAckPending),
		nats.ManualAck(),
		nats.AckExplicit(),
		nats.Description(config.ConsumerDescription),
		config.Deliver,
	)
	if err != nil {
		return nil, err
	}
	eos := newSubscriber(subscriberOpts{
		ctx:        config.Context,
		logger:     config.Logger,
		sub:        sub,
		handler:    config.Handler,
		maxfetch:   config.MaxDeliver,
		disableLog: config.DisableSubLogging,
	})
	return eos, nil
}

// NewQueueConsumer will create (or reuse) a queue consumer with default config
func NewQueueConsumer(logger logger.Logger, js nats.JetStreamContext, stream string, durable string, subject string, handler Handler, opts ...QueueOptsFunc) (Subscriber, error) {
	config := defaultQueueConfig(logger, js, stream, durable, subject, handler)
	for _, fn := range opts {
		if err := fn(&config); err != nil {
			return nil, err
		}
	}
	return newQueueConsumerWithConfig(config)
}
