package log

import (
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func formatEntry(t *testing.T, f *ModuleFormatter, entry *logrus.Entry) string {
	t.Helper()
	b, err := f.Format(entry)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func fixedEntry(level logrus.Level, msg string) *logrus.Entry {
	return &logrus.Entry{
		Logger:  logrus.New(),
		Time:    time.Date(2026, 10, 28, 10, 51, 22, 0, time.Local),
		Level:   level,
		Message: msg,
		Data:    logrus.Fields{},
	}
}

func TestModuleFormatterExactFormat(t *testing.T) {
	t.Parallel()
	e := fixedEntry(logrus.InfoLevel, "database connected")
	e.Data["module"] = ModuleDB
	got := formatEntry(t, &ModuleFormatter{}, e)
	want := "[DB][INFO] 2026-10-28 10:51:22: database connected\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestModuleFormatterDefaultModule(t *testing.T) {
	t.Parallel()
	got := formatEntry(t, &ModuleFormatter{}, fixedEntry(logrus.WarnLevel, "login failed"))
	want := "[APP][WARN] 2026-10-28 10:51:22: login failed\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestModuleFormatterFieldsSortedAndModuleNotRepeated(t *testing.T) {
	t.Parallel()
	e := fixedEntry(logrus.ErrorLevel, "request failed")
	e.Data["module"] = ModuleHTTP
	e.Data["path"] = "/api/user"
	e.Data["error"] = "connection refused"
	got := formatEntry(t, &ModuleFormatter{}, e)
	want := "[HTTP][ERROR] 2026-10-28 10:51:22: request failed error=\"connection refused\" path=/api/user\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestWithModule(t *testing.T) {
	t.Parallel()
	e := WithModule(ModuleAuth)
	if e.Data["module"] != ModuleAuth {
		t.Fatalf("module = %v, want %s", e.Data["module"], ModuleAuth)
	}
}
