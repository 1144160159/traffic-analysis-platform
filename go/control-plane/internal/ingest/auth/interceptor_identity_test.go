package auth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"strings"
	"testing"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

type testProbeIDCarrier struct{ probeID string }

func (request testProbeIDCarrier) GetProbeId() string { return request.probeID }

func mtlsIdentityContext(commonName, requestedProbeID string) context.Context {
	ctx := context.Background()
	if requestedProbeID != "" {
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs("x-probe-id", requestedProbeID))
	}
	certificate := &x509.Certificate{Subject: pkix.Name{CommonName: commonName}}
	return peer.NewContext(ctx, &peer.Peer{AuthInfo: credentials.TLSInfo{
		State: tls.ConnectionState{VerifiedChains: [][]*x509.Certificate{{certificate}}},
	}})
}

func TestExtractProbeIdentityUsesCertificateByDefault(t *testing.T) {
	interceptor := &Interceptor{config: InterceptorConfig{RequireMTLS: true}}
	identity, err := interceptor.extractProbeIdentity(mtlsIdentityContext("probe-node-a", "probe-node-a"))
	if err != nil {
		t.Fatalf("extractProbeIdentity() error = %v", err)
	}
	if identity.TransportID != "probe-node-a" || identity.EffectiveID != "probe-node-a" {
		t.Fatalf("unexpected identity: %#v", identity)
	}
}

func TestExtractProbeIdentityRejectsUnconfiguredDelegation(t *testing.T) {
	interceptor := &Interceptor{config: InterceptorConfig{RequireMTLS: true}}
	identity, err := interceptor.extractProbeIdentity(mtlsIdentityContext("probe-agent", "probe-node-a"))
	if err == nil || !strings.Contains(err.Error(), "not authorized to delegate") {
		t.Fatalf("expected delegation rejection, identity=%#v err=%v", identity, err)
	}
}

func TestExtractProbeIdentityAllowsConfiguredSharedWorkloadIdentity(t *testing.T) {
	interceptor := &Interceptor{config: InterceptorConfig{
		RequireMTLS: true, SharedMTLSIdentity: "probe-agent",
	}}
	identity, err := interceptor.extractProbeIdentity(mtlsIdentityContext("probe-agent", "probe-node-a"))
	if err != nil {
		t.Fatalf("extractProbeIdentity() error = %v", err)
	}
	if identity.TransportID != "probe-agent" || identity.EffectiveID != "probe-node-a" {
		t.Fatalf("unexpected delegated identity: %#v", identity)
	}
}

func TestExtractProbeIdentityRejectsInvalidDelegatedIdentity(t *testing.T) {
	interceptor := &Interceptor{config: InterceptorConfig{
		RequireMTLS: true, SharedMTLSIdentity: "probe-agent",
	}}
	_, err := interceptor.extractProbeIdentity(mtlsIdentityContext("probe-agent", "probe node a"))
	if err == nil || !strings.Contains(err.Error(), "invalid delegated probe-id") {
		t.Fatalf("expected invalid identity rejection, got %v", err)
	}
}

func TestBindEffectiveProbeIdentityDoesNotMutateCachedToken(t *testing.T) {
	original := &TokenInfo{TenantID: "default", ProbeID: "probe-agent", Scopes: []string{"ingest:write"}}
	bound := bindEffectiveProbeIdentity(original, "probe-node-a")
	bound.Scopes[0] = "changed"
	if original.ProbeID != "probe-agent" || original.Scopes[0] != "ingest:write" {
		t.Fatalf("cached token was mutated: %#v", original)
	}
	if bound.ProbeID != "probe-node-a" {
		t.Fatalf("effective identity not bound: %#v", bound)
	}
}

func TestExtractProbeIdentityWithoutMTLSUsesValidatedMetadata(t *testing.T) {
	interceptor := &Interceptor{}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-probe-id", "probe-node-a"))
	identity, err := interceptor.extractProbeIdentity(ctx)
	if err != nil {
		t.Fatalf("extractProbeIdentity() error = %v", err)
	}
	if identity.TransportID != "probe-node-a" || identity.EffectiveID != "probe-node-a" {
		t.Fatalf("unexpected identity: %#v", identity)
	}
}

func TestSharedIdentityCanUseProbeIDFromLegacyUnaryRequest(t *testing.T) {
	interceptor := &Interceptor{config: InterceptorConfig{
		RequireMTLS: true, SharedMTLSIdentity: "probe-agent",
	}}
	ctx := mtlsIdentityContext("probe-agent", "")
	ctx = interceptor.withRequestedProbeIdentity(ctx, testProbeIDCarrier{probeID: "probe-node-a"})
	identity, err := interceptor.extractProbeIdentity(ctx)
	if err != nil {
		t.Fatalf("extractProbeIdentity() error = %v", err)
	}
	if identity.TransportID != "probe-agent" || identity.EffectiveID != "probe-node-a" {
		t.Fatalf("unexpected legacy request identity: %#v", identity)
	}
}

func TestLegacyRequestCannotOverrideExplicitMetadata(t *testing.T) {
	interceptor := &Interceptor{config: InterceptorConfig{
		RequireMTLS: true, SharedMTLSIdentity: "probe-agent",
	}}
	ctx := mtlsIdentityContext("probe-agent", "probe-explicit")
	ctx = interceptor.withRequestedProbeIdentity(ctx, testProbeIDCarrier{probeID: "probe-body"})
	identity, err := interceptor.extractProbeIdentity(ctx)
	if err != nil {
		t.Fatalf("extractProbeIdentity() error = %v", err)
	}
	if identity.EffectiveID != "probe-explicit" {
		t.Fatalf("request body overrode explicit metadata: %#v", identity)
	}
}
