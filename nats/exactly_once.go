package nats

import (
	"context"

	"github.com/nats-io/nats.go"
	"github.com/shopmonkeyus/go-common/logger"
)

type ExactlyOnceConsumerConfig struct {
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
}

// NewExactlyOnceConsumer will create (or reuse) an exactly once durable consumer
func NewExactlyOnceConsumerWithConfig(config ExactlyOnceConsumerConfig) (Subscriber, error) {
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
	eos := newSubscriber(subscriberOpts{
		ctx:     config.Context,
		logger:  config.Logger,
		sub:     sub,
		handler: config.Handler,
	})
	return eos, nil
}

// NewExactlyOnceConsumer will create (or reuse) an exactly once durable consumer
func NewExactlyOnceConsumer(ctx context.Context, logger logger.Logger, js nats.JetStreamContext, stream string, durable string, description string, subject string, handler Handler) (Subscriber, error) {
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
