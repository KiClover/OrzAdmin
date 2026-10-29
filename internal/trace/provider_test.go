package trace

import (
	"context"
	"testing"

	"github.com/KiClover/orzAdmin/conf"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestNewTracerProviderExportsSpan(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	provider, err := newTracerProvider(conf.TraceConf{SampleRatio: 1}, "unit-test", exp)
	if err != nil {
		t.Fatalf("newTracerProvider: %v", err)
	}
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	_, span := provider.Tracer("test").Start(context.Background(), "op")
	span.End()
	if err := provider.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush: %v", err)
	}
	if spans := exp.GetSpans(); len(spans) != 1 || spans[0].Name != "op" {
		t.Fatalf("exported spans = %+v, want 1 named op", spans)
	}
}

func TestNewTracerProviderSampleZeroDropsSpan(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	provider, err := newTracerProvider(conf.TraceConf{SampleRatio: 0}, "unit-test", exp)
	if err != nil {
		t.Fatalf("newTracerProvider: %v", err)
	}
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	_, span := provider.Tracer("test").Start(context.Background(), "op")
	span.End()
	_ = provider.ForceFlush(context.Background())
	if got := len(exp.GetSpans()); got != 0 {
		t.Fatalf("exported spans = %d, want 0 (sample_ratio=0)", got)
	}
}

func TestNewResourceCarriesServiceName(t *testing.T) {
	res, err := newResource("svc-a", conf.TraceConf{Environment: "dev", Version: "v1.2.3"})
	if err != nil {
		t.Fatalf("newResource: %v", err)
	}
	got := map[string]string{}
	for _, kv := range res.Attributes() {
		got[string(kv.Key)] = kv.Value.AsString()
	}
	if got["service.name"] != "svc-a" {
		t.Fatalf("service.name = %q, want svc-a", got["service.name"])
	}
	if got["deployment.environment.name"] != "dev" {
		t.Fatalf("deployment.environment.name = %q, want dev", got["deployment.environment.name"])
	}
	if got["service.version"] != "v1.2.3" {
		t.Fatalf("service.version = %q, want v1.2.3", got["service.version"])
	}
}

func TestNewOTLPExporterRejectsBadEndpoint(t *testing.T) {
	if _, err := newOTLPExporter(context.Background(), conf.TraceConf{Endpoint: "http://[::1", Insecure: true}); err == nil {
		t.Fatal("expected error for malformed endpoint")
	}
}
