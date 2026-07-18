package api

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/repository"
)

func TestHasForensicsPermission(t *testing.T) {
	tests := []struct {
		name        string
		permissions []string
		required    string
		want        bool
	}{
		{name: "exact", permissions: []string{"pcap:read"}, required: "pcap:read", want: true},
		{name: "pcap wildcard", permissions: []string{"pcap:*"}, required: "pcap:download", want: true},
		{name: "admin wildcard", permissions: []string{"admin:*"}, required: "pcap:write", want: true},
		{name: "global wildcard", permissions: []string{"*"}, required: "pcap:write", want: true},
		{name: "read does not grant write", permissions: []string{"pcap:read"}, required: "pcap:write", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), httpx.ContextKeyPermissions, test.permissions)
			if got := hasForensicsPermission(ctx, test.required); got != test.want {
				t.Fatalf("hasForensicsPermission() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestFilterFixtureJobsUsesStructuredFields(t *testing.T) {
	jobs := []json.RawMessage{
		json.RawMessage(`{"job_id":"F-20260620-000128","status":"completed","params":{"asset_id":"asset-a","src_ip":"172.16.5.10","dst_ip":"185.22.14.9","src_port":44221,"dst_port":443,"protocol":"TLS"}}`),
		json.RawMessage(`{"job_id":"F-20260620-000127","status":"completed","params":{"asset_id":"asset-b","src_ip":"172.16.5.11","dst_ip":"8.8.8.8","src_port":53001,"dst_port":53,"protocol":"DNS"}}`),
	}

	tests := []struct {
		name   string
		filter repository.TaskListFilter
		wantID string
	}{
		{name: "source IP", filter: repository.TaskListFilter{SrcIP: "172.16.5.10"}, wantID: "F-20260620-000128"},
		{name: "destination port", filter: repository.TaskListFilter{Port: "53"}, wantID: "F-20260620-000127"},
		{name: "protocol case insensitive", filter: repository.TaskListFilter{Protocol: "tls"}, wantID: "F-20260620-000128"},
		{name: "tuple", filter: repository.TaskListFilter{Tuple: "172.16.5.10:44221 -> 185.22.14.9:443 TLS"}, wantID: "F-20260620-000128"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			filtered := filterFixtureJobs(jobs, test.filter)
			if len(filtered) != 1 {
				t.Fatalf("filterFixtureJobs() returned %d rows, want 1", len(filtered))
			}
			var job struct {
				JobID string `json:"job_id"`
			}
			if err := json.Unmarshal(filtered[0], &job); err != nil {
				t.Fatal(err)
			}
			if job.JobID != test.wantID {
				t.Fatalf("job id = %q, want %q", job.JobID, test.wantID)
			}
		})
	}
}
