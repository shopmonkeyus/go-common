package logger

import (
	"context"
	"sync"
)

type contextKey struct{}

var (
	defaultLogger     Logger
	defaultLoggerOnce sync.Once
)

func getDefaultLogger() Logger {
	defaultLoggerOnce.Do(func() {
		defaultLogger = NewZapLogger()
	})
	return defaultLogger
}

func ToContext(ctx context.Context, l Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, l)
}

func FromContext(ctx context.Context) Logger {
	l, ok := ctx.Value(contextKey{}).(Logger)
	if !ok {
		l = getDefaultLogger()
	}
	return l.WithContext(ctx)
}
