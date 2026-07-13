package consumer

import (
	"context"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/service"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

type BindingConsumer struct {
	reader *kafka.Reader
	svc    *service.AssetService
	logger *zap.Logger
}

func NewBindingConsumer(cfg config.KafkaConfig, svc *service.AssetService, logger *zap.Logger) (*BindingConsumer, error) {
	brokers := cfg.BrokerList()
	if len(brokers) == 0 {
		return nil, fmt.Errorf("asset kafka brokers required")
	}
	if cfg.Topic == "" {
		return nil, fmt.Errorf("asset kafka topic required")
	}
	if cfg.GroupID == "" {
		return nil, fmt.Errorf("asset kafka group id required")
	}
	if cfg.MinBytes <= 0 {
		cfg.MinBytes = 1
	}
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = 1 << 20
	}

	dialer, err := cfg.Security.Dialer("asset-service-binding-consumer")
	if err != nil {
		return nil, err
	}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        brokers,
		Topic:          cfg.Topic,
		GroupID:        cfg.GroupID,
		Dialer:         dialer,
		MinBytes:       cfg.MinBytes,
		MaxBytes:       cfg.MaxBytes,
		CommitInterval: time.Second,
		StartOffset:    kafka.FirstOffset,
	})

	return &BindingConsumer{reader: reader, svc: svc, logger: logger}, nil
}

func (c *BindingConsumer) Run(ctx context.Context) {
	c.logger.Info("asset binding consumer started")
	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			c.logger.Warn("fetch asset binding message failed", zap.Error(err))
			continue
		}

		bindings, err := decodeBindings(msg.Value)
		if err != nil {
			c.logger.Warn("decode asset binding message failed",
				zap.Int("partition", msg.Partition),
				zap.Int64("offset", msg.Offset),
				zap.Error(err))
			_ = c.reader.CommitMessages(ctx, msg)
			continue
		}

		accepted, rejected, err := c.svc.RecordMacIpBinding(ctx, bindings)
		if err != nil {
			c.logger.Warn("record asset bindings failed",
				zap.Int("bindings", len(bindings)),
				zap.Error(err))
			continue
		}

		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			c.logger.Warn("commit asset binding message failed",
				zap.Int("partition", msg.Partition),
				zap.Int64("offset", msg.Offset),
				zap.Error(err))
			continue
		}

		c.logger.Info("asset bindings recorded",
			zap.Int32("accepted", accepted),
			zap.Int32("rejected", rejected),
			zap.Int("partition", msg.Partition),
			zap.Int64("offset", msg.Offset))
	}
}

func (c *BindingConsumer) Close() error {
	return c.reader.Close()
}

func decodeBindings(data []byte) ([]*config.MacIpBinding, error) {
	var batch pb.RecordMacIpBindingRequest
	if err := proto.Unmarshal(data, &batch); err == nil && len(batch.Bindings) > 0 {
		return protoBindingsToConfig(batch.Bindings), nil
	}

	var single pb.MacIpBinding
	if err := proto.Unmarshal(data, &single); err != nil {
		return nil, err
	}
	if single.MacAddress == "" && single.IpAddress == "" {
		return nil, fmt.Errorf("empty asset binding message")
	}
	return protoBindingsToConfig([]*pb.MacIpBinding{&single}), nil
}

func protoBindingsToConfig(bindings []*pb.MacIpBinding) []*config.MacIpBinding {
	out := make([]*config.MacIpBinding, 0, len(bindings))
	for _, binding := range bindings {
		if binding == nil {
			continue
		}
		out = append(out, &config.MacIpBinding{
			MACAddress: binding.MacAddress,
			IPAddress:  binding.IpAddress,
			TenantID:   binding.TenantId,
			ObservedAt: binding.ObservedAt,
			Source:     binding.Source,
		})
	}
	return out
}
