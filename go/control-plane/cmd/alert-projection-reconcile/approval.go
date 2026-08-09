package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"
)

var (
	sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
	imagePattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9./_:-]*@sha256:[0-9a-f]{64}$`)
)

type repairReviewPackage struct {
	SchemaVersion       int                    `json:"schema_version"`
	Mode                string                 `json:"mode"`
	ExecutionAuthorized bool                   `json:"execution_authorized"`
	ProductionApplied   bool                   `json:"production_applied"`
	ProductionMutations []string               `json:"production_mutations"`
	Bindings            map[string]interface{} `json:"bindings"`
	ProposedExecution   struct {
		Argv []string `json:"argv"`
	} `json:"proposed_execution"`
}

type repairApproval struct {
	Status     string `json:"status"`
	ApprovedBy string `json:"approved_by"`
	ApprovedAt string `json:"approved_at"`
}

type repairApprovalBundle struct {
	SchemaVersion       int                       `json:"schema_version"`
	Mode                string                    `json:"mode"`
	ExecutionAuthorized bool                      `json:"execution_authorized"`
	ReviewSHA256        string                    `json:"review_package_sha256"`
	Image               string                    `json:"immutable_tool_image"`
	ApprovalNonce       string                    `json:"approval_nonce"`
	NotBefore           string                    `json:"not_before"`
	ExpiresAt           string                    `json:"expires_at"`
	RequestedBy         string                    `json:"requested_by"`
	Approvals           map[string]repairApproval `json:"approvals"`
}

func validateRepairApproval(
	mode, requestedBy, reviewPath, approvalPath, expectedReviewSHA, expectedApprovalSHA, expectedImage string,
	now time.Time, actualArgv []string,
) error {
	if !strings.EqualFold(strings.TrimSpace(mode), "repair") {
		return nil
	}
	if strings.TrimSpace(reviewPath) == "" || strings.TrimSpace(approvalPath) == "" {
		return fmt.Errorf("repair requires immutable review and approval bundle files")
	}
	if !sha256Pattern.MatchString(expectedReviewSHA) || !sha256Pattern.MatchString(expectedApprovalSHA) {
		return fmt.Errorf("repair requires lowercase SHA-256 bindings for review and approval files")
	}
	if !imagePattern.MatchString(expectedImage) {
		return fmt.Errorf("repair requires an immutable repository@sha256 image reference")
	}
	reviewBytes, err := os.ReadFile(reviewPath)
	if err != nil {
		return fmt.Errorf("read repair review package: %w", err)
	}
	if fileDigest(reviewBytes) != expectedReviewSHA {
		return fmt.Errorf("repair review package SHA-256 mismatch")
	}
	approvalBytes, err := os.ReadFile(approvalPath)
	if err != nil {
		return fmt.Errorf("read repair approval bundle: %w", err)
	}
	if fileDigest(approvalBytes) != expectedApprovalSHA {
		return fmt.Errorf("repair approval bundle SHA-256 mismatch")
	}
	var review repairReviewPackage
	if err := json.Unmarshal(reviewBytes, &review); err != nil {
		return fmt.Errorf("decode repair review package: %w", err)
	}
	if review.SchemaVersion != 1 || review.Mode != "REPAIR_REVIEW_PACKAGE" || review.ExecutionAuthorized || review.ProductionApplied || len(review.ProductionMutations) != 0 {
		return fmt.Errorf("repair review package must remain non-authorizing and non-mutating")
	}
	if len(review.ProposedExecution.Argv) == 0 {
		return fmt.Errorf("repair review package has no proposed argv")
	}
	shadowCapturedAt, ok := review.Bindings["shadow_captured_at"].(string)
	if !ok {
		return fmt.Errorf("repair review package has no shadow capture timestamp")
	}
	shadowTime, err := time.Parse(time.RFC3339, shadowCapturedAt)
	if err != nil || now.Before(shadowTime) || now.Sub(shadowTime) > 15*time.Minute {
		return fmt.Errorf("repair review shadow is expired or captured in the future")
	}
	boundDigest, _ := review.Bindings["immutable_tool_image_digest"].(string)
	if boundDigest == "" || !strings.HasSuffix(expectedImage, "@"+boundDigest) {
		return fmt.Errorf("repair image does not match the digest bound by the review package")
	}
	var approval repairApprovalBundle
	if err := json.Unmarshal(approvalBytes, &approval); err != nil {
		return fmt.Errorf("decode repair approval bundle: %w", err)
	}
	if approval.SchemaVersion != 1 || approval.Mode != "AUTHORIZED_BOUNDED_REPAIR" || !approval.ExecutionAuthorized {
		return fmt.Errorf("repair approval bundle is not authorized")
	}
	if approval.ReviewSHA256 != expectedReviewSHA || approval.Image != expectedImage {
		return fmt.Errorf("repair approval bundle artifact binding mismatch")
	}
	if strings.TrimSpace(approval.ApprovalNonce) == "" || strings.TrimSpace(approval.RequestedBy) == "" || approval.RequestedBy != requestedBy {
		return fmt.Errorf("repair approval operator or nonce binding mismatch")
	}
	notBefore, err := time.Parse(time.RFC3339, approval.NotBefore)
	if err != nil {
		return fmt.Errorf("invalid repair approval not_before: %w", err)
	}
	expiresAt, err := time.Parse(time.RFC3339, approval.ExpiresAt)
	if err != nil {
		return fmt.Errorf("invalid repair approval expires_at: %w", err)
	}
	if !expiresAt.After(notBefore) || expiresAt.Sub(notBefore) > 4*time.Hour || now.Before(notBefore) || now.After(expiresAt) {
		return fmt.Errorf("repair approval window is invalid or inactive")
	}
	required := []string{"sre", "qa", "security", "domain_accountable"}
	identities := map[string]struct{}{}
	for _, role := range required {
		item, ok := approval.Approvals[role]
		if !ok || item.Status != "APPROVED" || strings.TrimSpace(item.ApprovedBy) == "" {
			return fmt.Errorf("repair approval is missing for role %s", role)
		}
		if item.ApprovedBy == requestedBy {
			return fmt.Errorf("repair requester cannot self-approve role %s", role)
		}
		if _, duplicate := identities[item.ApprovedBy]; duplicate {
			return fmt.Errorf("repair approval identities must be distinct")
		}
		identities[item.ApprovedBy] = struct{}{}
		approvedAt, err := time.Parse(time.RFC3339, item.ApprovedAt)
		if err != nil || approvedAt.Before(notBefore) || approvedAt.After(expiresAt) {
			return fmt.Errorf("repair approval timestamp is invalid for role %s", role)
		}
	}
	expectedArgv := append([]string(nil), review.ProposedExecution.Argv...)
	for index, item := range expectedArgv {
		if item == "APPROVED_OPERATOR_REQUIRED" {
			expectedArgv[index] = requestedBy
		}
	}
	if len(actualArgv) == 0 {
		return fmt.Errorf("repair actual argv is missing")
	}
	filteredArgv, err := stripApprovalArgs(actualArgv)
	if err != nil {
		return err
	}
	filteredArgv[0] = filepath.Base(filteredArgv[0])
	if !reflect.DeepEqual(filteredArgv, expectedArgv) {
		return fmt.Errorf("repair actual argv does not match the approved review argv")
	}
	return nil
}

func stripApprovalArgs(argv []string) ([]string, error) {
	approvalFlags := map[string]struct{}{
		"--review-package": {}, "--approval-bundle": {}, "--expected-review-sha256": {},
		"--expected-approval-sha256": {}, "--expected-tool-image": {},
	}
	result := make([]string, 0, len(argv))
	for index := 0; index < len(argv); index++ {
		item := argv[index]
		if _, ok := approvalFlags[item]; !ok {
			result = append(result, item)
			continue
		}
		if index+1 >= len(argv) {
			return nil, fmt.Errorf("repair approval flag %s is missing its value", item)
		}
		index++
	}
	return result, nil
}

func fileDigest(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}
