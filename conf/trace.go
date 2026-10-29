package conf

import "time"

// TraceConf stores OpenTelemetry tracing configurations.
type TraceConf struct {
	Enabled         bool              `mapstructure:"enabled"`          // 总开关，默认 false（可选功能）
	Endpoint        string            `mapstructure:"endpoint"`         // OTLP/gRPC 地址，默认 localhost:4317；可写 http(s):// 形式
	Insecure        bool              `mapstructure:"insecure"`         // 明文 gRPC，默认 true（内网）
	Headers         map[string]string `mapstructure:"headers"`          // 后端鉴权/租户头
	SampleRatio     float64           `mapstructure:"sample_ratio"`     // 0.0~1.0，默认 1.0
	ExportTimeout   time.Duration     `mapstructure:"export_timeout"`   // 单批导出超时，默认 10s
	ShutdownTimeout time.Duration     `mapstructure:"shutdown_timeout"` // 退出 Flush/Shutdown 超时，默认 5s
	Environment     string            `mapstructure:"environment"`      // deployment.environment.name
	Version         string            `mapstructure:"version"`          // service.version
}
