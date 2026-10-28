package log

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/KiClover/orzAdmin/conf"
	"github.com/sirupsen/logrus"
	"go.uber.org/fx"
)

func TestFxWiringConstructsLoggerOnceAndStopsCleanly(t *testing.T) {
	snapshotStandardLogger(t)
	dir := t.TempDir()
	cfg := &conf.Config{Logger: conf.LoggerConf{
		Level: "info", OutputMode: "file", Formatter: "module_text",
		FilePath: filepath.Join(dir, "app.log"), MaxAgeDays: 30,
	}}

	var calls int32
	app := fx.New(
		fx.Supply(cfg),
		fx.Provide(func(lifecycle fx.Lifecycle, c *conf.Config) (*logrus.Logger, error) {
			atomic.AddInt32(&calls, 1)
			return NewLogger(lifecycle, c)
		}),
		fx.WithLogger(NewFxEventLogger), // 消费 *logrus.Logger，强制 fx.New 阶段实例化
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := app.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("NewLogger 执行次数 = %d, want 1", got)
	}

	logrus.Info("integration hello")
	if err := app.Stop(ctx); err != nil {
		t.Fatal(err)
	}

	// 文件有内容且 Stop 后句柄已释放（Windows 可删除）
	matches, _ := filepath.Glob(filepath.Join(dir, "app-*.log"))
	if len(matches) != 1 {
		t.Fatalf("matches = %v", matches)
	}
	content, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "integration hello") {
		t.Fatalf("content = %q", content)
	}
	if err := os.Remove(matches[0]); err != nil {
		t.Fatalf("Stop 后文件应可删除: %v", err)
	}
}
