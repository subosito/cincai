package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	logglobal "go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

var (
	mu       sync.Mutex
	global   *Provider
	ingressLog = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
)

// Provider holds cincai observability state.
type Provider struct {
	cfg            Config
	metricPrefix  string
	attached       bool // true when bound to embedder-owned global OTel (do not Shutdown exporters)
	tracerProvider *trace.TracerProvider
	meterProvider  *metric.MeterProvider
	loggerProvider *log.LoggerProvider
	Metrics        InstrumentSet
	tracer         oteltrace.Tracer
}

// Hook binds cincai spans and metrics to process-global OTel providers.
// The embedder owns OTLP export (Boot/Shutdown) — call Hook after host observability init.
// Example: hostobs.Boot("my-service"); cincai observability.Hook("cincai").
// Metric names use the default prefix "cincai" (e.g. cincai.ingress.requests).
func Hook(serviceName string) {
	HookWithPrefix(serviceName, DefaultMetricPrefix)
}

// HookWithPrefix is like Hook but uses a custom metric name prefix.
// Embedders can pass a custom prefix to get e.g. my-app.gateway.ingress.requests.
func HookWithPrefix(serviceName, metricPrefix string) {
	mu.Lock()
	defer mu.Unlock()
	if global != nil {
		return
	}
	name := strings.TrimSpace(serviceName)
	if name == "" {
		name = "cincai"
	}
	prefix := strings.TrimSpace(metricPrefix)
	if prefix == "" {
		prefix = DefaultMetricPrefix
	}
	mp := otel.GetMeterProvider()
	tp := otel.GetTracerProvider()
	global = &Provider{
		cfg:          Config{ServiceName: name, Enabled: true, MetricPrefix: prefix},
		metricPrefix: prefix,
		attached:     true,
		Metrics:      newMetrics(mp.Meter(name), prefix),
		tracer:       tp.Tracer(name),
	}
}

// Hooked reports whether Hook attached to embedder-owned global OTel.
func Hooked() bool {
	mu.Lock()
	defer mu.Unlock()
	return global != nil && global.attached
}

// Init configures global OTel export. No-op when OTEL_EXPORTER_OTLP_ENDPOINT is unset.
func Init(serviceName string) (*Provider, error) {
	mu.Lock()
	defer mu.Unlock()
	if global != nil {
		return global, nil
	}
	cfg := LoadConfig(serviceName)
	if !cfg.Enabled {
		global = &Provider{cfg: cfg, metricPrefix: cfg.MetricPrefix, Metrics: noopInst, tracer: otel.Tracer("cincai/noop")}
		return global, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	res, err := resource.Merge(
		resource.Default(),
		resource.NewSchemaless(semconv.ServiceName(cfg.ServiceName)),
	)
	if err != nil {
		return nil, fmt.Errorf("observability: resource: %w", err)
	}

	traceExp, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(normalizeOTLPEndpoint(cfg.Endpoint, "/v1/traces")),
		otlptracehttp.WithHeaders(cfg.Headers),
	)
	if err != nil {
		return nil, fmt.Errorf("observability: trace exporter: %w", err)
	}
	tp := trace.NewTracerProvider(
		trace.WithBatcher(traceExp),
		trace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	metricExp, err := otlpmetrichttp.New(ctx,
		otlpmetrichttp.WithEndpointURL(normalizeOTLPEndpoint(cfg.Endpoint, "/v1/metrics")),
		otlpmetrichttp.WithHeaders(cfg.Headers),
	)
	if err != nil {
		_ = tp.Shutdown(ctx)
		return nil, fmt.Errorf("observability: metric exporter: %w", err)
	}
	mp := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(metricExp, metric.WithInterval(30*time.Second))),
		metric.WithResource(res),
	)
	otel.SetMeterProvider(mp)
	metrics := newMetrics(mp.Meter(cfg.ServiceName), cfg.MetricPrefix)

	logExp, err := otlploghttp.New(ctx,
		otlploghttp.WithEndpointURL(normalizeOTLPEndpoint(cfg.Endpoint, "/v1/logs")),
		otlploghttp.WithHeaders(cfg.Headers),
	)
	if err != nil {
		_ = mp.Shutdown(ctx)
		_ = tp.Shutdown(ctx)
		return nil, fmt.Errorf("observability: log exporter: %w", err)
	}
	lp := log.NewLoggerProvider(
		log.WithResource(res),
		log.WithProcessor(log.NewBatchProcessor(logExp)),
	)
	logglobal.SetLoggerProvider(lp)

	jsonHandler := newTraceContextHandler(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	otelHandler := newTraceContextHandler(otelslog.NewHandler(cfg.ServiceName, otelslog.WithLoggerProvider(lp)))
	fanout := newFanoutHandler(jsonHandler, otelHandler)
	ingressLog = slog.New(fanout)
	slog.SetDefault(slog.New(fanout))

	global = &Provider{
		cfg:            cfg,
		metricPrefix:   cfg.MetricPrefix,
		attached:       false,
		tracerProvider: tp,
		meterProvider:  mp,
		loggerProvider: lp,
		Metrics:        metrics,
		tracer:         tp.Tracer(cfg.ServiceName),
	}
	LogInfo(ctx, "observability: OTLP export enabled",
		"service", cfg.ServiceName,
		"endpoint", cfg.Endpoint,
	)
	return global, nil
}

// Enabled reports whether spans and metrics are active (standalone export or Hook).
func Enabled() bool {
	mu.Lock()
	defer mu.Unlock()
	if global == nil {
		return false
	}
	return global.cfg.Enabled || global.attached
}

// Metrics returns global instruments (noop when disabled).
func Metrics() InstrumentSet {
	mu.Lock()
	defer mu.Unlock()
	if global == nil {
		return noopInst
	}
	return global.Metrics
}

// Tracer returns the service tracer (noop when disabled).
func Tracer() oteltrace.Tracer {
	mu.Lock()
	defer mu.Unlock()
	if global == nil || global.tracer == nil {
		return otel.Tracer("cincai/noop")
	}
	return global.tracer
}

// StartSpan starts a child span when tracing is enabled.
func StartSpan(ctx context.Context, name string, opts ...oteltrace.SpanStartOption) (context.Context, oteltrace.Span) {
	return Tracer().Start(ctx, name, opts...)
}

func metricPrefix() string {
	mu.Lock()
	defer mu.Unlock()
	if global == nil || global.metricPrefix == "" {
		return DefaultMetricPrefix
	}
	return global.metricPrefix
}

// Shutdown flushes exporters. Safe if export was disabled.
func Shutdown(ctx context.Context) error {
	mu.Lock()
	p := global
	global = nil
	ingressLog = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	mu.Unlock()
	if p == nil {
		return nil
	}
	if p.attached {
		return nil
	}
	var first error
	if p.loggerProvider != nil {
		if err := p.loggerProvider.Shutdown(ctx); err != nil && first == nil {
			first = err
		}
	}
	if p.meterProvider != nil {
		if err := p.meterProvider.Shutdown(ctx); err != nil && first == nil {
			first = err
		}
	}
	if p.tracerProvider != nil {
		if err := p.tracerProvider.Shutdown(ctx); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func normalizeOTLPEndpoint(endpoint, path string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return endpoint
	}
	endpoint = strings.TrimRight(endpoint, "/")
	if strings.HasSuffix(endpoint, path) {
		return endpoint
	}
	return endpoint + path
}

// RecordIngress emits one ingress log line and records metrics + span attributes.
func RecordIngress(ctx context.Context, rec *Recorder, status int, start time.Time) {
	if ctx == nil {
		ctx = context.Background()
	}
	if rec == nil {
		rec = &Recorder{}
	}
	wire := rec.Wire
	latency := time.Since(start)

	entry := RequestLog{
		Wire: wire, Model: rec.Model, ProviderRef: rec.ProviderRef, Protocol: rec.Protocol,
		Status: status, LatencyMs: latency.Milliseconds(), PrincipalID: rec.PrincipalID,
	}
	if !rec.Usage.Zero() {
		u := rec.Usage
		entry.Usage = &u
	}
	raw, _ := json.Marshal(entry)
	ingressLog.InfoContext(ctx, "ingress", "record", json.RawMessage(raw))

	Metrics().RecordIngress(ctx, wire, rec.Model, rec.ProviderRef, rec.Protocol, status, latency)

	if usageSink != nil {
		usageSink.RecordUsage(ctx, UsageEvent{
			Wire: wire, Model: rec.Model, ProviderRef: rec.ProviderRef, Protocol: rec.Protocol,
			PrincipalID: rec.PrincipalID, Status: status, LatencyMs: latency.Milliseconds(), Usage: rec.Usage,
		})
	}

	span := oteltrace.SpanFromContext(ctx)
	if !span.SpanContext().IsValid() {
		return
	}
	attrs := []attribute.KeyValue{
		attribute.String(metricPrefix()+".wire", wire),
		attribute.Int("http.status_code", status),
		attribute.Int64("latency_ms", latency.Milliseconds()),
	}
	if rec.Model != "" {
		attrs = append(attrs, attribute.String(metricPrefix()+".model", rec.Model))
	}
	if rec.ProviderRef != "" {
		attrs = append(attrs, attribute.String(metricPrefix()+".provider_ref", rec.ProviderRef))
	}
	if rec.Protocol != "" {
		attrs = append(attrs, attribute.String(metricPrefix()+".protocol", rec.Protocol))
	}
	if rec.PrincipalID != "" {
		attrs = append(attrs, attribute.String(metricPrefix()+".principal_id", rec.PrincipalID))
	}
	if corr := CorrelationIDFromContext(ctx); corr != "" {
		attrs = append(attrs, attribute.String(metricPrefix()+".correlation_id", corr))
	}
	span.SetAttributes(attrs...)
	if status >= 500 {
		span.SetStatus(codes.Error, fmt.Sprintf("http %d", status))
	}
}