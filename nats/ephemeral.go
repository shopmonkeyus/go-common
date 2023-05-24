package nats

import (
	"context"

	"github.com/nats-io/nats.go"
	"github.com/shopmonkeyus/go-common/logger"
)

type ephemeralConsumerConfig struct {
	Context             context.Context
	Logger              logger.Logger
	JetStream           nats.JetStreamContext
	StreamName          string
	ConsumerDescription string
	FilterSubject       string
	Handler             Handler
	DeliverPolicy       nats.DeliverPolicy
	Deliver             nats.SubOpt
	MaxDeliver          int
	MaxAckPending       int
}

type EphemeralOptsFunc func(config *ephemeralConsumerConfig) error

func defaultEphemeralConfig(logger logger.Logger, js nats.JetStreamContext, stream string, subject string, handler Handler) ephemeralConsumerConfig {
	return ephemeralConsumerConfig{
		Context:             context.Background(),
		Logger:              logger,
		JetStream:           js,
		StreamName:          stream,
		ConsumerDescription: `ephemeral consumer for ${stream}`,
		FilterSubject:       subject,
		Handler:             handler,
		DeliverPolicy:       nats.DeliverNewPolicy,
		Deliver:             nats.DeliverNew(),
		MaxDeliver:          1,
		MaxAckPending:       1000,
	}
}

// WithEphemeralMaxDeliver set the maximum deliver value
func WithEphemeralMaxDeliver(max int) EphemeralOptsFunc {
	return func(config *ephemeralConsumerConfig) error {
		config.MaxDeliver = max
		return nil
	}
}

// WithEphemeralMaxAckPending set the maximum ack pending value
func WithEphemeralMaxAckPending(max int) EphemeralOptsFunc {
	return func(config *ephemeralConsumerConfig) error {
		config.MaxAckPending = max
		return nil
	}
}

// WithEphemeralDelivery set the internal context
func WithEphemeralDelivery(policy nats.DeliverPolicy) EphemeralOptsFunc {
	return func(config *ephemeralConsumerConfig) error {
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

// WithEphemeralContext set the internal context
func WithEphemeralContext(context context.Context) EphemeralOptsFunc {
	return func(config *ephemeralConsumerConfig) error {
		config.Context = context
		return nil
	}
}

// WithEphemeralConsumerDescription set the consumer description
func WithEphemeralConsumerDescription(description string) EphemeralOptsFunc {
	return func(config *ephemeralConsumerConfig) error {
		config.ConsumerDescription = description
		return nil
	}
}

func newEphemeralConsumerWithConfig(config ephemeralConsumerConfig) (Subscriber, error) {
	_, err := config.JetStream.AddConsumer(config.StreamName, &nats.ConsumerConfig{
		Description:   config.ConsumerDescription,
		FilterSubject: config.FilterSubject,
		AckPolicy:     nats.AckExplicitPolicy,
		MaxAckPending: config.MaxAckPending,
		DeliverPolicy: config.DeliverPolicy,
		MaxDeliver:    config.MaxDeliver,
	})
	if err != nil {
		return nil, err
	}
	sub, err := config.JetStream.PullSubscribe(
		config.FilterSubject,
		"", // ephemeral durable must be set to empty string to make it ephemeral
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
		ctx:     config.Context,
		logger:  config.Logger,
		sub:     sub,
		handler: config.Handler,
	})
	return eos, nil
}

// NewEphemeralConsumer will create (or reuse) an ephemeral consumer
func NewEphemeralConsumer(logger logger.Logger, js nats.JetStreamContext, stream string, subject string, handler Handler, opts ...EphemeralOptsFunc) (Subscriber, error) {
	config := defaultEphemeralConfig(logger, js, stream, subject, handler)
	for _, fn := range opts {
		if err := fn(&config); err != nil {
			return nil, err
		}
	}
	return newEphemeralConsumerWithConfig(config)
}
