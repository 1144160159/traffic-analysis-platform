package kafka

import (
	"testing"
)

func TestSecurityDialerRejectsUnknownProtocol(t *testing.T) {
	dialer, err := (SecurityConfig{SecurityProtocol: "SASL_SS"}).Dialer("test-client")
	if err == nil {
		t.Fatal("Dialer() expected an error for an unknown security protocol")
	}
	if dialer != nil {
		t.Fatal("Dialer() returned a dialer for an unknown security protocol")
	}
}

func TestSecurityDialerAllowsExplicitPlaintext(t *testing.T) {
	dialer, err := (SecurityConfig{SecurityProtocol: "PLAINTEXT"}).Dialer("test-client")
	if err != nil {
		t.Fatalf("Dialer() error = %v", err)
	}
	if dialer != nil {
		t.Fatal("Dialer() should use kafka-go defaults for explicit PLAINTEXT")
	}
}

func TestSecurityTransportRejectsUnknownProtocol(t *testing.T) {
	transport, err := (SecurityConfig{SecurityProtocol: "SASL_SS"}).Transport("test-client")
	if err == nil {
		t.Fatal("Transport() expected an error for an unknown security protocol")
	}
	if transport != nil {
		t.Fatal("Transport() returned a transport for an unknown security protocol")
	}
}

func TestSecurityTransportPreservesClientIDForPlaintext(t *testing.T) {
	transport, err := (SecurityConfig{SecurityProtocol: "PLAINTEXT", ClientID: "dq-offset-reader"}).Transport("fallback")
	if err != nil {
		t.Fatalf("Transport() error = %v", err)
	}
	defer transport.CloseIdleConnections()
	if transport.ClientID != "dq-offset-reader" {
		t.Fatalf("ClientID = %q, want dq-offset-reader", transport.ClientID)
	}
	if transport.TLS != nil || transport.SASL != nil {
		t.Fatal("PLAINTEXT transport unexpectedly enabled TLS or SASL")
	}
}
