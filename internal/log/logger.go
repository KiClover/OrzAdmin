package log

import (
	"github.com/KiClover/orzAdmin/conf"
	"github.com/sirupsen/logrus"
	"go.uber.org/fx"
)

// NewLogger 返回全局标准 Logger。占位实现：配置应用与轮转见后续任务。
func NewLogger(_ fx.Lifecycle, _ *conf.Config) (*logrus.Logger, error) {
	return logrus.StandardLogger(), nil
}
