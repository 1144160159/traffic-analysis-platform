package s3client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
)

var restorationSHA256 = regexp.MustCompile(`^[0-9a-f]{64}$`)

var ErrQuarantineObjectConflict = errors.New("quarantine object authority conflicts with expected restoration content")

type ObjectAuthority struct {
	Bucket         string    `json:"bucket"`
	Key            string    `json:"object_key"`
	VersionID      string    `json:"object_version"`
	ETag           string    `json:"etag"`
	SizeBytes      int64     `json:"size_bytes"`
	SHA256         string    `json:"sha256"`
	ObservedAt     time.Time `json:"receipt_observed_at"`
	RetentionUntil time.Time `json:"retention_until"`
	LegalHold      bool      `json:"legal_hold"`
}

// VerifyRestorationBucket proves the quarantine bucket was provisioned by the
// storage control plane with both versioning and object lock. Runtime code must
// not create or mutate this governed bucket.
func (c *S3Client) VerifyRestorationBucket(ctx context.Context, bucket string) error {
	if strings.TrimSpace(bucket) == "" {
		return errors.New("restoration bucket is required")
	}
	exists, err := c.client.BucketExists(ctx, bucket)
	if err != nil {
		return fmt.Errorf("check restoration bucket: %w", err)
	}
	if !exists {
		return errors.New("restoration bucket is not provisioned")
	}
	versioning, err := c.client.GetBucketVersioning(ctx, bucket)
	if err != nil {
		return fmt.Errorf("read restoration bucket versioning: %w", err)
	}
	if !versioning.Enabled() {
		return errors.New("restoration bucket versioning is not enabled")
	}
	objectLock, _, _, _, err := c.client.GetObjectLockConfig(ctx, bucket)
	if err != nil {
		return fmt.Errorf("read restoration bucket object lock: %w", err)
	}
	if objectLock != "Enabled" {
		return errors.New("restoration bucket object lock is not enabled")
	}
	return nil
}

func validateObjectCoordinates(bucket, key, version string) error {
	if strings.TrimSpace(bucket) == "" || strings.TrimSpace(key) == "" || strings.TrimSpace(version) == "" {
		return errors.New("bucket, object key and version ID are required")
	}
	if strings.ContainsAny(bucket+key+version, "\r\n\x00") || strings.Contains(key, "..") || strings.HasPrefix(key, "/") {
		return errors.New("object coordinates are unsafe")
	}
	return nil
}

func (authority ObjectAuthority) Validate() error {
	if err := validateObjectCoordinates(authority.Bucket, authority.Key, authority.VersionID); err != nil {
		return err
	}
	if strings.TrimSpace(authority.ETag) == "" || authority.SizeBytes < 0 || !restorationSHA256.MatchString(authority.SHA256) {
		return errors.New("object authority lacks etag, size or lowercase SHA-256")
	}
	if authority.ObservedAt.IsZero() {
		return errors.New("object authority lacks observation time")
	}
	return nil
}

func sha256Bytes(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func objectMetadataValue(info minio.ObjectInfo, key string) string {
	if value := info.UserMetadata[key]; value != "" {
		return value
	}
	if value := info.UserMetadata["X-Amz-Meta-"+key]; value != "" {
		return value
	}
	return info.Metadata.Get("X-Amz-Meta-" + key)
}

func validateQuarantineCoordinates(bucket, key, tenantID, restorationID string) error {
	if strings.TrimSpace(bucket) == "" || strings.TrimSpace(tenantID) == "" || strings.TrimSpace(restorationID) == "" {
		return errors.New("quarantine bucket, tenant and restoration identity are required")
	}
	expectedPrefix := "tenants/" + tenantID + "/restorations/" + restorationID + "/"
	if !strings.HasPrefix(key, expectedPrefix) || strings.Contains(key, "..") || strings.ContainsAny(key, "\r\n\x00") {
		return errors.New("quarantine object key is not derived from tenant/restoration identity")
	}
	return nil
}

// ReadVerifiedObject pins both GET and HEAD to the immutable source version.
// It never falls back to the latest object and never returns bytes before the
// complete size, ETag and SHA authority has been checked.
func (c *S3Client) ReadVerifiedObject(
	ctx context.Context,
	bucket, key, version, expectedETag, expectedSHA string,
	expectedSize, maxSourceBytes int64,
) ([]byte, ObjectAuthority, error) {
	if err := validateObjectCoordinates(bucket, key, version); err != nil {
		return nil, ObjectAuthority{}, err
	}
	if expectedSize <= 0 || maxSourceBytes <= 0 || expectedSize > maxSourceBytes {
		return nil, ObjectAuthority{}, errors.New("source object size is absent or exceeds max_source_bytes")
	}
	if strings.TrimSpace(expectedETag) == "" || !restorationSHA256.MatchString(expectedSHA) {
		return nil, ObjectAuthority{}, errors.New("source object ETag or SHA-256 is invalid")
	}
	opts := minio.GetObjectOptions{VersionID: version, Checksum: true}
	info, err := c.client.StatObject(ctx, bucket, key, opts)
	if err != nil {
		return nil, ObjectAuthority{}, fmt.Errorf("stat immutable source object: %w", err)
	}
	if info.VersionID != version || info.Size != expectedSize || strings.Trim(info.ETag, `"`) != strings.Trim(expectedETag, `"`) {
		return nil, ObjectAuthority{}, errors.New("source object HEAD differs from index authority")
	}
	object, err := c.client.GetObject(ctx, bucket, key, opts)
	if err != nil {
		return nil, ObjectAuthority{}, fmt.Errorf("get immutable source object: %w", err)
	}
	defer object.Close()
	value, err := io.ReadAll(io.LimitReader(object, maxSourceBytes+1))
	if err != nil {
		return nil, ObjectAuthority{}, fmt.Errorf("read immutable source object: %w", err)
	}
	if int64(len(value)) != expectedSize || int64(len(value)) > maxSourceBytes {
		return nil, ObjectAuthority{}, errors.New("source object body size differs from authority")
	}
	if actual := sha256Bytes(value); actual != expectedSHA {
		return nil, ObjectAuthority{}, errors.New("source object SHA-256 differs from authority")
	}
	authority := ObjectAuthority{
		Bucket: bucket, Key: key, VersionID: version, ETag: strings.Trim(info.ETag, `"`),
		SizeBytes: info.Size, SHA256: expectedSHA, ObservedAt: time.Now().UTC(),
	}
	if err := authority.Validate(); err != nil {
		return nil, ObjectAuthority{}, err
	}
	return value, authority, nil
}

// FindQuarantineObject recovers the exact latest immutable version after a
// PUT-before-database crash. An existing key with different authority is a hard
// conflict and is never overwritten by a retry.
func (c *S3Client) FindQuarantineObject(
	ctx context.Context,
	bucket, key, tenantID, restorationID, expectedSHA string,
	expectedSize int64,
) (ObjectAuthority, bool, error) {
	if err := validateQuarantineCoordinates(bucket, key, tenantID, restorationID); err != nil {
		return ObjectAuthority{}, false, err
	}
	if !restorationSHA256.MatchString(expectedSHA) || expectedSize < 0 {
		return ObjectAuthority{}, false, errors.New("expected quarantine digest or size is invalid")
	}
	info, err := c.client.StatObject(ctx, bucket, key, minio.StatObjectOptions{Checksum: true})
	if err != nil {
		switch minio.ToErrorResponse(err).Code {
		case "NoSuchKey", "NoSuchObject", "NotFound":
			return ObjectAuthority{}, false, nil
		default:
			return ObjectAuthority{}, false, fmt.Errorf("HEAD existing quarantine object: %w", err)
		}
	}
	metadataSHA := objectMetadataValue(info, "sha256")
	metadataTenant := objectMetadataValue(info, "tenant-id")
	metadataRestoration := objectMetadataValue(info, "restoration-id")
	metadataQuarantined := objectMetadataValue(info, "quarantined")
	if strings.TrimSpace(info.VersionID) == "" || strings.TrimSpace(info.ETag) == "" ||
		info.Size != expectedSize || metadataSHA != expectedSHA || metadataTenant != tenantID ||
		metadataRestoration != restorationID || metadataQuarantined != "true" {
		return ObjectAuthority{}, false, ErrQuarantineObjectConflict
	}
	mode, retainedUntil, err := c.client.GetObjectRetention(ctx, bucket, key, info.VersionID)
	if err != nil {
		return ObjectAuthority{}, false, fmt.Errorf("read existing quarantine object retention: %w", err)
	}
	if mode == nil || (*mode != minio.Governance && *mode != minio.Compliance) || retainedUntil == nil || !retainedUntil.After(time.Now().UTC()) {
		return ObjectAuthority{}, false, ErrQuarantineObjectConflict
	}
	authority := ObjectAuthority{
		Bucket: bucket, Key: key, VersionID: info.VersionID, ETag: strings.Trim(info.ETag, `"`),
		SizeBytes: info.Size, SHA256: expectedSHA, ObservedAt: time.Now().UTC(),
		RetentionUntil: retainedUntil.UTC(),
	}
	if err := authority.Validate(); err != nil {
		return ObjectAuthority{}, false, err
	}
	return authority, true, nil
}

// PutQuarantineObject writes inert bytes to an already-derived tenant key and
// obtains an exact versioned HEAD receipt. Missing bucket versioning is a hard
// failure because an unversioned object cannot be restoration authority.
func (c *S3Client) PutQuarantineObject(
	ctx context.Context,
	bucket, key, tenantID, restorationID string,
	content []byte,
	contentType, expectedSHA string,
	retentionUntil time.Time,
) (ObjectAuthority, error) {
	if err := validateQuarantineCoordinates(bucket, key, tenantID, restorationID); err != nil {
		return ObjectAuthority{}, err
	}
	if !restorationSHA256.MatchString(expectedSHA) || sha256Bytes(content) != expectedSHA {
		return ObjectAuthority{}, errors.New("quarantine content SHA-256 differs from expected digest")
	}
	if contentType == "" || retentionUntil.IsZero() || !retentionUntil.After(time.Now().UTC()) {
		return ObjectAuthority{}, errors.New("quarantine content type and future retention are required")
	}
	putOptions := minio.PutObjectOptions{
		ContentType: contentType,
		UserMetadata: map[string]string{
			"sha256": expectedSHA, "tenant-id": tenantID,
			"restoration-id": restorationID, "quarantined": "true",
		},
		Mode:            minio.Governance,
		RetainUntilDate: retentionUntil.UTC(),
	}
	// The deterministic tenant/restoration key is write-once. Without this
	// precondition a lease predecessor that finishes late can create a second
	// version after a successor has already committed authority.
	putOptions.SetMatchETagExcept("*")
	info, err := c.client.PutObject(ctx, bucket, key, bytes.NewReader(content), int64(len(content)), putOptions)
	if err != nil {
		return ObjectAuthority{}, fmt.Errorf("put quarantine object: %w", err)
	}
	if strings.TrimSpace(info.VersionID) == "" {
		return ObjectAuthority{}, errors.New("quarantine bucket did not return an immutable object version")
	}
	stat, err := c.client.StatObject(ctx, bucket, key, minio.StatObjectOptions{
		VersionID: info.VersionID, Checksum: true,
	})
	if err != nil {
		return ObjectAuthority{}, fmt.Errorf("HEAD quarantine object version: %w", err)
	}
	metadataSHA := objectMetadataValue(stat, "sha256")
	if stat.VersionID != info.VersionID || stat.Size != int64(len(content)) || metadataSHA != expectedSHA {
		return ObjectAuthority{}, errors.New("quarantine object HEAD differs from committed bytes")
	}
	mode, retainedUntil, err := c.client.GetObjectRetention(ctx, bucket, key, stat.VersionID)
	if err != nil {
		return ObjectAuthority{}, fmt.Errorf("read committed quarantine object retention: %w", err)
	}
	minimumRetention := retentionUntil.UTC().Truncate(time.Second)
	if mode == nil || (*mode != minio.Governance && *mode != minio.Compliance) || retainedUntil == nil || retainedUntil.Before(minimumRetention) {
		return ObjectAuthority{}, errors.New("quarantine object retention differs from requested authority")
	}
	authority := ObjectAuthority{
		Bucket: bucket, Key: key, VersionID: stat.VersionID, ETag: strings.Trim(stat.ETag, `"`),
		SizeBytes: stat.Size, SHA256: expectedSHA, ObservedAt: time.Now().UTC(),
		RetentionUntil: retainedUntil.UTC(),
	}
	if err := authority.Validate(); err != nil {
		return ObjectAuthority{}, err
	}
	return authority, nil
}
