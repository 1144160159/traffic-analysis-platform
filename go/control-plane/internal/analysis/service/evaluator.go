package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/state"
)

// EvaluationContract 评估结果合同(评估 Run 的 S5 输出,写入机器摘要输入)。
// 由 evaluate_blind_package.py --emit-run-contract 产出;身份/口径 fail-closed。
type EvaluationContract struct {
	SchemaVersion       string   `json:"schema_version"`
	RunID               string   `json:"run_id"`
	ExecutionSpecSHA256 string   `json:"execution_spec_sha256"`
	PackagePath         string   `json:"package_path"`
	Accuracy            float64  `json:"accuracy"`
	DetectionRate       float64  `json:"detection_rate"`
	FalsePositiveRate   float64  `json:"false_positive_rate"`
	KnownAttackRecall   float64  `json:"known_attack_recall"`
	UnknownRecall       float64  `json:"unknown_recall"`
	CILowerAccuracy     float64  `json:"ci_lower_accuracy"`
	CIUpperFPR          float64  `json:"ci_upper_fpr"`
	StrataComplete      bool     `json:"strata_complete"`
	InvalidLabels       int      `json:"invalid_labels"`
	SampleCount         int      `json:"sample_count"`
	GatePassed          bool     `json:"gate_passed"`
	GateReasons         []string `json:"gate_reasons"`
	GeneratedAtMs       int64    `json:"generated_at_ms"`
}

// EvaluationExecutor 评估执行端口(生产:Python 盲评器;测试:桩)。
type EvaluationExecutor interface {
	RunEvaluation(ctx context.Context, packagePath, runID, executionSpecSHA256 string) (*EvaluationContract, error)
}

// PythonEvaluator 经 subprocess 调用 mlops 盲评器(凭证零传递,输出走文件)。
type PythonEvaluator struct {
	ScriptPath string
	Workdir    string
}

func NewPythonEvaluator(scriptPath, workdir string) *PythonEvaluator {
	return &PythonEvaluator{ScriptPath: scriptPath, Workdir: workdir}
}

// RunEvaluation 执行盲评并解析合同;命令失败/合同身份不匹配/门禁失败都显式返回。
func (e *PythonEvaluator) RunEvaluation(ctx context.Context, packagePath, runID, executionSpecSHA256 string) (*EvaluationContract, error) {
	outFile := fmt.Sprintf("/tmp/eval-contract-%s.json", strings.ReplaceAll(runID, "/", "_"))
	cmd := exec.CommandContext(ctx, "python3", e.ScriptPath,
		"--package", packagePath,
		"--run-id", runID,
		"--execution-spec-sha256", executionSpecSHA256,
		"--emit-run-contract", outFile)
	cmd.Dir = e.Workdir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("evaluator failed: %w (output: %.200s)", err, string(output))
	}
	raw, err := readFile(outFile)
	if err != nil {
		return nil, fmt.Errorf("read evaluation contract: %w", err)
	}
	var contract EvaluationContract
	if err := json.Unmarshal(raw, &contract); err != nil {
		return nil, fmt.Errorf("decode evaluation contract: %w", err)
	}
	if contract.RunID != runID || contract.ExecutionSpecSHA256 != executionSpecSHA256 {
		return nil, fmt.Errorf("evaluation contract identity mismatch")
	}
	return &contract, nil
}

// readFile 依赖注入点(测试可替换)。
var readFile = func(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// EvaluationGate 纯函数门禁:95%/5% + CI + strata + 非法标签零容忍。
func EvaluationGate(c EvaluationContract) (passed bool, reasons []string) {
	if !c.StrataComplete {
		reasons = append(reasons, "strata_incomplete")
	}
	if c.InvalidLabels != 0 {
		reasons = append(reasons, "invalid_labels_present")
	}
	if c.Accuracy < 0.95 {
		reasons = append(reasons, fmt.Sprintf("accuracy_lt_0.95:%.4f", c.Accuracy))
	}
	if c.FalsePositiveRate > 0.05 {
		reasons = append(reasons, fmt.Sprintf("fpr_gt_0.05:%.4f", c.FalsePositiveRate))
	}
	if c.CILowerAccuracy == 0 && c.CIUpperFPR == 0 {
		reasons = append(reasons, "ci_missing")
	} else if c.CILowerAccuracy < 0.95 || c.CIUpperFPR > 0.05 {
		reasons = append(reasons, "ci_out_of_bounds")
	}
	return len(reasons) == 0, reasons
}

// EvaluationService 评估 Run 终局:评估合同→闭包事实→真值表→三件套终态。
type EvaluationService struct {
	executor  EvaluationExecutor
	finalizer *FinalizerService
}

func NewEvaluationService(executor EvaluationExecutor, finalizer *FinalizerService) *EvaluationService {
	return &EvaluationService{executor: executor, finalizer: finalizer}
}

// EvaluateAndFinalize 执行评估并按门禁形成机器终局。
func (s *EvaluationService) EvaluateAndFinalize(ctx context.Context, tenantID, runID, executionSpecSHA256, packagePath string) (*EvaluationContract, error) {
	contract, err := s.executor.RunEvaluation(ctx, packagePath, runID, executionSpecSHA256)
	if err != nil {
		return nil, err
	}
	if contract == nil {
		return nil, fmt.Errorf("evaluator returned nil contract")
	}
	if contract.RunID != runID || contract.ExecutionSpecSHA256 != executionSpecSHA256 {
		return nil, fmt.Errorf("evaluation contract identity mismatch")
	}
	passed, reasons := EvaluationGate(*contract)

	facts := ClosureFactsForEvaluation(*contract, passed)
	findings, _ := json.Marshal(map[string]interface{}{
		"accuracy": contract.Accuracy, "false_positive_rate": contract.FalsePositiveRate,
		"ci_lower_accuracy": contract.CILowerAccuracy, "ci_upper_fpr": contract.CIUpperFPR,
		"gate_passed": passed, "gate_reasons": reasons,
	})
	limitations, _ := json.Marshal(map[string]interface{}{
		"package": contract.PackagePath, "strata_complete": contract.StrataComplete,
		"invalid_labels": contract.InvalidLabels, "sample_count": contract.SampleCount,
	})
	scope, _ := json.Marshal(map[string]string{
		"run_id": runID, "execution_spec_sha256": executionSpecSHA256,
	})
	_, err = s.finalizer.Finalize(ctx, FinalizeInput{
		TenantID:        tenantID,
		RunID:           runID,
		Facts:           facts,
		ScopeJSON:       scope,
		KeyFindingsJSON: findings,
		LimitationsJSON: limitations,
		EvidenceEntries: []byte(`[]`),
		DecisionInputs:  findings,
		NodeExactSet:    []byte(`["RECONCILE","MACHINE_FINALIZATION"]`),
		Differences:     []byte(`[]`),
	})
	if err != nil {
		return contract, err
	}
	return contract, nil
}

// ClosureFactsForEvaluation 评估结果→闭包事实(state.ClosureFacts)。
// 门禁失败=required node 失败语义(FAILED);门禁通过=SUCCEEDED,
// 结论按检测口径:有可信阳性→THREAT_FOUND,否则显式阴性→NO_THREAT_OBSERVED。
func ClosureFactsForEvaluation(c EvaluationContract, gatePassed bool) state.ClosureFacts {
	f := state.ClosureFacts{
		IdentityIntegrityOK:   true,
		FenceCountIntegrityOK: true,
	}
	if !gatePassed {
		f.RequiredNodeFailed = true
		return f
	}
	f.AllRequiredSucceeded = true
	f.EvidenceSufficient = true
	if c.DetectionRate > 0 {
		f.HasTrustedPositive = true
	} else {
		f.AllInputsExplicitNegative = true
	}
	return f
}
