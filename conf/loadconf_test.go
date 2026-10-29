package conf

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfigDefaults(t *testing.T) {
	cfg := loadConfig(filepath.Join(t.TempDir(), "missing.yaml"))
	if cfg.Logger.Level != "info" {
		t.Fatalf("Level = %q, want info", cfg.Logger.Level)
	}
	if cfg.Logger.OutputMode != "console" {
		t.Fatalf("OutputMode = %q, want console", cfg.Logger.OutputMode)
	}
	if cfg.Logger.Formatter != "module_text" {
		t.Fatalf("Formatter = %q, want module_text", cfg.Logger.Formatter)
	}
	if cfg.Logger.MaxAgeDays != 30 {
		t.Fatalf("MaxAgeDays = %d, want 30", cfg.Logger.MaxAgeDays)
	}
	if cfg.Logger.Compress {
		t.Fatal("Compress = true, want false")
	}
}

func TestLoadConfigFromYAML(t *testing.T) {
	file := filepath.Join(t.TempDir(), "cfg.yaml")
	yaml := "logger:\n  level: debug\n  output_mode: all\n  file_path: ./logs/app.log\n"
	if err := os.WriteFile(file, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := loadConfig(file)
	if cfg.Logger.Level != "debug" || cfg.Logger.OutputMode != "all" || cfg.Logger.FilePath != "./logs/app.log" {
		t.Fatalf("unexpected: %+v", cfg.Logger)
	}
	if cfg.Logger.MaxAgeDays != 30 {
		t.Fatalf("MaxAgeDays = %d, want default 30", cfg.Logger.MaxAgeDays)
	}
}

func TestLoadConfigEnvOverridesYAML(t *testing.T) {
	t.Setenv("LOG_LEVEL", "error")
	file := filepath.Join(t.TempDir(), "cfg.yaml")
	if err := os.WriteFile(file, []byte("logger:\n  level: info\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := loadConfig(file)
	if cfg.Logger.Level != "error" {
		t.Fatalf("Level = %q, want error (env wins)", cfg.Logger.Level)
	}
}

func TestLoadConfigEnvOnly(t *testing.T) {
	t.Setenv("LOG_OUTPUT_MODE", "file")
	t.Setenv("LOG_FILE_PATH", "./logs/x.log")
	cfg := loadConfig(filepath.Join(t.TempDir(), "missing.yaml"))
	if cfg.Logger.OutputMode != "file" || cfg.Logger.FilePath != "./logs/x.log" {
		t.Fatalf("unexpected: %+v", cfg.Logger)
	}
}

func TestLoadConfigTraceDefaultsOptionalOff(t *testing.T) {
	cfg := loadConfig(filepath.Join(t.TempDir(), "missing.yaml"))
	if cfg.Trace.Enabled {
		t.Fatal("Trace.Enabled = true, want false（链路追踪是可选功能，默认关闭）")
	}
	if cfg.Trace.Endpoint != "localhost:4317" {
		t.Fatalf("Trace.Endpoint = %q, want localhost:4317", cfg.Trace.Endpoint)
	}
	if !cfg.Trace.Insecure {
		t.Fatal("Trace.Insecure = false, want true")
	}
	if cfg.Trace.SampleRatio != 1.0 {
		t.Fatalf("Trace.SampleRatio = %v, want 1.0", cfg.Trace.SampleRatio)
	}
	if cfg.Trace.ExportTimeout != 10*time.Second {
		t.Fatalf("Trace.ExportTimeout = %v, want 10s", cfg.Trace.ExportTimeout)
	}
	if cfg.Trace.ShutdownTimeout != 5*time.Second {
		t.Fatalf("Trace.ShutdownTimeout = %v, want 5s", cfg.Trace.ShutdownTimeout)
	}
}

func TestLoadConfigTraceEnabledFromYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.yaml")
	if err := os.WriteFile(path, []byte("trace:\n  enabled: true\n  endpoint: \"jaeger:4317\"\n  sample_ratio: 0.2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := loadConfig(path)
	if !cfg.Trace.Enabled || cfg.Trace.Endpoint != "jaeger:4317" || cfg.Trace.SampleRatio != 0.2 {
		t.Fatalf("unexpected trace config: %+v", cfg.Trace)
	}
}
