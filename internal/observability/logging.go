// Package observability provides structured logging and minimal in-process
// metrics. Logs are JSON, one event per line, and ALWAYS go through the
// security redaction layer.
package observability

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"

	"github.com/somagen/scenario4/internal/security"
)

type ctxKey int

const (
	ctxKeyRequestID ctxKey = iota
	ctxKeyTenantID
)

// WithRequestID attaches a synthetic request id to the context.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyRequestID, id)
}

// RequestIDFrom extracts the request id, or "" if absent.
func RequestIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyRequestID).(string); ok {
		return v
	}
	return ""
}

// WithTenantID attaches a synthetic tenant id to the context.
func WithTenantID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyTenantID, id)
}

// TenantIDFrom extracts the tenant id, or "" if absent.
func TenantIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyTenantID).(string); ok {
		return v
	}
	return ""
}

// Logger writes JSON log lines to an io.Writer. A nil Logger writes to
// os.Stdout. All field values are passed through security.RedactValue so
// that the synthetic log never contains commercial content in clear text.
type Logger struct {
	mu  sync.Mutex
	w   io.Writer
	svc string
}

// New returns a logger tagged with the given service name.
func New(w io.Writer, svc string) *Logger {
	if w == nil {
		w = os.Stdout
	}
	return &Logger{w: w, svc: svc}
}

// Default returns a logger writing to stdout.
func Default(svc string) *Logger { return New(os.Stdout, svc) }

// Info logs an informational event.
func (l *Logger) Info(ctx context.Context, msg string, fields map[string]any) {
	l.write(ctx, "info", msg, fields)
}

// Warn logs a warning event.
func (l *Logger) Warn(ctx context.Context, msg string, fields map[string]any) {
	l.write(ctx, "warn", msg, fields)
}

// Error logs an error event.
func (l *Logger) Error(ctx context.Context, msg string, fields map[string]any) {
	l.write(ctx, "error", msg, fields)
}

func (l *Logger) write(ctx context.Context, level, msg string, fields map[string]any) {
	out := map[string]any{
		"ts":         time.Now().UTC().Format(time.RFC3339Nano),
		"level":      level,
		"service":    l.svc,
		"msg":        msg,
		"request_id": RequestIDFrom(ctx),
		"tenant_id":  TenantIDFrom(ctx),
	}
	for k, v := range fields {
		out[k] = v
	}
	// Force tenant_id / request_id through the redaction map in case a
	// caller also put them in `fields`.
	redacted := security.RedactMap(out)
	if rid, ok := redacted["request_id"].(string); ok {
		out["request_id"] = rid
	}
	if tid, ok := redacted["tenant_id"].(string); ok {
		out["tenant_id"] = tid
	}
	// Apply redaction to all caller-provided fields.
	for k := range fields {
		if v, ok := out[k].(string); ok {
			out[k] = security.RedactValue(k, v)
		}
	}
	enc, err := json.Marshal(out)
	if err != nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.w.Write(append(enc, '\n'))
}
