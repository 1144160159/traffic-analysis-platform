package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	commoncontracts "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/contracts"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	ingestconfig "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/control"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/server"
	"github.com/redis/go-redis/v9"
	segmentkafka "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

type ProbeControlPipelineDeps struct {
	Kafka    ingestconfig.KafkaConfig
	Redis    redis.UniversalClient
	Postgres *sql.DB
	Handler  *server.IngestHandler
	Logger   *zap.Logger
}

type ProbeControlPipelineRuntime struct {
	Generation *commonkafka.GenerationConsumer
	Bridge     *control.Bridge

	cancel    context.CancelFunc
	done      chan error
	ack       *commonkafka.KeyedProducer
	receipts  *commonkafka.KeyedProducer
	readiness *commonkafka.KeyedProducer
	dlq       *commonkafka.DLQProducer
	handler   *server.IngestHandler
	revoked   chan struct{}
	mu        sync.Mutex
	closed    bool
	admitted  bool
	bridgeSet bool
}

func initProbeControlPipeline(
	ctx context.Context,
	deps ProbeControlPipelineDeps,
	cfg ingestconfig.ProbeControlPipelineConfig,
) (*ProbeControlPipelineRuntime, error) {
	if err := cfg.Validate(deps.Kafka); err != nil {
		return nil, err
	}
	if cfg == (ingestconfig.ProbeControlPipelineConfig{}) {
		return &ProbeControlPipelineRuntime{}, nil
	}
	if deps.Redis == nil || deps.Postgres == nil || deps.Handler == nil {
		return nil, fmt.Errorf("enabled probe control pipeline requires Redis PostgreSQL and handler")
	}
	if !cfg.CommandConsumerEnabled {
		return nil, fmt.Errorf("enabled probe control capabilities require generation command consumer ownership")
	}
	store, err := control.NewRedisCommandStore(deps.Redis, 24*time.Hour)
	if err != nil {
		return nil, err
	}
	router, err := control.NewRouter(store)
	if err != nil {
		return nil, err
	}
	runtime := &ProbeControlPipelineRuntime{handler: deps.Handler, revoked: make(chan struct{})}
	closeOnError := func(cause error) (*ProbeControlPipelineRuntime, error) {
		_ = runtime.Close(context.Background())
		return nil, cause
	}
	if cfg.AckPublisherEnabled {
		runtime.ack, err = commonkafka.NewKeyedProducer(commonkafka.ProducerConfig{
			Brokers: deps.Kafka.Brokers, Topic: deps.Kafka.ProbeAckTopic,
			BatchSize: 100, BatchTimeout: 100 * time.Millisecond, MaxAttempts: deps.Kafka.MaxRetries,
			RequiredAcks: "all", Compression: "lz4", Async: false, Security: deps.Kafka.Security,
		}, deps.Logger)
		if err != nil {
			return closeOnError(err)
		}
		runtime.Bridge, err = control.NewBridge(store, &control.KafkaAckPublisher{Producer: runtime.ack})
		if err != nil {
			return closeOnError(err)
		}
		runtime.Bridge.SetLogger(deps.Logger)
		// 采集类操作 ACK → analysis.receipts.v1(调度中心回执权威输入;fail-closed)
		runtime.receipts, err = commonkafka.NewKeyedProducer(commonkafka.ProducerConfig{
			Brokers: deps.Kafka.Brokers, Topic: commoncontracts.TopicAnalysisReceipts,
			BatchSize: 100, BatchTimeout: 100 * time.Millisecond, MaxAttempts: deps.Kafka.MaxRetries,
			RequiredAcks: "all", Compression: "lz4", Async: false, Security: deps.Kafka.Security,
		}, deps.Logger)
		if err != nil {
			return closeOnError(err)
		}
		runtime.Bridge.SetStageReceiptPublisher(&control.KafkaStageReceiptPublisher{Producer: runtime.receipts})
	}
	runtime.Generation, err = commonkafka.NewGenerationConsumer(commonkafka.GenerationConsumerConfig{
		Brokers: deps.Kafka.Brokers, Topic: deps.Kafka.ProbeControlTopic,
		GroupID: deps.Kafka.ProbeControlGroup, StartOffset: segmentkafka.FirstOffset,
		Security: deps.Kafka.Security,
	}, deps.Logger)
	if err != nil {
		return closeOnError(err)
	}
	fetcher, err := commonkafka.NewAssignedPartitionFetcher(deps.Kafka.Brokers, deps.Kafka.Security)
	if err != nil {
		return closeOnError(err)
	}
	runtime.dlq = commonkafka.NewDLQProducer(commonkafka.DLQConfig{
		Brokers: deps.Kafka.Brokers, TargetTopic: deps.Kafka.DLQTopic,
		MaxRetries: deps.Kafka.MaxRetries, Security: deps.Kafka.Security,
	}, "ingest-gateway-probe-control-v2", deps.Logger)
	barrier, err := commonkafka.NewPostgresDLQAcknowledgementBarrier(deps.Postgres, deps.Kafka.ProbeControlGroup)
	if err != nil {
		return closeOnError(err)
	}
	processor, err := commonkafka.NewGenerationMessageProcessor(commonkafka.GenerationMessageProcessorConfig{
		AssignedPartitionFetcher: fetcher, ErrorClassifier: commonkafka.IsPermanent,
		DLQProducer: runtime.dlq, DLQAcknowledgementBarrier: barrier,
		CommitOffsets: func(generation *segmentkafka.Generation, offsets map[string]map[int]int64) error {
			return generation.CommitOffsets(offsets)
		},
		CommitObserver: func(*commonkafka.ReceivedMessage, int64) {},
	})
	if err != nil {
		return closeOnError(err)
	}
	ready := make(chan struct{})
	var readyOnce sync.Once
	var revokedOnce sync.Once
	markReady := func() { readyOnce.Do(func() { close(ready) }) }
	markRevoked := func() { revokedOnce.Do(func() { close(runtime.revoked) }) }
	if cfg.ReadinessPublisherEnabled {
		runtime.readiness, err = commonkafka.NewKeyedProducer(commonkafka.ProducerConfig{
			Brokers: deps.Kafka.Brokers, Topic: deps.Kafka.ProbeGroupReadinessTopic,
			BatchSize: 1, RequiredAcks: "all", Compression: "lz4", Async: false,
			MaxAttempts: deps.Kafka.MaxRetries, Security: deps.Kafka.Security,
		}, deps.Logger)
		if err != nil {
			return closeOnError(err)
		}
		publisher, publisherErr := control.NewProbeControlReadinessPublisher(runtime.readiness, "")
		if publisherErr != nil {
			return closeOnError(publisherErr)
		}
		if err := wireProbeControlGroupLifecycle(
			runtime.Generation, publisher, time.Minute, deps.Logger, markReady, markRevoked,
		); err != nil {
			return closeOnError(err)
		}
	} else if err := runtime.Generation.SetGroupLifecycleObserver(func(
		_ context.Context,
		receipt commonkafka.GroupLifecycleReceipt,
	) error {
		if receipt.State == commonkafka.GroupLifecycleReady {
			markReady()
		}
		if receipt.State == commonkafka.GroupLifecycleRevoked {
			markRevoked()
		}
		return nil
	}); err != nil {
		return closeOnError(err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	runtime.cancel = cancel
	runtime.done = make(chan error, 1)
	go func() { runtime.done <- router.StartGeneration(runCtx, runtime.Generation, processor) }()
	startupTimer := time.NewTimer(30 * time.Second)
	defer startupTimer.Stop()
	select {
	case <-ready:
		runtime.admitted = true
	case runErr := <-runtime.done:
		runtime.done = nil
		return closeOnError(fmt.Errorf("probe command generation stopped before assignment readiness: %w", runErr))
	case <-startupTimer.C:
		return closeOnError(fmt.Errorf("probe command generation assignment readiness timed out"))
	case <-ctx.Done():
		return closeOnError(ctx.Err())
	}
	if cfg.HeartbeatDeliveryEnabled {
		if runtime.Bridge == nil {
			return closeOnError(fmt.Errorf("probe heartbeat delivery requires ACK bridge"))
		}
		deps.Handler.SetProbeControlBridge(runtime.Bridge)
		runtime.bridgeSet = true
	}
	return runtime, nil
}

func wireProbeControlGroupLifecycle(
	runner *commonkafka.GenerationConsumer,
	publisher *control.ProbeControlReadinessPublisher,
	leaseTTL time.Duration,
	logger *zap.Logger,
	onReady func(),
	onRevoked func(),
) error {
	if runner == nil || publisher == nil {
		return fmt.Errorf("probe command generation and readiness publisher are required")
	}
	var mu sync.Mutex
	var renewalCancel context.CancelFunc
	var renewalDone chan error
	stopRenewal := func() error {
		mu.Lock()
		cancel, done := renewalCancel, renewalDone
		renewalCancel, renewalDone = nil, nil
		mu.Unlock()
		if cancel != nil {
			cancel()
		}
		if done != nil {
			if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
				return err
			}
		}
		return nil
	}
	return runner.SetGroupLifecycleObserver(func(ctx context.Context, receipt commonkafka.GroupLifecycleReceipt) error {
		if receipt.State == commonkafka.GroupLifecycleRevoked || receipt.State == commonkafka.GroupLifecycleStopped {
			if err := stopRenewal(); err != nil {
				return err
			}
		}
		if _, err := publisher.Publish(ctx, receipt, leaseTTL); err != nil {
			return err
		}
		if receipt.State == commonkafka.GroupLifecycleRevoked && onRevoked != nil {
			onRevoked()
		}
		if receipt.State != commonkafka.GroupLifecycleReady {
			return nil
		}
		if onReady != nil {
			onReady()
		}
		renewCtx, cancel := context.WithCancel(ctx)
		done := make(chan error, 1)
		mu.Lock()
		renewalCancel, renewalDone = cancel, done
		mu.Unlock()
		go func() {
			err := publisher.RunRenewal(renewCtx, receipt, leaseTTL)
			if err != nil && !errors.Is(err, context.Canceled) && logger != nil {
				logger.Error("Probe command readiness renewal stopped", zap.Error(err))
			}
			done <- err
		}()
		return nil
	})
}

func (runtime *ProbeControlPipelineRuntime) Close(ctx context.Context) error {
	if runtime == nil {
		return nil
	}
	runtime.mu.Lock()
	if runtime.closed {
		runtime.mu.Unlock()
		return nil
	}
	runtime.closed = true
	runtime.mu.Unlock()
	if runtime.cancel != nil {
		runtime.cancel()
	}
	var result error
	if runtime.admitted && runtime.revoked != nil {
		select {
		case <-runtime.revoked:
		case <-ctx.Done():
			result = ctx.Err()
		}
	}
	if runtime.bridgeSet && runtime.handler != nil {
		runtime.handler.SetProbeControlBridge(nil)
	}
	if runtime.done != nil {
		select {
		case err := <-runtime.done:
			if err != nil && !errors.Is(err, context.Canceled) {
				result = err
			}
		case <-ctx.Done():
			result = ctx.Err()
		}
	}
	if runtime.Generation != nil {
		result = errors.Join(result, runtime.Generation.Close())
	}
	if runtime.dlq != nil {
		result = errors.Join(result, runtime.dlq.Close())
	}
	if runtime.readiness != nil {
		result = errors.Join(result, runtime.readiness.Close())
	}
	if runtime.ack != nil {
		result = errors.Join(result, runtime.ack.Close())
		if runtime.receipts != nil {
			result = errors.Join(result, runtime.receipts.Close())
		}
	}
	return result
}
