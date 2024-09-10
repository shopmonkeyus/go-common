package nats

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/shopmonkeyus/go-common/logger"
)

type ConsumerConfig struct {
	Context   context.Context
	Logger    logger.Logger
	Config    jetstream.ConsumerConfig
	JetStream jetstream.JetStream
	Stream    string
}

// NewJetStreamConsumer creates a new JetStream consumer with the given configuration. This will create or update a consumer based on the given configuration.
func NewJetStreamConsumer(config ConsumerConfig) (jetstream.Consumer, error) {
	// create a context with a longer deadline for creating the consumer
	configConsumerCtx, cancelConfig := context.WithDeadline(config.Context, time.Now().Add(time.Minute))
	defer cancelConfig()

	// setup the consumer
	consumer, err := config.JetStream.Consumer(configConsumerCtx, config.Stream, config.Config.Durable)
	if err != nil {
		if !errors.Is(err, jetstream.ErrConsumerNotFound) {
			return nil, fmt.Errorf("error getting jetstream consumer %s for stream: %s: %w", config.Config.Durable, config.Stream, err)
		}
		config.Logger.Debug("consumer %s not found, creating", config.Config.Durable)
		// consumer not found, create it
		consumer, err = config.JetStream.CreateConsumer(configConsumerCtx, config.Stream, config.Config)
		if err != nil {
			return nil, fmt.Errorf("error creating jetstream consumer %s for strema: %s: %w", config.Config.Durable, config.Stream, err)
		}
	} else if consumer != nil {
		// consumer found, update it in case subjects changed
		config.Logger.Debug("consumer %s found for %s, updating", config.Config.Durable, config.Stream)
		consumer, err = config.JetStream.UpdateConsumer(configConsumerCtx, config.Stream, config.Config)
		if err != nil {
			return nil, fmt.Errorf("error updating jetstream consumer %s for stream: %s: %w", config.Config.Durable, config.Stream, err)
		}
	}
	cancelConfig()
	return consumer, nil
}
