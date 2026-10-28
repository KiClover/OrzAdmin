package log

import (
	"errors"
	"testing"

	"github.com/sirupsen/logrus"
	"go.uber.org/fx/fxevent"
)

type recordHook struct {
	entries []*logrus.Entry
}

func (h *recordHook) Levels() []logrus.Level { return logrus.AllLevels }
func (h *recordHook) Fire(e *logrus.Entry) error {
	h.entries = append(h.entries, e)
	return nil
}

func newRecordLogger() (*logrus.Logger, *recordHook) {
	l := logrus.New()
	// logrus.New() 默认 Info 级别，会先于 hook 过滤 Debug 条目；
	// 本测试需要断言 Debug 事件，因此调低到 Debug。
	l.SetLevel(logrus.DebugLevel)
	h := &recordHook{}
	l.AddHook(h)
	return l, h
}

func TestFxEventLoggerLevels(t *testing.T) {
	t.Parallel()
	logger, hook := newRecordLogger()
	fl := NewFxEventLogger(logger)

	fl.LogEvent(&fxevent.Started{})                              // Info
	fl.LogEvent(&fxevent.Started{Err: errors.New("boom")})       // Error
	fl.LogEvent(&fxevent.Provided{ConstructorName: "x"})         // Debug
	fl.LogEvent(&fxevent.RollingBack{StartErr: errors.New("x")}) // Error

	want := []logrus.Level{logrus.InfoLevel, logrus.ErrorLevel, logrus.DebugLevel, logrus.ErrorLevel}
	if len(hook.entries) != len(want) {
		t.Fatalf("entries = %d, want %d", len(hook.entries), len(want))
	}
	for i, e := range hook.entries {
		if e.Level != want[i] {
			t.Fatalf("entry %d level = %s, want %s", i, e.Level, want[i])
		}
		if e.Data["module"] != ModuleFx {
			t.Fatalf("entry %d module = %v, want FX", i, e.Data["module"])
		}
	}
}

func TestFxEventLoggerCoversAllEventTypes(t *testing.T) {
	t.Parallel()
	logger, hook := newRecordLogger()
	fl := NewFxEventLogger(logger)

	events := []fxevent.Event{
		&fxevent.OnStartExecuting{}, &fxevent.OnStartExecuted{},
		&fxevent.OnStopExecuting{}, &fxevent.OnStopExecuted{},
		&fxevent.Supplied{}, &fxevent.Provided{}, &fxevent.Replaced{},
		&fxevent.Decorated{}, &fxevent.BeforeRun{}, &fxevent.Run{},
		&fxevent.Invoking{}, &fxevent.Invoked{}, &fxevent.Started{},
		&fxevent.Stopping{}, &fxevent.Stopped{}, &fxevent.RollingBack{},
		&fxevent.RolledBack{}, &fxevent.LoggerInitialized{},
	}
	for _, e := range events {
		fl.LogEvent(e) // 不 panic 即覆盖
	}
	if len(hook.entries) != len(events) {
		t.Fatalf("entries = %d, want %d", len(hook.entries), len(events))
	}
}
