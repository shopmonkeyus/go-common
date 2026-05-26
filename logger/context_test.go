package logger

import (
	"context"
	"testing"
)

func TestToContextFromContext(t *testing.T) {
	ctx := context.Background()
	l := NewZapLogger(WithLevel(LevelInfo))

	ctx = ToContext(ctx, l)
	got := FromContext(ctx)
	if got != l {
		t.Fatal("expected same logger instance")
	}
}

func TestFromContextEmpty(t *testing.T) {
	ctx := context.Background()
	got := FromContext(ctx)
	if got == nil {
		t.Fatal("expected fallback logger, not nil")
	}
}
