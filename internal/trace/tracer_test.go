package trace

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/KiClover/orzAdmin/conf"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

func TestNewTracerDisabledIsNoop(t *testing.T) {
	cfg := &conf.Config{}
	cfg.Trace.Enabled = false
	app := fxtest.New(t,
		fx.Supply(cfg, ServiceName("unit-test")),
		Module(),
	)
	app.RequireStart()
	if Enabled() {
		t.Fatal("Enabled() = true, want false（trace.enabled=false）")
	}
	app.RequireStop()
}

func TestNewTracerRejectsBadSampleRatio(t *testing.T) {
	cfg := &conf.Config{}
	cfg.Trace.Enabled = true
	cfg.Trace.Endpoint = "127.0.0.1:4317"
	cfg.Trace.SampleRatio = 1.5
	app := fx.New(fx.Supply(cfg, ServiceName("unit-test")), Module())
	err := app.Start(t.Context())
	if err == nil {
		_ = app.Stop(t.Context())
		t.Fatal("expected startup error for sample_ratio=1.5")
	}
	if !strings.Contains(err.Error(), "trace.sample_ratio=1.5 超出 [0,1]") {
		t.Fatalf("startup error = %v, want sample_ratio rejection", err)
	}
	if app.Err() == nil || !errors.Is(app.Err(), context.Canceled) && !strings.Contains(app.Err().Error(), "sample_ratio") {
		t.Fatalf("Err() = %v, want sample_ratio error", app.Err())
	}
}

func TestEnabledTogglesWithLifecycle(t *testing.T) {
	t.Cleanup(func() { SetEnabledForTest(false) })
	cfg := &conf.Config{}
	cfg.Trace.Enabled = true
	cfg.Trace.Endpoint = "127.0.0.1:4317"
	cfg.Trace.SampleRatio = 1
	cfg.Trace.ExportTimeout = time.Second
	cfg.Trace.ShutdownTimeout = 500 * time.Millisecond
	app := fxtest.New(t, fx.Supply(cfg, ServiceName("unit-test")), Module())
	app.RequireStart()
	if !Enabled() {
		t.Fatal("Enabled() = false after start, want true")
	}
	app.RequireStop()
	if Enabled() {
		t.Fatal("Enabled() = true after stop, want false")
	}
}
