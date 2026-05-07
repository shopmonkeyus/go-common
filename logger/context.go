package logger

import "context"

type contextKey struct{}

// ToContext stores a Logger in the context.
func ToContext(ctx context.Context, log Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, log)
}

// FromContext retrieves a Logger from the context.
// If the logger supports GCP trace correlation (zapLogger), it automatically
// enriches the returned logger with OTEL span/trace IDs from the context.
// Returns a default production Zap logger if none is stored in the context.
func FromContext(ctx context.Context) Logger {
	if log, ok := ctx.Value(contextKey{}).(Logger); ok {
		if zl, ok := log.(*zapLogger); ok && zl.gcpTraceCorrelation {
			return zl.WithContext(ctx)
		}
		return log
	}
	return NewZapLogger()
}
