package entityresolution

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	ruleAssetExact = "ER-ASSET-ID-EXACT"
	ruleUserExact  = "ER-USER-ID-EXACT"
	ruleProbeExact = "ER-PROBE-ID-EXACT"
	ruleMACAsset   = "ER-MAC-ASSET-TEMPORAL"
	ruleIPAsset    = "ER-IP-ASSET-TEMPORAL"
	ruleCommunity  = "ER-COMMUNITY-CORRELATION-ONLY"
)

type normalizedObservation struct {
	tenantID       string
	source         SourceReference
	sourceTuple    string
	identifiers    []Identifier
	resolutionID   string
	observationSHA string
}

type anchorEvidence struct {
	entityID     string
	observedAtMS int64
}

type evidenceIndex struct {
	assetsByMAC map[string][]anchorEvidence
	assetsByIP  map[string][]anchorEvidence
}

func newNormalizedObservation(
	tenantID string,
	source SourceReference,
	sourceTuple string,
	identifiers []Identifier,
) normalizedObservation {
	resolutionDigest := sha256.Sum256([]byte(tenantID + "\x00" + sourceTuple))
	identityPayload := struct {
		TenantID    string          `json:"tenant_id"`
		Source      SourceReference `json:"source"`
		Identifiers []Identifier    `json:"identifiers"`
	}{tenantID, source, identifiers}
	payload, _ := json.Marshal(identityPayload)
	observationDigest := sha256.Sum256(payload)
	return normalizedObservation{
		tenantID:       tenantID,
		source:         source,
		sourceTuple:    sourceTuple,
		identifiers:    identifiers,
		resolutionID:   "er1-" + hex.EncodeToString(resolutionDigest[:]),
		observationSHA: hex.EncodeToString(observationDigest[:]),
	}
}

// Resolve deterministically evaluates observations at an explicit event-time boundary.
// It is pure and writes no external store; runtime projection remains default-off.
func Resolve(observations []Observation, asOfMS int64) ([]ResolutionResult, error) {
	normalized := make([]normalizedObservation, 0, len(observations))
	seenSources := make(map[string]string, len(observations))
	for index, observation := range observations {
		item, err := normalizeObservation(observation, asOfMS)
		if err != nil {
			return nil, fmt.Errorf("normalize observation %d: %w", index, err)
		}
		sourceIdentity := item.tenantID + "\x00" + item.sourceTuple
		if priorSHA, exists := seenSources[sourceIdentity]; exists {
			if priorSHA != item.observationSHA {
				return nil, fmt.Errorf("source tuple identity collision for %s", item.sourceTuple)
			}
			continue
		}
		seenSources[sourceIdentity] = item.observationSHA
		normalized = append(normalized, item)
	}
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i].resolutionID < normalized[j].resolutionID
	})

	index := buildEvidenceIndex(normalized)
	results := make([]ResolutionResult, 0, len(normalized))
	for _, observation := range normalized {
		result := resolveOne(observation, index)
		digest, err := decisionSHA256(result)
		if err != nil {
			return nil, fmt.Errorf("hash entity resolution result: %w", err)
		}
		result.DecisionSHA256 = digest
		results = append(results, result)
	}
	return results, nil
}

func buildEvidenceIndex(observations []normalizedObservation) evidenceIndex {
	index := evidenceIndex{
		assetsByMAC: make(map[string][]anchorEvidence),
		assetsByIP:  make(map[string][]anchorEvidence),
	}
	for _, observation := range observations {
		if observation.source.Rail != RailAssetAuthority {
			continue
		}
		assetIDs := valuesOf(observation.identifiers, IdentifierAssetID)
		if len(assetIDs) != 1 {
			continue
		}
		entityID := "asset:" + assetIDs[0].Value
		for _, identifier := range observation.identifiers {
			key := observation.tenantID + "\x00" + identifier.Value
			evidence := anchorEvidence{entityID: entityID, observedAtMS: observation.source.ObservedAtMS}
			switch identifier.Kind {
			case IdentifierMAC:
				index.assetsByMAC[key] = appendEvidence(index.assetsByMAC[key], evidence)
			case IdentifierIP:
				index.assetsByIP[key] = appendEvidence(index.assetsByIP[key], evidence)
			}
		}
	}
	return index
}

func appendEvidence(existing []anchorEvidence, candidate anchorEvidence) []anchorEvidence {
	for _, value := range existing {
		if value == candidate {
			return existing
		}
	}
	return append(existing, candidate)
}

func resolveOne(observation normalizedObservation, index evidenceIndex) ResolutionResult {
	result := ResolutionResult{
		ResolutionID:          observation.resolutionID,
		RuleVersion:           RuleVersion,
		TenantID:              observation.tenantID,
		Source:                observation.source,
		NormalizedIdentifiers: append([]Identifier(nil), observation.identifiers...),
		Entities:              []EntityMatch{},
		Issues:                []ResolutionIssue{},
		Correlations:          []Correlation{},
	}

	for _, role := range rolesOf(observation.identifiers,
		IdentifierAssetID, IdentifierMAC, IdentifierIP) {
		resolveAsset(&result, observation, index, role)
	}
	resolveDirectEntity(&result, observation, IdentifierUserID, "user", ruleUserExact,
		UserExactConfidencePPM, RailUserBehavior)
	resolveDirectEntity(&result, observation, IdentifierProbeID, "probe", ruleProbeExact,
		ProbeExactConfidencePPM, RailFlow, RailAssetBinding, RailProbeIngest)
	for _, community := range valuesOf(observation.identifiers, IdentifierCommunityID) {
		result.Correlations = append(result.Correlations, Correlation{
			Identifier:    community,
			ConfidencePPM: CommunityConfidencePPM,
			RuleID:        ruleCommunity,
		})
	}

	sortEntities(result.Entities)
	sortIssues(result.Issues)
	sort.Slice(result.Correlations, func(i, j int) bool {
		return result.Correlations[i].Identifier.Value < result.Correlations[j].Identifier.Value
	})
	result.Status = decisionStatus(result.Entities, result.Issues)
	return result
}

func resolveAsset(
	result *ResolutionResult,
	observation normalizedObservation,
	index evidenceIndex,
	role string,
) {
	issueCountBeforeRole := len(result.Issues)
	assetIDs := valuesOfRole(observation.identifiers, IdentifierAssetID, role)
	macs := valuesOfRole(observation.identifiers, IdentifierMAC, role)
	ips := valuesOfRole(observation.identifiers, IdentifierIP, role)
	scope := "asset:" + role

	if len(assetIDs) > 1 {
		result.Issues = append(result.Issues, issue(
			"conflict", "MULTIPLE_ASSET_ANCHORS", scope, assetIDs[0],
			prefixedValues("asset:", assetIDs), ruleAssetExact))
		return
	}
	if len(assetIDs) == 1 {
		assetID := "asset:" + assetIDs[0].Value
		if observation.source.Rail != RailAssetAuthority {
			result.Issues = append(result.Issues, issue(
				"conflict", "UNTRUSTED_ASSET_ANCHOR", scope, assetIDs[0],
				[]string{assetID}, ruleAssetExact))
			return
		}
		result.Entities = append(result.Entities, EntityMatch{
			EntityType: "asset", EntityID: assetID, Role: role,
			MatchedBy:     []Identifier{assetIDs[0]},
			ConfidencePPM: AssetExactConfidencePPM, RuleID: ruleAssetExact,
		})
		for _, identifier := range append(append([]Identifier{}, macs...), ips...) {
			candidates, ruleID := candidatesFor(observation, identifier, index)
			for _, candidate := range candidates {
				if candidate != assetID {
					result.Issues = append(result.Issues, issue(
						"conflict", "ANCHOR_IDENTIFIER_CONFLICT", scope, identifier,
						candidates, ruleID))
					break
				}
			}
		}
		return
	}

	macCandidates, macIdentifiers := candidateUnion(observation, macs, index)
	ipCandidates, ipIdentifiers := candidateUnion(observation, ips, index)
	if len(macCandidates) > 1 {
		result.Issues = append(result.Issues, issue(
			"ambiguous", "AMBIGUOUS_MAC_ASSET", scope, firstIdentifier(macs),
			macCandidates, ruleMACAsset))
	}
	if len(ipCandidates) > 1 {
		result.Issues = append(result.Issues, issue(
			"ambiguous", "AMBIGUOUS_IP_ASSET", scope, firstIdentifier(ips),
			ipCandidates, ruleIPAsset))
	}
	if len(macCandidates) == 1 && len(ipCandidates) == 1 && macCandidates[0] != ipCandidates[0] {
		result.Issues = append(result.Issues, issue(
			"conflict", "MAC_IP_TARGET_CONFLICT", scope, firstIdentifier(macs),
			uniqueSorted(append(macCandidates, ipCandidates...)), ruleMACAsset))
		return
	}
	if len(result.Issues) > issueCountBeforeRole {
		return
	}
	if len(macCandidates) == 1 {
		result.Entities = append(result.Entities, EntityMatch{
			EntityType: "asset", EntityID: macCandidates[0], Role: role, MatchedBy: macIdentifiers,
			ConfidencePPM: MACAssetConfidencePPM, RuleID: ruleMACAsset,
		})
		return
	}
	if len(ipCandidates) == 1 {
		result.Entities = append(result.Entities, EntityMatch{
			EntityType: "asset", EntityID: ipCandidates[0], Role: role, MatchedBy: ipIdentifiers,
			ConfidencePPM: IPAssetConfidencePPM, RuleID: ruleIPAsset,
		})
		return
	}
	if len(macs) > 0 || len(ips) > 0 {
		identifier := firstIdentifier(macs)
		ruleID := ruleMACAsset
		if identifier.Kind == "" {
			identifier = firstIdentifier(ips)
			ruleID = ruleIPAsset
		}
		result.Issues = append(result.Issues, issue(
			"unresolved", "ASSET_IDENTIFIER_UNRESOLVED", scope, identifier,
			[]string{}, ruleID))
	}
}

func resolveDirectEntity(
	result *ResolutionResult,
	observation normalizedObservation,
	kind IdentifierKind,
	entityType string,
	ruleID string,
	confidence int,
	authorityRails ...SourceRail,
) {
	for _, role := range rolesOf(observation.identifiers, kind) {
		values := valuesOfRole(observation.identifiers, kind, role)
		if len(values) > 1 {
			result.Issues = append(result.Issues, issue(
				"conflict", "MULTIPLE_"+strings.ToUpper(entityType)+"_ANCHORS", entityType+":"+role,
				values[0], prefixedValues(entityType+":", values), ruleID))
			continue
		}
		trusted := false
		for _, rail := range authorityRails {
			trusted = trusted || observation.source.Rail == rail
		}
		if !trusted {
			result.Issues = append(result.Issues, issue(
				"conflict", "UNTRUSTED_"+strings.ToUpper(entityType)+"_ANCHOR", entityType+":"+role,
				values[0], []string{entityType + ":" + values[0].Value}, ruleID))
			continue
		}
		result.Entities = append(result.Entities, EntityMatch{
			EntityType:    entityType,
			EntityID:      entityType + ":" + values[0].Value,
			Role:          role,
			MatchedBy:     []Identifier{values[0]},
			ConfidencePPM: confidence,
			RuleID:        ruleID,
		})
	}
}

func candidateUnion(
	observation normalizedObservation,
	identifiers []Identifier,
	index evidenceIndex,
) ([]string, []Identifier) {
	all := []string{}
	matched := []Identifier{}
	for _, identifier := range identifiers {
		candidates, _ := candidatesFor(observation, identifier, index)
		if len(candidates) > 0 {
			matched = append(matched, identifier)
			all = append(all, candidates...)
		}
	}
	return uniqueSorted(all), matched
}

func candidatesFor(
	observation normalizedObservation,
	identifier Identifier,
	index evidenceIndex,
) ([]string, string) {
	key := observation.tenantID + "\x00" + identifier.Value
	evidence := []anchorEvidence{}
	maximumAge := int64(0)
	ruleID := ""
	switch identifier.Kind {
	case IdentifierMAC:
		evidence = index.assetsByMAC[key]
		maximumAge = MACMaximumLinkAgeMS
		ruleID = ruleMACAsset
	case IdentifierIP:
		evidence = index.assetsByIP[key]
		maximumAge = IPMaximumLinkAgeMS
		ruleID = ruleIPAsset
	default:
		return []string{}, ""
	}
	candidates := []string{}
	for _, item := range evidence {
		if withinAge(observation.source.ObservedAtMS, item.observedAtMS, maximumAge) {
			candidates = append(candidates, item.entityID)
		}
	}
	return uniqueSorted(candidates), ruleID
}

func withinAge(left, right, maximum int64) bool {
	if left >= right {
		return left-right <= maximum
	}
	return right-left <= maximum
}

func valuesOf(identifiers []Identifier, kind IdentifierKind) []Identifier {
	values := []Identifier{}
	for _, identifier := range identifiers {
		if identifier.Kind == kind {
			values = append(values, identifier)
		}
	}
	return values
}

func valuesOfRole(identifiers []Identifier, kind IdentifierKind, role string) []Identifier {
	values := []Identifier{}
	for _, identifier := range identifiers {
		if identifier.Kind == kind && identifier.Role == role {
			values = append(values, identifier)
		}
	}
	return values
}

func rolesOf(identifiers []Identifier, kinds ...IdentifierKind) []string {
	wanted := make(map[IdentifierKind]struct{}, len(kinds))
	for _, kind := range kinds {
		wanted[kind] = struct{}{}
	}
	roles := []string{}
	for _, identifier := range identifiers {
		if _, ok := wanted[identifier.Kind]; ok {
			roles = append(roles, identifier.Role)
		}
	}
	return uniqueSorted(roles)
}

func prefixedValues(prefix string, identifiers []Identifier) []string {
	values := make([]string, 0, len(identifiers))
	for _, identifier := range identifiers {
		values = append(values, prefix+identifier.Value)
	}
	return uniqueSorted(values)
}

func firstIdentifier(values []Identifier) Identifier {
	if len(values) == 0 {
		return Identifier{}
	}
	return values[0]
}

func issue(
	class, code, scope string,
	identifier Identifier,
	candidates []string,
	ruleID string,
) ResolutionIssue {
	return ResolutionIssue{
		Class:              class,
		Code:               code,
		Scope:              scope,
		Identifier:         identifier,
		CandidateEntityIDs: uniqueSorted(candidates),
		RuleID:             ruleID,
	}
}

func uniqueSorted(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value != "" {
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

func sortEntities(values []EntityMatch) {
	for index := range values {
		sortIdentifiers(values[index].MatchedBy)
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].EntityType != values[j].EntityType {
			return values[i].EntityType < values[j].EntityType
		}
		if values[i].Role != values[j].Role {
			return values[i].Role < values[j].Role
		}
		return values[i].EntityID < values[j].EntityID
	})
}

func sortIssues(values []ResolutionIssue) {
	sort.Slice(values, func(i, j int) bool {
		left := values[i].Class + "\x00" + values[i].Code + "\x00" + values[i].Scope +
			"\x00" + string(values[i].Identifier.Kind) + "\x00" + values[i].Identifier.Role +
			"\x00" + values[i].Identifier.Value
		right := values[j].Class + "\x00" + values[j].Code + "\x00" + values[j].Scope +
			"\x00" + string(values[j].Identifier.Kind) + "\x00" + values[j].Identifier.Role +
			"\x00" + values[j].Identifier.Value
		return left < right
	})
}

func decisionStatus(entities []EntityMatch, issues []ResolutionIssue) ResolutionStatus {
	hasAmbiguous := false
	hasUnresolved := false
	for _, item := range issues {
		switch item.Class {
		case "conflict":
			return StatusConflict
		case "ambiguous":
			hasAmbiguous = true
		case "unresolved":
			hasUnresolved = true
		}
	}
	if hasAmbiguous {
		return StatusAmbiguous
	}
	if len(entities) > 0 && hasUnresolved {
		return StatusPartial
	}
	if len(entities) > 0 {
		return StatusAccepted
	}
	return StatusInsufficient
}

func decisionSHA256(result ResolutionResult) (string, error) {
	result.DecisionSHA256 = ""
	payload, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("marshal deterministic entity resolution result: %w", err)
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}
