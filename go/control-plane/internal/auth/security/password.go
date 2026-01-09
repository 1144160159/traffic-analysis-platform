////////////////////////////////////////////////////////////////////////////////
// FILE PATH: internal/auth/security/password.go
// 密码哈希和验证工具
////////////////////////////////////////////////////////////////////////////////

package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

// PasswordConfig 密码策略配置
type PasswordConfig struct {
	MinLength        int  `json:"min_length"`
	RequireUppercase bool `json:"require_uppercase"`
	RequireLowercase bool `json:"require_lowercase"`
	RequireDigit     bool `json:"require_digit"`
	RequireSpecial   bool `json:"require_special"`
	BcryptCost       int  `json:"bcrypt_cost"`
}

// DefaultPasswordConfig 默认密码策略
func DefaultPasswordConfig() PasswordConfig {
	return PasswordConfig{
		MinLength:        12,
		RequireUppercase: true,
		RequireLowercase: true,
		RequireDigit:     true,
		RequireSpecial:   true,
		BcryptCost:       bcrypt.DefaultCost, // 10
	}
}

// PasswordHasher 密码哈希器
type PasswordHasher struct {
	config PasswordConfig
}

// NewPasswordHasher 创建密码哈希器
func NewPasswordHasher(config PasswordConfig) *PasswordHasher {
	// 验证配置
	if config.BcryptCost < bcrypt.MinCost || config.BcryptCost > bcrypt.MaxCost {
		config.BcryptCost = bcrypt.DefaultCost
	}
	if config.MinLength < 8 {
		config.MinLength = 8
	}

	return &PasswordHasher{config: config}
}

// HashPassword 哈希密码
func (h *PasswordHasher) HashPassword(password string) (string, error) {
	if password == "" {
		return "", errors.New(errors.ErrCodeInvalidParameter, "Password cannot be empty")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), h.config.BcryptCost)
	if err != nil {
		return "", errors.Wrap(err, errors.ErrCodeInternal, "Failed to hash password")
	}

	return string(hash), nil
}

// VerifyPassword 验证密码
func (h *PasswordHasher) VerifyPassword(hashedPassword, password string) error {
	if hashedPassword == "" || password == "" {
		return errors.New(errors.ErrCodeInvalidParameter, "Password and hash cannot be empty")
	}

	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	if err != nil {
		if err == bcrypt.ErrMismatchedHashAndPassword {
			return errors.New(errors.ErrCodeInvalidCredentials, "Invalid password")
		}
		return errors.Wrap(err, errors.ErrCodeInternal, "Failed to verify password")
	}

	return nil
}

// ValidatePassword 验证密码强度
func (h *PasswordHasher) ValidatePassword(password string) error {
	if len(password) < h.config.MinLength {
		return errors.Newf(errors.ErrCodeInvalidParameter,
			"Password must be at least %d characters long", h.config.MinLength)
	}

	if h.config.RequireUppercase && !regexp.MustCompile(`[A-Z]`).MatchString(password) {
		return errors.New(errors.ErrCodeInvalidParameter,
			"Password must contain at least one uppercase letter")
	}

	if h.config.RequireLowercase && !regexp.MustCompile(`[a-z]`).MatchString(password) {
		return errors.New(errors.ErrCodeInvalidParameter,
			"Password must contain at least one lowercase letter")
	}

	if h.config.RequireDigit && !regexp.MustCompile(`[0-9]`).MatchString(password) {
		return errors.New(errors.ErrCodeInvalidParameter,
			"Password must contain at least one digit")
	}

	if h.config.RequireSpecial && !regexp.MustCompile(`[!@#$%^&*()_+\-=\[\]{};':"\\|,.<>\/?]`).MatchString(password) {
		return errors.New(errors.ErrCodeInvalidParameter,
			"Password must contain at least one special character")
	}

	return nil
}

// NeedsRehash 检查是否需要重新哈希（cost 变化时）
func (h *PasswordHasher) NeedsRehash(hashedPassword string) bool {
	cost, err := bcrypt.Cost([]byte(hashedPassword))
	if err != nil {
		return true
	}
	return cost != h.config.BcryptCost
}

// GenerateRandomPassword 生成随机密码
func (h *PasswordHasher) GenerateRandomPassword(length int) (string, error) {
	if length < h.config.MinLength {
		length = h.config.MinLength
	}

	const (
		uppercase = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
		lowercase = "abcdefghijklmnopqrstuvwxyz"
		digits    = "0123456789"
		special   = "!@#$%^&*()_+-=[]{}|;:,.<>?"
	)

	var chars string
	var required []byte

	// 确保包含必需的字符类型
	if h.config.RequireUppercase {
		chars += uppercase
		required = append(required, uppercase[randomInt(len(uppercase))])
	}
	if h.config.RequireLowercase {
		chars += lowercase
		required = append(required, lowercase[randomInt(len(lowercase))])
	}
	if h.config.RequireDigit {
		chars += digits
		required = append(required, digits[randomInt(len(digits))])
	}
	if h.config.RequireSpecial {
		chars += special
		required = append(required, special[randomInt(len(special))])
	}

	if chars == "" {
		chars = uppercase + lowercase + digits
	}

	// 生成剩余字符
	remaining := length - len(required)
	password := make([]byte, length)

	for i := 0; i < remaining; i++ {
		password[i] = chars[randomInt(len(chars))]
	}

	// 添加必需字符
	copy(password[remaining:], required)

	// 打乱顺序
	for i := len(password) - 1; i > 0; i-- {
		j := randomInt(i + 1)
		password[i], password[j] = password[j], password[i]
	}

	return string(password), nil
}

// randomInt 生成随机整数
func randomInt(max int) int {
	b := make([]byte, 4)
	rand.Read(b)
	return int(uint32(b[0])<<24|uint32(b[1])<<16|uint32(b[2])<<8|uint32(b[3])) % max
}

// TokenHasher Token 哈希器（用于存储 API Token）
type TokenHasher struct{}

// NewTokenHasher 创建 Token 哈希器
func NewTokenHasher() *TokenHasher {
	return &TokenHasher{}
}

// HashToken 哈希 Token（使用 bcrypt，与密码类似）
func (h *TokenHasher) HashToken(token string) (string, error) {
	if token == "" {
		return "", errors.New(errors.ErrCodeInvalidParameter, "Token cannot be empty")
	}

	// 对于 token，使用较低的 cost（因为 token 通常已经足够随机）
	hash, err := bcrypt.GenerateFromPassword([]byte(token), 10)
	if err != nil {
		return "", errors.Wrap(err, errors.ErrCodeInternal, "Failed to hash token")
	}

	return string(hash), nil
}

// VerifyToken 验证 Token
func (h *TokenHasher) VerifyToken(hashedToken, token string) error {
	if hashedToken == "" || token == "" {
		return errors.New(errors.ErrCodeInvalidParameter, "Token and hash cannot be empty")
	}

	err := bcrypt.CompareHashAndPassword([]byte(hashedToken), []byte(token))
	if err != nil {
		if err == bcrypt.ErrMismatchedHashAndPassword {
			return errors.New(errors.ErrCodeTokenInvalid, "Invalid token")
		}
		return errors.Wrap(err, errors.ErrCodeInternal, "Failed to verify token")
	}

	return nil
}

// GenerateSecureToken 生成安全的随机 Token
func (h *TokenHasher) GenerateSecureToken(length int) (string, error) {
	if length < 32 {
		length = 32 // 最小 32 字节
	}

	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", errors.Wrap(err, errors.ErrCodeInternal, "Failed to generate random token")
	}

	// Base64 URL 编码（URL 安全）
	return base64.URLEncoding.EncodeToString(b), nil
}

// GenerateAPIKey 生成 API Key（格式：tap_xxxxx_xxxxx）
func (h *TokenHasher) GenerateAPIKey(prefix string) (string, error) {
	if prefix == "" {
		prefix = "tap" // traffic-analysis-platform
	}

	// 生成随机部分
	part1, err := h.GenerateSecureToken(16)
	if err != nil {
		return "", err
	}

	part2, err := h.GenerateSecureToken(16)
	if err != nil {
		return "", err
	}

	// 清理 Base64 特殊字符，使用更友好的格式
	clean := func(s string) string {
		s = strings.ReplaceAll(s, "+", "")
		s = strings.ReplaceAll(s, "/", "")
		s = strings.ReplaceAll(s, "=", "")
		if len(s) > 20 {
			s = s[:20]
		}
		return strings.ToLower(s)
	}

	return fmt.Sprintf("%s_%s_%s", prefix, clean(part1), clean(part2)), nil
}

// ConstantTimeCompare 常量时间比较（防止时序攻击）
func ConstantTimeCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// PasswordStrength 密码强度评分
type PasswordStrength int

const (
	PasswordStrengthWeak PasswordStrength = iota
	PasswordStrengthFair
	PasswordStrengthGood
	PasswordStrengthStrong
	PasswordStrengthVeryStrong
)

func (s PasswordStrength) String() string {
	switch s {
	case PasswordStrengthWeak:
		return "weak"
	case PasswordStrengthFair:
		return "fair"
	case PasswordStrengthGood:
		return "good"
	case PasswordStrengthStrong:
		return "strong"
	case PasswordStrengthVeryStrong:
		return "very_strong"
	default:
		return "unknown"
	}
}

// CheckPasswordStrength 检查密码强度
func CheckPasswordStrength(password string) PasswordStrength {
	score := 0

	// 长度
	if len(password) >= 8 {
		score++
	}
	if len(password) >= 12 {
		score++
	}
	if len(password) >= 16 {
		score++
	}

	// 字符类型
	if regexp.MustCompile(`[a-z]`).MatchString(password) {
		score++
	}
	if regexp.MustCompile(`[A-Z]`).MatchString(password) {
		score++
	}
	if regexp.MustCompile(`[0-9]`).MatchString(password) {
		score++
	}
	if regexp.MustCompile(`[!@#$%^&*()_+\-=\[\]{};':"\\|,.<>\/?]`).MatchString(password) {
		score++
	}

	// 混合度
	uniqueChars := make(map[rune]bool)
	for _, c := range password {
		uniqueChars[c] = true
	}
	if len(uniqueChars) >= len(password)/2 {
		score++
	}

	// 评分
	switch {
	case score <= 2:
		return PasswordStrengthWeak
	case score <= 4:
		return PasswordStrengthFair
	case score <= 6:
		return PasswordStrengthGood
	case score <= 7:
		return PasswordStrengthStrong
	default:
		return PasswordStrengthVeryStrong
	}
}
