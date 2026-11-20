package utils

import "strings"

// NormalizeEmail 统一去除邮箱首尾空白并转为小写，用于存储、查询与验证码 Key 的规范化。
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// NormalizeEmailPtr 对非 nil 的邮箱指针执行规范化处理。
func NormalizeEmailPtr(email *string) *string {
	if email == nil {
		return nil
	}
	normalized := NormalizeEmail(*email)
	return &normalized
}
