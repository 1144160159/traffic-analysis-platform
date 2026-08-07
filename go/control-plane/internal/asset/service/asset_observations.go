package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

const (
	BindingChannelGRPC  = "grpc"
	BindingChannelKafka = "kafka"
)

// BindingProvenance identifies the delivery independently from the binding
// payload. Kafka offset is authoritative for broker replay; direct gRPC calls
// use the contract's observed_at plus their request identity.
type BindingProvenance struct {
	Channel     string
	Topic       string
	Partition   int
	Offset      int64
	MessageTime time.Time
	Actor       string
	TraceID     string
	RequestID   string
}

func (s *AssetService) RecordMacIpBinding(
	ctx context.Context,
	bindings []*config.MacIpBinding,
	provenance BindingProvenance,
) (accepted, rejected int32, err error) {
	if len(bindings) == 0 {
		return 0, 0, errors.New(errors.ErrCodeInvalidParameter, "at least one binding required")
	}
	provenance.Channel = strings.ToLower(strings.TrimSpace(provenance.Channel))
	provenance.Actor = strings.TrimSpace(provenance.Actor)
	provenance.TraceID = strings.TrimSpace(provenance.TraceID)
	if provenance.Actor == "" || provenance.TraceID == "" {
		return 0, 0, errors.New(errors.ErrCodeInvalidParameter, "binding actor and trace_id are required")
	}
	if provenance.Channel != BindingChannelGRPC && provenance.Channel != BindingChannelKafka {
		return 0, 0, errors.New(errors.ErrCodeInvalidParameter, "binding channel must be grpc or kafka")
	}
	if provenance.Channel == BindingChannelKafka && (strings.TrimSpace(provenance.Topic) == "" || provenance.Offset < 0) {
		return 0, 0, errors.New(errors.ErrCodeInvalidParameter, "Kafka topic and offset are required")
	}

	for index, binding := range bindings {
		if binding == nil || strings.TrimSpace(binding.MACAddress) == "" ||
			strings.TrimSpace(binding.IPAddress) == "" || strings.TrimSpace(binding.TenantID) == "" {
			rejected++
			continue
		}

		mac := normalizeMAC(binding.MACAddress)
		source := strings.TrimSpace(binding.Source)
		if source == "" {
			source = "passive"
		}
		observedAt := time.Time{}
		if binding.ObservedAt > 0 {
			observedAt = time.UnixMilli(binding.ObservedAt).UTC()
		} else if provenance.Channel == BindingChannelKafka && !provenance.MessageTime.IsZero() {
			observedAt = provenance.MessageTime.UTC()
		}
		if observedAt.IsZero() || observedAt.After(time.Now().UTC().Add(5*time.Minute)) {
			rejected++
			continue
		}

		identity := fmt.Sprintf(
			"grpc:%s:%s:%s:%s:%d:%d",
			strings.TrimSpace(binding.TenantID), mac, strings.TrimSpace(binding.IPAddress), source,
			observedAt.UnixMilli(), index,
		)
		if provenance.Channel == BindingChannelKafka {
			identity = fmt.Sprintf(
				"kafka:%s:%d:%d:%d",
				strings.TrimSpace(provenance.Topic), provenance.Partition, provenance.Offset, index,
			)
		}
		key := stableAssetCommandKey("asset-binding", identity)
		rec := &config.AssetRecord{
			TenantID:   strings.TrimSpace(binding.TenantID),
			IPAddress:  strings.TrimSpace(binding.IPAddress),
			MACAddress: mac,
			Source:     source,
			Vendor:     s.ouiCache.LookupVendor(mac),
		}
		_, upsertErr := s.UpsertAssetAtomic(ctx, rec, config.AssetUpsertCommand{
			ActionID:               config.AssetObservationUpsertAction,
			ResolveCurrentRevision: true,
			IdempotencyKey:         key,
			Actor:                  provenance.Actor,
			Reason:                 "passive MAC/IP binding observation",
			HistoryEventType:       "binding_observed",
			ObservedAt:             observedAt,
			TraceID:                provenance.TraceID,
			RequestID:              provenance.RequestID,
		})
		if upsertErr != nil {
			return accepted, rejected, fmt.Errorf("record binding %d: %w", index, upsertErr)
		}
		accepted++
	}
	return accepted, rejected, nil
}

func stableAssetCommandKey(namespace, identity string) string {
	digest := sha256.Sum256([]byte(namespace + "\x00" + identity))
	return namespace + ":sha256:" + hex.EncodeToString(digest[:])
}
