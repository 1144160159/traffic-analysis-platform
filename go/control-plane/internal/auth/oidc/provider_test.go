package oidc

import "testing"

func TestNormalizeScopes(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{name: "comma separated", input: "openid,profile,email", expect: "openid profile email"},
		{name: "space separated", input: "openid profile email", expect: "openid profile email"},
		{name: "mixed whitespace", input: " openid, profile  email ", expect: "openid profile email"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeScopes(tt.input); got != tt.expect {
				t.Fatalf("expected %q, got %q", tt.expect, got)
			}
		})
	}
}
