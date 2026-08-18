package s3client

import (
	"testing"

	"github.com/minio/minio-go/v7"

	commonerrors "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

func TestForensicsResultKeyIsTenantAndTaskDerived(t *testing.T) {
	valid := "tenants/tenant-a/forensics/jobs/task-a/pcap/result.pcap"
	if err := validateForensicsResultKey(valid, "tenant-a", "task-a"); err != nil {
		t.Fatalf("valid key rejected: %v", err)
	}
	for _, key := range []string{
		"tenants/tenant-b/forensics/jobs/task-a/pcap/result.pcap",
		"tenants/tenant-a/forensics/jobs/task-b/pcap/result.pcap",
		"tenants/tenant-a/forensics/jobs/task-a/../result.pcap",
		"/tenants/tenant-a/forensics/jobs/task-a/pcap/result.pcap",
	} {
		if err := validateForensicsResultKey(key, "tenant-a", "task-a"); err == nil {
			t.Fatalf("unsafe key accepted: %q", key)
		}
	}
}

func TestForensicsObjectReadErrorPreservesMissingVersionState(t *testing.T) {
	err := forensicsObjectReadError("HEAD exact version", minio.ErrorResponse{Code: "NoSuchVersion"})
	if !commonerrors.IsCode(err, commonerrors.ErrCodeResourceNotFound) {
		t.Fatalf("missing version was not mapped to resource-not-found: %v", err)
	}
	err = forensicsObjectReadError("HEAD exact version", minio.ErrorResponse{Code: "AccessDenied"})
	if commonerrors.IsCode(err, commonerrors.ErrCodeResourceNotFound) {
		t.Fatalf("access denial was collapsed into resource-not-found: %v", err)
	}
}
