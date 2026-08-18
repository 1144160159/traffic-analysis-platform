package converter

import (
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/model"
)

func TestCommandToProtoPreservesStableCommandIdentityAndVersion(t *testing.T) {
	cmd := &model.RuleCommand{
		EventID:    "11111111-1111-4111-8111-111111111111",
		Action:     "update",
		Timestamp:  time.UnixMilli(1720000000000),
		OperatorID: "operator-1",
		TraceID:    "trace-1",
		RequestID:  "request-1",
		Version:    7,
		Rule: &model.Rule{
			RuleID: "rule-1", TenantID: "tenant-1", Name: "known 攻击",
			Type: "port_scan", Engine: "internal", Description: "fixture <wire>",
			Conditions: map[string]interface{}{
				"threshold": 1.25,
				"ports":     []interface{}{80, 443},
			},
			Labels: []string{"known", "scan"}, Severity: "high",
			Version: 7, Priority: 60, Enabled: true, CreatedBy: "operator-1",
			CreatedAt: time.UnixMilli(1720000000000),
			UpdatedAt: time.UnixMilli(1720000001000),
		},
	}

	first := CommandToProto(cmd)
	retry := CommandToProto(cmd)

	if first.EventID != cmd.EventID || retry.EventID != cmd.EventID {
		t.Fatalf("retry identity drifted: first=%q retry=%q", first.EventID, retry.EventID)
	}
	if first.Version != 7 || first.RuleVersion != "v7" {
		t.Fatalf("version contract drifted: version=%d rule_version=%q", first.Version, first.RuleVersion)
	}
	if first.TraceID != "trace-1" || first.RequestID != "request-1" {
		t.Fatalf("trace/request metadata was not preserved: %+v", first)
	}
	if first.SchemaVersion != "1.1" || first.ChecksumAlgorithm != RuleChecksumAlgorithm {
		t.Fatalf("wire checksum contract drifted: %+v", first)
	}
	if first.Checksum != "b7b368d7a2b6544d5bf95e0b347ff9a5" {
		t.Fatalf("cross-language wire checksum drifted: %s", first.Checksum)
	}

	roundTrip := ProtoToCommand(first)
	if roundTrip.EventID != cmd.EventID || roundTrip.Version != 7 || roundTrip.TraceID != "trace-1" {
		t.Fatalf("command round trip lost identity: %+v", roundTrip)
	}
}
