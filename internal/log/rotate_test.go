package log

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func newTestWriter(t *testing.T, dir string, maxAge int, compress bool) *dailyRotateWriter {
	t.Helper()
	w, err := newDailyRotateWriter(filepath.Join(dir, "app.log"), maxAge, compress)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	return w
}

func setNow(t *testing.T, w *dailyRotateWriter, date time.Time) {
	t.Helper()
	w.mu.Lock()
	w.now = func() time.Time { return date }
	w.mu.Unlock()
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

var (
	day1 = time.Date(2026, 10, 27, 10, 0, 0, 0, time.Local)
	day2 = time.Date(2026, 10, 28, 10, 0, 0, 0, time.Local)
)

func TestRotateSameDaySameFile(t *testing.T) {
	dir := t.TempDir()
	w := newTestWriter(t, dir, 30, false)
	setNow(t, w, day1)

	// 构造时已按真实今天打开文件；重设时钟后触发一次轮转对齐到 day1
	if _, err := w.Write([]byte("line1\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("line2\n")); err != nil {
		t.Fatal(err)
	}

	got := readFile(t, filepath.Join(dir, "app-2026-10-27.log"))
	if got != "line1\nline2\n" {
		t.Fatalf("got %q", got)
	}
}

func TestRotateAcrossDays(t *testing.T) {
	dir := t.TempDir()
	w := newTestWriter(t, dir, 30, false)
	setNow(t, w, day1)
	if _, err := w.Write([]byte("day1\n")); err != nil {
		t.Fatal(err)
	}

	setNow(t, w, day2)
	if _, err := w.Write([]byte("day2\n")); err != nil {
		t.Fatal(err)
	}

	if got := readFile(t, filepath.Join(dir, "app-2026-10-27.log")); got != "day1\n" {
		t.Fatalf("old file got %q", got)
	}
	if got := readFile(t, filepath.Join(dir, "app-2026-10-28.log")); got != "day2\n" {
		t.Fatalf("new file got %q", got)
	}
}

func TestRotateRetention(t *testing.T) {
	dir := t.TempDir()
	// 所有日期均相对真实今天派生：构造函数会先用真实时钟执行一次 cleanupLocked，
	// 硬编码的历史日期会随时间推移在构造期被误删，导致断言失效（时钟耦合）。
	base := time.Now().Truncate(24 * time.Hour)
	old := filepath.Join(dir, "app-"+base.AddDate(0, 0, -5).Format(rotatedDateLayout)+".log")    // 早于 cutoff(base-3d)
	recent := filepath.Join(dir, "app-"+base.AddDate(0, 0, -1).Format(rotatedDateLayout)+".log") // 晚于 cutoff，且构造期真实清理不会触碰
	other := filepath.Join(dir, "other-"+base.AddDate(0, 0, -5).Format(rotatedDateLayout)+".log")
	for _, p := range []string{old, recent, other} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	w, err := newDailyRotateWriter(filepath.Join(dir, "app.log"), 3, false)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	setNow(t, w, base)                                      // cutoff = base-3d
	if _, err := w.Write([]byte("trigger\n")); err != nil { // 触发轮转+清理
		t.Fatal(err)
	}

	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("超期文件应被删除")
	}
	if _, err := os.Stat(recent); err != nil {
		t.Fatal("未超期文件应保留")
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatal("不匹配文件应保留")
	}
}

func TestRotateCompress(t *testing.T) {
	dir := t.TempDir()
	w := newTestWriter(t, dir, 30, true)
	setNow(t, w, day1)
	if _, err := w.Write([]byte("day1\n")); err != nil {
		t.Fatal(err)
	}
	setNow(t, w, day2)
	if _, err := w.Write([]byte("day2\n")); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "app-2026-10-27.log")); !os.IsNotExist(err) {
		t.Fatal("压缩后原文件应删除")
	}
	gz, err := os.Open(filepath.Join(dir, "app-2026-10-27.log.gz"))
	if err != nil {
		t.Fatal("应生成 .gz 文件")
	}
	defer gz.Close()
	zr, err := gzip.NewReader(gz)
	if err != nil {
		t.Fatal(err)
	}
	content, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "day1\n" {
		t.Fatalf("gz content = %q", content)
	}
}

func TestRotateCloseReleasesHandle(t *testing.T) {
	dir := t.TempDir()
	w := newTestWriter(t, dir, 30, false)
	setNow(t, w, day1)
	if _, err := w.Write([]byte("x\n")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	// Windows 下句柄未释放则 Remove 失败
	if err := os.Remove(filepath.Join(dir, "app-2026-10-27.log")); err != nil {
		t.Fatalf("Close 后文件应可删除: %v", err)
	}
}

func TestRotateConcurrentWrite(t *testing.T) {
	dir := t.TempDir()
	w := newTestWriter(t, dir, 30, false)
	setNow(t, w, day1)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				if _, err := w.Write([]byte("x\n")); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()

	content := readFile(t, filepath.Join(dir, "app-2026-10-27.log"))
	if got := strings.Count(content, "x\n"); got != 400 {
		t.Fatalf("lines = %d, want 400", got)
	}
}

// TestRotateErrorPathNoDeadlock 是评审 B1 的回归测试：旧实现在持锁区间内调用 logrus，
// 而标准 logger 的 Out 正是本 writer，logrus entry.write 持 Logger.mu 调用 Out.Write，
// 同一 goroutine 因此重入 Logger.mu（以及 Write 里的 w.mu），永久死锁。修复后错误在
// 释放 w.mu 后写 stderr，Write 必须立即返回。串行执行，不触碰 t.Parallel。
func TestRotateErrorPathNoDeadlock(t *testing.T) {
	const deadline = 2 * time.Second
	cases := []struct {
		name       string
		compress   bool
		maxAge     int
		wantInMsg  string
		triggerDay time.Time
		setup      func(t *testing.T, w *dailyRotateWriter)
	}{
		{
			name:      "cleanup remove failed",
			maxAge:    3,
			wantInMsg: "log cleanup: remove",
			// 超期文件删除失败（Windows 上文件被其他进程占用的真实触发场景）
			setup: func(t *testing.T, w *dailyRotateWriter) {
				t.Helper()
				stale := w.fileName(time.Now().AddDate(0, 0, -10).Format(rotatedDateLayout))
				if err := os.WriteFile(stale, []byte("stale\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				w.removeFile = func(string) error { return errors.New("injected remove failure") }
			},
			triggerDay: time.Now().AddDate(0, 0, 1),
		},
		{
			name:      "compress failed",
			wantInMsg: "log rotate: compress",
			compress:  true,
			setup: func(t *testing.T, w *dailyRotateWriter) {
				t.Helper()
				setNow(t, w, day1)
				if _, err := w.Write([]byte("day1\n")); err != nil {
					t.Fatal(err)
				}
				w.compressFile = func(string) error { return errors.New("injected compress failure") }
			},
			triggerDay: day2,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshotStandardLogger(t) // 本测试设置标准 logger 输出，结束恢复
			dir := t.TempDir()
			w := newTestWriter(t, dir, tc.maxAge, tc.compress)
			tc.setup(t, w)

			var errOut bytes.Buffer
			w.errOut = &errOut
			setNow(t, w, tc.triggerDay)

			// 生产配置（output_mode: file/all）下标准 logger 的 Out 就是这个 writer
			std := logrus.StandardLogger()
			std.SetOutput(w)
			std.SetLevel(logrus.InfoLevel)

			done := make(chan error, 1)
			go func() {
				logrus.Info("trigger rotation error path") // 与生产一致：Out.Write 在 Logger.mu 内调用
				_, err := w.Write([]byte("after-error\n"))
				done <- err
			}()
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("Write 返回错误: %v", err)
				}
			case <-time.After(deadline):
				t.Fatalf("Write 未在 %s 内返回：轮转错误路径自死锁（评审 B1 回归）", deadline)
			}
			if !strings.Contains(errOut.String(), tc.wantInMsg) {
				t.Fatalf("轮转错误未上报，stderr 内容 = %q", errOut.String())
			}
		})
	}
}

// TestRotateWriteAfterClose 覆盖 N4：Close 之后写入返回错误而不是空指针 panic。
func TestRotateWriteAfterClose(t *testing.T) {
	dir := t.TempDir()
	w := newTestWriter(t, dir, 30, false)
	setNow(t, w, day1)
	if _, err := w.Write([]byte("x\n")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("y\n")); !errors.Is(err, errWriterClosed) {
		t.Fatalf("err = %v, want %v", err, errWriterClosed)
	}
}
