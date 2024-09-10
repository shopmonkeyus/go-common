package nats

import (
	"encoding/json"
	"fmt"

	gnats "github.com/nats-io/nats.go"
	"github.com/shopmonkeyus/go-common/compress"
	"github.com/shopmonkeyus/go-common/logger"
	"github.com/vmihailenco/msgpack/v5"
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

// DecodeNatsMsg will decode the nats message into the provided interface.
func DecodeNatsMsg(msg *gnats.Msg, v interface{}) error {
	encoding := GetContentEncodingFromHeader(msg)
	gzipped := encoding == "gzip/json"
	msgpacked := encoding == "msgpack"
	var err error
	data := msg.Data
	if gzipped {
		data, err = compress.Gunzip(data)
	} else if msgpacked {
		var o any
		err = msgpack.Unmarshal(data, &o)
		if err == nil {
			data, err = json.Marshal(o)
		}
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return err
	}
	return nil
}
