package pki

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var generationPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// FileSet is one atomically projected Kubernetes Secret generation. ManifestFile
// binds the other four files by digest so a reload can never publish a mixed set.
type FileSet struct {
	CertificateFile  string
	PrivateKeyFile   string
	TrustBundleFile  string
	RevocationFile   string
	ManifestFile     string
	ServerDNSNames   []string
	ClientDNSNames   []string
	MinimumRemaining time.Duration
}

type GenerationManifest struct {
	SchemaVersion     int    `json:"schema_version"`
	Generation        string `json:"generation"`
	CertificateSHA256 string `json:"certificate_sha256"`
	PrivateKeySHA256  string `json:"private_key_sha256"`
	TrustSHA256       string `json:"trust_bundle_sha256"`
	RevocationSHA256  string `json:"revocation_sha256"`
}

type snapshot struct {
	generation string
	digest     string
	config     *tls.Config
}

// Reloader validates a complete candidate before atomically publishing it.
// A failed or interrupted rotation leaves the last valid snapshot serving.
type Reloader struct {
	files   FileSet
	current atomic.Pointer[snapshot]
	mu      sync.Mutex
	seen    map[string]string
	now     func() time.Time
}

func NewReloader(files FileSet) (*Reloader, error) {
	if err := validateFileSet(files); err != nil {
		return nil, err
	}
	files.ServerDNSNames = append([]string(nil), files.ServerDNSNames...)
	files.ClientDNSNames = append([]string(nil), files.ClientDNSNames...)
	r := &Reloader{files: files, seen: make(map[string]string), now: time.Now}
	if _, err := r.Reload(); err != nil {
		return nil, err
	}
	return r, nil
}

func validateFileSet(files FileSet) error {
	paths := map[string]string{
		"certificate": files.CertificateFile, "private key": files.PrivateKeyFile,
		"trust bundle": files.TrustBundleFile, "revocation list": files.RevocationFile,
		"generation manifest": files.ManifestFile,
	}
	for label, path := range paths {
		if strings.TrimSpace(path) == "" {
			return fmt.Errorf("%s file is required", label)
		}
	}
	if len(files.ServerDNSNames) == 0 || len(files.ClientDNSNames) == 0 {
		return errors.New("explicit server and client DNS identities are required")
	}
	for _, name := range append(append([]string(nil), files.ServerDNSNames...), files.ClientDNSNames...) {
		if strings.TrimSpace(name) == "" || strings.Contains(name, "*") {
			return fmt.Errorf("TLS identity must be explicit: %q", name)
		}
	}
	if files.MinimumRemaining <= 0 {
		return errors.New("minimum remaining certificate lifetime must be positive")
	}
	return nil
}

// Reload returns true only when it publishes a different valid generation.
func (r *Reloader) Reload() (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	manifestBytes, err := os.ReadFile(r.files.ManifestFile)
	if err != nil {
		return false, fmt.Errorf("read generation manifest: %w", err)
	}
	manifest, err := parseManifest(manifestBytes)
	if err != nil {
		return false, err
	}
	material, err := r.readBoundMaterial(manifest)
	if err != nil {
		return false, err
	}
	digestBytes := sha256.Sum256(manifestBytes)
	digest := hex.EncodeToString(digestBytes[:])
	if previous, ok := r.seen[manifest.Generation]; ok && previous != digest {
		return false, fmt.Errorf("generation %q was reused with different material", manifest.Generation)
	}
	if current := r.current.Load(); current != nil && current.generation == manifest.Generation && current.digest == digest {
		return false, nil
	}

	config, err := r.buildTLSConfig(material)
	if err != nil {
		return false, fmt.Errorf("validate generation %q: %w", manifest.Generation, err)
	}
	r.current.Store(&snapshot{generation: manifest.Generation, digest: digest, config: config})
	r.seen[manifest.Generation] = digest
	return true, nil
}

type boundMaterial struct {
	certificate []byte
	privateKey  []byte
	trust       []byte
	revocation  []byte
}

func (r *Reloader) readBoundMaterial(manifest GenerationManifest) (boundMaterial, error) {
	read := func(label, path, expected string) ([]byte, error) {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", label, err)
		}
		sum := sha256.Sum256(data)
		if !strings.EqualFold(hex.EncodeToString(sum[:]), expected) {
			return nil, fmt.Errorf("%s digest does not match generation manifest", label)
		}
		return data, nil
	}
	var out boundMaterial
	var err error
	if out.certificate, err = read("certificate", r.files.CertificateFile, manifest.CertificateSHA256); err != nil {
		return out, err
	}
	if out.privateKey, err = read("private key", r.files.PrivateKeyFile, manifest.PrivateKeySHA256); err != nil {
		return out, err
	}
	if out.trust, err = read("trust bundle", r.files.TrustBundleFile, manifest.TrustSHA256); err != nil {
		return out, err
	}
	if out.revocation, err = read("revocation list", r.files.RevocationFile, manifest.RevocationSHA256); err != nil {
		return out, err
	}
	return out, nil
}

func parseManifest(data []byte) (GenerationManifest, error) {
	var manifest GenerationManifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return manifest, fmt.Errorf("decode generation manifest: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return manifest, errors.New("generation manifest contains trailing data")
	}
	if manifest.SchemaVersion != 1 || !generationPattern.MatchString(manifest.Generation) {
		return manifest, errors.New("invalid generation manifest version or generation")
	}
	for label, value := range map[string]string{
		"certificate": manifest.CertificateSHA256, "private key": manifest.PrivateKeySHA256,
		"trust bundle": manifest.TrustSHA256, "revocation": manifest.RevocationSHA256,
	} {
		decoded, err := hex.DecodeString(value)
		if err != nil || len(decoded) != sha256.Size {
			return manifest, fmt.Errorf("invalid %s SHA-256", label)
		}
	}
	return manifest, nil
}

func (r *Reloader) buildTLSConfig(material boundMaterial) (*tls.Config, error) {
	keyPair, err := tls.X509KeyPair(material.certificate, material.privateKey)
	if err != nil {
		return nil, fmt.Errorf("load server key pair: %w", err)
	}
	trustPool, trustCerts, err := parseCertificates(material.trust)
	if err != nil {
		return nil, err
	}
	leaf, intermediates, err := parseLeafAndIntermediates(keyPair)
	if err != nil {
		return nil, err
	}
	now := r.now()
	if leaf.NotAfter.Before(now.Add(r.files.MinimumRemaining)) {
		return nil, fmt.Errorf("server certificate expires before renewal guard: %s", leaf.NotAfter.UTC())
	}
	var serverChain []*x509.Certificate
	for _, name := range r.files.ServerDNSNames {
		chains, err := leaf.Verify(x509.VerifyOptions{
			Roots: trustPool, Intermediates: intermediates, DNSName: name,
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, CurrentTime: now,
		})
		if err != nil {
			return nil, fmt.Errorf("server identity %q: %w", name, err)
		}
		if len(serverChain) == 0 && len(chains) > 0 {
			serverChain = chains[0]
		}
	}
	crls, err := parseRevocationLists(material.revocation, trustCerts, now)
	if err != nil {
		return nil, err
	}
	if len(serverChain) < 2 {
		return nil, errors.New("verified server certificate chain is incomplete")
	}
	serverCRL := findIssuerCRL(crls, serverChain[1], now)
	if serverCRL == nil {
		return nil, errors.New("server certificate issuer has no current revocation list")
	}
	for _, entry := range serverCRL.RevokedCertificateEntries {
		if entry.SerialNumber.Cmp(leaf.SerialNumber) == 0 {
			return nil, errors.New("server certificate is revoked")
		}
	}

	return &tls.Config{
		Certificates: []tls.Certificate{keyPair},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    trustPool,
		MinVersion:   tls.VersionTLS13,
		VerifyConnection: func(state tls.ConnectionState) error {
			if len(state.VerifiedChains) == 0 || len(state.VerifiedChains[0]) < 2 {
				return errors.New("verified client certificate chain is required")
			}
			client := state.VerifiedChains[0][0]
			if !matchesAnyDNSName(client, r.files.ClientDNSNames) {
				return errors.New("client certificate SAN is not an allowed workload identity")
			}
			issuer := state.VerifiedChains[0][1]
			crl := findIssuerCRL(crls, issuer, r.now())
			if crl == nil {
				return errors.New("client certificate issuer has no current revocation list")
			}
			for _, entry := range crl.RevokedCertificateEntries {
				if entry.SerialNumber.Cmp(client.SerialNumber) == 0 {
					return errors.New("client certificate is revoked")
				}
			}
			return nil
		},
	}, nil
}

func parseCertificates(data []byte) (*x509.CertPool, []*x509.Certificate, error) {
	pool := x509.NewCertPool()
	var certs []*x509.Certificate
	for rest := data; len(rest) > 0; {
		block, next := pem.Decode(rest)
		if block == nil {
			break
		}
		rest = next
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, nil, fmt.Errorf("parse trust certificate: %w", err)
		}
		pool.AddCert(cert)
		certs = append(certs, cert)
	}
	if len(certs) == 0 {
		return nil, nil, errors.New("trust bundle contains no certificates")
	}
	return pool, certs, nil
}

func parseLeafAndIntermediates(pair tls.Certificate) (*x509.Certificate, *x509.CertPool, error) {
	if len(pair.Certificate) == 0 {
		return nil, nil, errors.New("server certificate chain is empty")
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return nil, nil, fmt.Errorf("parse server leaf: %w", err)
	}
	intermediates := x509.NewCertPool()
	for _, raw := range pair.Certificate[1:] {
		cert, err := x509.ParseCertificate(raw)
		if err != nil {
			return nil, nil, fmt.Errorf("parse server intermediate: %w", err)
		}
		intermediates.AddCert(cert)
	}
	return leaf, intermediates, nil
}

func parseRevocationLists(data []byte, issuers []*x509.Certificate, now time.Time) ([]*x509.RevocationList, error) {
	var lists []*x509.RevocationList
	for rest := data; len(rest) > 0; {
		block, next := pem.Decode(rest)
		if block == nil {
			break
		}
		rest = next
		if block.Type != "X509 CRL" && block.Type != "CRL" {
			continue
		}
		list, err := x509.ParseRevocationList(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse revocation list: %w", err)
		}
		if now.Before(list.ThisUpdate) || !now.Before(list.NextUpdate) {
			return nil, errors.New("revocation list is not current")
		}
		validSigner := false
		for _, issuer := range issuers {
			if list.CheckSignatureFrom(issuer) == nil {
				validSigner = true
				break
			}
		}
		if !validSigner {
			return nil, errors.New("revocation list is not signed by the trust bundle")
		}
		lists = append(lists, list)
	}
	if len(lists) == 0 {
		return nil, errors.New("at least one signed revocation list is required")
	}
	return lists, nil
}

func matchesAnyDNSName(cert *x509.Certificate, names []string) bool {
	for _, name := range names {
		if cert.VerifyHostname(name) == nil {
			return true
		}
	}
	return false
}

func findIssuerCRL(lists []*x509.RevocationList, issuer *x509.Certificate, now time.Time) *x509.RevocationList {
	for _, list := range lists {
		if now.Before(list.ThisUpdate) || !now.Before(list.NextUpdate) {
			continue
		}
		if list.CheckSignatureFrom(issuer) == nil {
			return list
		}
	}
	return nil
}

func (r *Reloader) Generation() string {
	if current := r.current.Load(); current != nil {
		return current.generation
	}
	return ""
}

// ServerTLSConfig delegates each new handshake to the latest complete snapshot.
func (r *Reloader) ServerTLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS13,
		GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
			current := r.current.Load()
			if current == nil {
				return nil, errors.New("no validated PKI generation is available")
			}
			return current.config, nil
		},
	}
}

// Run periodically attempts reloads until ctx is cancelled. Invalid candidates
// are reported and ignored; the last valid generation remains active.
func (r *Reloader) Run(ctx context.Context, interval time.Duration, report func(error)) {
	if interval <= 0 {
		panic("PKI reload interval must be positive")
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := r.Reload(); err != nil && report != nil {
				report(err)
			}
		}
	}
}
