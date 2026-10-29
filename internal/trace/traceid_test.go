package trace

import (
	"context"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func TestResponseTraceIDFallsBackWhenNoSpan(t *testing.T) {
	c := app.NewContext(0)
	ResponseTraceID(context.Background(), c)

	if got := c.Response.Header.Get(TraceIDHeader); len(got) != 32 {
		t.Fatalf("X-Trace-Id = %q, want 32 hex chars", got)
	}
}

func TestResponseTraceIDUsesSpan(t *testing.T) {
	tid, err := oteltrace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil {
		t.Fatal(err)
	}
	sid, err := oteltrace.SpanIDFromHex("00f067aa0ba902b7")
	if err != nil {
		t.Fatal(err)
	}
	ctx := oteltrace.ContextWithSpanContext(context.Background(), oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
		TraceID:    tid,
		SpanID:     sid,
		TraceFlags: oteltrace.FlagsSampled,
	}))

	c := app.NewContext(0)
	ResponseTraceID(ctx, c)

	if got := c.Response.Header.Get(TraceIDHeader); got != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("X-Trace-Id = %q", got)
	}
}
