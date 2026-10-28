package log

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

const rotatedDateLayout = "2006-01-02"

// dailyRotateWriter 按天轮转的文件 writer：
// basePath "./logs/app.log" 实际写入 "./logs/app-2026-10-28.log"，所有日期文件同目录。
type dailyRotateWriter struct {
	mu         sync.Mutex
	basePath   string
	maxAgeDays int
	compress   bool
	now        func() time.Time // 可注入，便于测试

	file    *os.File
	curDate string // 当前文件对应日期
}

func newDailyRotateWriter(basePath string, maxAgeDays int, compress bool) (*dailyRotateWriter, error) {
	w := &dailyRotateWriter{
		basePath:   basePath,
		maxAgeDays: maxAgeDays,
		compress:   compress,
		now:        time.Now,
	}
	if err := w.rotateLocked(w.today()); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *dailyRotateWriter) today() string {
	return w.now().Format(rotatedDateLayout)
}

// fileName 派生当日文件名：./logs/app.log + 2026-10-28 -> ./logs/app-2026-10-28.log
func (w *dailyRotateWriter) fileName(date string) string {
	dir := filepath.Dir(w.basePath)
	base := filepath.Base(w.basePath)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	return filepath.Join(dir, fmt.Sprintf("%s-%s%s", stem, date, ext))
}

func (w *dailyRotateWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if today := w.today(); today != w.curDate {
		if err := w.rotateLocked(today); err != nil {
			return 0, err
		}
	}
	return w.file.Write(p)
}

// rotateLocked 关闭旧文件（可选压缩）、打开当日文件、触发保留清理。调用方须持锁。
// 轮转/清理/压缩失败不中断服务：记 Error 日志后继续，只有打开新文件失败才返回错误。
func (w *dailyRotateWriter) rotateLocked(today string) error {
	oldName := ""
	if w.file != nil {
		oldName = w.file.Name()
		if err := w.file.Close(); err != nil {
			logrus.WithError(err).Error("log rotate: close old file failed")
		}
		w.file = nil
	}
	if w.compress && oldName != "" {
		if err := gzipFile(oldName); err != nil {
			logrus.WithError(err).Error("log rotate: compress failed")
		}
	}
	if err := os.MkdirAll(filepath.Dir(w.basePath), 0o750); err != nil {
		return err
	}
	f, err := os.OpenFile(w.fileName(today), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		return err
	}
	w.file = f
	w.curDate = today
	w.cleanupLocked()
	return nil
}

// cleanupLocked 删除早于 max_age_days 的轮转文件；只删模式匹配的，绝不触碰其他文件。
func (w *dailyRotateWriter) cleanupLocked() {
	if w.maxAgeDays <= 0 {
		return
	}
	base := filepath.Base(w.basePath)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	pattern := filepath.Join(filepath.Dir(w.basePath), stem+"-*"+ext+"*")

	matches, err := filepath.Glob(pattern)
	if err != nil {
		logrus.WithError(err).Error("log cleanup: glob failed")
		return
	}
	cutoff := w.now().AddDate(0, 0, -w.maxAgeDays).Format(rotatedDateLayout)
	for _, m := range matches {
		date, ok := parseRotatedDate(m, stem, ext)
		if !ok {
			continue
		}
		if date < cutoff { // ISO 日期字符串可直接比较
			if err := os.Remove(m); err != nil {
				logrus.WithError(err).Errorf("log cleanup: remove %s failed", m)
			}
		}
	}
}

func parseRotatedDate(path, stem, ext string) (string, bool) {
	name := strings.TrimSuffix(filepath.Base(path), ".gz")
	if !strings.HasPrefix(name, stem+"-") || !strings.HasSuffix(name, ext) {
		return "", false
	}
	date := strings.TrimSuffix(strings.TrimPrefix(name, stem+"-"), ext)
	if _, err := time.Parse(rotatedDateLayout, date); err != nil {
		return "", false
	}
	return date, true
}

// Close 只关闭自己打开的文件，绝不关闭 os.Stdout。
func (w *dailyRotateWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

// gzipFile 把已轮转的日志文件压缩为 path+".gz" 并删除原文件。
func gzipFile(path string) error {
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer src.Close() // 错误路径兜底；成功路径下方显式 Close，二次关闭错误忽略

	dst, err := os.OpenFile(path+".gz", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(dst)
	_, copyErr := io.Copy(gz, src)
	gzErr := gz.Close()
	dstErr := dst.Close()
	if copyErr != nil {
		return copyErr
	}
	if gzErr != nil {
		return gzErr
	}
	if dstErr != nil {
		return dstErr
	}
	if err := src.Close(); err != nil { // Windows 下删除前必须先关闭句柄
		return err
	}
	return os.Remove(path)
}
