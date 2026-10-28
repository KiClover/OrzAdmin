package log

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"go.uber.org/fx/fxevent"
)

// FxEventLogger 将 Fx 生命周期事件转发到统一 Logger，模块前缀固定 [FX]。
type FxEventLogger struct {
	logger *logrus.Logger
}

func NewFxEventLogger(logger *logrus.Logger) fxevent.Logger {
	return &FxEventLogger{logger: logger}
}

func (l *FxEventLogger) LogEvent(e fxevent.Event) {
	entry := l.logger.WithField("module", ModuleFx)

	switch ev := e.(type) {
	case *fxevent.OnStartExecuting:
		entry.WithField("callee", ev.FunctionName).WithField("caller", ev.CallerName).
			Info("OnStart hook executing")
	case *fxevent.OnStartExecuted:
		f := entry.WithField("callee", ev.FunctionName).WithField("caller", ev.CallerName).
			WithField("runtime", ev.Runtime.String())
		logResult(f, ev.Err, "OnStart hook executed", "OnStart hook failed")
	case *fxevent.OnStopExecuting:
		entry.WithField("callee", ev.FunctionName).WithField("caller", ev.CallerName).
			Info("OnStop hook executing")
	case *fxevent.OnStopExecuted:
		f := entry.WithField("callee", ev.FunctionName).WithField("caller", ev.CallerName).
			WithField("runtime", ev.Runtime.String())
		logResult(f, ev.Err, "OnStop hook executed", "OnStop hook failed")
	case *fxevent.Supplied:
		f := entry.WithField("type", ev.TypeName).WithField("fx_module", ev.ModuleName)
		if ev.Err != nil {
			f.WithError(ev.Err).Error("supply failed")
		} else {
			f.Debug("supplied")
		}
	case *fxevent.Provided:
		f := entry.WithField("constructor", ev.ConstructorName).WithField("fx_module", ev.ModuleName)
		if ev.Err != nil {
			f.WithError(ev.Err).Error("provide failed")
		} else {
			f.Debug("provided")
		}
	case *fxevent.Replaced:
		f := entry.WithField("fx_module", ev.ModuleName)
		if ev.Err != nil {
			f.WithError(ev.Err).Error("replace failed")
		} else {
			f.Debug("replaced")
		}
	case *fxevent.Decorated:
		f := entry.WithField("decorator", ev.DecoratorName).WithField("fx_module", ev.ModuleName)
		if ev.Err != nil {
			f.WithError(ev.Err).Error("decorate failed")
		} else {
			f.Debug("decorated")
		}
	case *fxevent.BeforeRun:
		entry.WithField("name", ev.Name).WithField("kind", ev.Kind).Debug("before run")
	case *fxevent.Run:
		f := entry.WithField("name", ev.Name).WithField("kind", ev.Kind).
			WithField("runtime", ev.Runtime.String())
		if ev.Err != nil {
			f.WithError(ev.Err).Error("run failed")
		} else {
			f.Debug("run")
		}
	case *fxevent.Invoking:
		entry.WithField("callee", ev.FunctionName).Info("invoking")
	case *fxevent.Invoked:
		f := entry.WithField("callee", ev.FunctionName)
		logResult(f, ev.Err, "invoked", "invoke failed")
	case *fxevent.Started:
		logResult(entry, ev.Err, "started", "start failed")
	case *fxevent.Stopping:
		// ev.Signal 为 os.Signal 接口，零值事件为 nil，不能直接调 String()。
		f := entry
		if ev.Signal != nil {
			f = f.WithField("signal", ev.Signal.String())
		}
		f.Info("stopping")
	case *fxevent.Stopped:
		logResult(entry, ev.Err, "stopped", "stop failed")
	case *fxevent.RollingBack:
		entry.WithError(ev.StartErr).Error("rolling back")
	case *fxevent.RolledBack:
		logResult(entry, ev.Err, "rolled back", "rollback failed")
	case *fxevent.LoggerInitialized:
		f := entry.WithField("constructor", ev.ConstructorName)
		if ev.Err != nil {
			f.WithError(ev.Err).Error("custom logger initialization failed")
		} else {
			f.Debug("initialized custom event logger")
		}
	default:
		entry.WithField("event_type", fmt.Sprintf("%T", e)).Debug("fx event")
	}
}

// logResult 按 err 是否为 nil 选择 Info/Error。
func logResult(e *logrus.Entry, err error, okMsg, errMsg string) {
	if err != nil {
		e.WithError(err).Error(errMsg)
	} else {
		e.Info(okMsg)
	}
}
