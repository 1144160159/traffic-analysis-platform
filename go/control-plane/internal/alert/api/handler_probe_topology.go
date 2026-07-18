package api

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
)

type probeTopologyPointDTO struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type probeTopologyNodeDTO struct {
	ID            string                `json:"id"`
	ProbeID       string                `json:"probe_id"`
	Kind          string                `json:"kind"`
	Label         string                `json:"label"`
	Detail        string                `json:"detail"`
	Status        string                `json:"status"`
	Zone          string                `json:"zone"`
	Role          string                `json:"role"`
	BandwidthGbps float64               `json:"bandwidth_gbps"`
	Elevation     float64               `json:"elevation"`
	Position2D    probeTopologyPointDTO `json:"position_2d"`
	Position3D    probeTopologyPointDTO `json:"position_3d"`
}

type probeTopologyEdgeDTO struct {
	ID            string  `json:"id"`
	Source        string  `json:"source"`
	Target        string  `json:"target"`
	Kind          string  `json:"kind"`
	Status        string  `json:"status"`
	BandwidthGbps float64 `json:"bandwidth_gbps"`
}

type probeTopologyZoneDTO struct {
	ID        string                  `json:"id"`
	Label     string                  `json:"label"`
	Status    string                  `json:"status"`
	Polygon2D []probeTopologyPointDTO `json:"polygon_2d"`
	Polygon3D []probeTopologyPointDTO `json:"polygon_3d"`
}

type probeTopologyGraphDTO struct {
	Revision         string                 `json:"revision"`
	Source           string                 `json:"source"`
	ActiveMode       string                 `json:"active_mode"`
	CoordinateSystem string                 `json:"coordinate_system"`
	GeneratedAt      time.Time              `json:"generated_at"`
	Nodes            []probeTopologyNodeDTO `json:"nodes"`
	Edges            []probeTopologyEdgeDTO `json:"edges"`
	Zones            []probeTopologyZoneDTO `json:"zones"`
}

// GetProbeTopology returns a render-neutral graph. Both layouts are calculated
// by the API so the web client only maps graph coordinates into an SVG viewBox.
func (h *SystemHandler) GetProbeTopology(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.pgDB == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "postgres is not configured")
		return
	}
	if !h.requireProbeTopologyReadPermission(w, r) {
		return
	}
	mode := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mode")))
	if mode == "" {
		mode = "3d"
	}
	if mode != "2d" && mode != "3d" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_MODE", "mode must be 2d or 3d")
		return
	}

	rows, err := h.pgDB.QueryContext(ctx, `
		SELECT probe_id, name, status, hardware_info, software_version, last_heartbeat
		FROM probes
		WHERE tenant_id=$1
		ORDER BY probe_id ASC
		LIMIT 500`, queryTenantID(r))
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	defer rows.Close()

	probes := make([]probeDTO, 0)
	for rows.Next() {
		probe, scanErr := scanProbe(rows)
		if scanErr != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", scanErr.Error())
			return
		}
		probes = append(probes, probe)
	}
	if err := rows.Err(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}

	httpx.JSONSuccess(w, ctx, buildProbeTopologyGraph(probes, mode, time.Now().UTC()))
}

func buildProbeTopologyGraph(probes []probeDTO, mode string, generatedAt time.Time) probeTopologyGraphDTO {
	nodes := make([]probeTopologyNodeDTO, 0, len(probes))
	probeByID := make(map[string]probeDTO, len(probes))
	for _, probe := range probes {
		if probe.ProbeID == "" || probe.TopologyX <= 0 || probe.TopologyY <= 0 {
			continue
		}
		zone := strings.TrimSpace(probe.TopologyZone)
		if zone == "" {
			zone = "未分区"
		}
		role := strings.TrimSpace(probe.TopologyRole)
		if role == "" {
			role = "采集探针"
		}
		label := strings.TrimSpace(probe.Location)
		if label == "" {
			label = strings.TrimSpace(probe.Hostname)
		}
		if label == "" {
			label = probe.ProbeID
		}
		node := probeTopologyNodeDTO{
			ID: probe.ProbeID, ProbeID: probe.ProbeID, Kind: topologyNodeKind(role),
			Label: label, Detail: probe.ProbeID, Status: topologyTone(probe.Status), Zone: zone, Role: role,
			BandwidthGbps: probe.BandwidthMbps / 1000, Elevation: clampTopology(probe.TopologyZ, 1, 18),
			Position2D: probeTopologyPointDTO{X: clampTopology(probe.TopologyX, 4, 96), Y: clampTopology(probe.TopologyY, 5, 95)},
			Position3D: projectProbeTopology3D(probe.TopologyX, probe.TopologyY, probe.TopologyZ),
		}
		nodes = append(nodes, node)
		probeByID[probe.ProbeID] = probe
	}

	edgeByKey := make(map[string]probeTopologyEdgeDTO)
	for _, node := range nodes {
		probe := probeByID[node.ProbeID]
		for index, target := range probe.TopologyLinks {
			if _, ok := probeByID[target]; !ok || target == node.ID {
				continue
			}
			left, right := node.ID, target
			if left > right {
				left, right = right, left
			}
			key := left + "::" + right
			bandwidth := 0.0
			if index < len(probe.TopologyLinkBandwidths) {
				bandwidth = probe.TopologyLinkBandwidths[index]
			}
			status := worstTopologyTone(node.Status, topologyTone(probeByID[target].Status))
			if existing, ok := edgeByKey[key]; ok {
				existing.BandwidthGbps = math.Max(existing.BandwidthGbps, bandwidth)
				existing.Kind = topologyEdgeKind(existing.BandwidthGbps)
				existing.Status = worstTopologyTone(existing.Status, status)
				edgeByKey[key] = existing
				continue
			}
			edgeByKey[key] = probeTopologyEdgeDTO{
				ID: fmt.Sprintf("edge-%s-%s", left, right), Source: left, Target: right,
				Kind: topologyEdgeKind(bandwidth), Status: status, BandwidthGbps: bandwidth,
			}
		}
	}
	edges := make([]probeTopologyEdgeDTO, 0, len(edgeByKey))
	for _, edge := range edgeByKey {
		edges = append(edges, edge)
	}

	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	spreadProbeTopologyPositions(nodes, false)
	spreadProbeTopologyPositions(nodes, true)
	normalizeProbeTopologyPositions(nodes, false)
	normalizeProbeTopologyPositions(nodes, true)
	ensureProbeTopologySpacing(nodes, false, 7)
	ensureProbeTopologySpacing(nodes, true, 7)
	sort.Slice(edges, func(i, j int) bool { return edges[i].ID < edges[j].ID })
	return probeTopologyGraphDTO{
		Revision: fmt.Sprintf("probe-topology-%d-%d", len(nodes), generatedAt.Unix()),
		Source:   "postgres.probes.hardware_info", ActiveMode: mode, CoordinateSystem: "normalized-0-100",
		GeneratedAt: generatedAt, Nodes: nodes, Edges: edges, Zones: buildProbeTopologyZones(nodes),
	}
}

func (h *SystemHandler) requireProbeTopologyReadPermission(w http.ResponseWriter, r *http.Request) bool {
	ctx := r.Context()
	if hasAnySystemPermission(ctx, authmodel.ScopeProbeRead, authmodel.ScopeProbeWrite, authmodel.ScopeAdminAll) {
		return true
	}
	httpx.JSONError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "permission denied: probe:read required")
	return false
}

func normalizeProbeTopologyPositions(nodes []probeTopologyNodeDTO, use3D bool) {
	if len(nodes) < 2 {
		return
	}
	minX, maxX, minY, maxY := topologyBounds(nodes, use3D)
	if maxX-minX < 1 || maxY-minY < 1 {
		return
	}
	for index := range nodes {
		point := &nodes[index].Position2D
		minTargetY, maxTargetY := 11.0, 89.0
		if use3D {
			point = &nodes[index].Position3D
			minTargetY, maxTargetY = 14, 86
		}
		point.X = 8 + ((point.X - minX) / (maxX - minX) * 84)
		point.Y = minTargetY + ((point.Y - minY) / (maxY - minY) * (maxTargetY - minTargetY))
	}
}

func spreadProbeTopologyPositions(nodes []probeTopologyNodeDTO, use3D bool) {
	groups := map[string][]int{}
	for index, node := range nodes {
		point := node.Position2D
		if use3D {
			point = node.Position3D
		}
		key := fmt.Sprintf("%.1f:%.1f", point.X, point.Y)
		groups[key] = append(groups[key], index)
	}
	for _, indexes := range groups {
		if len(indexes) < 2 {
			continue
		}
		radius := math.Min(7, 2.8+float64(len(indexes))*0.55)
		for offset, index := range indexes {
			angle := (2 * math.Pi * float64(offset)) / float64(len(indexes))
			if use3D {
				nodes[index].Position3D.X = clampTopology(nodes[index].Position3D.X+math.Cos(angle)*radius, 4, 96)
				nodes[index].Position3D.Y = clampTopology(nodes[index].Position3D.Y+math.Sin(angle)*radius*.72, 5, 95)
			} else {
				nodes[index].Position2D.X = clampTopology(nodes[index].Position2D.X+math.Cos(angle)*radius, 4, 96)
				nodes[index].Position2D.Y = clampTopology(nodes[index].Position2D.Y+math.Sin(angle)*radius, 5, 95)
			}
		}
	}
}

// ensureProbeTopologySpacing relaxes near-colliding nodes after normalization.
// The deterministic fallback angle keeps identical inputs stable across calls.
func ensureProbeTopologySpacing(nodes []probeTopologyNodeDTO, use3D bool, minDistance float64) {
	for pass := 0; pass < 24; pass++ {
		moved := false
		for left := 0; left < len(nodes); left++ {
			for right := left + 1; right < len(nodes); right++ {
				leftPoint, rightPoint := &nodes[left].Position2D, &nodes[right].Position2D
				if use3D {
					leftPoint, rightPoint = &nodes[left].Position3D, &nodes[right].Position3D
				}
				dx, dy := rightPoint.X-leftPoint.X, rightPoint.Y-leftPoint.Y
				distance := math.Hypot(dx, dy)
				if distance >= minDistance {
					continue
				}
				if distance < .001 {
					angle := float64((left+1)*37+(right+1)*53) * math.Pi / 180
					dx, dy, distance = math.Cos(angle), math.Sin(angle), 1
				}
				push := (minDistance-distance)/2 + .02
				unitX, unitY := dx/distance, dy/distance
				leftPoint.X = clampTopology(leftPoint.X-unitX*push, 4, 96)
				leftPoint.Y = clampTopology(leftPoint.Y-unitY*push, 6, 94)
				rightPoint.X = clampTopology(rightPoint.X+unitX*push, 4, 96)
				rightPoint.Y = clampTopology(rightPoint.Y+unitY*push, 6, 94)
				moved = true
			}
		}
		if !moved {
			return
		}
	}
}

func buildProbeTopologyZones(nodes []probeTopologyNodeDTO) []probeTopologyZoneDTO {
	groups := map[string][]probeTopologyNodeDTO{}
	for _, node := range nodes {
		groups[node.Zone] = append(groups[node.Zone], node)
	}
	labels := make([]string, 0, len(groups))
	for label := range groups {
		labels = append(labels, label)
	}
	sort.Strings(labels)
	zones := make([]probeTopologyZoneDTO, 0, len(labels))
	for _, label := range labels {
		members := groups[label]
		min2X, max2X, min2Y, max2Y := topologyBounds(members, false)
		min3X, max3X, min3Y, max3Y := topologyBounds(members, true)
		status := "ok"
		for _, member := range members {
			status = worstTopologyTone(status, member.Status)
		}
		zones = append(zones, probeTopologyZoneDTO{
			ID: topologySlug(label), Label: label, Status: status,
			Polygon2D: topologyRectPolygon(min2X-7, max2X+7, min2Y-6, max2Y+6),
			Polygon3D: topologyDiamondPolygon(min3X-8, max3X+8, min3Y-6, max3Y+6),
		})
	}
	return zones
}

func topologyBounds(nodes []probeTopologyNodeDTO, use3D bool) (float64, float64, float64, float64) {
	minX, maxX, minY, maxY := 100.0, 0.0, 100.0, 0.0
	for _, node := range nodes {
		point := node.Position2D
		if use3D {
			point = node.Position3D
		}
		if point.X < minX {
			minX = point.X
		}
		if point.X > maxX {
			maxX = point.X
		}
		if point.Y < minY {
			minY = point.Y
		}
		if point.Y > maxY {
			maxY = point.Y
		}
	}
	return minX, maxX, minY, maxY
}

func topologyRectPolygon(minX, maxX, minY, maxY float64) []probeTopologyPointDTO {
	minX, maxX = clampTopology(minX, 2, 98), clampTopology(maxX, 2, 98)
	minY, maxY = clampTopology(minY, 2, 98), clampTopology(maxY, 2, 98)
	return []probeTopologyPointDTO{{X: minX, Y: minY}, {X: maxX, Y: minY}, {X: maxX, Y: maxY}, {X: minX, Y: maxY}}
}

func topologyDiamondPolygon(minX, maxX, minY, maxY float64) []probeTopologyPointDTO {
	minX, maxX = clampTopology(minX, 2, 98), clampTopology(maxX, 2, 98)
	minY, maxY = clampTopology(minY, 2, 98), clampTopology(maxY, 2, 98)
	return []probeTopologyPointDTO{{X: minX + 4, Y: minY}, {X: maxX, Y: minY + 3}, {X: maxX - 4, Y: maxY}, {X: minX, Y: maxY - 3}}
}

func projectProbeTopology3D(x, y, z float64) probeTopologyPointDTO {
	return probeTopologyPointDTO{
		X: clampTopology(14+x*0.68+(y-50)*0.18, 5, 95),
		Y: clampTopology(15+y*0.62-x*0.08-z*1.1, 6, 94),
	}
}

func topologyTone(status string) string {
	value := strings.ToLower(strings.TrimSpace(status))
	switch {
	case strings.Contains(value, "离线"), strings.Contains(value, "offline"), strings.Contains(value, "error"), strings.Contains(value, "risk"):
		return "risk"
	case strings.Contains(value, "告警"), strings.Contains(value, "degrad"), strings.Contains(value, "warn"):
		return "warn"
	default:
		return "ok"
	}
}

func worstTopologyTone(left, right string) string {
	rank := map[string]int{"ok": 0, "info": 1, "warn": 2, "risk": 3}
	if rank[right] > rank[left] {
		return right
	}
	return left
}

func topologyNodeKind(role string) string {
	value := strings.ToLower(role)
	if strings.Contains(value, "核心") || strings.Contains(value, "core") {
		return "core"
	}
	if strings.Contains(value, "汇聚") || strings.Contains(value, "switch") {
		return "switch"
	}
	if strings.Contains(value, "镜像") || strings.Contains(value, "mirror") {
		return "mirror"
	}
	return "probe"
}

func topologyEdgeKind(bandwidth float64) string {
	if bandwidth >= 40 {
		return "backbone"
	}
	if bandwidth >= 10 {
		return "uplink"
	}
	return "access"
}

func topologySlug(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.NewReplacer(" ", "-", "/", "-", "\\", "-", ".", "-").Replace(value)
	if value == "" {
		return "zone"
	}
	return value
}

func clampTopology(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
