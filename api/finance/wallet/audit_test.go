package wallet

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

// recordingAuditSink 在内存中保存审计事件，供 handler 与审计测试复用。
type recordingAuditSink struct {
	events []codeAuditEvent
}

func (s *recordingAuditSink) RecordCodeAccess(event codeAuditEvent) {
	s.events = append(s.events, event)
}

// last 返回最近一次审计事件；没有事件时返回 nil。
func (s *recordingAuditSink) last() *codeAuditEvent {
	if len(s.events) == 0 {
		return nil
	}
	return &s.events[len(s.events)-1]
}

func TestLogAuditSinkWritesMetadataOnly(t *testing.T) {
	var buf bytes.Buffer
	logger := logrus.New()
	logger.SetOutput(&buf)
	logger.SetFormatter(&logrus.JSONFormatter{})

	sink := logAuditSink{entry: logrus.NewEntry(logger)}
	sink.RecordCodeAccess(codeAuditEvent{
		Operator:   "9001",
		Action:     walletCodeActionList,
		RequestID:  "req-1",
		Filters:    map[string]string{"type": "UNIQUE", "status": ""},
		Count:      2,
		OccurredAt: time.Date(2026, 11, 12, 6, 30, 0, 0, time.UTC),
	})

	fields := decodeAuditLine(t, buf.String())
	if fields["audit"] != "wallet_redemption_code" {
		t.Fatalf("unexpected audit marker: %v", fields["audit"])
	}
	if fields["operator"] != "9001" || fields["action"] != walletCodeActionList {
		t.Fatalf("unexpected operator/action: %v %v", fields["operator"], fields["action"])
	}
	if fields["request_id"] != "req-1" {
		t.Fatalf("unexpected request id: %v", fields["request_id"])
	}
	if fields["record_count"] != float64(2) {
		t.Fatalf("unexpected count: %v", fields["record_count"])
	}
	if fields["denied"] != false {
		t.Fatalf("unexpected denied flag: %v", fields["denied"])
	}
	if fields["filter_type"] != "UNIQUE" {
		t.Fatalf("unexpected type filter: %v", fields["filter_type"])
	}
	if _, ok := fields["filter_status"]; ok {
		t.Fatal("empty filters must be skipped")
	}
	if !strings.Contains(buf.String(), "redemption code plaintext accessed") {
		t.Fatalf("missing audit message: %s", buf.String())
	}
}

func TestLogAuditSinkMarksDeniedAccess(t *testing.T) {
	var buf bytes.Buffer
	logger := logrus.New()
	logger.SetOutput(&buf)
	logger.SetFormatter(&logrus.JSONFormatter{})

	sink := logAuditSink{entry: logrus.NewEntry(logger)}
	sink.RecordCodeAccess(codeAuditEvent{
		Operator:   "9002",
		Action:     walletCodeActionCreate,
		Denied:     true,
		Reason:     "plaintext permission missing",
		OccurredAt: time.Now(),
	})

	fields := decodeAuditLine(t, buf.String())
	if fields["denied"] != true {
		t.Fatalf("unexpected denied flag: %v", fields["denied"])
	}
	if fields["reason"] != "plaintext permission missing" {
		t.Fatalf("unexpected reason: %v", fields["reason"])
	}
	if !strings.Contains(buf.String(), "redemption code plaintext denied") {
		t.Fatalf("missing denial message: %s", buf.String())
	}
}

// decodeAuditLine 解析单行 JSON 日志；多行输出说明格式被破坏。
func decodeAuditLine(t *testing.T, output string) map[string]any {
	t.Helper()
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		t.Fatal("no audit log emitted")
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) != 1 {
		t.Fatalf("expected one log line, got %d", len(lines))
	}
	fields := map[string]any{}
	if err := json.Unmarshal([]byte(lines[0]), &fields); err != nil {
		t.Fatalf("decode audit log: %v", err)
	}
	return fields
}
