package nats

import (
	"context"

	"github.com/nats-io/nats.go"
	"github.com/shopmonkeyus/go-common/logger"
)

type EphemeralConsumerConfig struct {
	Context             context.Context
	Logger              logger.Logger
	JetStream           nats.JetStreamContext
	StreamName          string
	ConsumerDescription string
	FilterSubject       string
	Handler             Handler
	DeliverPolicy       nats.DeliverPolicy
	Deliver             nats.SubOpt
}

// NewEphemeralConsumerWithConfig will create (or reuse) ephemeral consumer
func NewEphemeralConsumerWithConfig(config EphemeralConsumerConfig) (Subscriber, error) {
	maxAckPending := 1000
	deliver := config.Deliver
	if deliver == nil {
		deliver = nats.DeliverNew()
	}
	deliverPolicy := config.DeliverPolicy
	_, err := config.JetStream.AddConsumer(config.StreamName, &nats.ConsumerConfig{
		Description:   config.ConsumerDescription,
		FilterSubject: config.FilterSubject,
		AckPolicy:     nats.AckExplicitPolicy,
		MaxAckPending: maxAckPending,
		DeliverPolicy: deliverPolicy,
		MaxDeliver:    1,
	})
	if err != nil {
		return nil, err
	}
	sub, err := config.JetStream.PullSubscribe(
		config.FilterSubject,
		"",
		nats.MaxAckPending(maxAckPending),
		nats.ManualAck(),
		nats.AckExplicit(),
		nats.Description(config.ConsumerDescription),
		deliver,
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

// NewEphemeralConsumerDeliverAll will create (or reuse) an ephemeral consumer with default config which will deliver all messages (not just new ones)
func NewEphemeralConsumerDeliverAll(ctx context.Context, logger logger.Logger, js nats.JetStreamContext, stream string, description string, subject string, handler Handler) (Subscriber, error) {
	return NewEphemeralConsumerWithConfig(EphemeralConsumerConfig{
		Context:             ctx,
		Logger:              logger,
		JetStream:           js,
		StreamName:          stream,
		ConsumerDescription: description,
		FilterSubject:       subject,
		Handler:             handler,
		DeliverPolicy:       nats.DeliverAllPolicy,
		Deliver:             nats.DeliverAll(),
	})
}

// NewEphemeralConsumer will create (or reuse) an ephemeral consumer with default config
func NewEphemeralConsumer(ctx context.Context, logger logger.Logger, js nats.JetStreamContext, stream string, description string, subject string, handler Handler) (Subscriber, error) {
	return NewEphemeralConsumerWithConfig(EphemeralConsumerConfig{
		Context:             ctx,
		Logger:              logger,
		JetStream:           js,
		StreamName:          stream,
		ConsumerDescription: description,
		FilterSubject:       subject,
		Handler:             handler,
		DeliverPolicy:       nats.DeliverNewPolicy,
		Deliver:             nats.DeliverNew(),
	})
}
