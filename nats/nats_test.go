package nats

import (
	"fmt"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	natsserver "github.com/nats-io/nats-server/v2/test"
	"github.com/nats-io/nats.go"
	"github.com/shopmonkeyus/go-common/logger"
	"github.com/stretchr/testify/assert"
)

func RunTestServer(js bool) *server.Server {
	opts := natsserver.DefaultTestOptions
	opts.Port = 8222
	opts.Cluster.Name = "testing"
	opts.JetStream = js
	return natsserver.RunServer(&opts)
}

func TestNats(t *testing.T) {
	server := RunTestServer(false)
	log := logger.NewTestLogger()
	n, err := NewNats(log, "test", "nats://localhost:8222", nil)
	assert.NoError(t, err, "failed to connect to nats")
	assert.NotNil(t, n, "result was nil")
	n.Close()
	server.Shutdown()
	assert.Len(t, log.Logs, 1, "invalid number of log entries")
	assert.Equal(t, "DEBUG", log.Logs[0].Severity)
	assert.Equal(t, "NATS ping rtt: %v, host: %s (%s)", log.Logs[0].Message)
	assert.Len(t, log.Logs[0].Arguments, 3)
	assert.Equal(t, "nats://localhost:8222", log.Logs[0].Arguments[1])
	assert.Len(t, log.Logs[0].Arguments[2], 56, "invalid nats id")
}

func TestNatsWithOpts(t *testing.T) {
	server := RunTestServer(false)
	log := logger.NewTestLogger()
	n, err := NewNats(log, "test", "nats://localhost:8222,nats://foo:9822,nats://bar:9100", nil, nats.DontRandomize())
	assert.NoError(t, err, "failed to connect to nats")
	assert.NotNil(t, n, "result was nil")
	n.Close()
	server.Shutdown()
	assert.Len(t, log.Logs, 1, "invalid number of log entries")
	assert.Equal(t, "DEBUG", log.Logs[0].Severity)
	assert.Equal(t, "NATS ping rtt: %v, host: %s (%s)", log.Logs[0].Message)
	assert.Len(t, log.Logs[0].Arguments, 3)
	assert.Equal(t, "nats://localhost:8222", log.Logs[0].Arguments[1])
	assert.Len(t, log.Logs[0].Arguments[2], 56, "invalid nats id")
}

func TestExactlyOnceConsumer(t *testing.T) {
	server := RunTestServer(true)
	log := logger.NewTestLogger()
	n, err := NewNats(log, "test", "nats://localhost:8222", nil)
	assert.NoError(t, err, "failed to connect to nats")
	assert.NotNil(t, n, "result was nil")
	js, err := n.JetStream()
	assert.NoError(t, err, "failed to create jetstream")
	assert.NotNil(t, js, "js result was nil")
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     "test",
		Subjects: []string{"test.>"},
	})
	assert.NoError(t, err, "failed to create stream")
	var received string
	var msgid string
	handler := func(buf []byte, _msgid string) error {
		t.Log("received:", string(buf), "msgid:", _msgid)
		received = string(buf)
		msgid = _msgid
		return nil
	}
	sub, err := NewExactlyOnceConsumer(log, js, "test", "test", "test", "test.*", handler)
	assert.NoError(t, err, "failed to create consumer")
	assert.NotNil(t, sub, "sub result was nil")
	_msgid := fmt.Sprintf("%v", time.Now().Unix())
	_, err = js.Publish("test.test", []byte("hi"), nats.MsgId(_msgid))
	assert.NoError(t, err, "failed to publish")
	time.Sleep(time.Millisecond * 100)
	assert.Equal(t, "hi", received, "message didnt match")
	assert.Equal(t, _msgid, msgid, "msgid didnt match")
	sub.Close()
	n.Close()
	server.Shutdown()
}
