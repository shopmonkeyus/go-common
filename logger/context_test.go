package logger

import (
	"context"
	"testing"
)

func TestToContextFromContext(t *testing.T) {
	ctx := context.Background()
	l := NewZapLogger(WithLevel(LevelInfo))

	ctx = ToContext(ctx, l)
	got, ok := FromContext(ctx)
	if !ok {
		t.Fatal("expected logger in context")
	}
	if got != l {
		t.Fatal("expected same logger instance")
	}
}

func TestFromContextEmpty(t *testing.T) {
	ctx := context.Background()
	got, ok := FromContext(ctx)
	if ok {
		t.Fatal("expected no logger in empty context")
	}
	if got != nil {
		t.Fatal("expected nil logger from empty context")
	}
}

func TestFromContextOrDefault(t *testing.T) {
	fallback := NewZapLogger(WithLevel(LevelWarn))

	t.Run("returns fallback when empty", func(t *testing.T) {
		ctx := context.Background()
		got := FromContextOrDefault(ctx, fallback)
		if got != fallback {
			t.Fatal("expected fallback logger")
		}
	})

	t.Run("returns stored when present", func(t *testing.T) {
		stored := NewZapLogger(WithLevel(LevelInfo))
		ctx := ToContext(context.Background(), stored)
		got := FromContextOrDefault(ctx, fallback)
		if got != stored {
			t.Fatal("expected stored logger, not fallback")
		}
	})
}
