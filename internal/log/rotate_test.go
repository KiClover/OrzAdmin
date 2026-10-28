package log

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
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
	// 预置：超期文件、未超期文件、不匹配文件
	old := filepath.Join(dir, "app-2026-10-01.log")
	recent := filepath.Join(dir, "app-2026-10-27.log")
	other := filepath.Join(dir, "other-2026-10-01.log")
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
	setNow(t, w, day2)                                      // cutoff = 2026-10-25
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
