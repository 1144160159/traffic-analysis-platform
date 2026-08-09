package server

import (
	"strings"
	"testing"

	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

func TestBindPcapIndexIdentityUsesAuthenticatedContext(t *testing.T) {
	meta := &pb.PcapIndexMeta{}
	if err := bindPcapIndexIdentity(meta, "tenant-a", "probe-a"); err != nil {
		t.Fatalf("bindPcapIndexIdentity: %v", err)
	}
	if meta.TenantId != "tenant-a" || meta.ProbeId != "probe-a" {
		t.Fatalf("bound identity=%s/%s, want tenant-a/probe-a", meta.TenantId, meta.ProbeId)
	}
}

func TestBindPcapIndexIdentityRejectsTenantOrProbeMismatch(t *testing.T) {
	for name, meta := range map[string]*pb.PcapIndexMeta{
		"tenant": {TenantId: "tenant-b", ProbeId: "probe-a"},
		"probe":  {TenantId: "tenant-a", ProbeId: "probe-b"},
	} {
		t.Run(name, func(t *testing.T) {
			err := bindPcapIndexIdentity(meta, "tenant-a", "probe-a")
			if err == nil || !strings.Contains(err.Error(), "does not match authenticated") {
				t.Fatalf("bindPcapIndexIdentity error=%v, want authenticated mismatch", err)
			}
		})
	}
}

func TestPcapSuccessMessageIsNonFinalKafkaReceipt(t *testing.T) {
	if !strings.Contains(msgPcapKafkaAccepted, "Kafka") ||
		!strings.Contains(msgPcapKafkaAccepted, "downstream index pending") {
		t.Fatalf("PCAP ACK message is ambiguous: %q", msgPcapKafkaAccepted)
	}
}
