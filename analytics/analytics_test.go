package analytics

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	natsserver "github.com/nats-io/nats-server/v2/test"
	"github.com/nats-io/nats.go"
	"github.com/shopmonkeyus/go-common/logger"
	gnats "github.com/shopmonkeyus/go-common/nats"
	cstring "github.com/shopmonkeyus/go-common/string"
	"github.com/stretchr/testify/assert"
)

func RunTestServer(js bool) *server.Server {
	opts := natsserver.DefaultTestOptions
	opts.Port = 8222
	opts.Cluster.Name = "testing"
	opts.JetStream = js
	return natsserver.RunServer(&opts)
}

func TestAnalyticsBasic(t *testing.T) {
	server := RunTestServer(true)
	defer server.Shutdown()
	log := logger.NewTestLogger()
	n, err := gnats.NewNats(log, "test", "nats://localhost:8222", nil)
	assert.NoError(t, err, "failed to connect to nats")
	assert.NotNil(t, n, "result was nil")
	defer n.Close()
	js, err := n.JetStream()
	assert.NoError(t, err)
	js.AddStream(&nats.StreamConfig{
		Name:     "analytics",
		Subjects: []string{"analytics.>"},
	})
	var event Event
	var msg *nats.Msg
	handler := func(ctx context.Context, payload []byte, _msg *nats.Msg) error {
		if err := json.Unmarshal(payload, &event); err != nil {
			return err
		}
		msg = _msg
		return msg.AckSync()
	}
	sub, err := gnats.NewEphemeralConsumer(log, js, "analytics", "analytics.>", handler)
	assert.NoError(t, err)
	defer sub.Close()
	analytics, err := New(context.Background(), log, js)
	assert.NoError(t, err)
	assert.NoError(t, analytics.Queue("test", "click", "companyId", "locationId", nil))
	analytics.Close()
	time.Sleep(time.Millisecond * 100)
	assert.Equal(t, "dev", event.Region)
	assert.Equal(t, "dev", event.Branch)
	assert.Equal(t, "test", event.Name)
	assert.Equal(t, "click", event.Action)
	assert.NotEmpty(t, event.Timestamp)
	assert.False(t, event.Timestamp.IsZero())
	assert.Nil(t, event.Data)
	assert.Equal(t, "companyId", event.CompanyId)
	assert.Equal(t, "locationId", event.LocationId)
	assert.Nil(t, event.SessionId)
	assert.Nil(t, event.UserId)
	assert.Nil(t, event.RequestId)
	assert.Equal(t, "dev", msg.Header.Get("region"))
	assert.Equal(t, "companyId", msg.Header.Get("x-company-id"))
	assert.Equal(t, "locationId", msg.Header.Get("x-location-id"))
	assert.Empty(t, "", msg.Header.Get("x-user-id"))
	assert.NotEmpty(t, msg.Header.Get("Nats-Msg-Id"))
	assert.Equal(t, "analytics.companyId.locationId.test.click", msg.Subject)
}

func TestAnalyticsWithOverride(t *testing.T) {
	server := RunTestServer(true)
	defer server.Shutdown()
	log := logger.NewTestLogger()
	n, err := gnats.NewNats(log, "test", "nats://localhost:8222", nil)
	assert.NoError(t, err, "failed to connect to nats")
	assert.NotNil(t, n, "result was nil")
	defer n.Close()
	js, err := n.JetStream()
	assert.NoError(t, err)
	js.AddStream(&nats.StreamConfig{
		Name:     "analytics",
		Subjects: []string{"analytics.>"},
	})
	var event Event
	var msg *nats.Msg
	handler := func(ctx context.Context, payload []byte, _msg *nats.Msg) error {
		if err := json.Unmarshal(payload, &event); err != nil {
			return err
		}
		msg = _msg
		return msg.AckSync()
	}
	sub, err := gnats.NewEphemeralConsumer(log, js, "analytics", "analytics.>", handler)
	assert.NoError(t, err)
	defer sub.Close()
	id, err := cstring.GenerateRandomString(10)
	assert.NoError(t, err)
	analytics, err := New(context.Background(), log, js)
	assert.NoError(t, err)
	assert.NoError(t, analytics.Queue("test", "click", "companyId", "locationId", map[string]interface{}{"foo": "bar"},
		WithRegion("region"),
		WithBranch("branch"),
		WithUserId("userid"),
		WithSessionId("sessionid"),
		WithRequestId("requestid"),
		WithMessageId(id),
	))
	analytics.Close()
	time.Sleep(time.Millisecond * 100)
	assert.Equal(t, "region", event.Region)
	assert.Equal(t, "branch", event.Branch)
	assert.Equal(t, "test", event.Name)
	assert.Equal(t, "click", event.Action)
	assert.NotEmpty(t, event.Timestamp)
	assert.False(t, event.Timestamp.IsZero())
	assert.NotNil(t, event.Data)
	assert.NotNil(t, event.CompanyId)
	assert.NotNil(t, event.LocationId)
	assert.NotNil(t, event.SessionId)
	assert.NotNil(t, event.UserId)
	assert.NotNil(t, event.RequestId)
	assert.Equal(t, "companyId", event.CompanyId)
	assert.Equal(t, "locationId", event.LocationId)
	assert.Equal(t, "sessionid", *event.SessionId)
	assert.Equal(t, "userid", *event.UserId)
	assert.Equal(t, "requestid", *event.RequestId)
	assert.Equal(t, "region", msg.Header.Get("region"))
	assert.Equal(t, "companyId", msg.Header.Get("x-company-id"))
	assert.Equal(t, "locationId", msg.Header.Get("x-location-id"))
	assert.Equal(t, "userid", msg.Header.Get("x-user-id"))
	assert.Equal(t, id, msg.Header.Get("Nats-Msg-Id"))
	assert.Equal(t, map[string]interface{}{"foo": "bar"}, event.Data)
	assert.Equal(t, "analytics.companyId.locationId.test.click", msg.Subject)
}

func TestAnalyticsWithNoCompanyOrLocation(t *testing.T) {
	server := RunTestServer(true)
	defer server.Shutdown()
	log := logger.NewTestLogger()
	n, err := gnats.NewNats(log, "test", "nats://localhost:8222", nil)
	assert.NoError(t, err, "failed to connect to nats")
	assert.NotNil(t, n, "result was nil")
	defer n.Close()
	js, err := n.JetStream()
	assert.NoError(t, err)
	js.AddStream(&nats.StreamConfig{
		Name:     "analytics",
		Subjects: []string{"analytics.>"},
	})
	var event Event
	var msg *nats.Msg
	handler := func(ctx context.Context, payload []byte, _msg *nats.Msg) error {
		if err := json.Unmarshal(payload, &event); err != nil {
			return err
		}
		msg = _msg
		return msg.AckSync()
	}
	sub, err := gnats.NewEphemeralConsumer(log, js, "analytics", "analytics.>", handler)
	assert.NoError(t, err)
	defer sub.Close()
	id, err := cstring.GenerateRandomString(10)
	assert.NoError(t, err)
	analytics, err := New(context.Background(), log, js)
	assert.NoError(t, err)
	assert.NoError(t, analytics.Queue("test", "click", "", "", map[string]interface{}{"foo": "bar"},
		WithRegion("region"),
		WithBranch("branch"),
		WithUserId("userid"),
		WithSessionId("sessionid"),
		WithRequestId("requestid"),
		WithMessageId(id),
	))
	analytics.Close()
	time.Sleep(time.Millisecond * 100)
	assert.Equal(t, "region", event.Region)
	assert.Equal(t, "branch", event.Branch)
	assert.Equal(t, "test", event.Name)
	assert.Equal(t, "click", event.Action)
	assert.NotEmpty(t, event.Timestamp)
	assert.False(t, event.Timestamp.IsZero())
	assert.NotNil(t, event.Data)
	assert.NotNil(t, event.CompanyId)
	assert.NotNil(t, event.LocationId)
	assert.NotNil(t, event.SessionId)
	assert.NotNil(t, event.UserId)
	assert.NotNil(t, event.RequestId)
	assert.Empty(t, event.CompanyId)
	assert.Empty(t, event.LocationId)
	assert.Equal(t, "sessionid", *event.SessionId)
	assert.Equal(t, "userid", *event.UserId)
	assert.Equal(t, "requestid", *event.RequestId)
	assert.Equal(t, "region", msg.Header.Get("region"))
	assert.Empty(t, msg.Header.Get("x-company-id"))
	assert.Empty(t, msg.Header.Get("x-location-id"))
	assert.Equal(t, "userid", msg.Header.Get("x-user-id"))
	assert.Equal(t, id, msg.Header.Get("Nats-Msg-Id"))
	assert.Equal(t, map[string]interface{}{"foo": "bar"}, event.Data)
	assert.Equal(t, "analytics.NONE.NONE.test.click", msg.Subject)
}

func TestAnalyticsClosedErorr(t *testing.T) {
	server := RunTestServer(true)
	defer server.Shutdown()
	log := logger.NewTestLogger()
	n, err := gnats.NewNats(log, "test", "nats://localhost:8222", nil)
	assert.NoError(t, err, "failed to connect to nats")
	assert.NotNil(t, n, "result was nil")
	defer n.Close()
	js, err := n.JetStream()
	assert.NoError(t, err)
	js.AddStream(&nats.StreamConfig{
		Name:     "analytics",
		Subjects: []string{"analytics.>"},
	})
	analytics, err := New(context.Background(), log, js)
	assert.NoError(t, err)
	analytics.Close()
	err = analytics.Queue("test", "click", "companyId", "locationId", nil)
	assert.EqualError(t, err, ErrTrackerClosed.Error())
}
