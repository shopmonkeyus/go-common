package logger

import "testing"

func TestConsoleLogger(t *testing.T) {
	log := NewConsoleLogger()
	log.Trace("This should not be unreadable")
}
