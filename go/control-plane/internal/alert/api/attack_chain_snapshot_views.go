package api

import (
	"net/http"
	"sort"
	"strings"

	"github.com/gorilla/mux"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/attackchain"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
)

type attackChainSnapshotPhaseView struct {
	Stage       string   `json:"stage"`
	PathIDs     []string `json:"path_ids"`
	EdgeIDs     []string `json:"edge_ids"`
	Provenance  []string `json:"provenance"`
	EvidenceIDs []string `json:"evidence_ids"`
	Partial     bool     `json:"partial"`
}

type attackChainSnapshotEvidenceView struct {
	attackchain.EvidenceAnchor
	PathIDs []string `json:"path_ids"`
	Stages  []string `json:"stages"`
}

func (h *SystemHandler) loadAttackChainSnapshotV1(
	w http.ResponseWriter,
	r *http.Request,
) (attackchain.Snapshot, bool) {
	ctx := r.Context()
	if h.attackChainSnapshots == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "ATTACK_CHAIN_SNAPSHOT_UNAVAILABLE", "versioned attack-chain snapshot store is unavailable")
		return attackchain.Snapshot{}, false
	}
	snapshot, err := h.attackChainSnapshots.LoadCurrent(ctx, queryTenantID(r), mux.Vars(r)["id"])
	if err != nil {
		writeCampaignReadError(w, ctx, err)
		return attackchain.Snapshot{}, false
	}
	return snapshot, true
}

func attackChainSnapshotPaths(snapshot attackchain.Snapshot, stage string) []attackchain.Path {
	paths := append([]attackchain.Path{snapshot.CandidatePath}, snapshot.AlternativePaths...)
	stage = strings.TrimSpace(stage)
	if stage == "" {
		return paths
	}
	filtered := make([]attackchain.Path, 0, len(paths))
	for _, path := range paths {
		for _, edge := range path.Edges {
			if strings.EqualFold(strings.TrimSpace(edge.Stage), stage) {
				filtered = append(filtered, path)
				break
			}
		}
	}
	return filtered
}

func pageAttackChainPaths(items []attackchain.Path, limit, offset int) []attackchain.Path {
	if offset >= len(items) {
		return []attackchain.Path{}
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end]
}

func attackChainSnapshotPhases(snapshot attackchain.Snapshot) []attackChainSnapshotPhaseView {
	paths := append([]attackchain.Path{snapshot.CandidatePath}, snapshot.AlternativePaths...)
	views := make(map[string]*attackChainSnapshotPhaseView, len(snapshot.Stages))
	for _, stage := range snapshot.Stages {
		views[stage] = &attackChainSnapshotPhaseView{Stage: stage, PathIDs: []string{}, EdgeIDs: []string{}, Provenance: []string{}, EvidenceIDs: []string{}}
	}
	for _, path := range paths {
		for _, edge := range path.Edges {
			view := views[edge.Stage]
			if view == nil {
				view = &attackChainSnapshotPhaseView{Stage: edge.Stage, PathIDs: []string{}, EdgeIDs: []string{}, Provenance: []string{}, EvidenceIDs: []string{}}
				views[edge.Stage] = view
			}
			view.PathIDs = append(view.PathIDs, path.PathID)
			view.EdgeIDs = append(view.EdgeIDs, edge.EdgeID)
			view.Provenance = append(view.Provenance, edge.Provenance)
			view.Partial = view.Partial || path.Partial
			for _, evidence := range edge.Evidence {
				view.EvidenceIDs = append(view.EvidenceIDs, evidence.EvidenceID)
				view.Partial = view.Partial || !evidence.Available
			}
		}
	}
	result := make([]attackChainSnapshotPhaseView, 0, len(views))
	seen := make(map[string]struct{}, len(views))
	for _, stage := range snapshot.Stages {
		if view := views[stage]; view != nil {
			canonicalizeAttackChainPhaseView(view)
			result = append(result, *view)
			seen[stage] = struct{}{}
		}
	}
	extra := make([]string, 0)
	for stage := range views {
		if _, ok := seen[stage]; !ok {
			extra = append(extra, stage)
		}
	}
	sort.Strings(extra)
	for _, stage := range extra {
		view := views[stage]
		canonicalizeAttackChainPhaseView(view)
		result = append(result, *view)
	}
	return result
}

func canonicalizeAttackChainPhaseView(view *attackChainSnapshotPhaseView) {
	view.PathIDs = sortedUniqueAttackChainStrings(view.PathIDs)
	view.EdgeIDs = sortedUniqueAttackChainStrings(view.EdgeIDs)
	view.Provenance = sortedUniqueAttackChainStrings(view.Provenance)
	view.EvidenceIDs = sortedUniqueAttackChainStrings(view.EvidenceIDs)
}

func attackChainSnapshotEvidence(snapshot attackchain.Snapshot, evidenceType, stage string) []attackChainSnapshotEvidenceView {
	type aggregate struct {
		anchor attackchain.EvidenceAnchor
		paths  []string
		stages []string
	}
	byID := make(map[string]*aggregate)
	paths := append([]attackchain.Path{snapshot.CandidatePath}, snapshot.AlternativePaths...)
	for _, path := range paths {
		for _, edge := range path.Edges {
			if stage != "" && !strings.EqualFold(strings.TrimSpace(edge.Stage), stage) {
				continue
			}
			for _, anchor := range edge.Evidence {
				if !attackChainSnapshotEvidenceMatches(anchor, evidenceType) {
					continue
				}
				item := byID[anchor.EvidenceID]
				if item == nil {
					item = &aggregate{anchor: anchor}
					byID[anchor.EvidenceID] = item
				}
				item.paths = append(item.paths, path.PathID)
				item.stages = append(item.stages, edge.Stage)
			}
		}
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]attackChainSnapshotEvidenceView, 0, len(ids))
	for _, id := range ids {
		item := byID[id]
		result = append(result, attackChainSnapshotEvidenceView{
			EvidenceAnchor: item.anchor,
			PathIDs:        sortedUniqueAttackChainStrings(item.paths),
			Stages:         sortedUniqueAttackChainStrings(item.stages),
		})
	}
	return result
}

func attackChainSnapshotEvidenceMatches(anchor attackchain.EvidenceAnchor, evidenceType string) bool {
	if evidenceType == "" {
		return true
	}
	if evidenceType == "rule_model" {
		return anchor.Kind == "rule" || anchor.Kind == "model"
	}
	if evidenceType == "alert" {
		return anchor.Kind == "event"
	}
	if evidenceType == "graph" {
		return false
	}
	uri := strings.ToLower(anchor.ImmutableURI)
	return anchor.Kind == "event" && strings.Contains(uri, "/"+evidenceType+"/")
}

func pageAttackChainEvidence(items []attackChainSnapshotEvidenceView, limit, offset int) []attackChainSnapshotEvidenceView {
	if offset >= len(items) {
		return []attackChainSnapshotEvidenceView{}
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end]
}

func sortedUniqueAttackChainStrings(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			set[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
