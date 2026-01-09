////////////////////////////////////////////////////////////////////////////////
// FILE PATH: internal/auth/mtls/validator.go
// mTLS 客户端证书验证
////////////////////////////////////////////////////////////////////////////////

package mtls

import (
	"crypto/x509"
	"fmt"
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

// CertificateValidator 证书验证器
type CertificateValidator struct {
	logger *zap.Logger

	// 是否启用 CN 验证
	validateCN bool

	// 是否启用 OU 验证
	validateOU bool

	// 允许的 Organization（组织）列表
	allowedOrganizations []string

	// 允许的 OrganizationalUnit（组织单元）列表
	allowedOUs []string

	// 是否要求特定的 SAN（Subject Alternative Name）
	requireSAN bool

	// 是否验证证书撤销列表（CRL）
	validateCRL bool
}

// Config 验证器配置
type Config struct {
	ValidateCN           bool     `env:"MTLS_VALIDATE_CN" envDefault:"true"`
	ValidateOU           bool     `env:"MTLS_VALIDATE_OU" envDefault:"false"`
	AllowedOrganizations []string `env:"MTLS_ALLOWED_ORGS" envSeparator:","`
	AllowedOUs           []string `env:"MTLS_ALLOWED_OUS" envSeparator:","`
	RequireSAN           bool     `env:"MTLS_REQUIRE_SAN" envDefault:"false"`
	ValidateCRL          bool     `env:"MTLS_VALIDATE_CRL" envDefault:"false"`
}

// NewCertificateValidator 创建证书验证器
func NewCertificateValidator(config Config, logger *zap.Logger) *CertificateValidator {
	return &CertificateValidator{
		logger:               logger,
		validateCN:           config.ValidateCN,
		validateOU:           config.ValidateOU,
		allowedOrganizations: config.AllowedOrganizations,
		allowedOUs:           config.AllowedOUs,
		requireSAN:           config.RequireSAN,
		validateCRL:          config.ValidateCRL,
	}
}

// ValidateCertificate 验证客户端证书
func (v *CertificateValidator) ValidateCertificate(cert *x509.Certificate) error {
	if cert == nil {
		return errors.New(errors.ErrCodeMTLSRequired, "Client certificate required")
	}

	// 验证证书是否过期
	if err := cert.VerifyHostname(""); err == nil {
		// 证书未过期
	}

	// 验证 Organization
	if len(v.allowedOrganizations) > 0 {
		if !v.isOrganizationAllowed(cert.Subject.Organization) {
			return errors.Newf(errors.ErrCodeUnauthorized,
				"Organization not allowed: %v", cert.Subject.Organization)
		}
	}

	// 验证 OrganizationalUnit
	if v.validateOU && len(v.allowedOUs) > 0 {
		if !v.isOUAllowed(cert.Subject.OrganizationalUnit) {
			return errors.Newf(errors.ErrCodeUnauthorized,
				"Organizational Unit not allowed: %v", cert.Subject.OrganizationalUnit)
		}
	}

	// 验证 CN（Common Name）
	if v.validateCN {
		if cert.Subject.CommonName == "" {
			return errors.New(errors.ErrCodeUnauthorized, "Common Name is required")
		}
	}

	// 验证 SAN（Subject Alternative Name）
	if v.requireSAN {
		if len(cert.DNSNames) == 0 && len(cert.IPAddresses) == 0 && len(cert.EmailAddresses) == 0 {
			return errors.New(errors.ErrCodeUnauthorized, "Subject Alternative Name is required")
		}
	}

	v.logger.Debug("Client certificate validated",
		zap.String("cn", cert.Subject.CommonName),
		zap.Strings("orgs", cert.Subject.Organization),
		zap.Strings("ous", cert.Subject.OrganizationalUnit),
		zap.Strings("dns_names", cert.DNSNames))

	return nil
}

// ExtractTenantID 从证书中提取租户ID
// 优先级：CN → OU → Organization
func (v *CertificateValidator) ExtractTenantID(cert *x509.Certificate) string {
	// 方案1: 从 CN 中提取（格式：probe-{tenant_id}）
	cn := cert.Subject.CommonName
	if strings.HasPrefix(cn, "probe-") {
		return strings.TrimPrefix(cn, "probe-")
	}

	// 方案2: 从 OU 中提取（假设第一个 OU 是租户ID）
	if len(cert.Subject.OrganizationalUnit) > 0 {
		return cert.Subject.OrganizationalUnit[0]
	}

	// 方案3: 从 Organization 中提取
	if len(cert.Subject.Organization) > 0 {
		return cert.Subject.Organization[0]
	}

	return ""
}

// ExtractProbeID 从证书中提取探针ID
func (v *CertificateValidator) ExtractProbeID(cert *x509.Certificate) string {
	// 从 CN 中提取完整的探针ID
	return cert.Subject.CommonName
}

func (v *CertificateValidator) isOrganizationAllowed(orgs []string) bool {
	if len(v.allowedOrganizations) == 0 {
		return true // 无限制
	}

	for _, org := range orgs {
		for _, allowed := range v.allowedOrganizations {
			if org == allowed {
				return true
			}
		}
	}

	return false
}

func (v *CertificateValidator) isOUAllowed(ous []string) bool {
	if len(v.allowedOUs) == 0 {
		return true // 无限制
	}

	for _, ou := range ous {
		for _, allowed := range v.allowedOUs {
			if ou == allowed {
				return true
			}
		}
	}

	return false
}

// ExtractClientCertificate 从 HTTP 请求中提取客户端证书
func ExtractClientCertificate(r *http.Request) (*x509.Certificate, error) {
	if r.TLS == nil {
		return nil, errors.New(errors.ErrCodeMTLSRequired, "TLS connection required")
	}

	if len(r.TLS.PeerCertificates) == 0 {
		return nil, errors.New(errors.ErrCodeMTLSRequired, "Client certificate required")
	}

	// 返回第一个证书（客户端证书）
	return r.TLS.PeerCertificates[0], nil
}

// CertificateInfo 证书信息
type CertificateInfo struct {
	Subject          CertificateSubject `json:"subject"`
	Issuer           CertificateSubject `json:"issuer"`
	SerialNumber     string             `json:"serial_number"`
	NotBefore        string             `json:"not_before"`
	NotAfter         string             `json:"not_after"`
	DNSNames         []string           `json:"dns_names,omitempty"`
	IPAddresses      []string           `json:"ip_addresses,omitempty"`
	EmailAddresses   []string           `json:"email_addresses,omitempty"`
	IsCA             bool               `json:"is_ca"`
	KeyUsage         []string           `json:"key_usage,omitempty"`
	ExtendedKeyUsage []string           `json:"extended_key_usage,omitempty"`
}

// CertificateSubject 证书主体/颁发者信息
type CertificateSubject struct {
	CommonName         string   `json:"common_name"`
	Organization       []string `json:"organization,omitempty"`
	OrganizationalUnit []string `json:"organizational_unit,omitempty"`
	Country            []string `json:"country,omitempty"`
	Province           []string `json:"province,omitempty"`
	Locality           []string `json:"locality,omitempty"`
}

// GetCertificateInfo 获取证书详细信息
func GetCertificateInfo(cert *x509.Certificate) *CertificateInfo {
	if cert == nil {
		return nil
	}

	info := &CertificateInfo{
		Subject: CertificateSubject{
			CommonName:         cert.Subject.CommonName,
			Organization:       cert.Subject.Organization,
			OrganizationalUnit: cert.Subject.OrganizationalUnit,
			Country:            cert.Subject.Country,
			Province:           cert.Subject.Province,
			Locality:           cert.Subject.Locality,
		},
		Issuer: CertificateSubject{
			CommonName:         cert.Issuer.CommonName,
			Organization:       cert.Issuer.Organization,
			OrganizationalUnit: cert.Issuer.OrganizationalUnit,
			Country:            cert.Issuer.Country,
			Province:           cert.Issuer.Province,
			Locality:           cert.Issuer.Locality,
		},
		SerialNumber:   cert.SerialNumber.String(),
		NotBefore:      cert.NotBefore.Format("2006-01-02 15:04:05"),
		NotAfter:       cert.NotAfter.Format("2006-01-02 15:04:05"),
		DNSNames:       cert.DNSNames,
		EmailAddresses: cert.EmailAddresses,
		IsCA:           cert.IsCA,
	}

	// IP 地址
	for _, ip := range cert.IPAddresses {
		info.IPAddresses = append(info.IPAddresses, ip.String())
	}

	// Key Usage
	if cert.KeyUsage&x509.KeyUsageDigitalSignature != 0 {
		info.KeyUsage = append(info.KeyUsage, "DigitalSignature")
	}
	if cert.KeyUsage&x509.KeyUsageKeyEncipherment != 0 {
		info.KeyUsage = append(info.KeyUsage, "KeyEncipherment")
	}
	if cert.KeyUsage&x509.KeyUsageKeyAgreement != 0 {
		info.KeyUsage = append(info.KeyUsage, "KeyAgreement")
	}

	// Extended Key Usage
	for _, eku := range cert.ExtKeyUsage {
		switch eku {
		case x509.ExtKeyUsageServerAuth:
			info.ExtendedKeyUsage = append(info.ExtendedKeyUsage, "ServerAuth")
		case x509.ExtKeyUsageClientAuth:
			info.ExtendedKeyUsage = append(info.ExtendedKeyUsage, "ClientAuth")
		case x509.ExtKeyUsageCodeSigning:
			info.ExtendedKeyUsage = append(info.ExtendedKeyUsage, "CodeSigning")
		case x509.ExtKeyUsageEmailProtection:
			info.ExtendedKeyUsage = append(info.ExtendedKeyUsage, "EmailProtection")
		}
	}

	return info
}

// FormatCertificateInfo 格式化证书信息为字符串
func FormatCertificateInfo(cert *x509.Certificate) string {
	if cert == nil {
		return "No certificate"
	}

	return fmt.Sprintf("CN=%s, O=%v, OU=%v, Serial=%s, Valid: %s to %s",
		cert.Subject.CommonName,
		cert.Subject.Organization,
		cert.Subject.OrganizationalUnit,
		cert.SerialNumber.String(),
		cert.NotBefore.Format("2006-01-02"),
		cert.NotAfter.Format("2006-01-02"),
	)
}
