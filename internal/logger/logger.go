package logger

import (
	"context"
	"log/slog"
	"sync/atomic"
)

// defaultLogger is set by Init. When nil, Default falls back to slog.Default().
var defaultLogger atomic.Pointer[slog.Logger]

// contextKey is the key for logger values in Contexts.
type contextKey int

const (
	loggerKey contextKey = iota
	requestIDKey
)

// Init makes this package log through handler. A nil handler restores the
// fallback to slog.Default(). Init does not change slog's or log's defaults.
func Init(handler slog.Handler) {
	if handler == nil {
		defaultLogger.Store(nil)
		return
	}
	defaultLogger.Store(slog.New(handler))
}

// Default returns the logger set by Init, or slog.Default() if Init was not
// called. slog.Default() writes through the standard log package, so these
// lines share the output and timestamp format of the existing log.Printf calls.
func Default() *slog.Logger {
	if l := defaultLogger.Load(); l != nil {
		return l
	}
	return slog.Default()
}

// FromContext returns the logger stored in the context, or the default logger if none.
func FromContext(ctx context.Context) *slog.Logger {
	if ctx == nil {
		return Default()
	}
	if logger, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return logger
	}
	return Default()
}

// WithContext returns a new context with the logger attached.
func WithContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// WithRequestID returns a new context with a request ID and a logger that includes it.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	logger := FromContext(ctx).With("request_id", requestID)
	ctx = context.WithValue(ctx, requestIDKey, requestID)
	return WithContext(ctx, logger)
}

// RequestIDFromContext returns the request ID from the context, if any.
func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if requestID, ok := ctx.Value(requestIDKey).(string); ok {
		return requestID
	}
	return ""
}

// Info logs at Info level using the default logger.
func Info(msg string, args ...any) {
	Default().Info(msg, args...)
}

// Error logs at Error level using the default logger.
func Error(msg string, args ...any) {
	Default().Error(msg, args...)
}

// Warn logs at Warn level using the default logger.
func Warn(msg string, args ...any) {
	Default().Warn(msg, args...)
}

// Debug logs at Debug level using the default logger.
func Debug(msg string, args ...any) {
	Default().Debug(msg, args...)
}

// InfoContext logs at Info level with context.
func InfoContext(ctx context.Context, msg string, args ...any) {
	FromContext(ctx).InfoContext(ctx, msg, args...)
}

// ErrorContext logs at Error level with context.
func ErrorContext(ctx context.Context, msg string, args ...any) {
	FromContext(ctx).ErrorContext(ctx, msg, args...)
}

// WarnContext logs at Warn level with context.
func WarnContext(ctx context.Context, msg string, args ...any) {
	FromContext(ctx).WarnContext(ctx, msg, args...)
}

// DebugContext logs at Debug level with context.
func DebugContext(ctx context.Context, msg string, args ...any) {
	FromContext(ctx).DebugContext(ctx, msg, args...)
}

// With returns a new logger with the given attributes.
func With(args ...any) *slog.Logger {
	return Default().With(args...)
}
