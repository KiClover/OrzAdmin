package bootstrap_test

import (
	"testing"
	"time"

	"github.com/KiClover/orzAdmin/conf"
	internaltrace "github.com/KiClover/orzAdmin/internal/trace"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

func TestTraceModuleInstallsProvider(t *testing.T) {
	t.Cleanup(func() { otel.SetTracerProvider(tracenoop.NewTracerProvider()) })

	cfg := &conf.Config{}
	cfg.Trace.Enabled = true
	cfg.Trace.Endpoint = "127.0.0.1:4317"
	cfg.Trace.SampleRatio = 1
	cfg.Trace.ExportTimeout = time.Second
	cfg.Trace.ShutdownTimeout = 500 * time.Millisecond

	app := fxtest.New(t,
		fx.Supply(cfg, internaltrace.ServiceName("bootstrap-test")),
		internaltrace.Module(),
	)
	app.RequireStart()
	if _, ok := otel.GetTracerProvider().(*sdktrace.TracerProvider); !ok {
		t.Fatalf("global provider = %T, want *sdktrace.TracerProvider", otel.GetTracerProvider())
	}
	app.RequireStop()
}
