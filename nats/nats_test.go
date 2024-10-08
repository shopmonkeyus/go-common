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
	"github.com/vmihailenco/msgpack"
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
	defer server.Shutdown()
	log := logger.NewTestLogger()
	n, err := NewNats(log, "test", server.ClientURL(), nil)
	assert.NoError(t, err, "failed to connect to nats")
	assert.NotNil(t, n, "result was nil")
	n.Close()
	server.Shutdown()
	assert.Len(t, log.Logs, 1, "invalid number of log entries")
	assert.Equal(t, "DEBUG", log.Logs[0].Severity)
	assert.Equal(t, "NATS ping rtt: %v, host: %s (%s)", log.Logs[0].Message)
	assert.Len(t, log.Logs[0].Arguments, 3)
	assert.Equal(t, server.ClientURL(), log.Logs[0].Arguments[1])
	assert.Len(t, log.Logs[0].Arguments[2], 56, "invalid nats id")
}

func TestNatsWithOpts(t *testing.T) {
	server := RunTestServer(false)
	defer server.Shutdown()
	log := logger.NewTestLogger()
	n, err := NewNats(log, "test", server.ClientURL()+",nats://localhost:9822,nats://localhost:9100", nil, nats.DontRandomize())
	assert.NoError(t, err, "failed to connect to nats")
	assert.NotNil(t, n, "result was nil")
	n.Close()
	server.Shutdown()
	assert.Len(t, log.Logs, 1, "invalid number of log entries")
	assert.Equal(t, "DEBUG", log.Logs[0].Severity)
	assert.Equal(t, "NATS ping rtt: %v, host: %s (%s)", log.Logs[0].Message)
	assert.Len(t, log.Logs[0].Arguments, 3)
	assert.Equal(t, server.ClientURL(), log.Logs[0].Arguments[1])
	assert.Len(t, log.Logs[0].Arguments[2], 56, "invalid nats id")
}

func TestExactlyOnceConsumer(t *testing.T) {
	server := RunTestServer(true)
	defer server.Shutdown()
	log := logger.NewTestLogger()
	n, err := NewNats(log, "test", server.ClientURL(), nil)
	assert.NoError(t, err, "failed to connect to nats")
	assert.NotNil(t, n, "result was nil")
	js, err := n.JetStream()
	assert.NoError(t, err, "failed to create jetstream")
	assert.NotNil(t, js, "js result was nil")
	queue := fmt.Sprintf("stream%v", time.Now().Unix())
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     queue,
		Subjects: []string{queue + ".>"},
	})
	assert.NoError(t, err, "failed to create stream")
	var received string
	var msgid string
	handler := func(ctx context.Context, buf []byte, msg *nats.Msg) error {
		_msgid := GetMsgIdFromHeader(msg)
		t.Log("received:", string(buf), "msgid:", _msgid)
		received = string(buf)
		msgid = _msgid
		msg.AckSync()
		return nil
	}
	sub, err := NewExactlyOnceConsumer(log, js, queue, "test", queue+".*", handler, WithExactlyOnceReplicas(1))
	assert.NoError(t, err, "failed to create consumer")
	assert.NotNil(t, sub, "sub result was nil")
	_msgid := fmt.Sprintf("%v", time.Now().Unix())
	_, err = js.Publish(queue+".test", []byte("hi"), nats.MsgId(_msgid))
	assert.NoError(t, err, "failed to publish")
	time.Sleep(time.Millisecond * 100)
	assert.Equal(t, "hi", received, "message didnt match")
	assert.Equal(t, _msgid, msgid, "msgid didnt match")
	ci, err := js.ConsumerInfo(queue, "test")
	assert.NotNil(t, ci)
	assert.NoError(t, err)
	assert.Equal(t, "exactly once consumer for "+queue, ci.Config.Description)
	sub.Close()
	n.Close()
	server.Shutdown()
}

func TestExactlyOnceConsumerWithMsgPack(t *testing.T) {
	server := RunTestServer(true)
	defer server.Shutdown()
	log := logger.NewTestLogger()
	n, err := NewNats(log, "test", server.ClientURL(), nil)
	assert.NoError(t, err, "failed to connect to nats")
	assert.NotNil(t, n, "result was nil")
	js, err := n.JetStream()
	assert.NoError(t, err, "failed to create jetstream")
	assert.NotNil(t, js, "js result was nil")
	queue := fmt.Sprintf("streammsg%v", time.Now().Unix())
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     queue,
		Subjects: []string{queue + ".>"},
	})
	assert.NoError(t, err, "failed to create stream")
	var received string
	var msgid string
	handler := func(ctx context.Context, buf []byte, msg *nats.Msg) error {
		_msgid := GetMsgIdFromHeader(msg)
		t.Log("received:", string(buf), "msgid:", _msgid)
		received = string(buf)
		msgid = _msgid
		msg.AckSync()
		return nil
	}
	sub, err := NewExactlyOnceConsumer(log, js, queue, "test2", queue+".*", handler, WithExactlyOnceReplicas(1))
	assert.NoError(t, err, "failed to create consumer")
	assert.NotNil(t, sub, "sub result was nil")
	_msgid := fmt.Sprintf("%v", time.Now().Unix())
	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	assert.NoError(t, enc.Encode(map[string]any{"hi": "there"}))
	msg := nats.NewMsg(queue + ".test")
	msg.Data = buf.Bytes()
	SetContentEncodingHeader(msg, "msgpack")
	_, err = js.PublishMsg(msg, nats.MsgId(_msgid))
	assert.NoError(t, err, "failed to publish")
	time.Sleep(time.Second)
	assert.Equal(t, `{"hi":"there"}`, received, "message didnt match")
	assert.Equal(t, _msgid, msgid, "msgid didnt match")
	ci, err := js.ConsumerInfo(queue, "test2")
	assert.NotNil(t, ci)
	assert.NoError(t, err)
	assert.Equal(t, "exactly once consumer for "+queue, ci.Config.Description)
	sub.Close()
	n.Close()
	server.Shutdown()
}

func TestQueueConsumer(t *testing.T) {
	server := RunTestServer(true)
	defer server.Shutdown()
	log := logger.NewTestLogger()
	n, err := NewNats(log, "test", server.ClientURL(), nil)
	assert.NoError(t, err, "failed to connect to nats")
	assert.NotNil(t, n, "result was nil")
	queue := fmt.Sprintf("qc%v", time.Now().Unix())
	js, err := n.JetStream()
	assert.NoError(t, err, "failed to create jetstream")
	assert.NotNil(t, js, "js result was nil")
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     queue,
		Subjects: []string{queue + ".>"},
	})
	log.Debug("error: %v", err)
	assert.NoError(t, err, "failed to create stream")
	var received1 string
	var msgid1 string
	handler1 := func(ctx context.Context, buf []byte, msg *nats.Msg) error {
		_msgid := GetMsgIdFromHeader(msg)
		t.Log("1 received:", string(buf), "msgid:", _msgid)
		received1 = string(buf)
		msgid1 = _msgid
		msg.AckSync()
		return nil
	}
	var received2 string
	var msgid2 string
	handler2 := func(ctx context.Context, buf []byte, msg *nats.Msg) error {
		_msgid := GetMsgIdFromHeader(msg)
		t.Log("2 received:", string(buf), "msgid:", _msgid)
		received2 = string(buf)
		msgid2 = _msgid
		msg.AckSync()
		return nil
	}
	sub1, err := NewQueueConsumer(log, js, queue, "qtest1", queue+".*", handler1, WithQueueReplicas(1))
	assert.NoError(t, err, "failed to create consumer 1")
	assert.NotNil(t, sub1, "sub1 result was nil")
	sub2, err := NewQueueConsumer(log, js, queue, "qtest2", queue+".*", handler2, WithQueueReplicas(1))
	assert.NoError(t, err, "failed to create consumer 2")
	assert.NotNil(t, sub1, "sub2 result was nil")
	_msgid := fmt.Sprintf("%v", time.Now().Unix())
	_, err = js.Publish(queue+".test", []byte("hi"), nats.MsgId(_msgid))
	assert.NoError(t, err, "failed to publish")
	time.Sleep(time.Millisecond * 100)
	assert.Equal(t, "hi", received1, "message didnt match")
	assert.Equal(t, _msgid, msgid1, "msgid didnt match")
	assert.Equal(t, "hi", received2, "message didnt match")
	assert.Equal(t, _msgid, msgid2, "msgid didnt match")
	ci, err := js.ConsumerInfo(queue, "qtest1")
	assert.NotNil(t, ci)
	assert.NoError(t, err)
	assert.Equal(t, "queue consumer for "+queue, ci.Config.Description)
	sub1.Close()
	sub2.Close()
	n.Close()
	server.Shutdown()
}

func TestQueueConsumerLoadBalanced(t *testing.T) {
	server := RunTestServer(true)
	defer server.Shutdown()
	log := logger.NewTestLogger()
	queue := fmt.Sprintf("queuel%v", time.Now().Unix())
	subject := queue + ".>"
	message := queue + ".test"
	n, err := NewNats(log, "test", server.ClientURL(), nil)
	assert.NoError(t, err, "failed to connect to nats")
	assert.NotNil(t, n, "result was nil")
	js, err := n.JetStream()
	assert.NoError(t, err, "failed to create jetstream")
	assert.NotNil(t, js, "js result was nil")
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     queue,
		Subjects: []string{subject},
	})
	assert.NoError(t, err, "failed to create stream")
	var received1 string
	var msgid1 string
	handler1 := func(ctx context.Context, buf []byte, msg *nats.Msg) error {
		_msgid := GetMsgIdFromHeader(msg)
		t.Log("1 received:", string(buf), "msgid:", _msgid)
		received1 = string(buf)
		msgid1 = _msgid
		msg.AckSync()
		return nil
	}
	var received2 string
	var msgid2 string
	handler2 := func(ctx context.Context, buf []byte, msg *nats.Msg) error {
		_msgid := GetMsgIdFromHeader(msg)
		t.Log("2 received:", string(buf), "msgid:", _msgid)
		received2 = string(buf)
		msgid2 = _msgid
		msg.AckSync()
		return nil
	}
	sub1, err := NewQueueConsumer(log, js, queue, "qtest1", subject, handler1, WithQueueReplicas(1))
	assert.NoError(t, err, "failed to create consumer 1")
	assert.NotNil(t, sub1, "sub1 result was nil")
	sub2, err := NewQueueConsumer(log, js, queue, "qtest1", subject, handler2, WithQueueReplicas(1))
	assert.NoError(t, err, "failed to create consumer 2")
	assert.NotNil(t, sub1, "sub2 result was nil")
	_msgid1 := fmt.Sprintf("a-%v", time.Now().Unix())
	_msgid2 := fmt.Sprintf("b-%v", time.Now().Unix())
	_, err = js.Publish(message, []byte(_msgid1), nats.MsgId(_msgid1))
	assert.NoError(t, err, "failed to publish")
	_, err = js.Publish(message, []byte(_msgid2), nats.MsgId(_msgid2))
	assert.NoError(t, err, "failed to publish")
	time.Sleep(time.Millisecond * 100)
	assert.Equal(t, _msgid1, received1, "message1 didnt match")
	assert.Equal(t, _msgid1, msgid1, "msgid1 didnt match")
	assert.Equal(t, _msgid2, received2, "message2 didnt match")
	assert.Equal(t, _msgid2, msgid2, "msgid2 didnt match")
	sub1.Close()
	sub2.Close()
	n.Close()
	server.Shutdown()
}

func TestEphemeralConsumer(t *testing.T) {
	server := RunTestServer(true)
	defer server.Shutdown()
	log := logger.NewTestLogger()
	queue := fmt.Sprintf("ephem%v", time.Now().Unix())
	subject := queue + ".>"
	message := queue + ".test"
	n, err := NewNats(log, "test", server.ClientURL(), nil)
	assert.NoError(t, err, "failed to connect to nats")
	assert.NotNil(t, n, "result was nil")
	js, err := n.JetStream()
	assert.NoError(t, err, "failed to create jetstream")
	assert.NotNil(t, js, "js result was nil")
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     queue,
		Subjects: []string{subject},
	})
	assert.NoError(t, err, "failed to create stream")
	var received1 string
	var msgid1 string
	var wg sync.WaitGroup
	handler1 := func(ctx context.Context, buf []byte, msg *nats.Msg) error {
		defer wg.Done()
		_msgid := GetMsgIdFromHeader(msg)
		t.Log("1 received:", string(buf), "msgid:", _msgid)
		received1 = string(buf)
		msgid1 = _msgid
		return msg.AckSync()
	}
	wg.Add(1)
	sub1, err := NewEphemeralConsumer(log, js, queue, subject, handler1)
	assert.NoError(t, err, "failed to create consumer 1")
	assert.NotNil(t, sub1, "sub1 result was nil")
	_msgid1 := fmt.Sprintf("a-%v", time.Now().Unix())
	_, err = js.Publish(message, []byte(_msgid1), nats.MsgId(_msgid1))
	assert.NoError(t, err, "failed to publish")
	wg.Wait()
	assert.Equal(t, _msgid1, received1, "message1 didnt match")
	assert.Equal(t, _msgid1, msgid1, "msgid1 didnt match")
	sub1.Close()
	received1 = ""
	msgid1 = ""
	wg.Add(1)
	sub2, err := NewEphemeralConsumer(log, js, queue, subject, handler1, WithEphemeralDelivery(nats.DeliverAllPolicy))
	assert.NoError(t, err, "failed to create consumer 2")
	assert.NotNil(t, sub2, "sub2 result was nil")
	wg.Wait()
	assert.Equal(t, _msgid1, received1, "message1 didnt match")
	assert.Equal(t, _msgid1, msgid1, "msgid1 didnt match")
	sub2.Close()
	received1 = ""
	msgid1 = ""
	wg.Add(1)
	sub3, err := NewEphemeralConsumer(log, js, queue, subject, handler1, WithEphemeralDelivery(nats.DeliverAllPolicy))
	assert.NoError(t, err, "failed to create consumer 3")
	assert.NotNil(t, sub3, "sub3 result was nil")
	wg.Wait()
	assert.Equal(t, _msgid1, received1, "message1 didnt match")
	assert.Equal(t, _msgid1, msgid1, "msgid1 didnt match")
	ci := <-js.Consumers(queue)
	assert.NotNil(t, ci)
	assert.Equal(t, "ephemeral consumer for "+queue, ci.Config.Description)
	sub3.Close()
	n.Close()
	server.Shutdown()
}

func TestEphemeralConsumerAutoExtend(t *testing.T) {
	server := RunTestServer(true)
	defer server.Shutdown()
	log := logger.NewConsoleLogger()
	queue := fmt.Sprintf("aephem%v", time.Now().Unix())
	subject := queue + ".>"
	message := queue + ".test"
	n, err := NewNats(log, "test", server.ClientURL(), nil)
	assert.NoError(t, err, "failed to connect to nats")
	assert.NotNil(t, n, "result was nil")
	js, err := n.JetStream()
	assert.NoError(t, err, "failed to create jetstream")
	assert.NotNil(t, js, "js result was nil")
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     queue,
		Subjects: []string{subject},
	})
	assert.NoError(t, err, "failed to create stream")
	var received string
	var msgid string
	handler := func(ctx context.Context, buf []byte, msg *nats.Msg) error {
		_msgid := GetMsgIdFromHeader(msg)
		log.Info("received: %s, msgid: %s", string(buf), _msgid)
		time.Sleep(time.Second * 5) // block to force the extender to run
		received = string(buf)
		msgid = _msgid
		msg.AckSync()
		return nil
	}
	sub1, err := NewEphemeralConsumer(log, js, queue, subject, handler, WithEphemeralAckWait(time.Second*2))
	assert.NoError(t, err, "failed to create consumer 1")
	assert.NotNil(t, sub1, "sub1 result was nil")
	_msgid1 := fmt.Sprintf("a-%v", time.Now().Unix())
	_, err = js.Publish(message, []byte(_msgid1), nats.MsgId(_msgid1))
	assert.NoError(t, err, "failed to publish")
	time.Sleep(time.Second * 6)
	assert.Equal(t, _msgid1, received, "message1 didnt match")
	assert.Equal(t, _msgid1, msgid, "msgid1 didnt match")
	sub1.Close()
	n.Close()
	server.Shutdown()
}

func TestExactlyOnceConsumerConfigChanged(t *testing.T) {
	server := RunTestServer(true)
	defer server.Shutdown()
	log := logger.NewTestLogger()
	n, err := NewNats(log, "test", server.ClientURL(), nil)
	assert.NoError(t, err, "failed to connect to nats")
	assert.NotNil(t, n, "result was nil")
	js, err := n.JetStream()
	assert.NoError(t, err, "failed to create jetstream")
	assert.NotNil(t, js, "js result was nil")
	queue := fmt.Sprintf("stream%v", time.Now().Unix())
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     queue,
		Subjects: []string{queue + ".>"},
	})
	assert.NoError(t, err, "failed to create stream")
	handler := func(ctx context.Context, buf []byte, msg *nats.Msg) error {
		return nil
	}
	ci, err := js.AddConsumer(queue, &nats.ConsumerConfig{
		Durable:       "test",
		Name:          "test",
		Description:   "",
		FilterSubject: queue + ".*",
		AckPolicy:     nats.AckExplicitPolicy,
		MaxAckPending: 1,
		MaxDeliver:    1,
		DeliverPolicy: nats.DeliverNewPolicy,
		Replicas:      1,
	})
	assert.NoError(t, err)
	assert.NotNil(t, ci)
	sub, err := NewExactlyOnceConsumer(log, js, queue, "test", queue+".*", handler, WithExactlyOnceReplicas(1))
	assert.NoError(t, err)
	assert.NotNil(t, sub)
	sub.Close()
}

func TestQueueConsumerConfigChanged(t *testing.T) {
	server := RunTestServer(true)
	defer server.Shutdown()
	log := logger.NewTestLogger()
	n, err := NewNats(log, "test", server.ClientURL(), nil)
	assert.NoError(t, err, "failed to connect to nats")
	assert.NotNil(t, n, "result was nil")
	js, err := n.JetStream()
	assert.NoError(t, err, "failed to create jetstream")
	assert.NotNil(t, js, "js result was nil")
	queue := fmt.Sprintf("qstream%v", time.Now().Unix())
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     queue,
		Subjects: []string{queue + ".>"},
	})
	assert.NoError(t, err, "failed to create stream")
	handler := func(ctx context.Context, buf []byte, msg *nats.Msg) error {
		return nil
	}
	ci, err := js.AddConsumer(queue, &nats.ConsumerConfig{
		Durable:       "test",
		Name:          "test",
		Description:   "",
		FilterSubject: queue + ".*",
		AckPolicy:     nats.AckExplicitPolicy,
		DeliverPolicy: nats.DeliverNewPolicy,
		MaxDeliver:    1,
		MaxAckPending: 1000,
		Replicas:      1,
	})
	assert.NoError(t, err)
	assert.NotNil(t, ci)
	sub, err := NewQueueConsumer(log, js, queue, "test", queue+".*", handler, WithQueueReplicas(1))
	assert.NoError(t, err)
	assert.NotNil(t, sub)
	sub.Close()
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
