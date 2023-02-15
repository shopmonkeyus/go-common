package nats

import (
	"testing"

	"github.com/nats-io/nats-server/v2/server"
	natsserver "github.com/nats-io/nats-server/v2/test"
	"github.com/nats-io/nats.go"
	"github.com/shopmonkeyus/go-common/logger"
	"github.com/stretchr/testify/assert"
)

func RunTestServer() *server.Server {
	opts := natsserver.DefaultTestOptions
	opts.Port = 8222
	opts.Cluster.Name = "testing"
	return natsserver.RunServer(&opts)
}

func TestNats(t *testing.T) {
	server := RunTestServer()
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
	server := RunTestServer()
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
