package trace

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/kiosk404/echoryn/pkg/logger"
	"github.com/kiosk404/echoryn/pkg/utils/json"
)

// OTLPHTTPExporter exports spans to an OTLP-compatible HTTP endpoint.
//
// This is a lightweight implementation that sends spans in OTLP JSON format
// to endpoints like Jaeger, Grafana Tempo, or any OTLP collector.
//
// For production use with full protobuf encoding and advanced batching,
// consider using the official go.opentelemetry.io/otel/exporters/otlp package.
type OTLPHTTPExporter struct {
	endpoint string
	headers  map[string]string
	client   *http.Client
}

// OTLPHTTPExporterConfig configures the OTLP HTTP exporter.
type OTLPHTTPExporterConfig struct {
	// Endpoint is the OTLP HTTP endpoint (e.g., "http://localhost:4318/v1/traces").
	Endpoint string

	// Headers are custom HTTP headers sent with each request.
	Headers map[string]string

	// Timeout is the HTTP request timeout. Default: 10s.
	Timeout time.Duration
}

// NewOTLPHTTPExporter creates an OTLP HTTP exporter.
func NewOTLPHTTPExporter(cfg OTLPHTTPExporterConfig) *OTLPHTTPExporter {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = "http://localhost:4318/v1/traces"
	}
	return &OTLPHTTPExporter{
		endpoint: endpoint,
		headers:  cfg.Headers,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// ExportSpans serializes spans to OTLP JSON and sends them via HTTP POST.
func (e *OTLPHTTPExporter) ExportSpans(ctx context.Context, spans []*Span) error {
	if len(spans) == 0 {
		return nil
	}

	payload := e.buildOTLPPayload(spans)

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("otlp: failed to marshal spans: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("otlp: failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range e.headers {
		req.Header.Set(k, v)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("otlp: failed to send spans: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("otlp: server returned status %d", resp.StatusCode)
	}

	logger.Debug("[Trace] exported %d spans to OTLP endpoint %s", len(spans), e.endpoint)
	return nil
}

// Shutdown is a no-op for the HTTP exporter (no persistent connection).
func (e *OTLPHTTPExporter) Shutdown(_ context.Context) error {
	return nil
}

// buildOTLPPayload converts internal spans to OTLP JSON format.
//
// The OTLP JSON format follows the spec:
// https://opentelemetry.io/docs/specs/otlp/#otlphttp-request
func (e *OTLPHTTPExporter) buildOTLPPayload(spans []*Span) map[string]interface{} {
	otlpSpans := make([]map[string]interface{}, 0, len(spans))
	for _, span := range spans {
		otlpSpan := map[string]interface{}{
			"traceId":           string(span.TraceID),
			"spanId":            string(span.SpanID),
			"name":              span.Name,
			"kind":              mapSpanKindToOTLP(span.Kind),
			"startTimeUnixNano": fmt.Sprintf("%d", span.StartTime.UnixNano()),
			"endTimeUnixNano":   fmt.Sprintf("%d", span.EndTime.UnixNano()),
			"status":            mapStatusToOTLP(span.Status, span.StatusMessage),
			"attributes":        mapAttributesToOTLP(span.Attributes),
			"events":            mapEventsToOTLP(span.Events),
		}
		if span.ParentSpanID != "" {
			otlpSpan["parentSpanId"] = string(span.ParentSpanID)
		}
		otlpSpans = append(otlpSpans, otlpSpan)
	}

	return map[string]interface{}{
		"resourceSpans": []map[string]interface{}{
			{
				"resource": map[string]interface{}{
					"attributes": []map[string]interface{}{},
				},
				"scopeSpans": []map[string]interface{}{
					{
						"scope": map[string]interface{}{
							"name":    "echoryn.trace",
							"version": "1.0.0",
						},
						"spans": otlpSpans,
					},
				},
			},
		},
	}
}

// mapSpanKindToOTLP maps our SpanKind to OTLP SpanKind integer.
func mapSpanKindToOTLP(kind SpanKind) int {
	switch kind {
	case SpanKindLLMCall:
		return 3 // SPAN_KIND_CLIENT
	case SpanKindToolCall:
		return 2 // SPAN_KIND_SERVER
	default:
		return 1 // SPAN_KIND_INTERNAL
	}
}

// mapStatusToOTLP maps our SpanStatus to OTLP Status object.
func mapStatusToOTLP(status SpanStatus, message string) map[string]interface{} {
	result := map[string]interface{}{}
	switch status {
	case SpanStatusOK:
		result["code"] = 1 // STATUS_CODE_OK
	case SpanStatusError:
		result["code"] = 2 // STATUS_CODE_ERROR
		if message != "" {
			result["message"] = message
		}
	default:
		result["code"] = 0 // STATUS_CODE_UNSET
	}
	return result
}

// mapAttributesToOTLP converts a map of attributes to OTLP KeyValue format.
func mapAttributesToOTLP(attrs map[string]interface{}) []map[string]interface{} {
	if len(attrs) == 0 {
		return nil
	}
	result := make([]map[string]interface{}, 0, len(attrs))
	for k, v := range attrs {
		kv := map[string]interface{}{
			"key": k,
		}
		switch val := v.(type) {
		case string:
			kv["value"] = map[string]interface{}{"stringValue": val}
		case int, int32, int64:
			kv["value"] = map[string]interface{}{"intValue": fmt.Sprintf("%d", val)}
		case float32, float64:
			kv["value"] = map[string]interface{}{"doubleValue": val}
		case bool:
			kv["value"] = map[string]interface{}{"boolValue": val}
		default:
			kv["value"] = map[string]interface{}{"stringValue": fmt.Sprintf("%v", val)}
		}
		result = append(result, kv)
	}
	return result
}

// mapEventsToOTLP converts span events to OTLP Event format.
func mapEventsToOTLP(events []*SpanEvent) []map[string]interface{} {
	if len(events) == 0 {
		return nil
	}
	result := make([]map[string]interface{}, 0, len(events))
	for _, event := range events {
		result = append(result, map[string]interface{}{
			"name":         event.Name,
			"timeUnixNano": fmt.Sprintf("%d", event.Timestamp.UnixNano()),
			"attributes":   mapAttributesToOTLP(event.Attributes),
		})
	}
	return result
}

// Compile-time interface check.
var _ Exporter = (*OTLPHTTPExporter)(nil)
