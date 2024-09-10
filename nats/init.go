package nats

import (
	"fmt"

	gnats "github.com/nats-io/nats.go"
	"github.com/shopmonkeyus/go-common/logger"
)

// NewNats will return a new nats connections
func NewNats(log logger.Logger, name string, hosts string, credentials gnats.Option, opts ...gnats.Option) (*gnats.Conn, error) {
	_opts := make([]gnats.Option, len(opts))
	copy(_opts, opts)
	_opts = append(_opts, credentials)
	_opts = append(_opts, gnats.Name(name))
	nc, err := gnats.Connect(
		hosts,
		_opts...,
	)
	if err != nil {
		return nil, fmt.Errorf("error connecting to NATS hosts at %s. %w", hosts, err)
	}
	d, err := nc.RTT()
	if err != nil {
		return nil, fmt.Errorf("error testing round trip to NATS hosts at %s. %w", hosts, err)
	}
	log.Debug("NATS ping rtt: %v, host: %s (%s)", d, nc.ConnectedUrl(), nc.ConnectedServerName())
	return nc, nil
}
