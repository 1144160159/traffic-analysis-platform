// Package sourcequality records immutable ingress classifications without
// creating a second data-quality governance authority.
package sourcequality

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

type Rail string

const (
	RailFlow         Rail = "flow"
	RailAsset        Rail = "asset"
	RailDeviceLog    Rail = "device_log"
	RailUserBehavior Rail = "user_behavior"
)

type Category string

const (
	Accepted  Category = "accepted"
	Rejected  Category = "rejected"
	Invalid   Category = "invalid"
	Late      Category = "late"
	Duplicate Category = "duplicate"
	Conflict  Category = "conflict"
	Missing   Category = "missing"
)

var (
	hexSHA256  = regexp.MustCompile(`^[0-9a-f]{64}$`)
	validRails = map[Rail]struct{}{
		RailFlow: {}, RailAsset: {}, RailDeviceLog: {}, RailUserBehavior: {},
	}
	validCategories = map[Category]struct{}{
		Accepted: {}, Rejected: {}, Invalid: {}, Late: {}, Duplicate: {}, Conflict: {}, Missing: {},
	}
	ErrReceiptConflict = errors.New("source quality receipt identity collision")
)

type SourceTuple struct {
	Topic     string `json:"topic"`
	Partition int    `json:"partition"`
	Offset    int64  `json:"offset"`
}

type Receipt struct {
	ContractVersion string      `json:"contract_version"`
	ReceiptID       string      `json:"receipt_id"`
	TenantID        string      `json:"tenant_id"`
	Rail            Rail        `json:"rail"`
	ConsumerGroup   string      `json:"consumer_group"`
	Source          SourceTuple `json:"source"`
	Category        Category    `json:"category"`
	EventID         string      `json:"event_id"`
	SourceSHA256    string      `json:"source_sha256"`
	WatermarkMS     int64       `json:"watermark_ms"`
	ObservedAtMS    int64       `json:"observed_at_ms"`
	ReasonCode      string      `json:"reason_code"`
}

type Input struct {
	TenantID      string
	Rail          Rail
	ConsumerGroup string
	Source        SourceTuple
	Category      Category
	EventID       string
	SourceSHA256  string
	WatermarkMS   int64
	ObservedAtMS  int64
	ReasonCode    string
}

func Build(input Input) (Receipt, error) {
	input.TenantID = strings.TrimSpace(input.TenantID)
	input.ConsumerGroup = strings.TrimSpace(input.ConsumerGroup)
	input.Source.Topic = strings.TrimSpace(input.Source.Topic)
	input.EventID = strings.TrimSpace(input.EventID)
	input.SourceSHA256 = strings.TrimSpace(input.SourceSHA256)
	input.ReasonCode = strings.TrimSpace(input.ReasonCode)
	if input.TenantID == "" || input.ConsumerGroup == "" || input.Source.Topic == "" {
		return Receipt{}, fmt.Errorf("tenant_id, consumer_group and source topic are required")
	}
	if _, ok := validRails[input.Rail]; !ok {
		return Receipt{}, fmt.Errorf("unsupported source rail %q", input.Rail)
	}
	if _, ok := validCategories[input.Category]; !ok {
		return Receipt{}, fmt.Errorf("unsupported source category %q", input.Category)
	}
	if input.Source.Partition < 0 || input.Source.Offset < 0 {
		return Receipt{}, fmt.Errorf("source partition and offset must not be negative")
	}
	if !hexSHA256.MatchString(input.SourceSHA256) {
		return Receipt{}, fmt.Errorf("source_sha256 must be lowercase SHA-256")
	}
	if input.ObservedAtMS <= 0 {
		return Receipt{}, fmt.Errorf("observed_at_ms must be positive")
	}
	if input.Category == Accepted && input.ReasonCode != "" {
		return Receipt{}, fmt.Errorf("accepted receipt must not have a reason_code")
	}
	if input.Category != Accepted && input.ReasonCode == "" {
		return Receipt{}, fmt.Errorf("non-accepted receipt requires reason_code")
	}
	identity := fmt.Sprintf("source-quality/v1\x00%s\x00%s\x00%s\x00%s\x00%d\x00%d",
		input.TenantID, input.Rail, input.ConsumerGroup,
		input.Source.Topic, input.Source.Partition, input.Source.Offset)
	sum := sha256.Sum256([]byte(identity))
	return Receipt{
		ContractVersion: "source-quality-receipt/v1",
		ReceiptID:       "source-quality-" + hex.EncodeToString(sum[:]),
		TenantID:        input.TenantID,
		Rail:            input.Rail,
		ConsumerGroup:   input.ConsumerGroup,
		Source:          input.Source,
		Category:        input.Category,
		EventID:         input.EventID,
		SourceSHA256:    input.SourceSHA256,
		WatermarkMS:     input.WatermarkMS,
		ObservedAtMS:    input.ObservedAtMS,
		ReasonCode:      input.ReasonCode,
	}, nil
}

func (r Receipt) CanonicalDetail() ([]byte, error) {
	if rebuilt, err := Build(Input{
		TenantID: r.TenantID, Rail: r.Rail, ConsumerGroup: r.ConsumerGroup,
		Source: r.Source, Category: r.Category, EventID: r.EventID,
		SourceSHA256: r.SourceSHA256, WatermarkMS: r.WatermarkMS,
		ObservedAtMS: r.ObservedAtMS, ReasonCode: r.ReasonCode,
	}); err != nil {
		return nil, err
	} else if rebuilt.ReceiptID != r.ReceiptID || r.ContractVersion != "source-quality-receipt/v1" {
		return nil, fmt.Errorf("receipt identity or contract version is inconsistent")
	}
	return json.Marshal(r)
}

func HashSource(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
