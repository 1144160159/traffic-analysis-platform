package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/query"
)

const workbenchContinuationVersion = 1

type workbenchContinuation struct {
	Version     int    `json:"v"`
	TenantID    string `json:"tenant_id"`
	Fingerprint string `json:"fingerprint"`
	NodeLimit   int    `json:"node_limit"`
	PageSize    int    `json:"page_size"`
	SinceMS     int64  `json:"since_ms"`
	UntilMS     int64  `json:"until_ms"`
	ExpiresAt   int64  `json:"expires_at"`
}

func workbenchFilterFingerprint(filter query.WorkbenchFilter, pageSize, edgeLimit int) string {
	canonical := strings.Join([]string{
		filter.CenterID, fmt.Sprint(filter.Depth), filter.EntityType, filter.Site,
		filter.TimeRange, fmt.Sprint(pageSize), fmt.Sprint(edgeLimit),
	}, "\x00")
	digest := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(digest[:])
}

func encodeWorkbenchContinuation(payload workbenchContinuation, secret string) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return base64.RawURLEncoding.EncodeToString(body) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func decodeWorkbenchContinuation(raw, tenantID, fingerprint, secret string, now time.Time) (workbenchContinuation, error) {
	var payload workbenchContinuation
	if len(raw) == 0 || len(raw) > 4096 {
		return payload, fmt.Errorf("invalid continuation")
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 2 {
		return payload, fmt.Errorf("invalid continuation")
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return payload, fmt.Errorf("invalid continuation")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return payload, fmt.Errorf("invalid continuation")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	if !hmac.Equal(signature, mac.Sum(nil)) || json.Unmarshal(body, &payload) != nil {
		return workbenchContinuation{}, fmt.Errorf("invalid continuation")
	}
	if payload.Version != workbenchContinuationVersion || payload.TenantID != tenantID ||
		(fingerprint != "" && payload.Fingerprint != fingerprint) || payload.ExpiresAt <= now.Unix() ||
		payload.PageSize < 1 || payload.NodeLimit < payload.PageSize || payload.SinceMS < 0 || payload.UntilMS <= 0 {
		return workbenchContinuation{}, fmt.Errorf("invalid continuation")
	}
	return payload, nil
}

func workbenchHasEvidencePermission(ctxPermissions []string) bool {
	return authmodel.HasAnyScope(ctxPermissions, authmodel.ScopeEvidenceRead, authmodel.ScopeAdminAll, authmodel.ScopeAll)
}

// governWorkbenchGraph returns a response-owned copy. Secret-like keys are
// never exposed. Evidence references additionally require evidence:read even
// though graph:read is sufficient to view the topology itself.
func governWorkbenchGraph(graph *query.WorkbenchGraph, permissions []string) (*query.WorkbenchGraph, []string) {
	if graph == nil {
		return graph, []string{}
	}
	allowEvidence := workbenchHasEvidencePermission(permissions)
	redacted := make(map[string]bool)
	copyGraph := &query.WorkbenchGraph{
		CenterID: graph.CenterID, Truncated: graph.Truncated, TruncationReason: graph.TruncationReason,
		Nodes: make([]*query.WorkbenchNode, 0, len(graph.Nodes)), Edges: make([]*query.WorkbenchEdge, 0, len(graph.Edges)),
	}
	for _, node := range graph.Nodes {
		clone := *node
		clone.Metadata = redactWorkbenchMap(node.Metadata, allowEvidence, redacted)
		if !allowEvidence && node.EntityType == "evidence" && clone.Detail != "" {
			clone.Detail = "[REDACTED: evidence:read required]"
			redacted["node.detail"] = true
		}
		copyGraph.Nodes = append(copyGraph.Nodes, &clone)
	}
	for _, edge := range graph.Edges {
		clone := *edge
		clone.Attributes = redactWorkbenchMap(edge.Attributes, allowEvidence, redacted)
		if !allowEvidence && clone.EvidenceID != "" {
			clone.EvidenceID = ""
			redacted["edge.evidence_id"] = true
		}
		copyGraph.Edges = append(copyGraph.Edges, &clone)
	}
	fields := make([]string, 0, len(redacted))
	for field := range redacted {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return copyGraph, fields
}

func governWorkbenchPath(path *query.WorkbenchPath, permissions []string) (*query.WorkbenchPath, []string) {
	if path == nil {
		return path, []string{}
	}
	allowEvidence := workbenchHasEvidencePermission(permissions)
	redacted := make(map[string]bool)
	clone := *path
	clone.NodeIDs = append([]string(nil), path.NodeIDs...)
	clone.EvidenceIDs = append([]string(nil), path.EvidenceIDs...)
	clone.Edges = make([]*query.WorkbenchEdge, 0, len(path.Edges))
	if !allowEvidence && len(clone.EvidenceIDs) > 0 {
		clone.EvidenceIDs = []string{}
		redacted["path.evidence_ids"] = true
	}
	for _, edge := range path.Edges {
		edgeClone := *edge
		edgeClone.Attributes = redactWorkbenchMap(edge.Attributes, allowEvidence, redacted)
		if !allowEvidence && edgeClone.EvidenceID != "" {
			edgeClone.EvidenceID = ""
			redacted["edge.evidence_id"] = true
		}
		clone.Edges = append(clone.Edges, &edgeClone)
	}
	fields := make([]string, 0, len(redacted))
	for field := range redacted {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return &clone, fields
}

func redactWorkbenchMap(source map[string]interface{}, allowEvidence bool, redacted map[string]bool) map[string]interface{} {
	result := make(map[string]interface{}, len(source))
	for key, value := range source {
		lower := strings.ToLower(strings.TrimSpace(key))
		secretLike := strings.Contains(lower, "password") || strings.Contains(lower, "secret") ||
			strings.Contains(lower, "credential") || strings.Contains(lower, "private_key") ||
			strings.Contains(lower, "authorization") || lower == "token" || strings.HasSuffix(lower, "_token")
		evidenceLike := strings.Contains(lower, "evidence") || strings.Contains(lower, "object_key") ||
			strings.Contains(lower, "pcap") || strings.Contains(lower, "raw_payload")
		if secretLike || evidenceLike && !allowEvidence {
			redacted["metadata."+lower] = true
			continue
		}
		result[key] = value
	}
	return result
}
