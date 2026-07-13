package observability

import (
	"context"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// Recorder holds per-request fields populated by handlers before the outer log line.
type Recorder struct {
	Wire, Model, ProviderRef, Protocol, PrincipalID string
	Usage                                           Usage
}

type recorderKey struct{}

// AttachRecorder returns a context carrying an ingress Recorder.
func AttachRecorder(ctx context.Context) (context.Context, *Recorder) {
	rec := &Recorder{}
	return context.WithValue(ctx, recorderKey{}, rec), rec
}

// RecorderFrom returns the ingress Recorder attached to ctx, if any.
func RecorderFrom(ctx context.Context) *Recorder {
	rec, _ := ctx.Value(recorderKey{}).(*Recorder)
	return rec
}

type statusWriter struct {
	http.ResponseWriter
	code   int
	header bool
}

func (w *statusWriter) WriteHeader(code int) {
	w.code = code
	w.header = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if !w.header {
		w.code = http.StatusOK
		w.header = true
	}
	return w.ResponseWriter.Write(b)
}

// IngressLog wraps h and emits one ingress record per HTTP request.
func IngressLog(defaultWire string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ctx := ExtractHTTP(r.Context(), r.Header)
		ctx = WithCorrelationID(ctx, r.Header.Get(headerCorrelationID))

		route := defaultWire
		if route == "" {
			route = r.URL.Path
		}
		ctx, span := StartSpan(ctx, "ingress.forward",
			oteltrace.WithAttributes(attribute.String("cincai.route", route)),
		)
		defer span.End()

		ctx, rec := AttachRecorder(ctx)
		if defaultWire != "" {
			rec.Wire = defaultWire
		}
		sw := &statusWriter{ResponseWriter: w, code: http.StatusNotFound}
		h.ServeHTTP(sw, r.WithContext(ctx))
		if rec.Wire == "" {
			rec.Wire = defaultWire
		}
		if rec.Wire == "" {
			rec.Wire = r.URL.Path
		}
		RecordIngress(ctx, rec, sw.code, start)
	})
}