package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/consumer"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/fusion"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	segmentkafka "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

var fusionCandidateSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type FusionProjectionPipelineDeps struct {
	Kafka           config.KafkaConfig
	Postgres        *sql.DB
	ClickHouse      *sql.DB
	CandidateSHA256 string
	Logger          *zap.Logger
}

type FusionProjectionPipelineRuntime struct {
	cancel    context.CancelFunc
	runner    *commonkafka.GenerationConsumer
	dlq       *commonkafka.DLQProducer
	done      chan error
	readiness *fusion.ReadinessStore

	mu     sync.Mutex
	closed bool
}

func initFusionProjectionPipeline(
	ctx context.Context,
	deps FusionProjectionPipelineDeps,
) (*FusionProjectionPipelineRuntime, error) {
	runtime := &FusionProjectionPipelineRuntime{}
	if !deps.Kafka.FusionProjectionEnabled {
		return runtime, nil
	}
	if deps.Postgres == nil || deps.ClickHouse == nil || len(deps.Kafka.Brokers) == 0 ||
		deps.Kafka.FusionCommandTopic != fusion.SourceSyncTopic ||
		deps.Kafka.FusionProjectionGroup != fusion.SourceSyncGroup ||
		!fusionCandidateSHA256Pattern.MatchString(deps.CandidateSHA256) {
		return nil, fmt.Errorf("enabled fusion projection requires PostgreSQL, ClickHouse, exact topic/group and candidate SHA-256")
	}
	pipelineCtx, cancel := context.WithCancel(ctx)
	runtime.cancel = cancel
	closeOnError := func(cause error) (*FusionProjectionPipelineRuntime, error) {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		_ = runtime.Close(shutdownCtx)
		return nil, cause
	}
	reader, err := fusion.NewClickHouseSourceFactReader(deps.ClickHouse)
	if err != nil {
		return closeOnError(err)
	}
	projector, err := fusion.NewProjector(deps.Postgres, reader, fusion.MaxSourceFacts)
	if err != nil {
		return closeOnError(err)
	}
	adapter, err := consumer.NewFusionProjectionGenerationAdapter(projector, deps.Logger)
	if err != nil {
		return closeOnError(err)
	}
	runtime.runner, err = commonkafka.NewGenerationConsumer(commonkafka.GenerationConsumerConfig{
		Brokers: deps.Kafka.Brokers, Topic: deps.Kafka.FusionCommandTopic,
		GroupID: deps.Kafka.FusionProjectionGroup, StartOffset: segmentkafka.FirstOffset,
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
		Brokers: deps.Kafka.Brokers, TargetTopic: "dlq.v1", MaxRetries: 3, Security: deps.Kafka.Security,
	}, "alert-service-fusion-projection-v1", deps.Logger)
	barrier, err := commonkafka.NewPostgresDLQAcknowledgementBarrier(deps.Postgres, deps.Kafka.FusionProjectionGroup)
	if err != nil {
		return closeOnError(err)
	}
	processor, err := commonkafka.NewGenerationMessageProcessor(commonkafka.GenerationMessageProcessorConfig{
		AssignedPartitionFetcher: fetcher, ErrorClassifier: commonkafka.IsPermanent,
		DLQProducer: runtime.dlq, DLQAcknowledgementBarrier: barrier,
		CommitOffsets: func(generation *segmentkafka.Generation, offsets map[string]map[int]int64) error {
			return generation.CommitOffsets(offsets)
		},
		CommitObserver: func(message *commonkafka.ReceivedMessage, nextOffset int64) {
			if deps.Logger != nil {
				deps.Logger.Debug("Fusion projection offset committed", zap.String("topic", message.Topic),
					zap.Int("partition", message.Partition), zap.Int64("next_offset", nextOffset))
			}
		},
	})
	if err != nil {
		return closeOnError(err)
	}
	readiness, err := fusion.NewReadinessStore(deps.Postgres, deps.Kafka.FusionProjectionGroup)
	if err != nil {
		return closeOnError(err)
	}
	runtime.readiness = readiness
	ready := make(chan struct{})
	if err := bindFusionProjectionReadiness(runtime.runner, readiness, deps.CandidateSHA256, ready); err != nil {
		return closeOnError(err)
	}
	runtime.done = make(chan error, 1)
	go func() { runtime.done <- adapter.StartGeneration(pipelineCtx, runtime.runner, processor) }()
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	select {
	case <-ready:
		return runtime, nil
	case err := <-runtime.done:
		if err == nil {
			err = fmt.Errorf("fusion projection consumer stopped before readiness")
		}
		return closeOnError(err)
	case <-timer.C:
		return closeOnError(fmt.Errorf("fusion projection generation assignment readiness timed out"))
	case <-ctx.Done():
		return closeOnError(ctx.Err())
	}
}

func bindFusionProjectionReadiness(
	runner *commonkafka.GenerationConsumer,
	store *fusion.ReadinessStore,
	candidateSHA256 string,
	ready chan<- struct{},
) error {
	if runner == nil || store == nil || ready == nil {
		return fmt.Errorf("fusion readiness runner store and channel are required")
	}
	var mu sync.Mutex
	var renewCancel context.CancelFunc
	var renewDone chan error
	var readyOnce sync.Once
	return runner.SetGroupLifecycleObserver(func(ctx context.Context, lifecycle commonkafka.GroupLifecycleReceipt) error {
		mu.Lock()
		cancel, done := renewCancel, renewDone
		if lifecycle.State != commonkafka.GroupLifecycleReady {
			renewCancel, renewDone = nil, nil
		}
		mu.Unlock()
		if cancel != nil {
			cancel()
		}
		if done != nil {
			if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
				return err
			}
		}
		lease := time.Duration(0)
		if lifecycle.State == commonkafka.GroupLifecycleReady {
			lease = time.Minute
		}
		if err := store.RecordLifecycle(ctx, lifecycle, candidateSHA256, lease); err != nil {
			return err
		}
		if lifecycle.State != commonkafka.GroupLifecycleReady {
			return nil
		}
		readyOnce.Do(func() { close(ready) })
		renewCtx, cancel := context.WithCancel(ctx)
		done = make(chan error, 1)
		mu.Lock()
		renewCancel, renewDone = cancel, done
		mu.Unlock()
		go func() {
			ticker := time.NewTicker(20 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-renewCtx.Done():
					done <- renewCtx.Err()
					return
				case observedAt := <-ticker.C:
					renewed := lifecycle
					renewed.ObservedAt = observedAt.UTC()
					if err := store.RecordLifecycle(renewCtx, renewed, candidateSHA256, time.Minute); err != nil {
						done <- err
						return
					}
				}
			}
		}()
		return nil
	})
}

func (runtime *FusionProjectionPipelineRuntime) Close(ctx context.Context) error {
	if runtime == nil {
		return nil
	}
	runtime.mu.Lock()
	if runtime.closed {
		runtime.mu.Unlock()
		return nil
	}
	runtime.closed = true
	cancel, runner, dlq, done := runtime.cancel, runtime.runner, runtime.dlq, runtime.done
	runtime.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if runner != nil {
		_ = runner.Close()
	}
	var runErr error
	if done != nil {
		select {
		case runErr = <-done:
			if errors.Is(runErr, context.Canceled) || errors.Is(runErr, segmentkafka.ErrGroupClosed) {
				runErr = nil
			}
		case <-ctx.Done():
			runErr = ctx.Err()
		}
	}
	if dlq != nil {
		if err := dlq.Close(); runErr == nil {
			runErr = err
		}
	}
	return runErr
}
