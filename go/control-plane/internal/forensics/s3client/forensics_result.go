package s3client

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"

	commonerrors "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

var ErrForensicsResultConflict = errors.New("forensics result object conflicts with expected authority")

func forensicsObjectReadError(operation string, err error) error {
	response := minio.ToErrorResponse(err)
	switch response.Code {
	case "NoSuchKey", "NoSuchObject", "NoSuchVersion", "NotFound":
		return commonerrors.New(commonerrors.ErrCodeResourceNotFound, "authorized forensics result version not found")
	default:
		return fmt.Errorf("%s: %w", operation, err)
	}
}

func validateForensicsResultKey(key, tenantID, taskID string) error {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(taskID) == "" {
		return errors.New("forensics result tenant and task identities are required")
	}
	expected := "tenants/" + tenantID + "/forensics/jobs/" + taskID + "/"
	if !strings.HasPrefix(key, expected) || strings.Contains(key, "..") || strings.ContainsAny(key, "\r\n\x00") {
		return errors.New("forensics result key is not derived from tenant/task identity")
	}
	return nil
}

func (c *S3Client) FindForensicsResultObject(
	ctx context.Context,
	key, tenantID, taskID, expectedSHA string,
	expectedSize int64,
) (ObjectAuthority, bool, error) {
	if err := validateForensicsResultKey(key, tenantID, taskID); err != nil {
		return ObjectAuthority{}, false, err
	}
	if !restorationSHA256.MatchString(expectedSHA) || expectedSize <= 0 {
		return ObjectAuthority{}, false, errors.New("forensics result digest and positive size are required")
	}
	info, err := c.client.StatObject(ctx, c.resultBucket, key, minio.StatObjectOptions{Checksum: true})
	if err != nil {
		switch minio.ToErrorResponse(err).Code {
		case "NoSuchKey", "NoSuchObject", "NotFound":
			return ObjectAuthority{}, false, nil
		default:
			return ObjectAuthority{}, false, fmt.Errorf("HEAD existing forensics result: %w", err)
		}
	}
	if strings.TrimSpace(info.VersionID) == "" || strings.TrimSpace(info.ETag) == "" || info.Size != expectedSize ||
		objectMetadataValue(info, "sha256") != expectedSHA || objectMetadataValue(info, "tenant-id") != tenantID ||
		objectMetadataValue(info, "task-id") != taskID || objectMetadataValue(info, "artifact-kind") != "pcap-cut" ||
		objectMetadataValue(info, "inert") != "true" {
		return ObjectAuthority{}, false, ErrForensicsResultConflict
	}
	mode, retainedUntil, err := c.client.GetObjectRetention(ctx, c.resultBucket, key, info.VersionID)
	if err != nil {
		return ObjectAuthority{}, false, fmt.Errorf("read forensics result retention: %w", err)
	}
	if mode == nil || (*mode != minio.Governance && *mode != minio.Compliance) || retainedUntil == nil || !retainedUntil.After(time.Now().UTC()) {
		return ObjectAuthority{}, false, ErrForensicsResultConflict
	}
	authority := ObjectAuthority{
		Bucket: c.resultBucket, Key: key, VersionID: info.VersionID, ETag: strings.Trim(info.ETag, `"`),
		SizeBytes: info.Size, SHA256: expectedSHA, ObservedAt: time.Now().UTC(), RetentionUntil: retainedUntil.UTC(),
	}
	if err := authority.Validate(); err != nil {
		return ObjectAuthority{}, false, err
	}
	return authority, true, nil
}

// PutForensicsResultObject writes one deterministic, immutable PCAP result.
// Payload bytes remain inert and are never sent to a parser, shell or renderer.
func (c *S3Client) PutForensicsResultObject(
	ctx context.Context,
	key, tenantID, taskID string,
	content io.ReadSeeker,
	size int64,
	expectedSHA string,
	retentionUntil time.Time,
) (ObjectAuthority, error) {
	if err := validateForensicsResultKey(key, tenantID, taskID); err != nil {
		return ObjectAuthority{}, err
	}
	if content == nil || size <= 0 || !restorationSHA256.MatchString(expectedSHA) {
		return ObjectAuthority{}, errors.New("forensics result bytes differ from expected digest")
	}
	hasher := sha256.New()
	written, err := io.Copy(hasher, io.LimitReader(content, size+1))
	if err != nil || written != size || hex.EncodeToString(hasher.Sum(nil)) != expectedSHA {
		return ObjectAuthority{}, errors.New("forensics result bytes differ from expected digest")
	}
	if _, err := content.Seek(0, io.SeekStart); err != nil {
		return ObjectAuthority{}, fmt.Errorf("rewind verified forensics result: %w", err)
	}
	if !retentionUntil.After(time.Now().UTC()) {
		return ObjectAuthority{}, errors.New("future forensics result retention is required")
	}
	options := minio.PutObjectOptions{
		ContentType: "application/vnd.tcpdump.pcap",
		UserMetadata: map[string]string{
			"sha256": expectedSHA, "tenant-id": tenantID, "task-id": taskID,
			"artifact-kind": "pcap-cut", "inert": "true",
		},
		Mode: minio.Governance, RetainUntilDate: retentionUntil.UTC(),
	}
	options.SetMatchETagExcept("*")
	info, err := c.client.PutObject(ctx, c.resultBucket, key, content, size, options)
	if err != nil {
		return ObjectAuthority{}, fmt.Errorf("put immutable forensics result: %w", err)
	}
	if strings.TrimSpace(info.VersionID) == "" {
		return ObjectAuthority{}, errors.New("forensics result bucket did not return an immutable version")
	}
	stat, err := c.client.StatObject(ctx, c.resultBucket, key, minio.StatObjectOptions{VersionID: info.VersionID, Checksum: true})
	if err != nil {
		return ObjectAuthority{}, fmt.Errorf("HEAD immutable forensics result: %w", err)
	}
	if stat.VersionID != info.VersionID || stat.Size != size || objectMetadataValue(stat, "sha256") != expectedSHA {
		return ObjectAuthority{}, errors.New("forensics result HEAD differs from written authority")
	}
	authority := ObjectAuthority{
		Bucket: c.resultBucket, Key: key, VersionID: stat.VersionID, ETag: strings.Trim(stat.ETag, `"`),
		SizeBytes: stat.Size, SHA256: expectedSHA, ObservedAt: time.Now().UTC(), RetentionUntil: retentionUntil.UTC(),
	}
	if err := authority.Validate(); err != nil {
		return ObjectAuthority{}, err
	}
	return authority, nil
}

// VerifyForensicsResultAuthority binds a read to the exact final-manifest
// bucket, key and version. A later object version can never replace the bytes
// authorized by the PostgreSQL manifest.
func (c *S3Client) VerifyForensicsResultAuthority(
	ctx context.Context,
	authority ObjectAuthority,
	tenantID, taskID string,
) (ObjectAuthority, error) {
	if authority.Bucket != c.resultBucket {
		return ObjectAuthority{}, ErrForensicsResultConflict
	}
	if err := validateForensicsResultKey(authority.Key, tenantID, taskID); err != nil {
		return ObjectAuthority{}, err
	}
	if err := authority.Validate(); err != nil {
		return ObjectAuthority{}, err
	}
	info, err := c.client.StatObject(ctx, authority.Bucket, authority.Key, minio.StatObjectOptions{
		VersionID: authority.VersionID,
		Checksum:  true,
	})
	if err != nil {
		return ObjectAuthority{}, forensicsObjectReadError("HEAD authorized forensics result version", err)
	}
	if info.VersionID != authority.VersionID || strings.Trim(info.ETag, `"`) != strings.Trim(authority.ETag, `"`) ||
		info.Size != authority.SizeBytes || objectMetadataValue(info, "sha256") != authority.SHA256 ||
		objectMetadataValue(info, "tenant-id") != tenantID || objectMetadataValue(info, "task-id") != taskID ||
		objectMetadataValue(info, "artifact-kind") != "pcap-cut" || objectMetadataValue(info, "inert") != "true" {
		return ObjectAuthority{}, ErrForensicsResultConflict
	}
	mode, retainedUntil, err := c.client.GetObjectRetention(ctx, authority.Bucket, authority.Key, authority.VersionID)
	if err != nil {
		return ObjectAuthority{}, forensicsObjectReadError("read authorized forensics result retention", err)
	}
	if mode == nil || retainedUntil == nil || (*mode != minio.Governance && *mode != minio.Compliance) ||
		retainedUntil.Before(authority.RetentionUntil.Add(-time.Second)) {
		return ObjectAuthority{}, ErrForensicsResultConflict
	}
	verified := authority
	verified.ETag = strings.Trim(info.ETag, `"`)
	verified.ObservedAt = time.Now().UTC()
	verified.RetentionUntil = retainedUntil.UTC()
	return verified, nil
}

func (c *S3Client) OpenForensicsResultObject(
	ctx context.Context,
	authority ObjectAuthority,
	tenantID, taskID string,
) (io.ReadCloser, ObjectAuthority, error) {
	verified, err := c.VerifyForensicsResultAuthority(ctx, authority, tenantID, taskID)
	if err != nil {
		return nil, ObjectAuthority{}, err
	}
	object, err := c.client.GetObject(ctx, verified.Bucket, verified.Key, minio.GetObjectOptions{
		VersionID: verified.VersionID,
		Checksum:  true,
	})
	if err != nil {
		return nil, ObjectAuthority{}, fmt.Errorf("GET authorized forensics result version: %w", err)
	}
	return object, verified, nil
}

func (c *S3Client) PresignForensicsResultObject(
	ctx context.Context,
	authority ObjectAuthority,
	tenantID, taskID string,
	expiry time.Duration,
) (string, ObjectAuthority, error) {
	if expiry <= 0 || expiry > 24*time.Hour {
		return "", ObjectAuthority{}, errors.New("forensics result presign expiry must be within 24 hours")
	}
	verified, err := c.VerifyForensicsResultAuthority(ctx, authority, tenantID, taskID)
	if err != nil {
		return "", ObjectAuthority{}, err
	}
	requestParameters := make(url.Values)
	requestParameters.Set("versionId", verified.VersionID)
	signed, err := c.client.PresignedGetObject(ctx, verified.Bucket, verified.Key, expiry, requestParameters)
	if err != nil {
		return "", ObjectAuthority{}, fmt.Errorf("presign authorized forensics result version: %w", err)
	}
	return signed.String(), verified, nil
}
