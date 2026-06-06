////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/forensics/cutter/pcap_cutter.go
// 完整修复版：
// - ✅ M1: 修复并发控制死锁问题（使用 errgroup）
// - ✅ M2: 优化解压缩逻辑（处理大小写，支持 zstd）
// - ✅ 优化错误处理和资源清理
////////////////////////////////////////////////////////////////////////////////

package cutter

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"
	"github.com/klauspost/compress/zstd"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/index"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/s3client"
)

// CutQuery 裁剪查询参数
type CutQuery struct {
	TenantID    string
	ProbeID     string
	SrcIP       string
	DstIP       string
	SrcPort     uint16
	DstPort     uint16
	Protocol    uint8
	CommunityID string
	StartTime   int64
	EndTime     int64
	MaxPackets  int64
}

// Validate 验证查询参数
func (q *CutQuery) Validate() error {
	if q.TenantID == "" {
		return errors.New(errors.ErrCodeInvalidParameter, "tenant_id is required")
	}
	if q.StartTime == 0 || q.EndTime == 0 {
		return errors.New(errors.ErrCodeInvalidParameter, "start_time and end_time are required")
	}
	if q.EndTime < q.StartTime {
		return errors.New(errors.ErrCodeInvalidParameter, "end_time must be greater than start_time")
	}
	// 限制时间范围（最多 24 小时）
	if q.EndTime-q.StartTime > 24*60*60*1000 {
		return errors.New(errors.ErrCodeInvalidParameter, "time range cannot exceed 24 hours")
	}
	return nil
}

// ToIndexQuery 转换为索引查询
func (q *CutQuery) ToIndexQuery() *index.IndexQuery {
	return &index.IndexQuery{
		TenantID:    q.TenantID,
		ProbeID:     q.ProbeID,
		SrcIP:       q.SrcIP,
		DstIP:       q.DstIP,
		SrcPort:     q.SrcPort,
		DstPort:     q.DstPort,
		Protocol:    q.Protocol,
		CommunityID: q.CommunityID,
		StartTime:   q.StartTime,
		EndTime:     q.EndTime,
	}
}

// CutResult 裁剪结果
type CutResult struct {
	TotalPackets int64
	TotalBytes   int64
	FilesScanned int
	FileErrors   []FileError
	Duration     time.Duration
}

// FileError 文件处理错误
type FileError struct {
	FileKey string
	Error   string
}

// ProgressCallback 进度回调函数
type ProgressCallback func(filesProcessed, totalFiles int, packetsFound int64)

// Cutter PCAP 裁剪器
type Cutter struct {
	s3Client       *s3client.S3Client
	indexClient    *index.IndexClient
	logger         *zap.Logger
	maxConcurrent  int
	maxPackets     int64
	perFileTimeout time.Duration
	bufferSize     int
}

// CutterConfig 裁剪器配置
type CutterConfig struct {
	MaxConcurrent  int
	MaxPackets     int64
	PerFileTimeout time.Duration
	BufferSize     int
}

// DefaultCutterConfig 默认配置
func DefaultCutterConfig() CutterConfig {
	return CutterConfig{
		MaxConcurrent:  5,
		MaxPackets:     100000,
		PerFileTimeout: 60 * time.Second,
		BufferSize:     65536, // 64KB
	}
}

// NewCutter 创建裁剪器
func NewCutter(
	s3Client *s3client.S3Client,
	indexClient *index.IndexClient,
	maxConcurrent int,
	maxPackets int64,
	perFileTimeout time.Duration,
	logger *zap.Logger,
) *Cutter {
	cfg := DefaultCutterConfig()
	if maxConcurrent > 0 {
		cfg.MaxConcurrent = maxConcurrent
	}
	if maxPackets > 0 {
		cfg.MaxPackets = maxPackets
	}
	if perFileTimeout > 0 {
		cfg.PerFileTimeout = perFileTimeout
	}

	return NewCutterWithConfig(s3Client, indexClient, cfg, logger)
}

// NewCutterWithConfig 使用配置创建裁剪器
func NewCutterWithConfig(
	s3Client *s3client.S3Client,
	indexClient *index.IndexClient,
	cfg CutterConfig,
	logger *zap.Logger,
) *Cutter {
	return &Cutter{
		s3Client:       s3Client,
		indexClient:    indexClient,
		logger:         logger,
		maxConcurrent:  cfg.MaxConcurrent,
		maxPackets:     cfg.MaxPackets,
		perFileTimeout: cfg.PerFileTimeout,
		bufferSize:     cfg.BufferSize,
	}
}

// CutPCAP 执行 PCAP 裁剪（修复版：使用 errgroup 替代手动信号量）
func (c *Cutter) CutPCAP(
	ctx context.Context,
	query *CutQuery,
	output io.Writer,
	progressCb ProgressCallback,
) (*CutResult, error) {
	startTime := time.Now()

	ctx, span := otel.StartSpan(ctx, "Cutter.CutPCAP")
	defer span.End()

	// 验证查询参数
	if err := query.Validate(); err != nil {
		return nil, err
	}

	// 1. 查询索引获取匹配的文件
	indexQuery := query.ToIndexQuery()

	files, err := c.indexClient.LookupFiles(ctx, indexQuery)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeClickHouseError, "failed to lookup index")
	}

	if len(files) == 0 {
		return nil, errors.New(errors.ErrCodePcapNotFound, "no matching PCAP files found")
	}

	c.logger.Info("Found PCAP files for cutting",
		zap.Int("count", len(files)),
		zap.String("tenant_id", query.TenantID),
		zap.String("probe_id", query.ProbeID),
		zap.String("community_id", query.CommunityID))

	// 2. 初始化 PCAP Writer
	pcapWriter := pcapgo.NewWriter(output)
	if err := pcapWriter.WriteFileHeader(65535, layers.LinkTypeEthernet); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInternal, "failed to write PCAP header")
	}

	// 3. 构建过滤器（使用优化后的 BPF Filter）
	filter := BuildBPFFilter(query)

	// 4. 确定最大包数
	maxPackets := query.MaxPackets
	if maxPackets <= 0 || maxPackets > c.maxPackets {
		maxPackets = c.maxPackets
	}

	// 5. 处理文件（使用 errgroup 并发控制）
	result := &CutResult{
		FileErrors: make([]FileError, 0),
	}

	var (
		totalPackets  int64
		mu            sync.Mutex // 保护 result 和 FileErrors
		writerMu      sync.Mutex // 保护 pcapWriter
		filesComplete int32
	)

	// ✅ 使用 errgroup 管理并发，自动处理 context 取消和错误传播
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(c.maxConcurrent) // 限制并发数

	for i := range files {
		// 检查全局最大包数限制
		if atomic.LoadInt64(&totalPackets) >= maxPackets {
			c.logger.Info("Reached max packets limit, stopping",
				zap.Int64("max_packets", maxPackets),
				zap.Int64("current", totalPackets))
			break
		}

		// 捕获循环变量
		file := files[i]
		fileIndex := i

		g.Go(func() error {
			// 再次检查最大包数（因为是并发的）
			remaining := maxPackets - atomic.LoadInt64(&totalPackets)
			if remaining <= 0 {
				return nil
			}

			// 为每个文件创建带超时的 Context
			fileCtx, cancel := context.WithTimeout(gctx, c.perFileTimeout)
			defer cancel()

			count, bytes, err := c.processFile(fileCtx, file, filter, pcapWriter, &writerMu, remaining)

			if err != nil {
				c.logger.Warn("Failed to process file",
					zap.String("file_key", file.FileKey),
					zap.Int("file_index", fileIndex),
					zap.Error(err))

				mu.Lock()
				result.FileErrors = append(result.FileErrors, FileError{
					FileKey: file.FileKey,
					Error:   err.Error(),
				})
				mu.Unlock()

				// 单个文件失败不中断整体任务，除非是 context 取消
				if errors.IsCode(err, errors.ErrCodeInternal) || err == context.DeadlineExceeded {
					// 只有当 gctx 取消时才返回错误
					if gctx.Err() != nil {
						return gctx.Err()
					}
				}
				return nil
			}

			// 更新统计
			newTotal := atomic.AddInt64(&totalPackets, count)
			completed := atomic.AddInt32(&filesComplete, 1)

			mu.Lock()
			result.TotalPackets += count
			result.TotalBytes += bytes
			result.FilesScanned++
			mu.Unlock()

			// 进度回调
			if progressCb != nil {
				progressCb(int(completed), len(files), newTotal)
			}

			return nil
		})
	}

	// 等待所有 goroutine 完成
	if err := g.Wait(); err != nil {
		result.Duration = time.Since(startTime)
		// 如果是因为达到包限制而提前结束，不视为错误
		if atomic.LoadInt64(&totalPackets) >= maxPackets {
			return result, nil
		}
		return result, err
	}

	result.Duration = time.Since(startTime)

	c.logger.Info("PCAP cutting completed",
		zap.Int64("total_packets", result.TotalPackets),
		zap.Int64("total_bytes", result.TotalBytes),
		zap.Int("files_scanned", result.FilesScanned),
		zap.Int("file_errors", len(result.FileErrors)),
		zap.Duration("duration", result.Duration))

	return result, nil
}

// processFile 处理单个文件
func (c *Cutter) processFile(
	ctx context.Context,
	file *index.FileMetadata,
	filter *BPFFilter,
	writer *pcapgo.Writer,
	writerMu *sync.Mutex,
	remainingPackets int64,
) (int64, int64, error) {
	ctx, span := otel.StartSpan(ctx, "Cutter.processFile")
	defer span.End()

	c.logger.Debug("Processing file",
		zap.String("file_key", file.FileKey),
		zap.Int64("remaining_packets", remainingPackets))

	// 从 S3 获取对象
	obj, err := c.s3Client.GetObject(ctx, file.FileKey)
	if err != nil {
		return 0, 0, errors.Wrap(err, errors.ErrCodeMinIOError, "failed to get S3 object")
	}
	defer obj.Close()

	// 解压缩（如果需要）
	reader, closer, err := c.decompressReader(obj, file.FileKey)
	if err != nil {
		return 0, 0, err
	}
	if closer != nil {
		defer closer.Close()
	}

	// 解析 PCAP
	pcapReader, err := pcapgo.NewReader(reader)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to create PCAP reader: %w", err)
	}

	// 过滤并写入
	var matchCount, totalBytes int64

	for matchCount < remainingPackets {
		// 检查 context
		select {
		case <-ctx.Done():
			return matchCount, totalBytes, ctx.Err()
		default:
		}

		data, ci, err := pcapReader.ReadPacketData()
		if err == io.EOF {
			break
		}
		if err != nil {
			return matchCount, totalBytes, fmt.Errorf("failed to read packet: %w", err)
		}

		timestamp := ci.Timestamp.UnixMilli()
		if filter.Match(data, timestamp) {
			// 加锁写入（多文件并发时需要）
			writerMu.Lock()
			err := writer.WritePacket(ci, data)
			writerMu.Unlock()

			if err != nil {
				return matchCount, totalBytes, fmt.Errorf("failed to write packet: %w", err)
			}

			matchCount++
			totalBytes += int64(len(data))
		}
	}

	c.logger.Debug("File processed",
		zap.String("file_key", file.FileKey),
		zap.Int64("matched_packets", matchCount),
		zap.Int64("bytes", totalBytes))

	return matchCount, totalBytes, nil
}

// decompressReader 解压缩读取器（修复 M2: 忽略大小写，支持 zstd）
func (c *Cutter) decompressReader(r io.Reader, filename string) (io.Reader, io.Closer, error) {
	lowerFilename := strings.ToLower(filename)

	// 检查 zstd 压缩
	if strings.HasSuffix(lowerFilename, ".zst") || strings.HasSuffix(lowerFilename, ".zstd") {
		decoder, err := zstd.NewReader(r)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create zstd decoder: %w", err)
		}
		return decoder, decoder.IOReadCloser(), nil
	}

	// 可以在此添加 gzip 支持
	// if strings.HasSuffix(lowerFilename, ".gz") ...

	// 未压缩
	return r, nil, nil
}

// CutToFile 裁剪到文件（用于异步任务）
func (c *Cutter) CutToFile(
	ctx context.Context,
	query *CutQuery,
	outputKey string,
	progressCb ProgressCallback,
) (*CutResult, error) {
	ctx, span := otel.StartSpan(ctx, "Cutter.CutToFile")
	defer span.End()

	// 创建管道
	pr, pw := io.Pipe()

	// 结果通道
	resultCh := make(chan *CutResult, 1)
	errCh := make(chan error, 1)

	// 启动裁剪 goroutine
	go func() {
		defer pw.Close()

		result, err := c.CutPCAP(ctx, query, pw, progressCb)
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- result
	}()

	// 上传到 S3（流式上传）
	uploadErr := c.s3Client.PutObject(ctx, outputKey, pr, -1, "application/vnd.tcpdump.pcap")
	// 确保管道读取端关闭，避免裁剪 goroutine 阻塞
	_ = pr.Close()

	if uploadErr != nil {
		// 发生上传错误，尝试收集裁剪错误
		select {
		case <-resultCh:
		case <-errCh:
		case <-time.After(1 * time.Second): // 短暂等待
		}
		return nil, errors.Wrap(uploadErr, errors.ErrCodeMinIOError, "failed to upload result")
	}

	// 等待裁剪完成
	select {
	case result := <-resultCh:
		return result, nil
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Close 关闭裁剪器
func (c *Cutter) Close() error {
	// 目前无需特殊清理
	return nil
}
