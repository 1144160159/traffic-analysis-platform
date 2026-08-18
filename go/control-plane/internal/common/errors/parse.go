package errors

import (
	"strings"
)

// ParseErrorCode 从 "CODE: message" 前缀错误文本还原稳定错误码;
// 非契约错误返回 ok=false。用于仓储/适配层以契约码前缀返回的原始错误
// 在 API 边界保持 HTTP 语义码稳定(统一错误框架入口)。
func ParseErrorCode(err error) (code ErrorCode, message string, ok bool) {
	if err == nil {
		return "", "", false
	}
	msg := err.Error()
	for _, c := range GetAllErrorCodes() {
		prefix := string(c) + ": "
		if len(msg) >= len(prefix) && strings.HasPrefix(msg, prefix) {
			return c, msg[len(prefix):], true
		}
	}
	// 兼容旧式 "[CODE] message" 包装
	rest := msg
	if len(rest) >= 1 && rest[0] == '[' {
		if idx := strings.Index(rest, "] "); idx > 1 {
			codeStr := rest[1:idx]
			if ValidateErrorCode(ErrorCode(codeStr)) {
				return ErrorCode(codeStr), rest[idx+2:], true
			}
		}
	}
	return "", "", false
}
