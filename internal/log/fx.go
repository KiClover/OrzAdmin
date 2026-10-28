package log

import (
	"github.com/sirupsen/logrus"
	"go.uber.org/fx/fxevent"
)

// NewFxEventLogger 占位实现：丢弃 Fx 事件。最终转发实现见后续任务。
func NewFxEventLogger(_ *logrus.Logger) fxevent.Logger {
	return fxevent.NopLogger
}
