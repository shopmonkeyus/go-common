package nats

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	natsserver "github.com/nats-io/nats-server/v2/test"
	"github.com/nats-io/nats.go"
	"github.com/shopmonkeyus/go-common/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack"
)

// receivedMsg records the last message seen by a handler goroutine so tests
// can read it without a data race
type receivedMsg struct {
	lock  sync.Mutex
	data  string
	msgid string
}

func (r *receivedMsg) set(data, msgid string) {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.data = data
	r.msgid = msgid
}

func (r *receivedMsg) get() (string, string) {
	r.lock.Lock()
	defer r.lock.Unlock()
	return r.data, r.msgid
}

// received reports whether a message has been recorded yet
func (r *receivedMsg) received() bool {
	data, _ := r.get()
	return data != ""
}

// handler returns a Handler that records the message and acks it
func (r *receivedMsg) handler(t *testing.T) Handler {
	return func(ctx context.Context, buf []byte, msg *nats.Msg) error {
		msgid := GetMsgIdFromHeader(msg)
		t.Log("received:", string(buf), "msgid:", msgid)
		r.set(string(buf), msgid)
		return msg.AckSync()
	}
}

func RunTestServer(js bool) *server.Server {
	opts := natsserver.DefaultTestOptions
	opts.Port = 8222
	opts.Cluster.Name = "testing"
	opts.JetStream = js
	return natsserver.RunServer(&opts)
}

// jetStreamTest starts a jetstream test server, connects to it and creates a
// stream whose name starts with prefix, tearing everything down when the test
// ends. Nanosecond resolution matters for the stream name: the jetstream
// store dir persists across test server restarts, so a second-resolution name
// could collide with a recovered stream from a previous run and its msgid
// dedupe state
func jetStreamTest(t *testing.T, log logger.Logger, prefix string) (nats.JetStreamContext, string) {
	t.Helper()
	srv := RunTestServer(true)
	t.Cleanup(srv.Shutdown)
	n, err := NewNats(log, "test", srv.ClientURL(), nil)
	require.NoError(t, err, "failed to connect to nats")
	t.Cleanup(n.Close)
	js, err := n.JetStream()
	require.NoError(t, err, "failed to create jetstream")
	stream := fmt.Sprintf("%s%v", prefix, time.Now().UnixNano())
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     stream,
		Subjects: []string{stream + ".>"},
	})
	require.NoError(t, err, "failed to create stream")
	return js, stream
}

func testNewNats(t *testing.T, extraHosts string, opts ...nats.Option) {
	t.Helper()
	srv := RunTestServer(false)
	defer srv.Shutdown()
	log := logger.NewTestLogger()
	n, err := NewNats(log, "test", srv.ClientURL()+extraHosts, nil, opts...)
	assert.NoError(t, err, "failed to connect to nats")
	assert.NotNil(t, n, "result was nil")
	n.Close()
	srv.Shutdown()
	assert.Len(t, log.Logs, 1, "invalid number of log entries")
	assert.Equal(t, "DEBUG", log.Logs[0].Severity)
	assert.Equal(t, "NATS ping rtt: %v, host: %s (%s)", log.Logs[0].Message)
	assert.Len(t, log.Logs[0].Arguments, 3)
	assert.Equal(t, srv.ClientURL(), log.Logs[0].Arguments[1])
	assert.Len(t, log.Logs[0].Arguments[2], 56, "invalid nats id")
}

func TestNats(t *testing.T) {
	testNewNats(t, "")
}

func TestNatsWithOpts(t *testing.T) {
	testNewNats(t, ",nats://localhost:9822,nats://localhost:9100", nats.DontRandomize())
}

func TestExactlyOnceConsumer(t *testing.T) {
	log := logger.NewTestLogger()
	js, stream := jetStreamTest(t, log, "stream")
	var rcv receivedMsg
	sub, err := NewExactlyOnceConsumer(log, js, stream, "test", stream+".*", rcv.handler(t), WithExactlyOnceReplicas(1))
	require.NoError(t, err, "failed to create consumer")
	defer sub.Close()
	msgID := fmt.Sprintf("%v", time.Now().Unix())
	_, err = js.Publish(stream+".test", []byte("hi"), nats.MsgId(msgID))
	assert.NoError(t, err, "failed to publish")
	assert.Eventually(t, rcv.received, 10*time.Second, 50*time.Millisecond, "message not received")
	data, msgid := rcv.get()
	assert.Equal(t, "hi", data, "message didnt match")
	assert.Equal(t, msgID, msgid, "msgid didnt match")
	ci, err := js.ConsumerInfo(stream, "test")
	require.NoError(t, err)
	assert.Equal(t, "exactly once consumer for "+stream, ci.Config.Description)
}

func TestExactlyOnceConsumerWithMsgPack(t *testing.T) {
	log := logger.NewTestLogger()
	js, stream := jetStreamTest(t, log, "streammsg")
	var rcv receivedMsg
	sub, err := NewExactlyOnceConsumer(log, js, stream, "test2", stream+".*", rcv.handler(t), WithExactlyOnceReplicas(1))
	require.NoError(t, err, "failed to create consumer")
	defer sub.Close()
	var buf bytes.Buffer
	require.NoError(t, msgpack.NewEncoder(&buf).Encode(map[string]any{"hi": "there"}))
	msg := nats.NewMsg(stream + ".test")
	msg.Data = buf.Bytes()
	SetContentEncodingHeader(msg, "msgpack")
	msgID := fmt.Sprintf("%v", time.Now().Unix())
	_, err = js.PublishMsg(msg, nats.MsgId(msgID))
	assert.NoError(t, err, "failed to publish")
	assert.Eventually(t, rcv.received, 10*time.Second, 50*time.Millisecond, "message not received")
	data, msgid := rcv.get()
	assert.Equal(t, `{"hi":"there"}`, data, "message didnt match")
	assert.Equal(t, msgID, msgid, "msgid didnt match")
	ci, err := js.ConsumerInfo(stream, "test2")
	require.NoError(t, err)
	assert.Equal(t, "exactly once consumer for "+stream, ci.Config.Description)
}

func TestQueueConsumer(t *testing.T) {
	log := logger.NewTestLogger()
	js, stream := jetStreamTest(t, log, "qc")
	var rcv1, rcv2 receivedMsg
	sub1, err := NewQueueConsumer(log, js, stream, "qtest1", stream+".*", rcv1.handler(t), WithQueueReplicas(1))
	require.NoError(t, err, "failed to create consumer 1")
	defer sub1.Close()
	sub2, err := NewQueueConsumer(log, js, stream, "qtest2", stream+".*", rcv2.handler(t), WithQueueReplicas(1))
	require.NoError(t, err, "failed to create consumer 2")
	defer sub2.Close()
	msgID := fmt.Sprintf("%v", time.Now().Unix())
	_, err = js.Publish(stream+".test", []byte("hi"), nats.MsgId(msgID))
	assert.NoError(t, err, "failed to publish")
	// each durable gets its own copy of the message
	for i, rcv := range []*receivedMsg{&rcv1, &rcv2} {
		assert.Eventually(t, rcv.received, 10*time.Second, 50*time.Millisecond, "message %d not received", i+1)
		data, msgid := rcv.get()
		assert.Equal(t, "hi", data, "message %d didnt match", i+1)
		assert.Equal(t, msgID, msgid, "msgid %d didnt match", i+1)
	}
	ci, err := js.ConsumerInfo(stream, "qtest1")
	require.NoError(t, err)
	assert.Equal(t, "queue consumer for "+stream, ci.Config.Description)
}

func TestQueueConsumerLoadBalanced(t *testing.T) {
	log := logger.NewTestLogger()
	js, stream := jetStreamTest(t, log, "queuel")
	// the two consumers share one durable, so each message is delivered to
	// exactly one of them. which consumer gets which message depends on whose
	// pull request reaches the server first, so assert on the union instead of
	// a fixed pairing
	var lock sync.Mutex
	received := make(map[string]string) // msgid -> payload
	var deliveries int
	handler := func(who string) Handler {
		return func(ctx context.Context, buf []byte, msg *nats.Msg) error {
			if err := msg.AckSync(); err != nil {
				return err
			}
			msgid := GetMsgIdFromHeader(msg)
			t.Log(who, "received:", string(buf), "msgid:", msgid)
			lock.Lock()
			received[msgid] = string(buf)
			deliveries++
			lock.Unlock()
			return nil
		}
	}
	sub1, err := NewQueueConsumer(log, js, stream, "qtest1", stream+".>", handler("1"), WithQueueReplicas(1))
	require.NoError(t, err, "failed to create consumer 1")
	defer sub1.Close()
	sub2, err := NewQueueConsumer(log, js, stream, "qtest1", stream+".>", handler("2"), WithQueueReplicas(1))
	require.NoError(t, err, "failed to create consumer 2")
	defer sub2.Close()
	msgID1 := fmt.Sprintf("a-%v", time.Now().UnixNano())
	msgID2 := fmt.Sprintf("b-%v", time.Now().UnixNano())
	_, err = js.Publish(stream+".test", []byte(msgID1), nats.MsgId(msgID1))
	assert.NoError(t, err, "failed to publish")
	_, err = js.Publish(stream+".test", []byte(msgID2), nats.MsgId(msgID2))
	assert.NoError(t, err, "failed to publish")
	assert.Eventually(t, func() bool {
		lock.Lock()
		defer lock.Unlock()
		return len(received) == 2
	}, 10*time.Second, 50*time.Millisecond, "both messages not received")
	lock.Lock()
	assert.Equal(t, msgID1, received[msgID1], "message1 didnt match")
	assert.Equal(t, msgID2, received[msgID2], "message2 didnt match")
	assert.Equal(t, 2, deliveries, "each message should be delivered exactly once")
	lock.Unlock()
}

func TestQueueConsumerResubscribesWhenConsumerBreaks(t *testing.T) {
	log := logger.NewTestLogger()
	js, stream := jetStreamTest(t, log, "resub")
	var lock sync.Mutex
	received := make(map[string]bool)
	handler := func(ctx context.Context, buf []byte, msg *nats.Msg) error {
		// ack before recording so the test can't delete the consumer while the
		// ack is still in flight
		if err := msg.AckSync(); err != nil {
			return err
		}
		lock.Lock()
		received[string(buf)] = true
		lock.Unlock()
		return nil
	}
	got := func(key string) func() bool {
		return func() bool {
			lock.Lock()
			defer lock.Unlock()
			return received[key]
		}
	}
	sub, err := NewQueueConsumer(log, js, stream, "resub", stream+".*", handler, WithQueueReplicas(1), WithQueueDelivery(nats.DeliverAllPolicy))
	require.NoError(t, err, "failed to create consumer")
	defer sub.Close()
	_, err = js.Publish(stream+".test", []byte("before"))
	assert.NoError(t, err, "failed to publish")
	assert.Eventually(t, got("before"), 10*time.Second, 50*time.Millisecond, "first message not received")

	// deleting the consumer fails the pending fetch with a terminal error (the
	// same error class as a jetstream leadership change) and the subscriber
	// must recover by recreating the subscription
	require.NoError(t, js.DeleteConsumer(stream, "resub"), "failed to delete consumer")
	_, err = js.Publish(stream+".test", []byte("after"))
	assert.NoError(t, err, "failed to publish")
	assert.Eventually(t, got("after"), 15*time.Second, 50*time.Millisecond, "message after resubscribe not received")
}

func TestEphemeralConsumer(t *testing.T) {
	log := logger.NewTestLogger()
	js, stream := jetStreamTest(t, log, "ephem")
	subject := stream + ".>"
	var received, msgid string
	var wg sync.WaitGroup
	handler := func(ctx context.Context, buf []byte, msg *nats.Msg) error {
		defer wg.Done()
		received = string(buf)
		msgid = GetMsgIdFromHeader(msg)
		t.Log("received:", received, "msgid:", msgid)
		return msg.AckSync()
	}
	wg.Add(1)
	sub1, err := NewEphemeralConsumer(log, js, stream, subject, handler)
	require.NoError(t, err, "failed to create consumer 1")
	msgID := fmt.Sprintf("a-%v", time.Now().Unix())
	_, err = js.Publish(stream+".test", []byte(msgID), nats.MsgId(msgID))
	assert.NoError(t, err, "failed to publish")
	wg.Wait()
	assert.Equal(t, msgID, received, "message1 didnt match")
	assert.Equal(t, msgID, msgid, "msgid1 didnt match")
	sub1.Close()

	// a fresh consumer with the deliver-all policy re-receives the message
	received, msgid = "", ""
	wg.Add(1)
	sub2, err := NewEphemeralConsumer(log, js, stream, subject, handler, WithEphemeralDelivery(nats.DeliverAllPolicy))
	require.NoError(t, err, "failed to create consumer 2")
	wg.Wait()
	assert.Equal(t, msgID, received, "message2 didnt match")
	assert.Equal(t, msgID, msgid, "msgid2 didnt match")
	sub2.Close()

	received, msgid = "", ""
	wg.Add(1)
	sub3, err := NewEphemeralConsumer(log, js, stream, subject, handler, WithEphemeralDelivery(nats.DeliverAllPolicy))
	require.NoError(t, err, "failed to create consumer 3")
	wg.Wait()
	assert.Equal(t, msgID, received, "message3 didnt match")
	assert.Equal(t, msgID, msgid, "msgid3 didnt match")
	ci := <-js.Consumers(stream)
	require.NotNil(t, ci)
	assert.Equal(t, "ephemeral consumer for "+stream, ci.Config.Description)
	sub3.Close()
}

func TestEphemeralConsumerAutoExtend(t *testing.T) {
	log := logger.NewConsoleLogger()
	js, stream := jetStreamTest(t, log, "aephem")
	var rcv receivedMsg
	handler := func(ctx context.Context, buf []byte, msg *nats.Msg) error {
		msgid := GetMsgIdFromHeader(msg)
		log.Info("received: %s, msgid: %s", string(buf), msgid)
		time.Sleep(time.Second * 5) // block to force the extender to run
		rcv.set(string(buf), msgid)
		return msg.AckSync()
	}
	sub, err := NewEphemeralConsumer(log, js, stream, stream+".>", handler, WithEphemeralAckWait(time.Second*2))
	require.NoError(t, err, "failed to create consumer 1")
	defer sub.Close()
	msgID := fmt.Sprintf("a-%v", time.Now().Unix())
	_, err = js.Publish(stream+".test", []byte(msgID), nats.MsgId(msgID))
	assert.NoError(t, err, "failed to publish")
	assert.Eventually(t, rcv.received, 15*time.Second, 100*time.Millisecond, "message not received")
	data, msgid := rcv.get()
	assert.Equal(t, msgID, data, "message1 didnt match")
	assert.Equal(t, msgID, msgid, "msgid1 didnt match")
}

// testConsumerConfigChanged pre-creates a durable whose config differs from
// what the constructor wants (empty description) and verifies the constructor
// updates the consumer instead of failing
func testConsumerConfigChanged(t *testing.T, maxAckPending int, newConsumer func(log logger.Logger, js nats.JetStreamContext, stream string, handler Handler) (Subscriber, error)) {
	t.Helper()
	log := logger.NewTestLogger()
	js, stream := jetStreamTest(t, log, "cfg")
	handler := func(ctx context.Context, buf []byte, msg *nats.Msg) error {
		return nil
	}
	ci, err := js.AddConsumer(stream, &nats.ConsumerConfig{
		Durable:       "test",
		Name:          "test",
		FilterSubject: stream + ".*",
		AckPolicy:     nats.AckExplicitPolicy,
		DeliverPolicy: nats.DeliverNewPolicy,
		MaxDeliver:    1,
		MaxAckPending: maxAckPending,
		Replicas:      1,
	})
	require.NoError(t, err)
	require.NotNil(t, ci)
	sub, err := newConsumer(log, js, stream, handler)
	require.NoError(t, err)
	require.NotNil(t, sub)
	sub.Close()
}

func TestExactlyOnceConsumerConfigChanged(t *testing.T) {
	testConsumerConfigChanged(t, 1, func(log logger.Logger, js nats.JetStreamContext, stream string, handler Handler) (Subscriber, error) {
		return NewExactlyOnceConsumer(log, js, stream, "test", stream+".*", handler, WithExactlyOnceReplicas(1))
	})
}

func TestQueueConsumerConfigChanged(t *testing.T) {
	testConsumerConfigChanged(t, 1000, func(log logger.Logger, js nats.JetStreamContext, stream string, handler Handler) (Subscriber, error) {
		return NewQueueConsumer(log, js, stream, "test", stream+".*", handler, WithQueueReplicas(1))
	})
}

func TestDiffConfig(t *testing.T) {
	msg, ok := diffConfig(nats.ConsumerConfig{
		Durable:       "test",
		Name:          "test",
		Description:   "",
		FilterSubject: "test.*",
		AckPolicy:     nats.AckExplicitPolicy,
		DeliverPolicy: nats.DeliverNewPolicy,
		MaxDeliver:    1,
		MaxAckPending: 1000,
		Replicas:      1,
	}, nats.ConsumerConfig{
		Durable:       "test",
		Name:          "test",
		Description:   "",
		FilterSubject: "test.>",
		AckPolicy:     nats.AckExplicitPolicy,
		DeliverPolicy: nats.DeliverNewPolicy,
		MaxDeliver:    1,
		MaxAckPending: 1000,
		Replicas:      1,
	})
	assert.False(t, ok)
	assert.Equal(t, "filter subject: test.* != test.>", msg)
}

func TestDiffConfig2(t *testing.T) {
	msg, ok := diffConfig(nats.ConsumerConfig{
		Durable:         "test",
		Name:            "test",
		Description:     "",
		FilterSubject:   "test.*",
		AckPolicy:       nats.AckExplicitPolicy,
		DeliverPolicy:   nats.DeliverNewPolicy,
		MaxDeliver:      1,
		MaxAckPending:   1000,
		Replicas:        1,
		MaxRequestBatch: 10,
	}, nats.ConsumerConfig{
		Durable:         "test",
		Name:            "test",
		Description:     "",
		FilterSubject:   "test.*",
		AckPolicy:       nats.AckExplicitPolicy,
		DeliverPolicy:   nats.DeliverNewPolicy,
		MaxDeliver:      1,
		MaxAckPending:   1000,
		Replicas:        1,
		MaxRequestBatch: 100,
	})
	assert.False(t, ok)
	assert.Equal(t, "max_fetch: 10 != 100", msg)
}
