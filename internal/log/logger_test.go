package log

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/sirupsen/logrus"
	"go.uber.org/fx/fxtest"
)

func snapshotStandardLogger(t *testing.T) {
	t.Helper()
	l := logrus.StandardLogger()
	out, formatter, level, reportCaller := l.Out, l.Formatter, l.GetLevel(), l.ReportCaller
	t.Cleanup(func() {
		l.SetOutput(out)
		l.SetFormatter(formatter)
		l.SetLevel(level)
		l.ReportCaller = reportCaller
	})
}

func validConf() *conf.Config {
	return &conf.Config{Logger: conf.LoggerConf{
		Level: "info", OutputMode: "console", Formatter: "module_text", MaxAgeDays: 30,
	}}
}

func TestNewLoggerValidation(t *testing.T) {
	snapshotStandardLogger(t)
	std := logrus.StandardLogger()
	origOut, origFormatter, origLevel := std.Out, std.Formatter, std.GetLevel()

	cases := []struct {
		name    string
		mutate  func(*conf.Config)
		wantErr error
	}{
		{"bad level", func(c *conf.Config) { c.Logger.Level = "nope" }, ErrLevelNotSupported},
		{"bad output mode", func(c *conf.Config) { c.Logger.OutputMode = "nope" }, ErrOutputModeNotSupported},
		{"bad formatter", func(c *conf.Config) { c.Logger.Formatter = "nope" }, ErrFormatterNotSupported},
		{"file mode empty path", func(c *conf.Config) { c.Logger.OutputMode = "file" }, ErrFilePathRequired},
		{"all mode empty path", func(c *conf.Config) { c.Logger.OutputMode = "all" }, ErrFilePathRequired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validConf()
			tc.mutate(cfg)
			_, err := NewLogger(fxtest.NewLifecycle(t), cfg)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			// 失败路径不得留下半配置的全局状态
			if std.Out != origOut || std.Formatter != origFormatter || std.GetLevel() != origLevel {
				t.Fatal("失败后全局 Logger 状态被修改")
			}
		})
	}
}

func TestNewLoggerReturnsStandardLogger(t *testing.T) {
	snapshotStandardLogger(t)
	logger, err := NewLogger(fxtest.NewLifecycle(t), validConf())
	if err != nil {
		t.Fatal(err)
	}
	if logger != logrus.StandardLogger() {
		t.Fatal("应返回标准 Logger，保证包级 logrus.* 调用生效")
	}
	if _, ok := logger.Formatter.(*ModuleFormatter); !ok {
		t.Fatalf("Formatter = %T, want *ModuleFormatter", logger.Formatter)
	}
}

func TestNewLoggerFileModeWritesRotatedFile(t *testing.T) {
	snapshotStandardLogger(t)
	dir := t.TempDir()
	cfg := validConf()
	cfg.Logger.OutputMode = "all"
	cfg.Logger.FilePath = filepath.Join(dir, "app.log")

	lc := fxtest.NewLifecycle(t)
	if _, err := NewLogger(lc, cfg); err != nil {
		t.Fatal(err)
	}
	// fx v1.24 的 Stop 只执行"OnStart 已被调用"的 OnStop hook，必须先 RequireStart
	// 才能在 RequireStop 时真正走到关闭文件的 OnStop。
	lc.RequireStart()
	logrus.Info("hello file")

	// 当日文件应存在且内容符合格式（日期部分用 Glob 匹配）
	matches, err := filepath.Glob(filepath.Join(dir, "app-*.log"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("matches = %v, err = %v", matches, err)
	}
	content, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	if got := string(content); !strings.HasPrefix(got, "[APP][INFO] ") || !strings.HasSuffix(got, ": hello file\n") {
		t.Fatalf("content = %q", got)
	}

	// OnStop：先切回 stderr 再关文件；Stop 后句柄释放可删除
	lc.RequireStop()
	if err := os.Remove(matches[0]); err != nil {
		t.Fatalf("OnStop 后文件应可删除: %v", err)
	}
}
