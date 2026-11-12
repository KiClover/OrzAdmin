package wallet

import (
	"time"

	"github.com/sirupsen/logrus"
)

// 审计动作，用于区分是创建、列表还是状态变更返回了明文码。
const (
	walletCodeActionCreate = "create"
	walletCodeActionList   = "list"
	walletCodeActionStatus = "status"
)

// codeAuditEvent 记录一次兑换码明文相关操作。
//
// 该结构刻意不包含码值：审计只需要回答“谁在什么时候读了哪些码”，
// 明文一旦写入日志就失去了独立权限的意义。
type codeAuditEvent struct {
	Operator   string
	Action     string
	RequestID  string
	Filters    map[string]string
	Count      int
	Denied     bool
	Reason     string
	OccurredAt time.Time
}

// auditSink 接收明文读取审计；默认实现写结构化日志，测试可替换。
type auditSink interface {
	RecordCodeAccess(event codeAuditEvent)
}

// logAuditSink 把审计事件写进统一日志，字段固定且不含码值。
type logAuditSink struct {
	entry *logrus.Entry
}

// newLogAuditSink 返回默认日志审计实现。
func newLogAuditSink() auditSink {
	return logAuditSink{entry: logger}
}

// RecordCodeAccess 输出结构化审计日志。
func (s logAuditSink) RecordCodeAccess(event codeAuditEvent) {
	message := "redemption code plaintext accessed"
	if event.Denied {
		message = "redemption code plaintext denied"
	}

	fields := logrus.Fields{
		"audit":        "wallet_redemption_code",
		"operator":     event.Operator,
		"action":       event.Action,
		"request_id":   event.RequestID,
		"record_count": event.Count,
		"denied":       event.Denied,
		"reason":       event.Reason,
		"occurred_at":  event.OccurredAt.UTC().Format(time.RFC3339Nano),
	}
	for name, value := range event.Filters {
		if value == "" {
			continue
		}
		fields["filter_"+name] = value
	}
	s.entry.WithFields(fields).Info(message)
}
