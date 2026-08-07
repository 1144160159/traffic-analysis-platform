package persistence

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

// TestAlertWriterRealStrictOpenSearchMapping is accepted only against the
// owned, loopback-only sentinel cluster created by the alignment runner. It
// proves both single and bulk production writers are accepted by the exact v2
// strict mapping; it does not create or mutate index schema itself.
func TestAlertWriterRealStrictOpenSearchMapping(t *testing.T) {
	baseURL := strings.TrimRight(os.Getenv("ALERT_PERSISTENCE_EPHEMERAL_OS_URL"), "/")
	if baseURL == "" {
		t.Skip("ALERT_PERSISTENCE_EPHEMERAL_OS_URL is not set")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	host, _, err := net.SplitHostPort(parsed.Host)
	if err != nil || parsed.Scheme != "http" || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		t.Fatalf("refusing non-loopback OpenSearch endpoint %q: %v", baseURL, err)
	}
	if os.Getenv("ALERT_PERSISTENCE_EPHEMERAL_OS_SENTINEL") != "ephemeral-only" {
		t.Fatal("refusing OpenSearch endpoint without explicit ephemeral sentinel")
	}
	response, err := http.Get(baseURL + "/codex-ephemeral-asset-projection-sentinel/_doc/ephemeral-only")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var sentinel struct {
		Found  bool `json:"found"`
		Source struct {
			Marker string `json:"marker"`
		} `json:"_source"`
	}
	if err := json.NewDecoder(response.Body).Decode(&sentinel); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !sentinel.Found || sentinel.Source.Marker != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel OpenSearch: status=%s sentinel=%+v", response.Status, sentinel)
	}

	writer, err := NewOpenSearchWriter([]string{baseURL}, "", "", "alerts-v2-write", true, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	base := time.Date(2026, 8, 7, 11, 30, 0, 123_000_000, time.UTC)
	makeAlert := func(id string, offset time.Duration) *Alert {
		observed := base.Add(offset)
		return &Alert{
			TenantID: "tenant-alert-os-g1", AlertID: id, Fingerprint: "fingerprint-" + id,
			CommunityID: "community-" + id, SessionID: "session-" + id,
			SrcIP: "192.0.2.91", DstIP: "203.0.113.91", SrcPort: 49001, DstPort: 443,
			Protocol: 6, AlertType: "strict-mapping-regression", Labels: []string{"integration"},
			Score: 0.91, Severity: "high", FirstSeen: observed.Add(-time.Second), LastSeen: observed,
			Count: 1, Status: "new", Assignee: "alignment-runner", UpdatedTs: observed,
			StateVersion: 9, ModelVersion: "model-g1", RuleVersion: "rule-g1", FeatureSetID: "feature-g1",
			EvidenceIDs: []string{"evidence-" + id}, EventID: "event-" + id,
			TraceID: "0123456789abcdef0123456789abcdef",
		}
	}
	single := makeAlert("single", 0)
	if err := writer.WriteAlert(context.Background(), single); err != nil {
		t.Fatal(err)
	}
	batch := []*Alert{makeAlert("batch-a", time.Second), makeAlert("batch-b", 2*time.Second)}
	if err := writer.WriteBatch(context.Background(), batch); err != nil {
		t.Fatal(err)
	}
	for _, expected := range append([]*Alert{single}, batch...) {
		response, err := http.Get(baseURL + "/alerts-v2-read/_doc/" + expected.AlertID)
		if err != nil {
			t.Fatal(err)
		}
		var stored struct {
			Found   bool  `json:"found"`
			Version int64 `json:"_version"`
			Source  Alert `json:"_source"`
		}
		decodeErr := json.NewDecoder(response.Body).Decode(&stored)
		response.Body.Close()
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		if response.StatusCode != http.StatusOK || !stored.Found ||
			stored.Version != AlertSourceVersion(expected) ||
			stored.Source.AlertID != expected.AlertID || stored.Source.EventID != expected.EventID ||
			stored.Source.TraceID != expected.TraceID || stored.Source.StateVersion != expected.StateVersion ||
			!stored.Source.UpdatedTs.Equal(expected.UpdatedTs) {
			t.Fatalf("unexpected strict-mapping document status=%s stored=%+v", response.Status, stored)
		}
	}
}
