package nats

import (
	"fmt"

	gnats "github.com/nats-io/nats.go"
	"github.com/shopmonkeyus/go-common/logger"
)

// NewNats will return a new nats connections
func NewNats(log logger.Logger, name string, hosts string, credentials gnats.Option) (*gnats.Conn, error) {
	nc, err := gnats.Connect(
		hosts,
		gnats.Name(name),
		credentials,
	)
	if err != nil {
		return nil, fmt.Errorf("error connecting to NATS hosts at %s. %s", hosts, err)
	}
	d, err := nc.RTT()
	if err != nil {
		return nil, fmt.Errorf("error testing round trip to NATS hosts at %s. %s", hosts, err)
	}
	log.Debug("NATS ping rtt: %v, host: %s (%s)", d, nc.ConnectedUrl(), nc.ConnectedServerName())
	return nc, nil
}
