//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/dedup"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

const (
	kafkaBroker = "localhost:9092"
	kafkaTopic  = "detections.v1"
	redisAddr   = "localhost:6379"
)

func TestE2E_AlertDedup(t *testing.T) {
	ctx := context.Background()

	// 创建 Kafka 生产者
	writer := kafka.NewWriter(kafka.WriterConfig{
		Brokers: []string{kafkaBroker},
		Topic:   kafkaTopic,
	})
	defer writer.Close()

	// 创建 Redis 客户端
	rdb := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})
	defer rdb.Close()

	logger := zap.NewNop()
	redisDedup := dedup.NewRedisDedup(rdb, 10*time.Minute, logger)

	// 生成测试事件
	detection := &pb.DetectionEvent{
		Header: &pb.EventHeader{
			EventId:  "e2e-test-001",
			TenantId: "tenant-e2e",
			EventTs:  time.Now().UnixMilli(),
		},
		DetectionId:   "detection-e2e-001",
		CommunityId:   "1:e2e:test",
		DetectionType: "PortScan",
		Tuple: &pb.FiveTuple{
			SrcIp:    "192.168.1.100",
			DstIp:    "10.0.0.1",
			SrcPort:  12345,
			DstPort:  80,
			Protocol: 6,
		},
		Labels:   []string{"Reconnaissance"},
		Score:    0.95,
		Severity: "high",
	}

	// 序列化
	data, err := proto.Marshal(detection)
	if err != nil {
		t.Fatalf("Failed to marshal detection: %v", err)
	}

	// 发送到 Kafka
	err = writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(detection.Header.TenantId),
		Value: data,
	})
	if err != nil {
		t.Fatalf("Failed to write to Kafka: %v", err)
	}

	// 等待消费
	time.Sleep(3 * time.Second)

	// 验证去重
	fingerprint := dedup.CalculateFingerprint(detection, 10)
	count, err := redisDedup.GetCount(ctx, fingerprint)
	if err != nil {
		t.Fatalf("Failed to get dedup count: %v", err)
	}

	if count < 1 {
		t.Errorf("Expected count >= 1, got %d", count)
	}

	t.Logf("E2E test passed: fingerprint=%s, count=%d", fingerprint, count)
}

func TestE2E_HighThroughput(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping high throughput test in short mode")
	}

	ctx := context.Background()

	writer := kafka.NewWriter(kafka.WriterConfig{
		Brokers:   []string{kafkaBroker},
		Topic:     kafkaTopic,
		BatchSize: 100,
	})
	defer writer.Close()

	// 发送 10000 个事件
	totalEvents := 10000
	startTime := time.Now()

	for i := 0; i < totalEvents; i++ {
		detection := &pb.DetectionEvent{
			Header: &pb.EventHeader{
				EventId:  "perf-" + string(rune(i)),
				TenantId: "tenant-perf",
				EventTs:  time.Now().UnixMilli(),
			},
			DetectionId:   "detection-perf",
			CommunityId:   "1:perf:test",
			DetectionType: "Test",
			Tuple: &pb.FiveTuple{
				SrcIp: "192.168.1.1",
				DstIp: "10.0.0.1",
			},
			Score:    0.5,
			Severity: "low",
		}

		data, _ := proto.Marshal(detection)

		err := writer.WriteMessages(ctx, kafka.Message{
			Key:   []byte(detection.Header.TenantId),
			Value: data,
		})
		if err != nil {
			t.Errorf("Failed to write message %d: %v", i, err)
		}
	}

	elapsed := time.Since(startTime)
	tps := float64(totalEvents) / elapsed.Seconds()

	t.Logf("Throughput: %.2f TPS (%d events in %v)", tps, totalEvents, elapsed)

	if tps < 1000 {
		t.Errorf("Throughput too low: %.2f TPS", tps)
	}
}
