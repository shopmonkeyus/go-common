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
	"github.com/stretchr/testify/require"
)

func RunTestServer(js bool) *server.Server {
	opts := natsserver.DefaultTestOptions
	opts.Port = 8221
	opts.Cluster.Name = "testing"
	opts.JetStream = js
	return natsserver.RunServer(&opts)
}

// setupAnalyticsTest starts a jetstream test server with an analytics stream
// and returns a connected jetstream context, tearing everything down when the
// test ends
func setupAnalyticsTest(t *testing.T) (*logger.TestLogger, nats.JetStreamContext) {
	t.Helper()
	srv := RunTestServer(true)
	t.Cleanup(srv.Shutdown)
	log := logger.NewTestLogger()
	n, err := gnats.NewNats(log, "test", srv.ClientURL(), nil)
	require.NoError(t, err, "failed to connect to nats")
	t.Cleanup(n.Close)
	js, err := n.JetStream()
	require.NoError(t, err)
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     "analytics",
		Subjects: []string{"analytics.>"},
	})
	require.NoError(t, err, "failed to create stream")
	return log, js
}

// capturedEvent records the first analytics event seen by an ephemeral
// consumer so tests can assert on it after wait returns
type capturedEvent struct {
	event    Event
	msg      *nats.Msg
	received chan struct{}
}

// subscribeCapture subscribes an ephemeral consumer on the analytics stream
// that captures the first event it receives
func subscribeCapture(t *testing.T, log logger.Logger, js nats.JetStreamContext) *capturedEvent {
	t.Helper()
	c := &capturedEvent{received: make(chan struct{})}
	handler := func(ctx context.Context, payload []byte, msg *nats.Msg) error {
		if err := json.Unmarshal(payload, &c.event); err != nil {
			return err
		}
		c.msg = msg
		if err := msg.AckSync(); err != nil {
			return err
		}
		close(c.received)
		return nil
	}
	sub, err := gnats.NewEphemeralConsumer(log, js, "analytics", "analytics.>", handler)
	require.NoError(t, err)
	t.Cleanup(func() { sub.Close() })
	return c
}

// wait blocks until the event arrives or fails the test after a timeout
func (c *capturedEvent) wait(t *testing.T) {
	t.Helper()
	select {
	case <-c.received:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for analytics event")
	}
}

func TestAnalyticsBasic(t *testing.T) {
	log, js := setupAnalyticsTest(t)
	c := subscribeCapture(t, log, js)
	analytics, err := New(context.Background(), log, js)
	require.NoError(t, err)
	assert.NoError(t, analytics.Queue("test", "companyId", "locationId", nil))
	analytics.Close()
	c.wait(t)
	event, msg := c.event, c.msg
	assert.Equal(t, "dev", event.Region)
	assert.Equal(t, "dev", event.Branch)
	assert.Equal(t, "test", event.Name)
	assert.False(t, event.Timestamp.IsZero())
	require.NotNil(t, event.Data)
	data := event.Data.(map[string]any)
	assert.Nil(t, data["payload"])
	require.NotNil(t, data["context"])
	assert.Equal(t, "server", data["context"].(map[string]any)["location"])
	assert.Equal(t, "companyId", event.CompanyId)
	assert.Equal(t, "locationId", event.LocationId)
	assert.Nil(t, event.SessionId)
	assert.Nil(t, event.UserId)
	assert.Nil(t, event.RequestId)
	assert.Equal(t, "dev", gnats.GetRegionFromHeader(msg))
	assert.Equal(t, "companyId", gnats.GetCompanyIdFromHeader(msg))
	assert.Equal(t, "locationId", gnats.GetLocationIdFromHeader(msg))
	assert.Empty(t, gnats.GetUserIdFromHeader(msg))
	assert.NotEmpty(t, gnats.GetMsgIdFromHeader(msg))
	assert.Equal(t, "analytics.companyId.locationId.test", msg.Subject)
}

// testAnalyticsQueueWithOverrides exercises Queue with every override option
// set, parameterized by company/location so the NONE fallback path shares the
// same assertions
func testAnalyticsQueueWithOverrides(t *testing.T, companyId, locationId, wantSubject string) {
	t.Helper()
	log, js := setupAnalyticsTest(t)
	c := subscribeCapture(t, log, js)
	id, err := cstring.GenerateRandomString(10)
	require.NoError(t, err)
	analytics, err := New(context.Background(), log, js)
	require.NoError(t, err)
	assert.NoError(t, analytics.Queue("test", companyId, locationId, map[string]any{"foo": "bar"},
		WithRegion("region"),
		WithBranch("branch"),
		WithUserId("userid"),
		WithSessionId("sessionid"),
		WithRequestId("requestid"),
		WithMessageId(id),
	))
	analytics.Close()
	c.wait(t)
	event, msg := c.event, c.msg
	assert.Equal(t, "region", event.Region)
	assert.Equal(t, "branch", event.Branch)
	assert.Equal(t, "test", event.Name)
	assert.False(t, event.Timestamp.IsZero())
	require.NotNil(t, event.Data)
	assert.Equal(t, companyId, event.CompanyId)
	assert.Equal(t, locationId, event.LocationId)
	require.NotNil(t, event.SessionId)
	require.NotNil(t, event.UserId)
	require.NotNil(t, event.RequestId)
	assert.Equal(t, "sessionid", *event.SessionId)
	assert.Equal(t, "userid", *event.UserId)
	assert.Equal(t, "requestid", *event.RequestId)
	assert.Equal(t, "region", gnats.GetRegionFromHeader(msg))
	assert.Equal(t, companyId, gnats.GetCompanyIdFromHeader(msg))
	assert.Equal(t, locationId, gnats.GetLocationIdFromHeader(msg))
	assert.Equal(t, "userid", gnats.GetUserIdFromHeader(msg))
	assert.Equal(t, id, gnats.GetMsgIdFromHeader(msg))
	assert.Equal(t, map[string]any{"foo": "bar"}, event.Data.(map[string]any)["payload"])
	assert.Equal(t, wantSubject, msg.Subject)
}

func TestAnalyticsWithOverride(t *testing.T) {
	testAnalyticsQueueWithOverrides(t, "companyId", "locationId", "analytics.companyId.locationId.test")
}

func TestAnalyticsWithNoCompanyOrLocation(t *testing.T) {
	testAnalyticsQueueWithOverrides(t, "", "", "analytics.NONE.NONE.test")
}

func TestAnalyticsClosedError(t *testing.T) {
	log, js := setupAnalyticsTest(t)
	analytics, err := New(context.Background(), log, js)
	require.NoError(t, err)
	analytics.Close()
	err = analytics.Queue("test", "companyId", "locationId", nil)
	assert.EqualError(t, err, ErrTrackerClosed.Error())
}

func TestSafeToken(t *testing.T) {
	assert.False(t, isValidName("a b"))
	assert.False(t, isValidName("a%b"))
	assert.False(t, isValidName("a^b"))
	assert.False(t, isValidName("1bc"))
	assert.False(t, isValidName("abc."))
	assert.False(t, isValidName("abc-"))
	assert.False(t, isValidName("abc_"))
	assert.True(t, isValidName("ab"))
	assert.True(t, isValidName("a.b"))
	assert.True(t, isValidName("a.b"))
	assert.True(t, isValidName("a.b-c"))
	assert.True(t, isValidName("a.b_c"))
	assert.True(t, isValidName("a.1"))
}
