package consumer

import (
	"context"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/threatintel"
)

// ThreatIntelAdapter 适配 threatintel.Service 到 ThreatIntelEnricher 接口
type ThreatIntelAdapter struct {
	svc *threatintel.Service
}

func NewThreatIntelAdapter(svc *threatintel.Service) *ThreatIntelAdapter {
	return &ThreatIntelAdapter{svc: svc}
}

func (a *ThreatIntelAdapter) EnrichAlert(ctx context.Context, srcIP, dstIP string) *ThreatEnrichment {
	enrich := a.svc.EnrichAlert(ctx, srcIP, dstIP)
	if enrich == nil {
		return nil
	}
	return &ThreatEnrichment{
		IPs:       convertReputationMap(enrich.IPs),
		Tags:      enrich.Tags,
		RiskScore: enrich.RiskScore,
	}
}

func convertReputationMap(ips map[string]threatintel.Reputation) map[string]string {
	result := make(map[string]string, len(ips))
	for ip, rep := range ips {
		result[ip] = string(rep)
	}
	return result
}
