package repository

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/s3client"
)

func validTaskManifestForUnitTest() VersionedTaskManifest {
	payload := []byte(`{"manifest_version":1,"tenant_id":"tenant-a","task_id":"task-a","executable":false,"automatic_open":false}`)
	digest := sha256.Sum256(payload)
	return VersionedTaskManifest{
		TenantID: "tenant-a", TaskID: "task-a", ManifestSHA256: hex.EncodeToString(digest[:]),
		ManifestJSON: payload, Status: TaskStatusCompleted,
		ResultObject: s3client.ObjectAuthority{
			Bucket: "results", Key: "tenants/tenant-a/forensics/jobs/task-a/pcap/result.pcap",
			VersionID: "version-1", ETag: "etag-1", SizeBytes: 10, SHA256: hex.EncodeToString(digest[:]),
			ObservedAt: time.Now().UTC(), RetentionUntil: time.Now().UTC().Add(time.Hour),
		},
	}
}

func TestVersionedTaskManifestRejectsDigestIdentityAndUnsafeStatus(t *testing.T) {
	valid := validTaskManifestForUnitTest()
	if err := validateVersionedTaskManifest(valid); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
	for _, mutation := range []func(*VersionedTaskManifest){
		func(value *VersionedTaskManifest) { value.ManifestSHA256 = "bad" },
		func(value *VersionedTaskManifest) { value.ManifestJSON = []byte(`{}`) },
		func(value *VersionedTaskManifest) { value.Status = TaskStatusFailed },
		func(value *VersionedTaskManifest) { value.ResultObject.VersionID = "" },
	} {
		candidate := valid
		mutation(&candidate)
		if err := validateVersionedTaskManifest(candidate); err == nil {
			t.Fatalf("unsafe manifest accepted: %+v", candidate)
		}
	}
}

func TestTaskMutationCanCommitExplicitPartialTerminalState(t *testing.T) {
	task := &Task{Status: TaskStatusProcessing}
	if err := applyTaskMutation(task, taskMutation{Operation: "complete", Status: TaskStatusPartial}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if task.Status != TaskStatusPartial || task.Progress != 100 || task.CompletedAt == nil {
		t.Fatalf("partial terminal state not preserved: %+v", task)
	}
}
