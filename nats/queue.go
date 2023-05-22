package nats

import (
	"context"

	"github.com/nats-io/nats.go"
	"github.com/shopmonkeyus/go-common/logger"
)

type QueueConsumerConfig struct {
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
}

// NewQueueConsumerWithConfig will create (or reuse) queue consumer
func NewQueueConsumerWithConfig(config QueueConsumerConfig) (Subscriber, error) {
	maxAckPending := 1000
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
		MaxDeliver:    1,
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

// NewQueueConsumer will create (or reuse) a queue consumer with default config
func NewQueueConsumer(ctx context.Context, logger logger.Logger, js nats.JetStreamContext, stream string, durable string, description string, subject string, handler Handler) (Subscriber, error) {
	return NewQueueConsumerWithConfig(QueueConsumerConfig{
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
