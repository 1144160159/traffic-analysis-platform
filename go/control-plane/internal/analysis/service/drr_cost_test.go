package service

import (
	"encoding/json"
	"testing"
)

// §76.45.3 DRR 向量折算:冻结权重 v1 的确定性输出。
func TestDRRVectorCost(t *testing.T) {
	cases := []struct {
		name   string
		vector string
		want   int64
	}{
		{"empty vector defaults to one quantum", `{}`, 1000},
		{"null vector defaults to one quantum", `null`, 1000},
		{"cpu 2", `{"cpu":2}`, 2000},
		{"cpu 2 + memory 4GiB", `{"cpu":2,"memory_gb":4}`, 4000},
		{"gpu 1", `{"gpu":1}`, 16000},
		{"io 10 MB/s", `{"io_mbps":10}`, 1000},
		{"mixed", `{"cpu":1,"memory_gb":2,"gpu":0.5,"io_mbps":10}`, 11000},
		{"unknown fields ignored", `{"cpu":1,"whatever":99}`, 1000},
		{"fractional cpu rounds up", `{"cpu":1.2}`, 1200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DRRVectorCost(json.RawMessage(tc.vector))
			if err != nil {
				t.Fatalf("DRRVectorCost(%s): %v", tc.vector, err)
			}
			if got != tc.want {
				t.Fatalf("DRRVectorCost(%s)=%d want %d", tc.vector, got, tc.want)
			}
		})
	}
	if _, err := DRRVectorCost(json.RawMessage(`{"cpu":-1}`)); err == nil {
		t.Fatal("negative cpu must fail-closed")
	}
	if _, err := DRRVectorCost(json.RawMessage(`{"cpu":"two"}`)); err == nil {
		t.Fatal("non-numeric cpu must fail-closed")
	}
	if _, err := DRRVectorCost(json.RawMessage(`not json`)); err == nil {
		t.Fatal("malformed vector must fail-closed")
	}
}
