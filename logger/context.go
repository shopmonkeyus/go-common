package logger

import "context"

type contextKey struct{}

func ToContext(ctx context.Context, l Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, l)
}

func FromContext(ctx context.Context) (Logger, bool) {
	l, ok := ctx.Value(contextKey{}).(Logger)
	return l, ok
}

func FromContextOrDefault(ctx context.Context, fallback Logger) Logger {
	if l, ok := FromContext(ctx); ok {
		return l
	}
	return fallback
}
