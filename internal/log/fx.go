package log

import (
	"github.com/sirupsen/logrus"
	"go.uber.org/fx/fxevent"
)

func FxLogger(logger *logrus.Logger) fxevent.Logger {
	return &fxlogrus.LogrusLogger{Logger: logger}
}
