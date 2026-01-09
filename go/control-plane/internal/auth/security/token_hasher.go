////////////////////////////////////////////////////////////////////////////////
// FILE: internal/auth/security/token_hasher.go
// 新增文件：Token 生成和哈希工具
// 功能：
// 1. 生成随机 API Token（格式：tap_xxxxx_xxxxx）
// 2. 哈希 Token（bcrypt）
// 3. 验证 Token
////////////////////////////////////////////////////////////////////////////////

package security

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

// TokenHasher Token 哈希器
type TokenHasher struct {
	bcryptCost int
}

// NewTokenHasher 创建 Token 哈希器
func NewTokenHasher() *TokenHasher {
	return &TokenHasher{
		bcryptCost: 10, // 对于 Token 使用较低的 cost（Token 已经足够随机）
	}
}

// GenerateAPIKey 生成 API Key（格式：tap_xxxxx_xxxxx）
// prefix: Token 前缀（如 tap, tus, tpr, tsv）
//   - tap: API Token
//   - tus: User Token
//   - tpr: Probe Token
//   - tsv: Service Token
func (h *TokenHasher) GenerateAPIKey(tokenType model.TokenType) (plainToken, prefix string, err error) {
	// 确定前缀
	var prefixStr string
	switch tokenType {
	case model.TokenTypeAPI:
		prefixStr = "tap" // traffic-analysis-platform api
	case model.TokenTypeUser:
		prefixStr = "tus" // traffic-analysis-platform user
	case model.TokenTypeProbe:
		prefixStr = "tpr" // traffic-analysis-platform probe
	case model.TokenTypeService:
		prefixStr = "tsv" // traffic-analysis-platform service
	default:
		prefixStr = "tap"
	}

	// 生成两部分随机字符串
	part1, err := h.generateRandomString(16)
	if err != nil {
		return "", "", errors.Wrap(err, errors.ErrCodeInternal, "Failed to generate token part 1")
	}

	part2, err := h.generateRandomString(16)
	if err != nil {
		return "", "", errors.Wrap(err, errors.ErrCodeInternal, "Failed to generate token part 2")
	}

	// 清理 Base64 特殊字符，生成友好格式
	clean := func(s string) string {
		s = strings.ReplaceAll(s, "+", "")
		s = strings.ReplaceAll(s, "/", "")
		s = strings.ReplaceAll(s, "=", "")
		if len(s) > 20 {
			s = s[:20]
		}
		return strings.ToLower(s)
	}

	cleanPart1 := clean(part1)
	cleanPart2 := clean(part2)

	// 构建完整 Token
	fullToken := fmt.Sprintf("%s_%s_%s", prefixStr, cleanPart1, cleanPart2)
	tokenPrefix := fmt.Sprintf("%s_%s", prefixStr, cleanPart1[:8]) // 前缀用于识别

	return fullToken, tokenPrefix, nil
}

// HashToken 哈希 Token（使用 bcrypt）
func (h *TokenHasher) HashToken(plainToken string) (string, error) {
	if plainToken == "" {
		return "", errors.New(errors.ErrCodeInvalidParameter, "Token cannot be empty")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(plainToken), h.bcryptCost)
	if err != nil {
		return "", errors.Wrap(err, errors.ErrCodeInternal, "Failed to hash token")
	}

	return string(hash), nil
}

// VerifyToken 验证 Token
func (h *TokenHasher) VerifyToken(hashedToken, plainToken string) error {
	if hashedToken == "" || plainToken == "" {
		return errors.New(errors.ErrCodeInvalidParameter, "Token and hash cannot be empty")
	}

	err := bcrypt.CompareHashAndPassword([]byte(hashedToken), []byte(plainToken))
	if err != nil {
		if err == bcrypt.ErrMismatchedHashAndPassword {
			return errors.New(errors.ErrCodeTokenInvalid, "Invalid token")
		}
		return errors.Wrap(err, errors.ErrCodeInternal, "Failed to verify token")
	}

	return nil
}

// generateRandomString 生成随机字符串
func (h *TokenHasher) generateRandomString(length int) (string, error) {
	if length < 8 {
		length = 8 // 最小 8 字节
	}

	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	// Base64 URL 编码（URL 安全）
	return base64.URLEncoding.EncodeToString(b), nil
}

// GenerateSecureToken 生成通用安全 Token（用于其他场景）
func (h *TokenHasher) GenerateSecureToken(length int) (string, error) {
	if length < 32 {
		length = 32 // 最小 32 字节
	}

	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", errors.Wrap(err, errors.ErrCodeInternal, "Failed to generate random token")
	}

	return base64.URLEncoding.EncodeToString(b), nil
}

// TokenInfo Token 信息提取
type TokenInfo struct {
	Prefix    string
	Type      model.TokenType
	IsValid   bool
	CreatedAt string // 可选：从 Token 中提取创建时间（如果编码）
}

// ParseTokenPrefix 解析 Token 前缀
func ParseTokenPrefix(token string) TokenInfo {
	parts := strings.Split(token, "_")
	if len(parts) < 2 {
		return TokenInfo{IsValid: false}
	}

	prefix := parts[0]
	var tokenType model.TokenType

	switch prefix {
	case "tap":
		tokenType = model.TokenTypeAPI
	case "tus":
		tokenType = model.TokenTypeUser
	case "tpr":
		tokenType = model.TokenTypeProbe
	case "tsv":
		tokenType = model.TokenTypeService
	default:
		return TokenInfo{IsValid: false}
	}

	return TokenInfo{
		Prefix:  prefix + "_" + parts[1][:8],
		Type:    tokenType,
		IsValid: true,
	}
}
