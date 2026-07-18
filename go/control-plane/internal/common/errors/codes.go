////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/common/errors/codes.go
// 修复版本 v2：
// 1. 修复 #11：增加多语言支持框架
// 2. 新增错误消息模板系统
// 3. 增加错误码分组和查询功能
// 4. 提供默认的中英文错误消息
////////////////////////////////////////////////////////////////////////////////

package errors

import (
	"fmt"
	"sync"
)

// ErrorCode 错误码类型
type ErrorCode string

// 错误码分类：
// 1xxx - 认证授权错误
// 2xxx - 参数验证错误
// 3xxx - 业务逻辑错误
// 4xxx - 资源操作错误
// 5xxx - 系统内部错误
// 6xxx - 外部依赖错误

const (
	// 认证授权错误 (1xxx)
	ErrCodeUnauthorized       ErrorCode = "AUTH_1001"
	ErrCodeTokenExpired       ErrorCode = "AUTH_1002"
	ErrCodeTokenInvalid       ErrorCode = "AUTH_1003"
	ErrCodePermissionDenied   ErrorCode = "AUTH_1004"
	ErrCodeTenantNotFound     ErrorCode = "AUTH_1005"
	ErrCodeUserNotFound       ErrorCode = "AUTH_1006"
	ErrCodeInvalidCredentials ErrorCode = "AUTH_1007"
	ErrCodeSessionExpired     ErrorCode = "AUTH_1008"
	ErrCodeMTLSRequired       ErrorCode = "AUTH_1009"
	ErrCodeQuotaExceeded      ErrorCode = "AUTH_1010"

	// 参数验证错误 (2xxx)
	ErrCodeInvalidRequest   ErrorCode = "VALID_2001"
	ErrCodeMissingParameter ErrorCode = "VALID_2002"
	ErrCodeInvalidParameter ErrorCode = "VALID_2003"
	ErrCodeInvalidFormat    ErrorCode = "VALID_2004"
	ErrCodeOutOfRange       ErrorCode = "VALID_2005"
	ErrCodeDuplicateValue   ErrorCode = "VALID_2006"

	// 业务逻辑错误 (3xxx)
	ErrCodeAlertNotFound          ErrorCode = "BIZ_3001"
	ErrCodeRuleNotFound           ErrorCode = "BIZ_3002"
	ErrCodeDeploymentNotFound     ErrorCode = "BIZ_3003"
	ErrCodeInvalidStateTransition ErrorCode = "BIZ_3004"
	ErrCodeVersionConflict        ErrorCode = "BIZ_3005"
	ErrCodeRollbackFailed         ErrorCode = "BIZ_3006"
	ErrCodeGrayDeploymentActive   ErrorCode = "BIZ_3007"
	ErrCodePcapNotFound           ErrorCode = "BIZ_3008"
	ErrCodeSessionNotFound        ErrorCode = "BIZ_3009"
	ErrCodeEntityNotFound         ErrorCode = "BIZ_3010"
	ErrCodeDedupConflict          ErrorCode = "BIZ_3011"
	ErrCodeModelNotFound          ErrorCode = "BIZ_3012"
	ErrCodeModelVersionNotFound   ErrorCode = "BIZ_3013"

	// 资源操作错误 (4xxx)
	ErrCodeResourceNotFound  ErrorCode = "RES_4001"
	ErrCodeResourceExists    ErrorCode = "RES_4002"
	ErrCodeResourceLocked    ErrorCode = "RES_4003"
	ErrCodeResourceDeleted   ErrorCode = "RES_4004"
	ErrCodeConcurrentModify  ErrorCode = "RES_4005"
	ErrCodeGraphNodeNotFound ErrorCode = "RES_4006"
	ErrCodeGraphEdgeNotFound ErrorCode = "RES_4007"
	ErrCodeSpaceNotFound     ErrorCode = "RES_4008"

	// 系统内部错误 (5xxx)
	ErrCodeInternal           ErrorCode = "SYS_5001"
	ErrCodeDatabaseError      ErrorCode = "SYS_5002"
	ErrCodeCacheError         ErrorCode = "SYS_5003"
	ErrCodeSerializationError ErrorCode = "SYS_5004"
	ErrCodeConfigError        ErrorCode = "SYS_5005"
	ErrCodeTimeout            ErrorCode = "SYS_5006"

	// 外部依赖错误 (6xxx)
	ErrCodeKafkaError         ErrorCode = "EXT_6001"
	ErrCodeClickHouseError    ErrorCode = "EXT_6002"
	ErrCodeNebulaGraphError   ErrorCode = "EXT_6003"
	ErrCodeOpenSearchError    ErrorCode = "EXT_6004"
	ErrCodeRedisError         ErrorCode = "EXT_6005"
	ErrCodeMinIOError         ErrorCode = "EXT_6006"
	ErrCodePostgresError      ErrorCode = "EXT_6007"
	ErrCodeOIDCError          ErrorCode = "EXT_6008"
	ErrCodeArkimeError        ErrorCode = "EXT_6009"
	ErrCodeServiceUnavailable ErrorCode = "EXT_6010"

	// 认证授权补充
	ErrCodeUserNotActive ErrorCode = "AUTH_1011"

	// 系统内部补充
	ErrCodeNotImplemented ErrorCode = "SYS_5007"
)

// String 返回错误码字符串
func (c ErrorCode) String() string {
	return string(c)
}

// HTTPStatus 返回对应的HTTP状态码
func (c ErrorCode) HTTPStatus() int {
	switch {
	case c >= "AUTH_1001" && c <= "AUTH_1010":
		if c == ErrCodeQuotaExceeded {
			return 429
		}
		return 401
	case c >= "VALID_2001" && c <= "VALID_2006":
		return 400
	case c >= "BIZ_3001" && c <= "BIZ_3013":
		if c == ErrCodeVersionConflict || c == ErrCodeConcurrentModify || c == ErrCodeGrayDeploymentActive {
			return 409
		}
		return 404
	case c == "AUTH_1011":
		return 401
	case c >= "RES_4001" && c <= "RES_4008":
		if c == ErrCodeResourceNotFound || c == ErrCodeGraphNodeNotFound || c == ErrCodeGraphEdgeNotFound || c == ErrCodeSpaceNotFound {
			return 404
		}
		if c == ErrCodeResourceExists || c == ErrCodeConcurrentModify {
			return 409
		}
		return 400
	case c >= "SYS_5001" && c <= "SYS_5007":
		if c == ErrCodeTimeout {
			return 504
		}
		if c == ErrCodeNotImplemented {
			return 501
		}
		return 500
	case c >= "EXT_6001" && c <= "EXT_6010":
		if c == ErrCodeServiceUnavailable {
			return 503
		}
		return 502
	default:
		return 500
	}
}

// IsRetryable 判断是否可重试
func (c ErrorCode) IsRetryable() bool {
	switch c {
	case ErrCodeTimeout,
		ErrCodeKafkaError,
		ErrCodeClickHouseError,
		ErrCodeNebulaGraphError,
		ErrCodeOpenSearchError,
		ErrCodeMinIOError,
		ErrCodeRedisError,
		ErrCodePostgresError,
		ErrCodeOIDCError,
		ErrCodeServiceUnavailable:
		return true
	default:
		return false
	}
}

// Category 返回错误分类
func (c ErrorCode) Category() string {
	prefix := string(c)[:3]
	switch prefix {
	case "AUT":
		return "authentication"
	case "VAL":
		return "validation"
	case "BIZ":
		return "business"
	case "RES":
		return "resource"
	case "SYS":
		return "system"
	case "EXT":
		return "external"
	default:
		return "unknown"
	}
}

// ==================== 修复 #11：多语言支持框架 ====================

// Language 语言代码
type Language string

const (
	LanguageEnglish            Language = "en"
	LanguageChinese            Language = "zh"
	LanguageChineseTraditional Language = "zh-TW"
	LanguageJapanese           Language = "ja"
	LanguageKorean             Language = "ko"
)

// ErrorMessage 错误消息定义
type ErrorMessage struct {
	Code     ErrorCode
	Messages map[Language]string
	Template bool // 是否为模板消息（包含占位符）
}

// Get 获取指定语言的错误消息
func (em *ErrorMessage) Get(lang Language, args ...interface{}) string {
	msg, ok := em.Messages[lang]
	if !ok {
		// 回退到英文
		msg = em.Messages[LanguageEnglish]
	}

	if em.Template && len(args) > 0 {
		return fmt.Sprintf(msg, args...)
	}

	return msg
}

// ErrorMessageRegistry 错误消息注册表
type ErrorMessageRegistry struct {
	mu       sync.RWMutex
	messages map[ErrorCode]*ErrorMessage
	fallback Language
}

// globalRegistry 全局错误消息注册表
var globalRegistry = &ErrorMessageRegistry{
	messages: make(map[ErrorCode]*ErrorMessage),
	fallback: LanguageEnglish,
}

// init 初始化默认错误消息
func init() {
	// 注册默认错误消息
	registerDefaultMessages()
}

// Register 注册错误消息
func (r *ErrorMessageRegistry) Register(msg *ErrorMessage) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages[msg.Code] = msg
}

// Get 获取错误消息
func (r *ErrorMessageRegistry) Get(code ErrorCode, lang Language, args ...interface{}) string {
	r.mu.RLock()
	msg, ok := r.messages[code]
	r.mu.RUnlock()

	if !ok {
		// 未注册的错误码，返回默认消息
		return fmt.Sprintf("Error: %s", code)
	}

	return msg.Get(lang, args...)
}

// SetFallbackLanguage 设置回退语言
func (r *ErrorMessageRegistry) SetFallbackLanguage(lang Language) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fallback = lang
}

// GetMessage 从全局注册表获取错误消息
func GetMessage(code ErrorCode, lang Language, args ...interface{}) string {
	return globalRegistry.Get(code, lang, args...)
}

// RegisterMessage 向全局注册表注册错误消息
func RegisterMessage(msg *ErrorMessage) {
	globalRegistry.Register(msg)
}

// ==================== 默认错误消息定义 ====================

func registerDefaultMessages() {
	// 认证授权错误
	RegisterMessage(&ErrorMessage{
		Code: ErrCodeUnauthorized,
		Messages: map[Language]string{
			LanguageEnglish: "Unauthorized access",
			LanguageChinese: "未授权访问",
		},
	})

	RegisterMessage(&ErrorMessage{
		Code: ErrCodeTokenExpired,
		Messages: map[Language]string{
			LanguageEnglish: "Token has expired",
			LanguageChinese: "令牌已过期",
		},
	})

	RegisterMessage(&ErrorMessage{
		Code: ErrCodeTokenInvalid,
		Messages: map[Language]string{
			LanguageEnglish: "Invalid token",
			LanguageChinese: "无效的令牌",
		},
	})

	RegisterMessage(&ErrorMessage{
		Code: ErrCodePermissionDenied,
		Messages: map[Language]string{
			LanguageEnglish: "Permission denied",
			LanguageChinese: "权限不足",
		},
	})

	RegisterMessage(&ErrorMessage{
		Code: ErrCodeTenantNotFound,
		Messages: map[Language]string{
			LanguageEnglish: "Tenant not found",
			LanguageChinese: "租户不存在",
		},
	})

	RegisterMessage(&ErrorMessage{
		Code: ErrCodeUserNotFound,
		Messages: map[Language]string{
			LanguageEnglish: "User not found",
			LanguageChinese: "用户不存在",
		},
	})

	RegisterMessage(&ErrorMessage{
		Code: ErrCodeQuotaExceeded,
		Messages: map[Language]string{
			LanguageEnglish: "Quota exceeded",
			LanguageChinese: "配额已超限",
		},
	})

	// 参数验证错误
	RegisterMessage(&ErrorMessage{
		Code: ErrCodeInvalidRequest,
		Messages: map[Language]string{
			LanguageEnglish: "Invalid request",
			LanguageChinese: "无效的请求",
		},
	})

	RegisterMessage(&ErrorMessage{
		Code:     ErrCodeMissingParameter,
		Template: true,
		Messages: map[Language]string{
			LanguageEnglish: "Missing required parameter: %s",
			LanguageChinese: "缺少必需参数：%s",
		},
	})

	RegisterMessage(&ErrorMessage{
		Code:     ErrCodeInvalidParameter,
		Template: true,
		Messages: map[Language]string{
			LanguageEnglish: "Invalid parameter: %s",
			LanguageChinese: "无效参数：%s",
		},
	})

	RegisterMessage(&ErrorMessage{
		Code: ErrCodeInvalidFormat,
		Messages: map[Language]string{
			LanguageEnglish: "Invalid format",
			LanguageChinese: "格式错误",
		},
	})

	RegisterMessage(&ErrorMessage{
		Code:     ErrCodeOutOfRange,
		Template: true,
		Messages: map[Language]string{
			LanguageEnglish: "Value out of range: %s",
			LanguageChinese: "数值超出范围：%s",
		},
	})

	// 业务逻辑错误
	RegisterMessage(&ErrorMessage{
		Code:     ErrCodeAlertNotFound,
		Template: true,
		Messages: map[Language]string{
			LanguageEnglish: "Alert not found: %s",
			LanguageChinese: "告警不存在：%s",
		},
	})

	RegisterMessage(&ErrorMessage{
		Code:     ErrCodeRuleNotFound,
		Template: true,
		Messages: map[Language]string{
			LanguageEnglish: "Rule not found: %s",
			LanguageChinese: "规则不存在：%s",
		},
	})

	RegisterMessage(&ErrorMessage{
		Code:     ErrCodeModelNotFound,
		Template: true,
		Messages: map[Language]string{
			LanguageEnglish: "Model not found: %s",
			LanguageChinese: "Model not found: %s",
		},
	})

	RegisterMessage(&ErrorMessage{
		Code:     ErrCodeModelVersionNotFound,
		Template: true,
		Messages: map[Language]string{
			LanguageEnglish: "Model version not found: %s",
			LanguageChinese: "Model version not found: %s",
		},
	})

	RegisterMessage(&ErrorMessage{
		Code: ErrCodeVersionConflict,
		Messages: map[Language]string{
			LanguageEnglish: "Version conflict detected",
			LanguageChinese: "检测到版本冲突",
		},
	})

	RegisterMessage(&ErrorMessage{
		Code: ErrCodeRollbackFailed,
		Messages: map[Language]string{
			LanguageEnglish: "Rollback failed",
			LanguageChinese: "回滚失败",
		},
	})

	RegisterMessage(&ErrorMessage{
		Code:     ErrCodePcapNotFound,
		Template: true,
		Messages: map[Language]string{
			LanguageEnglish: "PCAP file not found: %s",
			LanguageChinese: "PCAP 文件不存在：%s",
		},
	})

	RegisterMessage(&ErrorMessage{
		Code:     ErrCodeSessionNotFound,
		Template: true,
		Messages: map[Language]string{
			LanguageEnglish: "Session not found: %s",
			LanguageChinese: "会话不存在：%s",
		},
	})

	// 资源操作错误
	RegisterMessage(&ErrorMessage{
		Code:     ErrCodeResourceNotFound,
		Template: true,
		Messages: map[Language]string{
			LanguageEnglish: "Resource not found: %s",
			LanguageChinese: "资源不存在：%s",
		},
	})

	RegisterMessage(&ErrorMessage{
		Code:     ErrCodeResourceExists,
		Template: true,
		Messages: map[Language]string{
			LanguageEnglish: "Resource already exists: %s",
			LanguageChinese: "资源已存在：%s",
		},
	})

	RegisterMessage(&ErrorMessage{
		Code: ErrCodeResourceLocked,
		Messages: map[Language]string{
			LanguageEnglish: "Resource is locked",
			LanguageChinese: "资源已被锁定",
		},
	})

	// 系统内部错误
	RegisterMessage(&ErrorMessage{
		Code: ErrCodeInternal,
		Messages: map[Language]string{
			LanguageEnglish: "Internal server error",
			LanguageChinese: "服务器内部错误",
		},
	})

	RegisterMessage(&ErrorMessage{
		Code: ErrCodeDatabaseError,
		Messages: map[Language]string{
			LanguageEnglish: "Database operation failed",
			LanguageChinese: "数据库操作失败",
		},
	})

	RegisterMessage(&ErrorMessage{
		Code: ErrCodeCacheError,
		Messages: map[Language]string{
			LanguageEnglish: "Cache operation failed",
			LanguageChinese: "缓存操作失败",
		},
	})

	RegisterMessage(&ErrorMessage{
		Code: ErrCodeTimeout,
		Messages: map[Language]string{
			LanguageEnglish: "Operation timed out",
			LanguageChinese: "操作超时",
		},
	})

	// 外部依赖错误
	RegisterMessage(&ErrorMessage{
		Code: ErrCodeKafkaError,
		Messages: map[Language]string{
			LanguageEnglish: "Kafka operation failed",
			LanguageChinese: "Kafka 操作失败",
		},
	})

	RegisterMessage(&ErrorMessage{
		Code: ErrCodeClickHouseError,
		Messages: map[Language]string{
			LanguageEnglish: "ClickHouse operation failed",
			LanguageChinese: "ClickHouse 操作失败",
		},
	})

	RegisterMessage(&ErrorMessage{
		Code: ErrCodeServiceUnavailable,
		Messages: map[Language]string{
			LanguageEnglish: "Service temporarily unavailable",
			LanguageChinese: "服务暂时不可用",
		},
	})
}

// ==================== 辅助函数 ====================

// GetErrorInfo 获取错误码的完整信息
func GetErrorInfo(code ErrorCode, lang Language) ErrorInfo {
	return ErrorInfo{
		Code:       code,
		HTTPStatus: code.HTTPStatus(),
		Category:   code.Category(),
		Retryable:  code.IsRetryable(),
		Message:    GetMessage(code, lang),
	}
}

// ErrorInfo 错误码完整信息
type ErrorInfo struct {
	Code       ErrorCode `json:"code"`
	HTTPStatus int       `json:"http_status"`
	Category   string    `json:"category"`
	Retryable  bool      `json:"retryable"`
	Message    string    `json:"message"`
}

// GetAllErrorCodes 获取所有错误码（用于文档生成）
func GetAllErrorCodes() []ErrorCode {
	return []ErrorCode{
		// 认证授权
		ErrCodeUnauthorized,
		ErrCodeTokenExpired,
		ErrCodeTokenInvalid,
		ErrCodePermissionDenied,
		ErrCodeTenantNotFound,
		ErrCodeUserNotFound,
		ErrCodeInvalidCredentials,
		ErrCodeSessionExpired,
		ErrCodeMTLSRequired,
		ErrCodeQuotaExceeded,
		ErrCodeUserNotActive,

		// 参数验证
		ErrCodeInvalidRequest,
		ErrCodeMissingParameter,
		ErrCodeInvalidParameter,
		ErrCodeInvalidFormat,
		ErrCodeOutOfRange,
		ErrCodeDuplicateValue,

		// 业务逻辑
		ErrCodeAlertNotFound,
		ErrCodeRuleNotFound,
		ErrCodeDeploymentNotFound,
		ErrCodeInvalidStateTransition,
		ErrCodeVersionConflict,
		ErrCodeRollbackFailed,
		ErrCodeGrayDeploymentActive,
		ErrCodePcapNotFound,
		ErrCodeSessionNotFound,
		ErrCodeEntityNotFound,
		ErrCodeDedupConflict,
		ErrCodeModelNotFound,
		ErrCodeModelVersionNotFound,

		// 资源操作
		ErrCodeResourceNotFound,
		ErrCodeResourceExists,
		ErrCodeResourceLocked,
		ErrCodeResourceDeleted,
		ErrCodeConcurrentModify,
		ErrCodeGraphNodeNotFound,
		ErrCodeGraphEdgeNotFound,
		ErrCodeSpaceNotFound,

		// 系统内部
		ErrCodeInternal,
		ErrCodeDatabaseError,
		ErrCodeCacheError,
		ErrCodeSerializationError,
		ErrCodeConfigError,
		ErrCodeTimeout,
		ErrCodeNotImplemented,

		// 外部依赖
		ErrCodeKafkaError,
		ErrCodeClickHouseError,
		ErrCodeNebulaGraphError,
		ErrCodeOpenSearchError,
		ErrCodeMinIOError,
		ErrCodeRedisError,
		ErrCodePostgresError,
		ErrCodeOIDCError,
		ErrCodeArkimeError,
		ErrCodeServiceUnavailable,
	}
}

// GetErrorCodesByCategory 按分类获取错误码
func GetErrorCodesByCategory(category string) []ErrorCode {
	var codes []ErrorCode
	for _, code := range GetAllErrorCodes() {
		if code.Category() == category {
			codes = append(codes, code)
		}
	}
	return codes
}

// ==================== 用于测试的辅助函数 ====================

// ValidateErrorCode 验证错误码是否合法
func ValidateErrorCode(code ErrorCode) bool {
	for _, c := range GetAllErrorCodes() {
		if c == code {
			return true
		}
	}
	return false
}
