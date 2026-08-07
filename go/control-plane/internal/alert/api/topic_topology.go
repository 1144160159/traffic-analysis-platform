package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	exfilTopicTopologyMaxPaths        = 12
	exfilTopicTopologyMaxSources      = 5
	exfilTopicTopologyMaxProtocols    = 4
	exfilTopicTopologyMaxDestinations = 5
	aptTopicTopologyMaxCampaigns      = 3
	aptTopicTopologyMaxPhases         = 6
	aptTopicTopologyMaxEntities       = 6
	aptTopicTopologyMaxEvidence       = 6
)

type topicTopologyNodeDTO struct {
	ID     string  `json:"id"`
	Label  string  `json:"label"`
	Detail string  `json:"detail"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Tone   string  `json:"tone"`
	Width  int     `json:"width"`
	Height int     `json:"height"`
	Icon   string  `json:"icon"`
}

type topicTopologyLinkDTO struct {
	Source   string  `json:"source"`
	Target   string  `json:"target"`
	Value    float64 `json:"value"`
	Tone     string  `json:"tone"`
	LineType string  `json:"line_type"`
	Label    string  `json:"label"`
}

type topicTopologyAggregate struct {
	SessionCount int64
	UploadBytes  uint64
	PathCount    int64
	Risk         string
}

type topicTopologyLinkAggregate struct {
	Source       string
	Target       string
	SessionCount int64
	UploadBytes  uint64
	Risk         string
}

func stableTopicTopologyID(prefix, value string) string {
	digest := sha256.Sum256([]byte(prefix + "\x00" + strings.TrimSpace(value)))
	return prefix + "-" + hex.EncodeToString(digest[:8])
}

func topicTopologyAxisPosition(index, total int) float64 {
	if total <= 1 {
		return 50
	}
	return 10 + (80 * float64(index) / float64(total-1))
}

func topicTopologyRiskRank(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "critical", "high":
		return 3
	case "medium", "warn", "warning":
		return 2
	case "low", "ok":
		return 1
	default:
		return 0
	}
}

func strongerTopicTopologyRisk(current, candidate string) string {
	if topicTopologyRiskRank(candidate) > topicTopologyRiskRank(current) {
		return strings.ToLower(strings.TrimSpace(candidate))
	}
	return current
}

func topicTopologyNodeTone(risk, fallback string) string {
	switch topicTopologyRiskRank(risk) {
	case 3:
		return "risk"
	case 2:
		return "warn"
	default:
		return fallback
	}
}

func topicTopologyLinkTone(risk string) string {
	switch topicTopologyRiskRank(risk) {
	case 3:
		return "risk"
	case 2:
		return "warn"
	default:
		return "info"
	}
}

func appendUniqueBounded(values *[]string, seen map[string]struct{}, value string, limit int) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if _, ok := seen[value]; ok {
		return true
	}
	if len(*values) >= limit {
		return false
	}
	seen[value] = struct{}{}
	*values = append(*values, value)
	return true
}

func canAppendUniqueBounded(values []string, seen map[string]struct{}, value string, limit int) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if _, ok := seen[value]; ok {
		return true
	}
	return len(values) < limit
}

func buildExfiltrationTopicTopology(paths []encryptedExfiltrationPathDTO) ([]topicTopologyNodeDTO, []topicTopologyLinkDTO) {
	sourceOrder := make([]string, 0, exfilTopicTopologyMaxSources)
	protocolOrder := make([]string, 0, exfilTopicTopologyMaxProtocols)
	destinationOrder := make([]string, 0, exfilTopicTopologyMaxDestinations)
	sourceSeen := make(map[string]struct{})
	protocolSeen := make(map[string]struct{})
	destinationSeen := make(map[string]struct{})
	sourceAggregates := make(map[string]*topicTopologyAggregate)
	protocolAggregates := make(map[string]*topicTopologyAggregate)
	destinationAggregates := make(map[string]*topicTopologyAggregate)
	linkOrder := make([]string, 0, exfilTopicTopologyMaxPaths*2)
	linkAggregates := make(map[string]*topicTopologyLinkAggregate)
	accepted := 0

	addLink := func(source, target string, path encryptedExfiltrationPathDTO) {
		key := source + "\x00" + target
		aggregate, ok := linkAggregates[key]
		if !ok {
			aggregate = &topicTopologyLinkAggregate{Source: source, Target: target}
			linkAggregates[key] = aggregate
			linkOrder = append(linkOrder, key)
		}
		aggregate.SessionCount += path.SessionCount
		aggregate.UploadBytes += path.UploadBytes
		aggregate.Risk = strongerTopicTopologyRisk(aggregate.Risk, path.Risk)
	}
	addNodeAggregate := func(aggregates map[string]*topicTopologyAggregate, key string, path encryptedExfiltrationPathDTO) {
		aggregate, ok := aggregates[key]
		if !ok {
			aggregate = &topicTopologyAggregate{}
			aggregates[key] = aggregate
		}
		aggregate.SessionCount += path.SessionCount
		aggregate.UploadBytes += path.UploadBytes
		aggregate.PathCount++
		aggregate.Risk = strongerTopicTopologyRisk(aggregate.Risk, path.Risk)
	}

	for _, path := range paths {
		if accepted >= exfilTopicTopologyMaxPaths {
			break
		}
		source := strings.TrimSpace(path.SrcIP)
		protocol := strings.TrimSpace(path.Protocol)
		destination := strings.TrimSpace(path.DstIP)
		if source == "" || protocol == "" || destination == "" {
			continue
		}
		if !canAppendUniqueBounded(sourceOrder, sourceSeen, source, exfilTopicTopologyMaxSources) ||
			!canAppendUniqueBounded(protocolOrder, protocolSeen, protocol, exfilTopicTopologyMaxProtocols) ||
			!canAppendUniqueBounded(destinationOrder, destinationSeen, destination, exfilTopicTopologyMaxDestinations) {
			continue
		}
		appendUniqueBounded(&sourceOrder, sourceSeen, source, exfilTopicTopologyMaxSources)
		appendUniqueBounded(&protocolOrder, protocolSeen, protocol, exfilTopicTopologyMaxProtocols)
		appendUniqueBounded(&destinationOrder, destinationSeen, destination, exfilTopicTopologyMaxDestinations)

		accepted++
		addNodeAggregate(sourceAggregates, source, path)
		addNodeAggregate(protocolAggregates, protocol, path)
		addNodeAggregate(destinationAggregates, destination, path)
		sourceID := stableTopicTopologyID("source", source)
		protocolID := stableTopicTopologyID("protocol", protocol)
		destinationID := stableTopicTopologyID("destination", destination)
		addLink(sourceID, protocolID, path)
		addLink(protocolID, destinationID, path)
	}

	if accepted == 0 {
		return []topicTopologyNodeDTO{}, []topicTopologyLinkDTO{}
	}

	nodes := make([]topicTopologyNodeDTO, 0, len(sourceOrder)+len(protocolOrder)+len(destinationOrder))
	for index, source := range sourceOrder {
		aggregate := sourceAggregates[source]
		nodes = append(nodes, topicTopologyNodeDTO{
			ID: stableTopicTopologyID("source", source), Label: source,
			Detail: fmt.Sprintf("%d 会话 / %d 路径", aggregate.SessionCount, aggregate.PathCount),
			X:      12, Y: topicTopologyAxisPosition(index, len(sourceOrder)),
			Tone: topicTopologyNodeTone(aggregate.Risk, "asset"), Width: 132, Height: 54, Icon: "server",
		})
	}
	for index, protocol := range protocolOrder {
		aggregate := protocolAggregates[protocol]
		nodes = append(nodes, topicTopologyNodeDTO{
			ID: stableTopicTopologyID("protocol", protocol), Label: protocol,
			Detail: fmt.Sprintf("%d 会话 / %d 路径", aggregate.SessionCount, aggregate.PathCount),
			X:      50, Y: topicTopologyAxisPosition(index, len(protocolOrder)),
			Tone: "protocol", Width: 124, Height: 54, Icon: "protocol",
		})
	}
	for index, destination := range destinationOrder {
		aggregate := destinationAggregates[destination]
		nodes = append(nodes, topicTopologyNodeDTO{
			ID: stableTopicTopologyID("destination", destination), Label: destination,
			Detail: fmt.Sprintf("%d 会话 / %d 路径", aggregate.SessionCount, aggregate.PathCount),
			X:      88, Y: topicTopologyAxisPosition(index, len(destinationOrder)),
			Tone: topicTopologyNodeTone(aggregate.Risk, "destination"), Width: 140, Height: 54, Icon: "global",
		})
	}

	links := make([]topicTopologyLinkDTO, 0, len(linkOrder))
	for _, key := range linkOrder {
		aggregate := linkAggregates[key]
		links = append(links, topicTopologyLinkDTO{
			Source: aggregate.Source, Target: aggregate.Target,
			Value: float64(aggregate.UploadBytes), Tone: topicTopologyLinkTone(aggregate.Risk),
			LineType: "solid", Label: fmt.Sprintf("%d 会话", aggregate.SessionCount),
		})
	}
	return nodes, links
}

type aptPhaseVisual struct {
	ID   string
	Tone string
	Icon string
}

func aptPhaseTopologyVisual(phase string) aptPhaseVisual {
	normalized := strings.ToLower(strings.TrimSpace(phase))
	normalized = strings.NewReplacer("-", "_", " ", "_").Replace(normalized)
	known := map[string]aptPhaseVisual{
		"reconnaissance":      {ID: "phase-recon", Tone: "protocol", Icon: "campaign"},
		"initial_access":      {ID: "phase-initial", Tone: "protocol", Icon: "initial"},
		"execution":           {ID: "phase-execute", Tone: "warn", Icon: "execute"},
		"persistence":         {ID: "phase-persistence", Tone: "warn", Icon: "persist"},
		"defense_evasion":     {ID: "phase-evasion", Tone: "warn", Icon: "evasion"},
		"credential_access":   {ID: "phase-credential", Tone: "warn", Icon: "credential"},
		"discovery":           {ID: "phase-discovery", Tone: "protocol", Icon: "discovery"},
		"lateral_movement":    {ID: "phase-lateral", Tone: "warn", Icon: "lateral"},
		"command_and_control": {ID: "phase-c2", Tone: "risk", Icon: "c2"},
		"exfiltration":        {ID: "phase-exfil", Tone: "risk", Icon: "exfil"},
	}
	if visual, ok := known[normalized]; ok {
		return visual
	}
	return aptPhaseVisual{ID: stableTopicTopologyID("phase", normalized), Tone: "protocol", Icon: "campaign"}
}

func buildAPTTopicTopology(campaigns []campaignDTO) ([]topicTopologyNodeDTO, []topicTopologyLinkDTO) {
	selected := campaigns
	if len(selected) > aptTopicTopologyMaxCampaigns {
		selected = selected[:aptTopicTopologyMaxCampaigns]
	}
	phaseOrder := make([]string, 0, aptTopicTopologyMaxPhases)
	entityOrder := make([]string, 0, aptTopicTopologyMaxEntities)
	evidenceOrder := make([]string, 0, aptTopicTopologyMaxEvidence)
	phaseSeen := make(map[string]struct{})
	entitySeen := make(map[string]struct{})
	evidenceSeen := make(map[string]struct{})
	links := make([]topicTopologyLinkDTO, 0, len(selected)*8)
	linkSeen := make(map[string]struct{})

	addLink := func(source, target, tone, label string, value float64) {
		key := source + "\x00" + target
		if _, ok := linkSeen[key]; ok {
			return
		}
		linkSeen[key] = struct{}{}
		links = append(links, topicTopologyLinkDTO{Source: source, Target: target, Value: value, Tone: tone, LineType: "solid", Label: label})
	}

	for _, campaign := range selected {
		campaignID := strings.TrimSpace(campaign.CampaignID)
		if campaignID == "" {
			continue
		}
		campaignNodeID := stableTopicTopologyID("campaign", campaignID)
		for _, phase := range campaign.AttackPhases {
			phase = strings.TrimSpace(phase)
			if !appendUniqueBounded(&phaseOrder, phaseSeen, phase, aptTopicTopologyMaxPhases) {
				continue
			}
			visual := aptPhaseTopologyVisual(phase)
			addLink(campaignNodeID, visual.ID, visual.Tone, "战役阶段", campaign.Score)
		}
		for _, entity := range campaign.Entities {
			entity = strings.TrimSpace(entity)
			if !appendUniqueBounded(&entityOrder, entitySeen, entity, aptTopicTopologyMaxEntities) {
				continue
			}
			addLink(campaignNodeID, stableTopicTopologyID("asset", entity), "info", "影响实体", campaign.Score)
		}
		for _, alertID := range campaign.Alerts {
			alertID = strings.TrimSpace(alertID)
			if !appendUniqueBounded(&evidenceOrder, evidenceSeen, alertID, aptTopicTopologyMaxEvidence) {
				continue
			}
			addLink(campaignNodeID, stableTopicTopologyID("evidence", alertID), "purple", "关联告警", campaign.Score)
		}
	}

	nodes := make([]topicTopologyNodeDTO, 0, len(selected)+len(phaseOrder)+len(entityOrder)+len(evidenceOrder))
	for index, campaign := range selected {
		campaignID := strings.TrimSpace(campaign.CampaignID)
		if campaignID == "" {
			continue
		}
		risk := "medium"
		if campaign.Score >= 0.75 {
			risk = "high"
		}
		nodes = append(nodes, topicTopologyNodeDTO{
			ID: stableTopicTopologyID("campaign", campaignID), Label: campaignID,
			Detail: fmt.Sprintf("%d 实体 / %d 告警", len(campaign.Entities), len(campaign.Alerts)),
			X:      topicTopologyAxisPosition(index, len(selected)), Y: 10,
			Tone: topicTopologyNodeTone(risk, "warn"), Width: 144, Height: 58, Icon: "campaign",
		})
	}
	for index, phase := range phaseOrder {
		visual := aptPhaseTopologyVisual(phase)
		nodes = append(nodes, topicTopologyNodeDTO{
			ID: visual.ID, Label: phase, Detail: "ATT&CK 阶段",
			X: topicTopologyAxisPosition(index, len(phaseOrder)), Y: 38,
			Tone: visual.Tone, Width: 124, Height: 54, Icon: visual.Icon,
		})
	}
	for index, entity := range entityOrder {
		nodes = append(nodes, topicTopologyNodeDTO{
			ID: stableTopicTopologyID("asset", entity), Label: entity, Detail: "战役影响实体",
			X: topicTopologyAxisPosition(index, len(entityOrder)), Y: 66,
			Tone: "asset", Width: 132, Height: 54, Icon: "server",
		})
	}
	for index, alertID := range evidenceOrder {
		nodes = append(nodes, topicTopologyNodeDTO{
			ID: stableTopicTopologyID("evidence", alertID), Label: alertID, Detail: "关联告警证据",
			X: topicTopologyAxisPosition(index, len(evidenceOrder)), Y: 90,
			Tone: "probe", Width: 132, Height: 54, Icon: "evidence",
		})
	}
	return nodes, links
}
