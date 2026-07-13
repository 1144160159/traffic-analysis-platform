package kafka

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl"
	"github.com/segmentio/kafka-go/sasl/plain"
	"github.com/segmentio/kafka-go/sasl/scram"
)

type SecurityConfig struct {
	SecurityProtocol string `env:"KAFKA_SECURITY_PROTOCOL"`
	SASLMechanism    string `env:"KAFKA_SASL_MECHANISM" envDefault:"SCRAM-SHA-512"`
	SASLUsername     string `env:"KAFKA_SASL_USERNAME"`
	SASLPassword     string `env:"KAFKA_SASL_PASSWORD"`
	TLSCAFile        string `env:"KAFKA_TLS_CA_FILE"`
	TLSServerName    string `env:"KAFKA_TLS_SERVER_NAME"`
	TLSSkipVerify    bool   `env:"KAFKA_TLS_SKIP_VERIFY" envDefault:"false"`
	ClientID         string `env:"KAFKA_CLIENT_ID"`
}

func (c SecurityConfig) Enabled() bool {
	protocol := c.normalizedProtocol()
	return protocol == "SSL" ||
		protocol == "SASL_SSL" ||
		protocol == "SASL_PLAINTEXT" ||
		c.SASLUsername != "" ||
		c.SASLPassword != "" ||
		c.TLSCAFile != "" ||
		c.TLSServerName != "" ||
		c.TLSSkipVerify
}

func (c SecurityConfig) Dialer(defaultClientID string) (*kafka.Dialer, error) {
	if err := c.validateProtocol(); err != nil {
		return nil, err
	}
	if !c.Enabled() {
		return nil, nil
	}

	mechanism, err := c.saslMechanism()
	if err != nil {
		return nil, err
	}

	tlsConfig, err := c.tlsConfig()
	if err != nil {
		return nil, err
	}

	clientID := strings.TrimSpace(c.ClientID)
	if clientID == "" {
		clientID = defaultClientID
	}

	return &kafka.Dialer{
		Timeout:       10 * time.Second,
		DualStack:     true,
		ClientID:      clientID,
		TLS:           tlsConfig,
		SASLMechanism: mechanism,
	}, nil
}

func (c SecurityConfig) validateProtocol() error {
	switch c.normalizedProtocol() {
	case "", "PLAINTEXT", "SSL", "SASL_SSL", "SASL_PLAINTEXT":
		return nil
	default:
		return fmt.Errorf("unsupported kafka security protocol %q", c.SecurityProtocol)
	}
}

func (c SecurityConfig) normalizedProtocol() string {
	return strings.ToUpper(strings.TrimSpace(c.SecurityProtocol))
}

func (c SecurityConfig) saslMechanism() (sasl.Mechanism, error) {
	protocol := c.normalizedProtocol()
	if protocol != "SASL_SSL" && protocol != "SASL_PLAINTEXT" && c.SASLUsername == "" && c.SASLPassword == "" {
		return nil, nil
	}
	if c.SASLUsername == "" || c.SASLPassword == "" {
		return nil, fmt.Errorf("kafka SASL username and password must both be configured")
	}

	switch strings.ToUpper(strings.TrimSpace(c.SASLMechanism)) {
	case "", "SCRAM-SHA-512":
		return scram.Mechanism(scram.SHA512, c.SASLUsername, c.SASLPassword)
	case "SCRAM-SHA-256":
		return scram.Mechanism(scram.SHA256, c.SASLUsername, c.SASLPassword)
	case "PLAIN":
		return plain.Mechanism{Username: c.SASLUsername, Password: c.SASLPassword}, nil
	default:
		return nil, fmt.Errorf("unsupported kafka SASL mechanism %q", c.SASLMechanism)
	}
}

func (c SecurityConfig) tlsConfig() (*tls.Config, error) {
	protocol := c.normalizedProtocol()
	if protocol != "SSL" && protocol != "SASL_SSL" && c.TLSCAFile == "" && c.TLSServerName == "" && !c.TLSSkipVerify {
		return nil, nil
	}

	cfg := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		ServerName:         strings.TrimSpace(c.TLSServerName),
		InsecureSkipVerify: c.TLSSkipVerify, //nolint:gosec // Explicit opt-in for isolated smoke environments only.
	}
	if c.TLSCAFile == "" {
		return cfg, nil
	}

	pemData, err := os.ReadFile(c.TLSCAFile)
	if err != nil {
		return nil, fmt.Errorf("read kafka TLS CA file: %w", err)
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(pemData) {
		return nil, fmt.Errorf("kafka TLS CA file %s does not contain PEM certificates", c.TLSCAFile)
	}
	cfg.RootCAs = pool
	return cfg, nil
}
