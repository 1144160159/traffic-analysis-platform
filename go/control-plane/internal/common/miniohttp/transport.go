package miniohttp

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// NewTransport returns a MinIO-scoped transport. Internal MinIO TLS must use
// the governed private CA. A nil transport preserves the current plaintext
// client behavior until the atomic cutover bundle is explicitly activated.
func NewTransport(secure bool, caFile string) (http.RoundTripper, error) {
	caFile = strings.TrimSpace(caFile)
	if !secure {
		if caFile != "" {
			return nil, fmt.Errorf("MinIO CA file cannot be configured while TLS is disabled")
		}
		return nil, nil
	}
	if caFile == "" {
		return nil, fmt.Errorf("MinIO TLS requires S3_CA_CERT")
	}

	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("read MinIO CA file: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("MinIO CA file contains no valid PEM certificate")
	}

	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("default HTTP transport has unexpected type")
	}
	transport := base.Clone()
	transport.TLSClientConfig = &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    roots,
	}
	return transport, nil
}
