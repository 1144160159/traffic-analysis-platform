package miniohttp

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewTransportPlaintext(t *testing.T) {
	transport, err := NewTransport(false, "")
	if err != nil || transport != nil {
		t.Fatalf("expected nil plaintext transport, got transport=%v err=%v", transport, err)
	}
	if _, err := NewTransport(false, "/tmp/ca.crt"); err == nil {
		t.Fatal("expected plaintext plus CA to fail")
	}
}

func TestNewTransportTLSFailsClosed(t *testing.T) {
	if _, err := NewTransport(true, ""); err == nil {
		t.Fatal("expected TLS without CA to fail")
	}
	path := filepath.Join(t.TempDir(), "ca.crt")
	if err := os.WriteFile(path, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewTransport(true, path); err == nil {
		t.Fatal("expected invalid CA to fail")
	}
}

func TestNewTransportTLSUsesPrivateRootsAndTLS12(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test-ca"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "ca.crt")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}

	roundTripper, err := NewTransport(true, path)
	if err != nil {
		t.Fatal(err)
	}
	transport, ok := roundTripper.(*http.Transport)
	if !ok {
		t.Fatalf("unexpected transport type %T", roundTripper)
	}
	if transport.TLSClientConfig == nil || transport.TLSClientConfig.RootCAs == nil {
		t.Fatal("expected private root pool")
	}
	if transport.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("minimum TLS version=%d", transport.TLSClientConfig.MinVersion)
	}
}
