package service

import (
	"context"
	"fmt"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
)

// HumanReportService 人读报告(独立 ReportState,失败不回退 Run;事务内不碰 MinIO)。
type HumanReportService struct {
	repo *repository.Repo
}

func NewHumanReportService(repo *repository.Repo) *HumanReportService { return &HumanReportService{repo: repo} }

// RequestReport 请求报告:验终态+摘要,身份幂等,插 QUEUED+outbox。
func (s *HumanReportService) RequestReport(ctx context.Context, tenantID, runID, templateRevision, locale string) (reportID string, replayed bool, err error) {
	if templateRevision == "" {
		templateRevision = "default-v1"
	}
	if locale == "" {
		locale = "zh-CN"
	}
	ref, err := s.repo.GetRunSummaryHash(ctx, tenantID, runID)
	if err != nil {
		return "", false, err
	}
	if ref.SummarySHA256 == "" || !ref.SummaryExists {
		return "", false, fmt.Errorf("machine summary not finalized for run")
	}
	requestHash := identityHash(tenantID, runID, ref.SummarySHA256, templateRevision, locale)
	idempotencyKey := identityHash("human-report", tenantID, runID, ref.SummarySHA256, templateRevision, locale)
	return s.repo.RequestHumanReportAtomic(ctx, tenantID, runID, ref.SummarySHA256, templateRevision, locale, requestHash, idempotencyKey)
}

// ApplyWorkerReceipt 报告 worker 对象 ACK(object_key/sha256/size + 源摘要 hash)。
func (s *HumanReportService) ApplyWorkerReceipt(ctx context.Context, tenantID, reportID, objectKey, objectSHA256 string, objectSize int64, sourceSummarySHA256 string) (string, error) {
	if len(objectSHA256) != 64 {
		return "", fmt.Errorf("object_sha256 must be 64 hex chars")
	}
	return s.repo.ApplyHumanReportReceiptAtomic(ctx, tenantID, reportID, objectKey, objectSHA256, objectSize, "v1", sourceSummarySHA256)
}

// VerifyAndConfirm 独立对象权威 verifier:仅校验 hash/size 后推进 AVAILABLE。
// 对象字节校验由 MinIO HEAD 对账完成(调用方提供确认后的 hash/size)。
func (s *HumanReportService) VerifyAndConfirm(ctx context.Context, tenantID, reportID, objectSHA256 string, objectSize int64) error {
	return s.repo.ConfirmHumanReportObjectAtomic(ctx, tenantID, reportID, objectSHA256, objectSize)
}
