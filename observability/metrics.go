package observability

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// DefaultMetricPrefix is the metric name prefix for standalone cincai usage.
const DefaultMetricPrefix = "cincai"

// InstrumentSet holds OTel instruments (noop-safe).
type InstrumentSet struct {
	ingressTotal    metric.Int64Counter
	ingressDuration metric.Float64Histogram
	upstreamTotal   metric.Int64Counter
	upstreamDuration metric.Float64Histogram
}

func newMetrics(meter metric.Meter, prefix string) InstrumentSet {
	p := strings.TrimSpace(prefix)
	if p == "" {
		p = DefaultMetricPrefix
	}
	return InstrumentSet{
		ingressTotal:     mustCounter(meter, p+".ingress.requests", "Ingress HTTP requests"),
		ingressDuration:  mustHistogram(meter, p+".ingress.duration_ms", "Ingress request latency", "ms"),
		upstreamTotal:    mustCounter(meter, p+".upstream.requests", "Upstream HTTP requests"),
		upstreamDuration: mustHistogram(meter, p+".upstream.duration_ms", "Upstream HTTP latency", "ms"),
	}
}

func mustCounter(m metric.Meter, name, desc string) metric.Int64Counter {
	c, err := m.Int64Counter(name, metric.WithDescription(desc))
	if err != nil {
		panic(err)
	}
	return c
}

func mustHistogram(m metric.Meter, name, desc, unit string) metric.Float64Histogram {
	h, err := m.Float64Histogram(name, metric.WithDescription(desc), metric.WithUnit(unit))
	if err != nil {
		panic(err)
	}
	return h
}

func statusClass(status int) string {
	if status <= 0 {
		return "unknown"
	}
	return fmt.Sprintf("%dxx", status/100)
}

// RecordIngress records ingress metrics for one HTTP request.
func (m InstrumentSet) RecordIngress(ctx context.Context, wire, model, providerRef, protocol string, status int, d time.Duration) {
	attrs := []attribute.KeyValue{
		attribute.String("wire", wire),
		attribute.String("status_class", statusClass(status)),
		attribute.Int("status", status),
	}
	if providerRef != "" {
		attrs = append(attrs, attribute.String("provider_ref", providerRef))
	}
	if protocol != "" {
		attrs = append(attrs, attribute.String("protocol", protocol))
	}
	if model != "" {
		attrs = append(attrs, attribute.String("model", model))
	}
	m.ingressTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
	m.ingressDuration.Record(ctx, float64(d.Milliseconds()), metric.WithAttributes(attrs...))
}

// RecordUpstream records upstream HTTP metrics.
func (m InstrumentSet) RecordUpstream(ctx context.Context, host string, status int, d time.Duration) {
	attrs := []attribute.KeyValue{
		attribute.String("host", host),
		attribute.String("status_class", statusClass(status)),
		attribute.Int("status", status),
	}
	m.upstreamTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
	m.upstreamDuration.Record(ctx, float64(d.Milliseconds()), metric.WithAttributes(attrs...))
}

var noopInst = newMetrics(otel.Meter("cincai/noop"), DefaultMetricPrefix)