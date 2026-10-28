package log

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const rotatedDateLayout = "2006-01-02"

// errWriterClosed 表示已 Close 的 writer 被再次写入（避免空指针 panic）。
var errWriterClosed = errors.New("dailyRotateWriter: writer is closed")

// dailyRotateWriter 按天轮转的文件 writer：
// basePath "./logs/app.log" 实际写入 "./logs/app-2026-10-28.log"，所有日期文件同目录。
//
// 本 writer 就是标准 logger 的 Out：logrus entry.write 持有 Logger.mu 调用 Write，
// 因此持锁区间内绝不能再调用 logrus，否则同一 goroutine 重入 Logger.mu/w.mu 会永久死锁。
type dailyRotateWriter struct {
	mu         sync.Mutex
	basePath   string
	maxAgeDays int
	compress   bool
	now        func() time.Time // 可注入，便于测试

	removeFile   func(string) error // 可注入，便于测试清理失败路径；默认 os.Remove
	compressFile func(string) error // 可注入，便于测试压缩失败路径；默认 gzipFile
	errOut       io.Writer          // 轮转错误出口；默认 os.Stderr

	file    *os.File
	curDate string // 当前文件对应日期
}

func newDailyRotateWriter(basePath string, maxAgeDays int, compress bool) (*dailyRotateWriter, error) {
	w := &dailyRotateWriter{
		basePath:     basePath,
		maxAgeDays:   maxAgeDays,
		compress:     compress,
		now:          time.Now,
		removeFile:   os.Remove,
		compressFile: gzipFile,
		errOut:       os.Stderr,
	}
	deferred, err := w.rotateLocked(w.today())
	if err != nil {
		return nil, err
	}
	w.reportRotationErrors(deferred) // 构造期尚未成为任何 logger 的输出，无重入风险
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
	n, deferred, err := w.writeLocked(p)
	w.mu.Unlock()
	// 只能在释放 w.mu 之后上报，且必须绕开 logrus：logrus 调用方持有 Logger.mu，
	// 再调 logrus 会在同一 goroutine 上重入该互斥锁（评审 B1）。
	w.reportRotationErrors(deferred)
	return n, err
}

// writeLocked 持有 w.mu 完成轮转与写入，把需要在解锁后上报的错误信息返回给调用方。
func (w *dailyRotateWriter) writeLocked(p []byte) (n int, deferred []string, err error) {
	if today := w.today(); today != w.curDate {
		deferred, err = w.rotateLocked(today)
		if err != nil {
			return 0, deferred, err
		}
	}
	if w.file == nil { // Close 之后再写入不 panic
		return 0, deferred, errWriterClosed
	}
	n, err = w.file.Write(p)
	return n, deferred, err
}

// rotateLocked 关闭旧文件（可选压缩）、打开当日文件、触发保留清理。调用方须持锁。
// 轮转/清理/压缩失败不中断服务：收集错误信息返回给调用方在解锁后上报，
// 只有打开新文件失败才返回 error。
func (w *dailyRotateWriter) rotateLocked(today string) ([]string, error) {
	var deferred []string
	oldName := ""
	if w.file != nil {
		oldName = w.file.Name()
		if err := w.file.Close(); err != nil {
			deferred = append(deferred, fmt.Sprintf("log rotate: close old file failed: %v", err))
		}
		w.file = nil
	}
	if w.compress && oldName != "" {
		if err := w.compressFile(oldName); err != nil {
			deferred = append(deferred, fmt.Sprintf("log rotate: compress %s failed: %v", oldName, err))
		}
	}
	if err := os.MkdirAll(filepath.Dir(w.basePath), 0o750); err != nil {
		return deferred, err
	}
	f, err := os.OpenFile(w.fileName(today), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		return deferred, err
	}
	w.file = f
	w.curDate = today
	deferred = append(deferred, w.cleanupLocked()...)
	return deferred, nil
}

// cleanupLocked 删除早于 max_age_days 的轮转文件；只删模式匹配的，绝不触碰其他文件。
// 与 rotateLocked 一样，错误信息只返回、不在持锁期间上报。
func (w *dailyRotateWriter) cleanupLocked() []string {
	if w.maxAgeDays <= 0 {
		return nil
	}
	base := filepath.Base(w.basePath)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	pattern := filepath.Join(filepath.Dir(w.basePath), stem+"-*"+ext+"*")

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return []string{fmt.Sprintf("log cleanup: glob failed: %v", err)}
	}
	var deferred []string
	cutoff := w.now().AddDate(0, 0, -w.maxAgeDays).Format(rotatedDateLayout)
	for _, m := range matches {
		date, ok := parseRotatedDate(m, stem, ext)
		if !ok {
			continue
		}
		if date < cutoff { // ISO 日期字符串可直接比较
			if err := w.removeFile(m); err != nil {
				deferred = append(deferred, fmt.Sprintf("log cleanup: remove %s failed: %v", m, err))
			}
		}
	}
	return deferred
}

// reportRotationErrors 上报轮转/清理错误。调用方必须已释放 w.mu，且不得改用 logrus：
// 本 writer 是标准 logger 的 Out，logrus entry.write 调用 Out.Write 期间持有 Logger.mu，
// 此处再调 logrus 会重入 Logger.mu（以及 Write 里的 w.mu），造成永久死锁（评审 B1）。
// 直接写 stderr 不经过任何 logger 互斥锁，是唯一可证明安全的出口。
func (w *dailyRotateWriter) reportRotationErrors(msgs []string) {
	if len(msgs) == 0 {
		return
	}
	out := w.errOut
	if out == nil {
		out = os.Stderr
	}
	for _, m := range msgs {
		fmt.Fprintf(out, "[log] %s\n", m)
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
