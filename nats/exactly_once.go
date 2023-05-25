package nats

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/shopmonkeyus/go-common/logger"
)

type exactlyOnceConsumerConfig struct {
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
	MaxDeliver          int
	Replicas            int
}

type ExactlyOnceOptsFunc func(config *exactlyOnceConsumerConfig) error

func defaultExactlyOnceConfig(logger logger.Logger, js nats.JetStreamContext, stream string, durable string, subject string, handler Handler) exactlyOnceConsumerConfig {
	return exactlyOnceConsumerConfig{
		Context:             context.Background(),
		Logger:              logger,
		JetStream:           js,
		StreamName:          stream,
		DurableName:         durable,
		ConsumerDescription: fmt.Sprintf("exactly once consumer for %s", stream),
		FilterSubject:       subject,
		Handler:             handler,
		DeliverPolicy:       nats.DeliverNewPolicy,
		Deliver:             nats.DeliverNew(),
		MaxDeliver:          1,
		Replicas:            3,
	}
}

// WithExactlyOnceMaxDeliver set the maximum deliver value
func WithExactlyOnceMaxDeliver(max int) ExactlyOnceOptsFunc {
	return func(config *exactlyOnceConsumerConfig) error {
		config.MaxDeliver = max
		return nil
	}
}

// WithExactlyOnceReplicas set the number of replicas for the consumer
func WithExactlyOnceReplicas(replicas int) ExactlyOnceOptsFunc {
	return func(config *exactlyOnceConsumerConfig) error {
		config.Replicas = replicas
		return nil
	}
}

// WithExactlyOnceDelivery set the internal context
func WithExactlyOnceDelivery(policy nats.DeliverPolicy) ExactlyOnceOptsFunc {
	return func(config *exactlyOnceConsumerConfig) error {
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

// WithExactlyOnceContext set the internal context
func WithExactlyOnceContext(context context.Context) ExactlyOnceOptsFunc {
	return func(config *exactlyOnceConsumerConfig) error {
		config.Context = context
		return nil
	}
}

// WithExactlyOnceConsumerDescription set the consumer description
func WithExactlyOnceConsumerDescription(description string) ExactlyOnceOptsFunc {
	return func(config *exactlyOnceConsumerConfig) error {
		config.ConsumerDescription = description
		return nil
	}
}

func newExactlyOnceConsumerWithConfig(config exactlyOnceConsumerConfig) (Subscriber, error) {
	ci, _ := config.JetStream.ConsumerInfo(config.StreamName, config.StreamName)
	if ci == nil {
		if _, err := config.JetStream.AddConsumer(config.StreamName, &nats.ConsumerConfig{
			Durable:       config.DurableName,
			Name:          config.DurableName,
			Description:   config.ConsumerDescription,
			FilterSubject: config.FilterSubject,
			AckPolicy:     nats.AckExplicitPolicy,
			MaxAckPending: 1,
			MaxDeliver:    1,
			DeliverPolicy: config.DeliverPolicy,
			Replicas:      config.Replicas,
		}); err != nil && !isConsumerNameAlreadyExistsError(err) {
			return nil, err
		}
	}
	sub, err := config.JetStream.PullSubscribe(
		config.FilterSubject,
		config.DurableName,
		nats.MaxAckPending(1),
		nats.ManualAck(),
		nats.AckExplicit(),
		nats.Description(config.ConsumerDescription),
		config.Deliver,
	)
	if err != nil {
		return nil, err
	}
	eos := newSubscriber(subscriberOpts{
		ctx:      config.Context,
		logger:   config.Logger,
		sub:      sub,
		handler:  config.Handler,
		maxfetch: 1,
	})
	return eos, nil
}

// NewExactlyOnceConsumer will create (or reuse) an exactly once durable consumer
func NewExactlyOnceConsumer(logger logger.Logger, js nats.JetStreamContext, stream string, durable string, subject string, handler Handler, opts ...ExactlyOnceOptsFunc) (Subscriber, error) {
	config := defaultExactlyOnceConfig(logger, js, stream, durable, subject, handler)
	for _, fn := range opts {
		if err := fn(&config); err != nil {
			return nil, err
		}
	}
	return newExactlyOnceConsumerWithConfig(config)
}
