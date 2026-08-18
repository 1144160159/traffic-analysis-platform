package sourcequality

import (
	"fmt"
	"sort"
)

type PartitionExpectation struct {
	TenantID        string
	Rail            Rail
	ConsumerGroup   string
	Topic           string
	Partition       int
	FirstOffset     int64
	CommittedOffset int64
	LogEndOffset    int64
}

type PartitionResult struct {
	PartitionExpectation
	ReceiptCount   int
	MissingOffsets []int64
	ExtraOffsets   []int64
	Lag            int64
}

type Reconciliation struct {
	Partitions []PartitionResult
	Categories map[Category]int
	AllMatch   bool
}

var allRails = []Rail{RailFlow, RailAsset, RailDeviceLog, RailUserBehavior}

// ReconcileAllRails is the M06 completion gate. Partition-level callers may
// use Reconcile, while the milestone gate must prove that every source rail is
// explicitly represented in the expected committed ranges.
func ReconcileAllRails(
	receipts []Receipt,
	expectations []PartitionExpectation,
) (Reconciliation, error) {
	present := make(map[Rail]bool, len(allRails))
	for _, expectation := range expectations {
		present[expectation.Rail] = true
	}
	for _, rail := range allRails {
		if !present[rail] {
			return Reconciliation{}, fmt.Errorf("missing source rail expectation: %s", rail)
		}
	}
	return Reconcile(receipts, expectations)
}

// BuildMissingReceipts turns committed-offset gaps into deterministic terminal
// source-quality signals. Callers must persist and reuse observedAtMS for a
// reconciliation run so exact retries cannot drift the append-only detail.
func BuildMissingReceipts(
	reconciliation Reconciliation,
	observedAtMS int64,
) ([]Receipt, error) {
	if observedAtMS <= 0 {
		return nil, fmt.Errorf("missing-receipt observation time must be positive")
	}
	var receipts []Receipt
	for _, partition := range reconciliation.Partitions {
		for _, offset := range partition.MissingOffsets {
			receipt, err := Build(Input{
				TenantID:      partition.TenantID,
				Rail:          partition.Rail,
				ConsumerGroup: partition.ConsumerGroup,
				Source: SourceTuple{
					Topic: partition.Topic, Partition: partition.Partition, Offset: offset,
				},
				Category:     Missing,
				SourceSHA256: HashSource(nil),
				WatermarkMS:  -1,
				ObservedAtMS: observedAtMS,
				ReasonCode:   "MISSING_SOURCE_RECEIPT",
			})
			if err != nil {
				return nil, fmt.Errorf("build missing receipt: %w", err)
			}
			receipts = append(receipts, receipt)
		}
	}
	return receipts, nil
}

// Reconcile checks exactly the offsets below the committed next-offset. It does
// not interpret broker topic existence as a real-source receipt.
func Reconcile(receipts []Receipt, expectations []PartitionExpectation) (Reconciliation, error) {
	result := Reconciliation{Categories: make(map[Category]int), AllMatch: true}
	byPartition := make(map[string]map[int64]struct{}, len(expectations))
	for _, expectation := range expectations {
		if expectation.TenantID == "" || expectation.ConsumerGroup == "" || expectation.Topic == "" ||
			expectation.Partition < 0 || expectation.FirstOffset < 0 ||
			expectation.CommittedOffset < expectation.FirstOffset ||
			expectation.LogEndOffset < expectation.CommittedOffset {
			return Reconciliation{}, fmt.Errorf("invalid partition expectation")
		}
		key := partitionKey(expectation.TenantID, expectation.Rail, expectation.ConsumerGroup, expectation.Topic, expectation.Partition)
		if _, duplicate := byPartition[key]; duplicate {
			return Reconciliation{}, fmt.Errorf("duplicate partition expectation")
		}
		byPartition[key] = make(map[int64]struct{})
	}
	for _, receipt := range receipts {
		if _, err := receipt.CanonicalDetail(); err != nil {
			return Reconciliation{}, fmt.Errorf("invalid receipt %s: %w", receipt.ReceiptID, err)
		}
		key := partitionKey(receipt.TenantID, receipt.Rail, receipt.ConsumerGroup, receipt.Source.Topic, receipt.Source.Partition)
		offsets, expected := byPartition[key]
		if !expected {
			return Reconciliation{}, fmt.Errorf("receipt belongs to an unexpected tenant/source partition")
		}
		if _, duplicate := offsets[receipt.Source.Offset]; duplicate {
			return Reconciliation{}, fmt.Errorf("duplicate receipt source tuple")
		}
		offsets[receipt.Source.Offset] = struct{}{}
		result.Categories[receipt.Category]++
	}
	for _, expectation := range expectations {
		key := partitionKey(expectation.TenantID, expectation.Rail, expectation.ConsumerGroup, expectation.Topic, expectation.Partition)
		offsets := byPartition[key]
		partition := PartitionResult{PartitionExpectation: expectation, ReceiptCount: len(offsets), Lag: expectation.LogEndOffset - expectation.CommittedOffset}
		for offset := expectation.FirstOffset; offset < expectation.CommittedOffset; offset++ {
			if _, found := offsets[offset]; !found {
				partition.MissingOffsets = append(partition.MissingOffsets, offset)
			}
		}
		for offset := range offsets {
			if offset < expectation.FirstOffset || offset >= expectation.CommittedOffset {
				partition.ExtraOffsets = append(partition.ExtraOffsets, offset)
			}
		}
		sort.Slice(partition.ExtraOffsets, func(i, j int) bool { return partition.ExtraOffsets[i] < partition.ExtraOffsets[j] })
		if len(partition.MissingOffsets) != 0 || len(partition.ExtraOffsets) != 0 || partition.Lag != 0 {
			result.AllMatch = false
		}
		result.Partitions = append(result.Partitions, partition)
	}
	return result, nil
}

func partitionKey(tenantID string, rail Rail, group, topic string, partition int) string {
	return fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%d", tenantID, rail, group, topic, partition)
}
