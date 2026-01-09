package s3client

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
)

// S3Client S3/MinIO 客户端
type S3Client struct {
	client       *minio.Client
	bucket       string
	resultBucket string
	logger       *zap.Logger
}

// NewS3Client 创建 S3 客户端
func NewS3Client(
	endpoint, accessKey, secretKey, bucket string,
	useSSL bool,
	resultBucket string,
	logger *zap.Logger,
) (*S3Client, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create minio client: %w", err)
	}

	// 如果未指定结果 bucket，使用同一个
	if resultBucket == "" {
		resultBucket = bucket
	}

	s3Client := &S3Client{
		client:       client,
		bucket:       bucket,
		resultBucket: resultBucket,
		logger:       logger,
	}

	return s3Client, nil
}

// GetObject 获取对象
func (c *S3Client) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	ctx, span := otel.StartSpan(ctx, "S3Client.GetObject")
	defer span.End()

	obj, err := c.client.GetObject(ctx, c.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeMinIOError, fmt.Sprintf("failed to get object %s", key))
	}

	return obj, nil
}

// GetObjectFromBucket 从指定 bucket 获取对象
func (c *S3Client) GetObjectFromBucket(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	ctx, span := otel.StartSpan(ctx, "S3Client.GetObjectFromBucket")
	defer span.End()

	obj, err := c.client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeMinIOError, fmt.Sprintf("failed to get object %s/%s", bucket, key))
	}

	return obj, nil
}

// GetObjectRange 获取对象的指定范围
func (c *S3Client) GetObjectRange(ctx context.Context, key string, start, end int64) (io.ReadCloser, error) {
	ctx, span := otel.StartSpan(ctx, "S3Client.GetObjectRange")
	defer span.End()

	opts := minio.GetObjectOptions{}
	if err := opts.SetRange(start, end); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInvalidParameter, "invalid range")
	}

	obj, err := c.client.GetObject(ctx, c.bucket, key, opts)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeMinIOError, "failed to get object range")
	}

	return obj, nil
}

// PutObject 上传对象
func (c *S3Client) PutObject(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error {
	ctx, span := otel.StartSpan(ctx, "S3Client.PutObject")
	defer span.End()

	opts := minio.PutObjectOptions{
		ContentType: contentType,
	}

	_, err := c.client.PutObject(ctx, c.resultBucket, key, reader, size, opts)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeMinIOError, fmt.Sprintf("failed to put object %s", key))
	}

	c.logger.Debug("Object uploaded",
		zap.String("bucket", c.resultBucket),
		zap.String("key", key))

	return nil
}

// PutObjectToBucket 上传到指定 bucket
func (c *S3Client) PutObjectToBucket(ctx context.Context, bucket, key string, reader io.Reader, size int64, contentType string) error {
	ctx, span := otel.StartSpan(ctx, "S3Client.PutObjectToBucket")
	defer span.End()

	opts := minio.PutObjectOptions{
		ContentType: contentType,
	}

	_, err := c.client.PutObject(ctx, bucket, key, reader, size, opts)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeMinIOError, fmt.Sprintf("failed to put object %s/%s", bucket, key))
	}

	return nil
}

// DeleteObject 删除对象
func (c *S3Client) DeleteObject(ctx context.Context, key string) error {
	ctx, span := otel.StartSpan(ctx, "S3Client.DeleteObject")
	defer span.End()

	err := c.client.RemoveObject(ctx, c.resultBucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeMinIOError, fmt.Sprintf("failed to delete object %s", key))
	}

	return nil
}

// GetPresignedURL 获取预签名 URL
func (c *S3Client) GetPresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	ctx, span := otel.StartSpan(ctx, "S3Client.GetPresignedURL")
	defer span.End()

	// 先尝试从结果 bucket 获取
	url, err := c.client.PresignedGetObject(ctx, c.resultBucket, key, expiry, nil)
	if err != nil {
		// 尝试从主 bucket 获取
		url, err = c.client.PresignedGetObject(ctx, c.bucket, key, expiry, nil)
		if err != nil {
			return "", errors.Wrap(err, errors.ErrCodeMinIOError, "failed to generate presigned URL")
		}
	}

	return url.String(), nil
}

// ObjectExists 检查对象是否存在
func (c *S3Client) ObjectExists(ctx context.Context, key string) (bool, error) {
	ctx, span := otel.StartSpan(ctx, "S3Client.ObjectExists")
	defer span.End()

	// 先检查结果 bucket
	_, err := c.client.StatObject(ctx, c.resultBucket, key, minio.StatObjectOptions{})
	if err == nil {
		return true, nil
	}

	errResp := minio.ToErrorResponse(err)
	if errResp.Code == "NoSuchKey" {
		// 检查主 bucket
		_, err = c.client.StatObject(ctx, c.bucket, key, minio.StatObjectOptions{})
		if err == nil {
			return true, nil
		}
		errResp = minio.ToErrorResponse(err)
		if errResp.Code == "NoSuchKey" {
			return false, nil
		}
		return false, err
	}

	return false, err
}

// GetObjectInfo 获取对象信息
func (c *S3Client) GetObjectInfo(ctx context.Context, key string) (*minio.ObjectInfo, error) {
	ctx, span := otel.StartSpan(ctx, "S3Client.GetObjectInfo")
	defer span.End()

	info, err := c.client.StatObject(ctx, c.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		errResp := minio.ToErrorResponse(err)
		if errResp.Code == "NoSuchKey" {
			return nil, errors.New(errors.ErrCodeResourceNotFound, "object not found")
		}
		return nil, errors.Wrap(err, errors.ErrCodeMinIOError, "failed to stat object")
	}

	return &info, nil
}

// ListObjects 列出对象
func (c *S3Client) ListObjects(ctx context.Context, prefix string, maxKeys int) ([]minio.ObjectInfo, error) {
	ctx, span := otel.StartSpan(ctx, "S3Client.ListObjects")
	defer span.End()

	opts := minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	}

	var objects []minio.ObjectInfo
	for obj := range c.client.ListObjects(ctx, c.bucket, opts) {
		if obj.Err != nil {
			return nil, errors.Wrap(obj.Err, errors.ErrCodeMinIOError, "failed to list objects")
		}
		objects = append(objects, obj)
		if maxKeys > 0 && len(objects) >= maxKeys {
			break
		}
	}

	return objects, nil
}

// Ping 检查连接
func (c *S3Client) Ping(ctx context.Context) error {
	ctx, span := otel.StartSpan(ctx, "S3Client.Ping")
	defer span.End()

	// 检查 bucket 是否存在
	exists, err := c.client.BucketExists(ctx, c.bucket)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeMinIOError, "failed to check bucket")
	}
	if !exists {
		return errors.Newf(errors.ErrCodeMinIOError, "bucket %s does not exist", c.bucket)
	}

	return nil
}

// EnsureBucket 确保 bucket 存在
func (c *S3Client) EnsureBucket(ctx context.Context, bucket string) error {
	exists, err := c.client.BucketExists(ctx, bucket)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeMinIOError, "failed to check bucket")
	}

	if !exists {
		err = c.client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
		if err != nil {
			return errors.Wrap(err, errors.ErrCodeMinIOError, "failed to create bucket")
		}
		c.logger.Info("Bucket created", zap.String("bucket", bucket))
	}

	return nil
}

// GetBucket 获取主 bucket 名称
func (c *S3Client) GetBucket() string {
	return c.bucket
}

// GetResultBucket 获取结果 bucket 名称
func (c *S3Client) GetResultBucket() string {
	return c.resultBucket
}
