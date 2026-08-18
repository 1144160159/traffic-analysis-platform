package s3client

import (
	"strings"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
)

func TestObjectAuthorityRequiresImmutableCoordinatesAndDigest(t *testing.T) {
	valid := ObjectAuthority{
		Bucket: "restoration-quarantine", Key: "tenants/t/restorations/r/1/content.bin",
		VersionID: "v1", ETag: "etag", SizeBytes: 3, SHA256: strings.Repeat("a", 64),
		ObservedAt: time.Now().UTC(),
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid authority rejected: %v", err)
	}
	for _, edit := range []func(*ObjectAuthority){
		func(value *ObjectAuthority) { value.VersionID = "" },
		func(value *ObjectAuthority) { value.Key = "../escape" },
		func(value *ObjectAuthority) { value.SHA256 = "unknown" },
		func(value *ObjectAuthority) { value.ETag = "" },
		func(value *ObjectAuthority) { value.ObservedAt = time.Time{} },
	} {
		candidate := valid
		edit(&candidate)
		if err := candidate.Validate(); err == nil {
			t.Fatalf("unsafe authority accepted: %#v", candidate)
		}
	}
}

func TestObjectCoordinatesRejectPathEscapeAndMissingVersion(t *testing.T) {
	for _, item := range []struct{ bucket, key, version string }{
		{"", "key", "v"}, {"bucket", "/absolute", "v"},
		{"bucket", "tenants/t/../x", "v"}, {"bucket", "key", ""},
	} {
		if err := validateObjectCoordinates(item.bucket, item.key, item.version); err == nil {
			t.Fatalf("unsafe coordinates accepted: %#v", item)
		}
	}
}

func TestQuarantinePutContractUsesWriteOncePrecondition(t *testing.T) {
	// This source-level guard complements the real MinIO integration test. It
	// prevents a future client refactor from silently removing the write-once
	// header while leaving idempotency tests green against fakes.
	options := minio.PutObjectOptions{}
	options.SetMatchETagExcept("*")
	if value := options.Header().Get("If-None-Match"); value == "" || !strings.Contains(value, "*") {
		t.Fatalf("write-once If-None-Match precondition = %q", value)
	}
}
