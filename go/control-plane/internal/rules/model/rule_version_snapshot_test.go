package model

import (
	"crypto/md5" // #nosec G501 -- compatibility fixture for historical rows
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

func TestRuleVersionSnapshotRoundTripAndIntegrity(t *testing.T) {
	rule := snapshotTestRule()
	contentURI, checksum, err := EncodeRuleVersionSnapshot(rule)
	if err != nil {
		t.Fatalf("EncodeRuleVersionSnapshot() error = %v", err)
	}
	if !strings.HasPrefix(contentURI, RuleVersionContentURIPrefix) {
		t.Fatalf("content URI %q is not inline", contentURI)
	}
	if len(checksum) != len(RuleVersionChecksumPrefix)+64 || !strings.HasPrefix(checksum, RuleVersionChecksumPrefix) {
		t.Fatalf("checksum %q is not tagged SHA-256", checksum)
	}

	decoded, err := DecodeRuleVersionSnapshot(&RuleVersion{
		RuleVersionID: "rule-1-v2",
		RuleID:        rule.RuleID,
		TenantID:      rule.TenantID,
		Version:       rule.Version,
		ContentURI:    contentURI,
		Checksum:      checksum,
	})
	if err != nil {
		t.Fatalf("DecodeRuleVersionSnapshot() error = %v", err)
	}
	if decoded.RuleID != rule.RuleID || decoded.Version != rule.Version || decoded.Name != rule.Name {
		t.Fatalf("decoded snapshot = %#v", decoded)
	}

	tampered := contentURI + " "
	_, err = DecodeRuleVersionSnapshot(&RuleVersion{
		RuleVersionID: "rule-1-v2",
		ContentURI:    tampered,
		Checksum:      checksum,
	})
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("tampered snapshot error = %v, want checksum mismatch", err)
	}
}

func TestRuleVersionSnapshotReadsLegacyMD5ButRejectsMissingChecksum(t *testing.T) {
	rule := snapshotTestRule()
	contentURI, _, err := EncodeRuleVersionSnapshot(rule)
	if err != nil {
		t.Fatal(err)
	}
	content := []byte(strings.TrimPrefix(contentURI, RuleVersionContentURIPrefix))
	legacy := md5.Sum(content) // #nosec G401 -- compatibility fixture only
	version := &RuleVersion{
		RuleVersionID: "rule-1-v2",
		ContentURI:    contentURI,
		Checksum:      hex.EncodeToString(legacy[:]),
	}
	if _, err := DecodeRuleVersionSnapshot(version); err != nil {
		t.Fatalf("legacy checksum should remain readable: %v", err)
	}
	version.Checksum = ""
	if _, err := DecodeRuleVersionSnapshot(version); err == nil || !strings.Contains(err.Error(), "checksum is missing") {
		t.Fatalf("missing checksum error = %v", err)
	}
}

func snapshotTestRule() *Rule {
	now := time.Date(2026, 8, 14, 15, 0, 0, 0, time.UTC)
	return &Rule{
		RuleID:     "rule-1",
		TenantID:   "tenant-1",
		Name:       "blocked_dns",
		Type:       string(RuleTypeBlacklist),
		Engine:     string(EngineInternal),
		Conditions: map[string]interface{}{"domain": "blocked.example"},
		Severity:   string(SeverityHigh),
		Enabled:    true,
		Priority:   75,
		Version:    2,
		Status:     string(RuleStatusActive),
		CreatedBy:  "creator",
		UpdatedBy:  "editor",
		CreatedAt:  now.Add(-time.Hour),
		UpdatedAt:  now,
	}
}
