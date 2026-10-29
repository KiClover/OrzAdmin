package trace

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/KiClover/orzAdmin/conf"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const (
	defaultEndpoint      = "localhost:4317"
	defaultExportTimeout = 10 * time.Second
)

// newOTLPExporter 构造 OTLP/gRPC 导出器；endpoint 支持 host:port 与 http(s):// URL 两种写法。
func newOTLPExporter(ctx context.Context, c conf.TraceConf) (sdktrace.SpanExporter, error) {
	endpoint := strings.TrimSpace(c.Endpoint)
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	timeout := c.ExportTimeout
	if timeout <= 0 {
		timeout = defaultExportTimeout
	}
	opts := []otlptracegrpc.Option{otlptracegrpc.WithTimeout(timeout)}

	if strings.Contains(endpoint, "://") {
		u, err := url.Parse(endpoint)
		if err != nil {
			return nil, fmt.Errorf("trace.endpoint %q 无法解析: %w", endpoint, err)
		}
		if u.Scheme == "http" {
			opts = append(opts, otlptracegrpc.WithInsecure())
		}
		opts = append(opts, otlptracegrpc.WithEndpointURL(endpoint))
	} else {
		if c.Insecure {
			opts = append(opts, otlptracegrpc.WithInsecure())
		}
		opts = append(opts, otlptracegrpc.WithEndpoint(endpoint))
	}
	if len(c.Headers) > 0 {
		opts = append(opts, otlptracegrpc.WithHeaders(c.Headers))
	}

	exp, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("创建 OTLP 导出器失败 (endpoint=%s): %w", endpoint, err)
	}
	return exp, nil
}

// newResource 构造资源属性：service.name 必填，environment/version 可选。
func newResource(serviceName string, c conf.TraceConf) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{attribute.String("service.name", serviceName)}
	if v := strings.TrimSpace(c.Environment); v != "" {
		attrs = append(attrs, attribute.String("deployment.environment.name", v))
	}
	if v := strings.TrimSpace(c.Version); v != "" {
		attrs = append(attrs, attribute.String("service.version", v))
	}
	return resource.Merge(resource.Default(), resource.NewSchemaless(attrs...))
}

// newTracerProvider 用给定导出器构建 provider；测试注入 tracetest.InMemoryExporter。
func newTracerProvider(c conf.TraceConf, serviceName string, exp sdktrace.SpanExporter) (*sdktrace.TracerProvider, error) {
	res, err := newResource(serviceName, c)
	if err != nil {
		return nil, fmt.Errorf("构建 trace 资源失败: %w", err)
	}
	return sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(c.SampleRatio))),
		sdktrace.WithBatcher(exp),
	), nil
}
