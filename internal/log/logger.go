package log

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/sirupsen/logrus"
	"go.uber.org/fx"
)

var (
	ErrLevelNotSupported      = errors.New("log level not supported")
	ErrOutputModeNotSupported = errors.New("output mode not supported")
	ErrFormatterNotSupported  = errors.New("formatter not supported")
	ErrFilePathRequired       = errors.New("log file path is required when output_mode is file or all")
)

// NewLogger 校验并应用统一日志配置到 logrus 标准 Logger（包级 logrus.* 调用零改动生效）。
// 先完成全部纯校验，再打开文件，最后一次性修改全局状态；失败路径不留下半配置状态。
func NewLogger(lifecycle fx.Lifecycle, cfg *conf.Config) (*logrus.Logger, error) {
	logCfg := cfg.Logger

	// 1. 纯校验（不触碰全局状态）
	level, err := logrus.ParseLevel(logCfg.Level)
	if err != nil {
		return nil, fmt.Errorf("%w: %q", ErrLevelNotSupported, logCfg.Level)
	}

	var formatter logrus.Formatter
	switch logCfg.Formatter {
	case "", "module_text":
		formatter = &ModuleFormatter{}
	case "text":
		formatter = &logrus.TextFormatter{}
	case "json":
		formatter = &logrus.JSONFormatter{}
	default:
		return nil, fmt.Errorf("%w: %q", ErrFormatterNotSupported, logCfg.Formatter)
	}

	switch logCfg.OutputMode {
	case "console":
	case "file", "all":
		if logCfg.FilePath == "" {
			return nil, ErrFilePathRequired
		}
	default:
		return nil, fmt.Errorf("%w: %q", ErrOutputModeNotSupported, logCfg.OutputMode)
	}

	// 2. 打开文件输出（失败即返回，全局 Logger 未被修改）
	var rotator *dailyRotateWriter
	if logCfg.OutputMode == "file" || logCfg.OutputMode == "all" {
		rotator, err = newDailyRotateWriter(logCfg.FilePath, logCfg.MaxAgeDays, logCfg.Compress)
		if err != nil {
			return nil, fmt.Errorf("open log file: %w", err)
		}
	}

	// 3. 一次性应用全局配置
	logger := logrus.StandardLogger()
	logger.SetLevel(level)
	logger.SetFormatter(formatter)
	switch logCfg.OutputMode {
	case "console":
		logger.SetOutput(os.Stdout)
	case "file":
		logger.SetOutput(rotator)
	case "all":
		logger.SetOutput(io.MultiWriter(os.Stdout, rotator))
	}

	// 4. 生命周期：先切回 stderr 再关文件——Fx 在 hook 返回后还会写 OnStopExecuted/Stopped 事件，
	// 直接关文件会写到已关闭句柄。
	if rotator != nil {
		lifecycle.Append(fx.Hook{
			OnStop: func(context.Context) error {
				logger.SetOutput(os.Stderr)
				return rotator.Close()
			},
		})
	}

	return logger, nil
}
