package conf

import (
	"os"
	"path/filepath"
	"testing"
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
