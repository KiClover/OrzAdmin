package log

import (
	"context"
	"errors"
	"github.com/KiClover/orzfrp/conf"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"io"
	"os"
)

var tracer = otel.Tracer("github.com/KiClover/orzfrp")

var (
	ErrOutputModeNotSupported = errors.New("output mode not supported")
	ErrFormatterNotSupported  = errors.New("formatter not supported")
)

func NewLogger(ctc context.Context, config *conf.LoggerConfig) (*logrus.Logger, error) {
	ctx, span := tracer.Start(ctc, "log.NewLogger")
	defer span.End()

	logger := logrus.StandardLogger()
	logger.SetReportCaller(true)

	level, err := logrus.ParseLevel(config.Level)
	if err != nil {
		logrus.WithContext(ctx).WithError(err).Errorf("failed to parse log level %s", config.Level)
		return nil, err
	}
	logger.SetLevel(level)

	// set output mode
	switch config.OutputMode {
	case "console":
		logger.SetOutput(os.Stdout)
	case "file":
		file, err := os.OpenFile(config.FilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			logrus.WithContext(ctx).WithError(err).Errorf("failed to open log file %s", config.FilePath)
			return nil, err
		}
		logger.SetOutput(file)
	case "all":
		file, err := os.OpenFile(config.FilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			logrus.WithContext(ctx).WithError(err).Errorf("failed to open log file %s", config.FilePath)
			return nil, err
		}
		mw := io.MultiWriter(os.Stdout, file)
		logger.SetOutput(mw)
	default:
		logrus.WithContext(ctx).Errorf("output mode %s not supported", config.OutputMode)
		return nil, ErrOutputModeNotSupported
	}

	// set formatter
	switch config.Formatter {
	case "text":
		logger.SetFormatter(&logrus.TextFormatter{})
	case "json":
		logger.SetFormatter(&logrus.JSONFormatter{})
	default:
		logrus.WithContext(ctx).Errorf("formatter %s not supported", config.Formatter)
		return nil, ErrFormatterNotSupported
	}
	return logger, err
}
