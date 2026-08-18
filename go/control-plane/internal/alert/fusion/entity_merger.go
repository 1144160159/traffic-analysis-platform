package fusion

import (
	"fmt"
	"sort"
	"strings"
)

type disjointSet struct {
	parent []int
	rank   []int
}

func newDisjointSet(size int) *disjointSet {
	parent := make([]int, size)
	for index := range parent {
		parent[index] = index
	}
	return &disjointSet{parent: parent, rank: make([]int, size)}
}

func (set *disjointSet) find(value int) int {
	if set.parent[value] != value {
		set.parent[value] = set.find(set.parent[value])
	}
	return set.parent[value]
}

func (set *disjointSet) union(left, right int) {
	leftRoot, rightRoot := set.find(left), set.find(right)
	if leftRoot == rightRoot {
		return
	}
	if set.rank[leftRoot] < set.rank[rightRoot] {
		leftRoot, rightRoot = rightRoot, leftRoot
	}
	set.parent[rightRoot] = leftRoot
	if set.rank[leftRoot] == set.rank[rightRoot] {
		set.rank[leftRoot]++
	}
}

func MergeSourceEntities(
	entities []BoundSourceEntityFact,
	relations []BoundSourceRelationFact,
) ([]CanonicalEntity, []CanonicalRelation, error) {
	if len(entities) == 0 {
		return []CanonicalEntity{}, []CanonicalRelation{}, nil
	}
	set := newDisjointSet(len(entities))
	strongIdentityOwner := make(map[string]int)
	for index, entity := range entities {
		for key, value := range entity.Fact.Identifiers {
			if !isStrongFusionIdentifier(key) || strings.TrimSpace(value) == "" {
				continue
			}
			token := key + "=" + strings.TrimSpace(value)
			if owner, exists := strongIdentityOwner[token]; exists {
				set.union(index, owner)
			} else {
				strongIdentityOwner[token] = index
			}
		}
	}
	components := make(map[int][]int)
	for index := range entities {
		root := set.find(index)
		components[root] = append(components[root], index)
	}
	canonical := make([]CanonicalEntity, 0, len(components))
	refToCanonical := make(map[string]string, len(entities))
	for _, indexes := range components {
		identifiers := make(map[string]string)
		sourceSet := make(map[string]struct{})
		refs := make([]string, 0, len(indexes))
		kinds := make([]string, 0, len(indexes))
		assetIDs := make(map[string]struct{})
		userIDs := make(map[string]struct{})
		for _, index := range indexes {
			entity := entities[index]
			ref := entity.SourceSnapshotID + ":" + entity.Fact.SourceEntityID
			refs = append(refs, ref)
			sourceSet[entity.SourceID] = struct{}{}
			kinds = append(kinds, entity.Fact.EntityKind)
			for key, value := range entity.Fact.Identifiers {
				if previous, exists := identifiers[key]; !exists || value < previous {
					identifiers[key] = value
				}
				if key == "asset_id" {
					assetIDs[value] = struct{}{}
				}
				if key == "user_id" {
					userIDs[value] = struct{}{}
				}
			}
		}
		if len(assetIDs) > 1 || len(userIDs) > 1 {
			return nil, nil, fmt.Errorf("%w: a strong identifier maps to multiple authoritative entities", ErrIdentityConflict)
		}
		sort.Strings(refs)
		identityToken := preferredIdentityToken(identifiers)
		if identityToken == "" {
			return nil, nil, fmt.Errorf("%w: merged entity has no stable identity", ErrIdentityConflict)
		}
		entityID := stableHex("fusion-data-entity-v1", identityToken)
		for _, index := range indexes {
			refToCanonical[entities[index].SourceSnapshotID+":"+entities[index].Fact.SourceEntityID] = entityID
		}
		sources := sortedKeys(sourceSet)
		canonical = append(canonical, CanonicalEntity{
			EntityID: entityID, EntityKind: preferredEntityKind(kinds), Identifiers: identifiers,
			SourceEntityRefs: refs, SourceCount: len(sources), Confidence: float64(len(sources)) / float64(len(requiredSourceIDs)),
			Provenance: map[string]interface{}{
				"algorithm": "strong-identifier-union-v1", "sources": sources,
				"source_entity_refs": refs, "confidence_semantics": "source_coverage_not_accuracy",
			},
		})
	}
	sort.Slice(canonical, func(i, j int) bool { return canonical[i].EntityID < canonical[j].EntityID })
	canonicalRelations := make([]CanonicalRelation, 0, len(relations))
	seenRelations := make(map[string]struct{})
	for _, bound := range relations {
		sourceRef := bound.SourceSnapshotID + ":" + bound.Fact.SourceEntityID
		targetRef := bound.SourceSnapshotID + ":" + bound.Fact.TargetEntityID
		sourceID, sourceOK := refToCanonical[sourceRef]
		targetID, targetOK := refToCanonical[targetRef]
		if !sourceOK || !targetOK {
			return nil, nil, fmt.Errorf("%w: relation endpoint is absent from entity snapshot", ErrIdentityConflict)
		}
		if sourceID == targetID {
			continue
		}
		evidence := append([]string(nil), bound.Fact.EvidenceEventIDs...)
		sort.Strings(evidence)
		relationID := stableHex("fusion-data-relation-v1", sourceID, targetID, bound.Fact.RelationKind, strings.Join(evidence, ","))
		if _, exists := seenRelations[relationID]; exists {
			continue
		}
		seenRelations[relationID] = struct{}{}
		canonicalRelations = append(canonicalRelations, CanonicalRelation{
			RelationID: relationID, SourceEntityID: sourceID, TargetEntityID: targetID,
			RelationKind: bound.Fact.RelationKind, EdgeOrigin: "observed", EventTime: bound.Fact.EventTime,
			Confidence: 1, EvidenceRefs: evidence,
			Provenance: map[string]interface{}{
				"source_id": bound.SourceID, "source_snapshot_id": bound.SourceSnapshotID,
				"source_relation_id": bound.Fact.SourceRelationID, "edge_origin": "observed",
			},
		})
	}
	sort.Slice(canonicalRelations, func(i, j int) bool { return canonicalRelations[i].RelationID < canonicalRelations[j].RelationID })
	return canonical, canonicalRelations, nil
}

func isStrongFusionIdentifier(key string) bool {
	switch key {
	case "asset_id", "user_id", "mac", "ip":
		return true
	default:
		return false
	}
}

func preferredIdentityToken(identifiers map[string]string) string {
	for _, key := range []string{"asset_id", "user_id", "mac", "ip", "hostname", "username"} {
		if value := strings.TrimSpace(identifiers[key]); value != "" {
			return key + "=" + value
		}
	}
	return ""
}

func preferredEntityKind(values []string) string {
	rank := map[string]int{"unknown": 0, "ip": 1, "device": 2, "host": 3, "service": 4, "user": 5, "asset": 6}
	best, bestRank := "unknown", -1
	for _, value := range values {
		if valueRank, exists := rank[value]; exists && valueRank > bestRank {
			best, bestRank = value, valueRank
		}
	}
	return best
}

func sortedKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
