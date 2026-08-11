package nats

import (
	"fmt"

	gnats "github.com/nats-io/nats.go"
	"github.com/shopmonkeyus/go-common/logger"
)

// NewNats will return a new nats connection that keeps reconnecting if the
// connection is lost. The defaults can be overridden with opts.
func NewNats(log logger.Logger, name, hosts string, credentials gnats.Option, opts ...gnats.Option) (*gnats.Conn, error) {
	connectOpts := append([]gnats.Option{
		gnats.MaxReconnects(-1), // reconnect forever instead of giving up
		gnats.DisconnectErrHandler(func(_ *gnats.Conn, err error) {
			if err != nil {
				log.Warn("nats disconnected: %s", err)
			}
		}),
		gnats.ReconnectHandler(func(nc *gnats.Conn) {
			log.Info("nats reconnected to %s", nc.ConnectedUrl())
		}),
	}, opts...)
	connectOpts = append(connectOpts, credentials, gnats.Name(name))
	nc, err := gnats.Connect(hosts, connectOpts...)
	if err != nil {
		return nil, fmt.Errorf("error connecting to NATS hosts at %s. %w", hosts, err)
	}
	rtt, err := nc.RTT()
	if err != nil {
		return nil, fmt.Errorf("error testing round trip to NATS hosts at %s. %w", hosts, err)
	}
	log.Debug("NATS ping rtt: %v, host: %s (%s)", rtt, nc.ConnectedUrl(), nc.ConnectedServerName())
	return nc, nil
}
