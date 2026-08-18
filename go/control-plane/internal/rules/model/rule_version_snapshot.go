package model

import (
	"crypto/md5" // #nosec G501 -- read-only compatibility for historical version rows
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	RuleVersionContentURIPrefix = "inline:"
	RuleVersionChecksumPrefix   = "sha256:"
)

// EncodeRuleVersionSnapshot serializes the immutable version payload and
// returns the content URI and checksum persisted in rule_versions. New
// snapshots always use an explicitly tagged SHA-256 checksum.
func EncodeRuleVersionSnapshot(rule *Rule) (string, string, error) {
	if rule == nil {
		return "", "", fmt.Errorf("rule version snapshot is nil")
	}
	content, err := json.Marshal(rule)
	if err != nil {
		return "", "", fmt.Errorf("marshal rule version snapshot: %w", err)
	}
	sum := sha256.Sum256(content)
	return RuleVersionContentURIPrefix + string(content), RuleVersionChecksumPrefix + hex.EncodeToString(sum[:]), nil
}

// DecodeRuleVersionSnapshot verifies the bytes before decoding them. Untagged
// 32-character MD5 checksums remain readable for versions created by the
// previous repository path; empty or unknown checksum formats fail closed.
func DecodeRuleVersionSnapshot(version *RuleVersion) (*Rule, error) {
	if version == nil {
		return nil, fmt.Errorf("rule version is nil")
	}
	if !strings.HasPrefix(version.ContentURI, RuleVersionContentURIPrefix) {
		return nil, fmt.Errorf("rule version %s does not contain an inline snapshot", version.RuleVersionID)
	}
	content := []byte(strings.TrimPrefix(version.ContentURI, RuleVersionContentURIPrefix))
	if len(content) == 0 {
		return nil, fmt.Errorf("rule version %s snapshot is empty", version.RuleVersionID)
	}
	if err := verifyRuleVersionChecksum(content, strings.TrimSpace(version.Checksum)); err != nil {
		return nil, fmt.Errorf("rule version %s: %w", version.RuleVersionID, err)
	}

	var rule Rule
	if err := json.Unmarshal(content, &rule); err != nil {
		return nil, fmt.Errorf("decode rule version snapshot: %w", err)
	}
	return &rule, nil
}

func verifyRuleVersionChecksum(content []byte, checksum string) error {
	if checksum == "" {
		return fmt.Errorf("snapshot checksum is missing")
	}

	var actual string
	switch {
	case strings.HasPrefix(checksum, RuleVersionChecksumPrefix):
		expected := strings.TrimPrefix(checksum, RuleVersionChecksumPrefix)
		if len(expected) != sha256.Size*2 || expected != strings.ToLower(expected) {
			return fmt.Errorf("snapshot SHA-256 checksum is malformed")
		}
		sum := sha256.Sum256(content)
		actual = RuleVersionChecksumPrefix + hex.EncodeToString(sum[:])
	case len(checksum) == md5.Size*2 && checksum == strings.ToLower(checksum):
		// Historical rows stored an untagged MD5 checksum. It is accepted only
		// for compatibility; every newly written snapshot uses SHA-256 above.
		sum := md5.Sum(content) // #nosec G401 -- compatibility verification only
		actual = hex.EncodeToString(sum[:])
	default:
		return fmt.Errorf("snapshot checksum algorithm is unsupported")
	}

	if subtle.ConstantTimeCompare([]byte(actual), []byte(checksum)) != 1 {
		return fmt.Errorf("snapshot checksum mismatch")
	}
	return nil
}
