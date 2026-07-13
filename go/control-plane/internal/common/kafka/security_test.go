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
