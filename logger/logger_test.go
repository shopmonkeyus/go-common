package logger

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConsoleLogger(t *testing.T) {
	log := NewConsoleLogger()
	log.Trace("This should not be unreadable")
}

type testSink struct {
	buf []byte
}

func (s *testSink) Write(buf []byte) error {
	s.buf = buf
	return nil
}

func TestGCloudLogger(t *testing.T) {
	sink := &testSink{}
	log := NewGCloudLoggerWithSink(sink)
	glog := log.(*gcloudLogger)
	tv := time.Date(2023, 10, 22, 12, 30, 0, 0, time.UTC)
	glog.ts = &tv
	log.Trace("Hi")
	assert.Equal(t, `{"timestamp":"2023-10-22T12:30:00Z","message":"Hi","severity":"TRACE"}`, string(sink.buf))
	wlog := log.WithPrefix("[hi]")
	glog = wlog.(*gcloudLogger)
	glog.ts = &tv
	wlog.Debug("hi")
	assert.Equal(t, `{"timestamp":"2023-10-22T12:30:00Z","message":"hi","severity":"DEBUG","component":"hi"}`, string(sink.buf))
}
