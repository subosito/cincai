package observability

import (
	"os"
	"strings"
)

// Config holds OTLP export settings (standard OTEL_* env vars).
type Config struct {
	Enabled      bool
	ServiceName  string
	MetricPrefix string
	Endpoint     string
	Headers      map[string]string
}

// LoadConfig reads OTEL env. Export is off when endpoint is empty or OTEL_SDK_DISABLED=true.
func LoadConfig(serviceName string) Config {
	return LoadConfigWithPrefix(serviceName, DefaultMetricPrefix)
}

// LoadConfigWithPrefix is like LoadConfig but sets a custom metric name prefix.
func LoadConfigWithPrefix(serviceName, metricPrefix string) Config {
	prefix := strings.TrimSpace(metricPrefix)
	if prefix == "" {
		prefix = DefaultMetricPrefix
	}
	if truthy(os.Getenv("OTEL_SDK_DISABLED")) {
		return Config{ServiceName: serviceName, MetricPrefix: prefix}
	}
	endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if endpoint == "" {
		return Config{ServiceName: strings.TrimSpace(serviceName), MetricPrefix: prefix}
	}
	if name := strings.TrimSpace(os.Getenv("OTEL_SERVICE_NAME")); name != "" {
		serviceName = name
	}
	return Config{
		Enabled:      true,
		ServiceName:  strings.TrimSpace(serviceName),
		MetricPrefix: prefix,
		Endpoint:     endpoint,
		Headers:      otlpHeaders(),
	}
}

func otlpHeaders() map[string]string {
	out := parseHeaderKV(os.Getenv("OTEL_EXPORTER_OTLP_HEADERS"))
	if token := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_AUTH_TOKEN")); token != "" {
		if _, ok := out["Authorization"]; !ok {
			out["Authorization"] = "Bearer " + token
		}
	}
	return out
}

func parseHeaderKV(raw string) map[string]string {
	out := make(map[string]string)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out
	}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k != "" {
			out[k] = v
		}
	}
	return out
}

func truthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
