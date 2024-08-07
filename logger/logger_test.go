package logger

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type testSink struct {
	buf []byte
}

func (s *testSink) Write(buf []byte) (int, error) {
	s.buf = buf
	return len(buf), nil
}

func TestGCloudLogger(t *testing.T) {
	sink := &testSink{}
	log := NewGCloudLoggerWithSink(sink, LevelTrace)
	jlog := log.(*jsonLogger)
	tv := time.Date(2023, 10, 22, 12, 30, 0, 0, time.UTC)
	jlog.ts = &tv
	log.Trace("Hi")
	assert.Equal(t, `{"timestamp":"2023-10-22T12:30:00Z","message":"Hi","severity":"TRACE"}`, string(sink.buf))
	wlog := log.WithPrefix("[hi]")
	jlog = wlog.(*jsonLogger)
	jlog.ts = &tv
	wlog.Debug("hi")
	assert.Equal(t, `{"timestamp":"2023-10-22T12:30:00Z","message":"hi","severity":"DEBUG","component":"hi"}`, string(sink.buf))
	w2log := wlog.WithPrefix("[bye]")
	jlog = w2log.(*jsonLogger)
	jlog.ts = &tv
	w2log.Debug("hi")
	assert.Equal(t, `{"timestamp":"2023-10-22T12:30:00Z","message":"hi","severity":"DEBUG","component":"hi, bye"}`, string(sink.buf))
}

func TestCombinedLogger(t *testing.T) {
	sink := &testSink{}
	log := NewTestLogger()
	jsonLog := NewJSONLoggerWithSink(sink, LevelTrace)
	tv := time.Date(2023, 10, 22, 12, 30, 0, 0, time.UTC)
	jsonLog.(*jsonLogger).ts = &tv
	combined := NewMultiLogger(log, jsonLog)
	combined.Info("Ayyyyyy")
	assert.Len(t, log.Logs, 1)
	assert.Equal(t, `{"timestamp":"2023-10-22T12:30:00Z","message":"Ayyyyyy","severity":"INFO"}`, string(sink.buf))
}
