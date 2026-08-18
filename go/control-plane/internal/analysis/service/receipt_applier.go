package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/contract"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
)

// ReceiptMessage analysis.receipts.v1 消息(JSON;与 contract.StageReceipt 同构,
// 额外携带 event_id 供 inbox 去重)。
type ReceiptMessage struct {
	EventID         string          `json:"event_id"`
	SchemaVersion   string          `json:"schema_version"`
	TenantID        string          `json:"tenant_id"`
	RunID           string          `json:"run_id"`
	ExecutionNodeID string          `json:"execution_node_id"`
	Attempt         int32           `json:"attempt"`
	FencingToken    string          `json:"fencing_token"`
	Provider        string          `json:"provider"`
	InputCount      int64           `json:"input_count"`
	OutputCount     int64           `json:"output_count"`
	ErrorCount      int64           `json:"error_count"`
	RejectCount     int64           `json:"reject_count"`
	WatermarkMs     int64           `json:"watermark_ms"`
	Fence           json.RawMessage `json:"fence"`
	PayloadHash     string          `json:"payload_hash"`
}

// ReceiptApplier 回执权威应用端口:decode → 校验 → ApplyStageReceiptAtomic。
type ReceiptApplier struct {
	repo   *repository.Repo
	logger *zap.Logger
}

// NewReceiptApplier 构造。
func NewReceiptApplier(repo *repository.Repo, logger *zap.Logger) *ReceiptApplier {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ReceiptApplier{repo: repo, logger: logger}
}

// Apply 应用一条回执(确定性非法消息不重试;临时失败向上抛由消费端退避)。
func (a *ReceiptApplier) Apply(ctx context.Context, raw []byte) error {
	var msg ReceiptMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		a.logger.Warn("receipt malformed json; quarantined (no retry)",
			zap.String("raw", string(raw)[:minInt(len(raw), 200)]))
		return nil
	}
	if msg.RunID == "" || msg.ExecutionNodeID == "" || msg.EventID == "" {
		a.logger.Warn("receipt missing identity; quarantined", zap.Any("msg", msg))
		return nil
	}
	fenceJSON := msg.Fence
	if fenceJSON == nil {
		fenceJSON = json.RawMessage(`{}`)
	}
	tuple := fmt.Sprintf("%s|%s|%d", msg.RunID, msg.ExecutionNodeID, msg.Attempt)
	payloadHash := msg.PayloadHash
	if payloadHash == "" {
		h := sha256.Sum256([]byte(string(fenceJSON) + "|" + tuple))
		payloadHash = hex.EncodeToString(h[:])
	}
	// error_count>0 的执行结果是确定性失败(如探针 stale_command_revision/
	// expired/executor error 回执);不得把失败执行标记为 SUCCEEDED。
	newState := "SUCCEEDED"
	if msg.ErrorCount > 0 {
		newState = "FAILED"
	}
	_, err := a.repo.ApplyStageReceiptAtomic(ctx, repository.ReceiptCommand{
		TenantID:        msg.TenantID,
		RunID:           msg.RunID,
		EventID:         msg.EventID,
		TupleHash:       tuple,
		ExecutionNodeID: msg.ExecutionNodeID,
		Attempt:         msg.Attempt,
		FencingToken:    msg.FencingToken,
		Provider:        firstNonEmpty(msg.Provider, "flink-run-receipt"),
		InputCount:      msg.InputCount,
		OutputCount:     msg.OutputCount,
		ErrorCount:      msg.ErrorCount,
		RejectCount:     msg.RejectCount,
		WatermarkMs:     msg.WatermarkMs,
		FenceJSON:       fenceJSON,
		PayloadHash:     payloadHash,
		ExpectedState:   "RUNNING",
		NewState:        newState,
	})
	if err != nil {
		// STALE_FENCE / LATE_TERMINAL / 冲突 均为确定性 outcome(已同事务落库),
		// 按可消费处理;其余(DB 临时故障)上抛重试。
		msg := err.Error()
		if strings.Contains(msg, "stale fence") || strings.Contains(msg, "attempt CAS") ||
			strings.Contains(msg, "not found") {
			a.logger.Warn("receipt deterministically rejected", zap.Error(err))
			return nil
		}
		return fmt.Errorf("apply receipt: %w", err)
	}
	return nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var _ = contract.ErrCodeStaleFence
