package nats

import (
	"encoding/json"

	gnats "github.com/nats-io/nats.go"
	"github.com/shopmonkeyus/go-common/compress"
	"github.com/vmihailenco/msgpack/v5"
)

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
