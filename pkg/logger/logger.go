package logger

import (
	"context"
	"log/slog"
	"os"
)

type Logger = slog.Logger

type Config struct {
	Level  string
	Format string
}

var (
	defaultLogger *slog.Logger
)

func init() {
	base := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	defaultLogger = slog.New(NewContextHandler(base))
	slog.SetDefault(defaultLogger)
}

func Setup(cfg Config) {
	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level: parseLevel(cfg.Level),
	}

	if cfg.Format == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	// Wrap with ContextHandler so every *Context call automatically
	// inherits trace_id / request_id / tenant_id / user_id. The wrapper
	// is no-op for callers that pass context.Background() or nil.
	defaultLogger = slog.New(NewContextHandler(handler))
	slog.SetDefault(defaultLogger)
}

func Get() *Logger {
	return defaultLogger
}

func Info(msg string, args ...any) {
	defaultLogger.Info(msg, args...)
}

func Error(msg string, args ...any) {
	defaultLogger.Error(msg, args...)
}

func Debug(msg string, args ...any) {
	defaultLogger.Debug(msg, args...)
}

func Warn(msg string, args ...any) {
	defaultLogger.Warn(msg, args...)
}

// InfoContext / ErrorContext / WarnContext / DebugContext route the call
// through slog.Record's context-aware path so ContextHandler can inject
// trace_id, request_id, tenant_id and user_id automatically. Prefer
// these in handlers / services where a ctx is available.
func InfoContext(ctx context.Context, msg string, args ...any) {
	defaultLogger.InfoContext(ctx, msg, args...)
}

func ErrorContext(ctx context.Context, msg string, args ...any) {
	defaultLogger.ErrorContext(ctx, msg, args...)
}

func WarnContext(ctx context.Context, msg string, args ...any) {
	defaultLogger.WarnContext(ctx, msg, args...)
}

func DebugContext(ctx context.Context, msg string, args ...any) {
	defaultLogger.DebugContext(ctx, msg, args...)
}

func With(args ...any) *Logger {
	return defaultLogger.With(args...)
}

// FromContext returns the package logger. The handler already pulls
// per-request attributes from ctx, so the same logger value is enough —
// callers must use *Context methods (InfoContext, etc.) to keep that
// correlation, otherwise the ctx is dropped and the attributes don't
// appear.
func FromContext(ctx context.Context) *Logger {
	return defaultLogger
}

func parseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
