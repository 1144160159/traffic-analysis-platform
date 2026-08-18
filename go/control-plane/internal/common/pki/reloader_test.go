package pki

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type testIdentity struct {
	cert    *x509.Certificate
	key     *ecdsa.PrivateKey
	certPEM []byte
	keyPEM  []byte
}

func TestReloaderRejectsWrongCAExpiredAndServerSANMismatch(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	rootA := newTestCA(t, "root-a", now)
	rootB := newTestCA(t, "root-b", now)
	validServer := newTestLeaf(t, rootA, "server", []string{"ingest-gateway"}, false, now.Add(-time.Hour), now.Add(48*time.Hour))
	expiredServer := newTestLeaf(t, rootA, "expired", []string{"ingest-gateway"}, false, now.Add(-48*time.Hour), now.Add(-time.Hour))
	crlA := newTestCRL(t, rootA, now)

	cases := []struct {
		name   string
		server testIdentity
		trust  []byte
		crl    []byte
		want   string
	}{
		{name: "wrong-ca", server: validServer, trust: rootB.certPEM, crl: crlA, want: "unknown authority"},
		{name: "expired", server: expiredServer, trust: rootA.certPEM, crl: crlA, want: "expires before renewal guard"},
		{name: "san-mismatch", server: validServer, trust: rootA.certPEM, crl: crlA, want: "server identity"},
		{name: "server-revoked", server: validServer, trust: rootA.certPEM, crl: newTestCRL(t, rootA, now, validServer.cert.SerialNumber), want: "server certificate is revoked"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			names := []string{"ingest-gateway"}
			if tc.name == "san-mismatch" {
				names = []string{"other-gateway"}
			}
			files := writeGeneration(t, t.TempDir(), "g1", tc.server, tc.trust, tc.crl, names)
			_, err := NewReloader(files)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("NewReloader() error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestReloaderRejectsClientSANAndRevokedSerial(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	root := newTestCA(t, "root", now)
	server := newTestLeaf(t, root, "server", []string{"ingest-gateway"}, false, now.Add(-time.Hour), now.Add(48*time.Hour))
	allowed := newTestLeaf(t, root, "allowed", []string{"probe-agent"}, true, now.Add(-time.Hour), now.Add(24*time.Hour))
	wrongSAN := newTestLeaf(t, root, "wrong-san", []string{"other-agent"}, true, now.Add(-time.Hour), now.Add(24*time.Hour))

	files := writeGeneration(t, t.TempDir(), "g1", server, root.certPEM, newTestCRL(t, root, now), []string{"ingest-gateway"})
	reloader, err := NewReloader(files)
	if err != nil {
		t.Fatal(err)
	}
	if err := handshake(reloader, root.certPEM, allowed); err != nil {
		t.Fatalf("allowed client handshake failed: %v", err)
	}
	if err := handshake(reloader, root.certPEM, wrongSAN); err == nil {
		t.Fatal("client with unapproved SAN completed handshake")
	}

	writeGenerationAtPaths(t, files, "g2", server, root.certPEM, newTestCRL(t, root, now, allowed.cert.SerialNumber))
	if changed, err := reloader.Reload(); err != nil || !changed {
		t.Fatalf("reload revoked generation = (%v, %v), want (true, nil)", changed, err)
	}
	if err := handshake(reloader, root.certPEM, allowed); err == nil {
		t.Fatal("revoked client completed handshake")
	}
}

func TestInterruptedRotationKeepsLastValidSnapshot(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	root := newTestCA(t, "root", now)
	server := newTestLeaf(t, root, "server", []string{"ingest-gateway"}, false, now.Add(-time.Hour), now.Add(48*time.Hour))
	client := newTestLeaf(t, root, "client", []string{"probe-agent"}, true, now.Add(-time.Hour), now.Add(24*time.Hour))
	files := writeGeneration(t, t.TempDir(), "stable", server, root.certPEM, newTestCRL(t, root, now), []string{"ingest-gateway"})
	reloader, err := NewReloader(files)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(files.CertificateFile, []byte("interrupted projection"), 0o600); err != nil {
		t.Fatal(err)
	}
	if changed, err := reloader.Reload(); err == nil || changed || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("interrupted reload = (%v, %v), want digest failure without publish", changed, err)
	}
	if got := reloader.Generation(); got != "stable" {
		t.Fatalf("generation after failed reload = %q, want stable", got)
	}
	if err := handshake(reloader, root.certPEM, client); err != nil {
		t.Fatalf("last valid snapshot stopped serving: %v", err)
	}
}

func TestDualTrustRotationThenOldIssuerRetirement(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	rootA := newTestCA(t, "root-a", now)
	rootB := newTestCA(t, "root-b", now)
	serverA := newTestLeaf(t, rootA, "server-a", []string{"ingest-gateway"}, false, now.Add(-time.Hour), now.Add(48*time.Hour))
	serverB := newTestLeaf(t, rootB, "server-b", []string{"ingest-gateway"}, false, now.Add(-time.Hour), now.Add(48*time.Hour))
	clientA := newTestLeaf(t, rootA, "client-a", []string{"probe-agent"}, true, now.Add(-time.Hour), now.Add(24*time.Hour))
	clientB := newTestLeaf(t, rootB, "client-b", []string{"probe-agent"}, true, now.Add(-time.Hour), now.Add(24*time.Hour))
	dir := t.TempDir()
	files := writeGeneration(t, dir, "issuer-a", serverA, rootA.certPEM, newTestCRL(t, rootA, now), []string{"ingest-gateway"})
	reloader, err := NewReloader(files)
	if err != nil {
		t.Fatal(err)
	}

	dualTrust := append(append([]byte(nil), rootA.certPEM...), rootB.certPEM...)
	dualCRL := append(newTestCRL(t, rootA, now), newTestCRL(t, rootB, now)...)
	writeGenerationAtPaths(t, files, "dual-trust", serverB, dualTrust, dualCRL)
	if changed, err := reloader.Reload(); err != nil || !changed {
		t.Fatalf("dual trust reload = (%v, %v)", changed, err)
	}
	if err := handshake(reloader, rootB.certPEM, clientA); err != nil {
		t.Fatalf("old client failed during dual trust: %v", err)
	}
	if err := handshake(reloader, rootB.certPEM, clientB); err != nil {
		t.Fatalf("new client failed during dual trust: %v", err)
	}

	writeGenerationAtPaths(t, files, "issuer-b", serverB, rootB.certPEM, newTestCRL(t, rootB, now))
	if changed, err := reloader.Reload(); err != nil || !changed {
		t.Fatalf("issuer retirement reload = (%v, %v)", changed, err)
	}
	if err := handshake(reloader, rootB.certPEM, clientA); err == nil {
		t.Fatal("old issuer remained trusted after retirement")
	}
	if err := handshake(reloader, rootB.certPEM, clientB); err != nil {
		t.Fatalf("new issuer client failed after promotion: %v", err)
	}
}

func newTestCA(t *testing.T, commonName string, now time.Time) testIdentity {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: commonName},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(365 * 24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return testIdentity{cert: cert, key: key, certPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}
}

func newTestLeaf(t *testing.T, issuer testIdentity, commonName string, dns []string, client bool, notBefore, notAfter time.Time) testIdentity {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		t.Fatal(err)
	}
	usage := x509.ExtKeyUsageServerAuth
	if client {
		usage = x509.ExtKeyUsageClientAuth
	}
	template := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: commonName}, DNSNames: dns,
		NotBefore: notBefore, NotAfter: notAfter, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, issuer.cert, &key.PublicKey, issuer.key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return testIdentity{
		cert: cert, key: key,
		certPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		keyPEM:  pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}),
	}
}

func newTestCRL(t *testing.T, issuer testIdentity, now time.Time, revoked ...*big.Int) []byte {
	t.Helper()
	entries := make([]x509.RevocationListEntry, 0, len(revoked))
	for _, serial := range revoked {
		entries = append(entries, x509.RevocationListEntry{SerialNumber: serial, RevocationTime: now.Add(-time.Minute)})
	}
	der, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
		Number: big.NewInt(now.UnixNano()), ThisUpdate: now.Add(-time.Minute),
		NextUpdate: now.Add(24 * time.Hour), RevokedCertificateEntries: entries,
	}, issuer.cert, issuer.key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: der})
}

func writeGeneration(t *testing.T, dir, generation string, server testIdentity, trust, crl []byte, serverNames []string) FileSet {
	t.Helper()
	files := FileSet{
		CertificateFile: filepath.Join(dir, "tls.crt"), PrivateKeyFile: filepath.Join(dir, "tls.key"),
		TrustBundleFile: filepath.Join(dir, "ca.crt"), RevocationFile: filepath.Join(dir, "client.crl.pem"),
		ManifestFile: filepath.Join(dir, "generation.json"), ServerDNSNames: serverNames,
		ClientDNSNames: []string{"probe-agent"}, MinimumRemaining: time.Hour,
	}
	writeGenerationAtPaths(t, files, generation, server, trust, crl)
	return files
}

func writeGenerationAtPaths(t *testing.T, files FileSet, generation string, server testIdentity, trust, crl []byte) {
	t.Helper()
	write := func(path string, data []byte, mode os.FileMode) string {
		if err := os.WriteFile(path, data, mode); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		return hex.EncodeToString(sum[:])
	}
	manifest := GenerationManifest{SchemaVersion: 1, Generation: generation}
	manifest.CertificateSHA256 = write(files.CertificateFile, server.certPEM, 0o600)
	manifest.PrivateKeySHA256 = write(files.PrivateKeyFile, server.keyPEM, 0o600)
	manifest.TrustSHA256 = write(files.TrustBundleFile, trust, 0o600)
	manifest.RevocationSHA256 = write(files.RevocationFile, crl, 0o600)
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.ManifestFile, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func handshake(reloader *Reloader, serverRoots []byte, client testIdentity) error {
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(serverRoots) {
		return &x509.UnknownAuthorityError{}
	}
	clientPair, err := tls.X509KeyPair(client.certPEM, client.keyPEM)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()
	serverResult := make(chan error, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			serverResult <- acceptErr
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		serverResult <- tls.Server(conn, reloader.ServerTLSConfig()).Handshake()
	}()
	clientConn, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err != nil {
		return err
	}
	defer clientConn.Close()
	_ = clientConn.SetDeadline(time.Now().Add(3 * time.Second))
	clientTLS := tls.Client(clientConn, &tls.Config{
		RootCAs: roots, Certificates: []tls.Certificate{clientPair}, ServerName: "ingest-gateway", MinVersion: tls.VersionTLS13,
	})
	clientErr := clientTLS.Handshake()
	serverErr := <-serverResult
	if clientErr != nil {
		return clientErr
	}
	return serverErr
}
