package logger

import (
	"context"
	"log/slog"

	pkgctx "github.com/oginoino/toca/pkg/context"
	"go.opentelemetry.io/otel/trace"
)

// ContextHandler wraps any slog.Handler so every log line emitted with a
// non-nil context automatically inherits the OTEL trace context plus the
// request-scoped identifiers (request_id, tenant_id, user_id) without each
// caller having to pass them as attributes.
//
// Why a separate handler instead of using `logger.With(...)` per call:
//   - call sites don't have to remember to attach those fields, so logs stay
//     correlatable even in code paths the author didn't think about
//   - OTEL trace_id/span_id are pulled from the live span, so logs and
//     traces line up automatically in Jaeger/Grafana
//   - the wrapper composes with any inner handler (text, JSON, custom) so
//     we can change format without touching this logic
type ContextHandler struct {
	inner slog.Handler
}

func NewContextHandler(inner slog.Handler) slog.Handler {
	if inner == nil {
		return nil
	}
	return &ContextHandler{inner: inner}
}

func (h *ContextHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.inner.Enabled(ctx, l)
}

func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if ctx == nil {
		return h.inner.Handle(ctx, r)
	}

	// OTEL trace context — only emit when the span is valid (not the no-op
	// sampler that returns zero IDs).
	if span := trace.SpanFromContext(ctx); span != nil {
		sc := span.SpanContext()
		if sc.IsValid() {
			r.AddAttrs(
				slog.String("trace_id", sc.TraceID().String()),
				slog.String("span_id", sc.SpanID().String()),
			)
		}
	}

	// Request-scoped attributes. Each is best-effort — absent fields are
	// silently skipped so this handler is safe to install before the auth
	// middleware has populated them.
	if reqID := pkgctx.GetRequestID(ctx); reqID != "" {
		r.AddAttrs(slog.String("request_id", reqID))
	}
	if tenantID, err := pkgctx.GetTenantID(ctx); err == nil && tenantID != "" {
		r.AddAttrs(slog.String("tenant_id", tenantID))
	}
	if accountID, err := pkgctx.GetAccountID(ctx); err == nil && accountID != "" {
		r.AddAttrs(slog.String("user_id", accountID))
	}

	return h.inner.Handle(ctx, r)
}

func (h *ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ContextHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *ContextHandler) WithGroup(name string) slog.Handler {
	return &ContextHandler{inner: h.inner.WithGroup(name)}
}
