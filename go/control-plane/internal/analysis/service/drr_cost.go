// Package service DRR 向量折算(§76.45.3):cost 用冻结权重对 CPU/内存/GPU/IO
// 资源向量折算为 milli 单位标量,写入队列行与 DRR 台账。
// 权重为代码冻结常量 v1;变更必须升版并保留旧版读取(同 canonicalization 纪律)。
package service

import (
	"encoding/json"
	"fmt"
)

// drrWeightsV1 §76.45.3 冻结权重 v1(milli/单位):
// cpu=1000/core、memory=500/GiB、gpu=16000/device、io=100 per MB/s。
// 空向量按 1 quantum(1000 milli)计;非数值字段 fail-closed(不猜测)。
type drrWeightsV1 struct {
	cpuPerCore      int64
	memoryPerGiB    int64
	gpuPerDevice    int64
	ioPerMbps       int64
}

var frozenDRRWeights = drrWeightsV1{
	cpuPerCore:   1000,
	memoryPerGiB: 500,
	gpuPerDevice: 16000,
	ioPerMbps:    100,
}

// drrVector 资源向量(数值型;未知字段忽略以支持前向兼容)。
type drrVector struct {
	CPU      float64 `json:"cpu"`
	MemoryGB float64 `json:"memory_gb"`
	GPU      float64 `json:"gpu"`
	IOMbps   float64 `json:"io_mbps"`
}

// DRRQuantumMilli 单量子(milli):DRR 每轮公平服务单位。
const DRRQuantumMilli int64 = 1000

// DRRVectorCost 折算资源向量为 DRR cost(milli)。
// 空/非法 JSON 向量按最小 1 quantum;任一数值字段非法(非数/负数)返回错误。
func DRRVectorCost(vector json.RawMessage) (int64, error) {
	if len(vector) == 0 || string(vector) == "null" {
		return DRRQuantumMilli, nil
	}
	var v drrVector
	if err := json.Unmarshal(vector, &v); err != nil {
		return 0, fmt.Errorf("resource vector malformed: %w", err)
	}
	if v.CPU < 0 || v.MemoryGB < 0 || v.GPU < 0 || v.IOMbps < 0 {
		return 0, fmt.Errorf("resource vector quantities must be non-negative")
	}
	cost := int64(v.CPU*float64(frozenDRRWeights.cpuPerCore) +
		v.MemoryGB*float64(frozenDRRWeights.memoryPerGiB) +
		v.GPU*float64(frozenDRRWeights.gpuPerDevice) +
		v.IOMbps*float64(frozenDRRWeights.ioPerMbps) + 0.5)
	if cost < DRRQuantumMilli {
		cost = DRRQuantumMilli
	}
	return cost, nil
}
