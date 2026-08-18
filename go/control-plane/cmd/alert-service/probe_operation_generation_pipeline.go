package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/api"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/consumer"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	segmentkafka "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

type ProbeOperationPipelineDeps struct {
	Kafka   config.KafkaConfig
	DB      *sql.DB
	Handler *api.SystemHandler
	Logger  *zap.Logger
}

type ProbeOperationPipelineRuntime struct {
	cancel context.CancelFunc
	done   []chan error

	generations []*commonkafka.GenerationConsumer
	dlqs        []*commonkafka.DLQProducer
	command     *commonkafka.KeyedProducer
	lifecycle   *commonkafka.KeyedProducer
	handler     *api.SystemHandler
	mu          sync.Mutex
	closed      bool
}

func initProbeOperationPipelines(
	ctx context.Context,
	deps ProbeOperationPipelineDeps,
	cfg config.ProbeOperationPipelineConfig,
) (*ProbeOperationPipelineRuntime, error) {
	if err := cfg.Validate(deps.Kafka); err != nil {
		return nil, err
	}
	runtime := &ProbeOperationPipelineRuntime{handler: deps.Handler}
	if cfg == (config.ProbeOperationPipelineConfig{}) {
		if deps.Handler != nil {
			deps.Handler.SetProbeOperationAckFeatureFlag(false)
		}
		return runtime, nil
	}
	if deps.DB == nil || deps.Handler == nil {
		return nil, fmt.Errorf("enabled probe operation pipeline requires PostgreSQL and SystemHandler")
	}
	pipelineCtx, cancel := context.WithCancel(ctx)
	runtime.cancel = cancel
	closeOnError := func(cause error) (*ProbeOperationPipelineRuntime, error) {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		_ = runtime.Close(shutdownCtx)
		return nil, cause
	}
	deps.Handler.SetProbeOperationAckFeatureFlag(cfg.DesiredWriterEnabled)

	if cfg.ControlPublisherEnabled {
		var err error
		runtime.command, err = commonkafka.NewKeyedProducer(commonkafka.ProducerConfig{
			Brokers: deps.Kafka.Brokers, Topic: "probe.control.v2", BatchSize: 100,
			RequiredAcks: "all", Compression: "lz4", Async: false, Security: deps.Kafka.Security,
		}, deps.Logger)
		if err != nil {
			return closeOnError(err)
		}
		deps.Handler.SetProbeOperationProducer(runtime.command)
	}
	if cfg.LifecyclePublisherEnabled {
		var err error
		runtime.lifecycle, err = commonkafka.NewKeyedProducer(commonkafka.ProducerConfig{
			Brokers: deps.Kafka.Brokers, Topic: deps.Kafka.ProbeEventTopic, BatchSize: 100,
			RequiredAcks: "all", Compression: "lz4", Async: false, Security: deps.Kafka.Security,
		}, deps.Logger)
		if err != nil {
			return closeOnError(err)
		}
		deps.Handler.SetProbeOperationEventProducer(runtime.lifecycle)
	}

	readinessStore, err := api.NewProbePipelineReadinessStore(deps.DB)
	if err != nil {
		return closeOnError(err)
	}
	if cfg.AckConsumerEnabled || cfg.LifecycleConsumerEnabled || cfg.ReadinessConsumerEnabled {
		if err := startProbeAuthorityGenerationConsumers(pipelineCtx, runtime, deps, cfg, readinessStore); err != nil {
			return closeOnError(err)
		}
	}
	if cfg.DispatcherEnabled {
		gate, err := api.NewProbeDispatcherGate(readinessStore)
		if err != nil {
			return closeOnError(err)
		}
		deps.Handler.SetProbeDispatcherGate(gate)
		if err := deps.Handler.StartProbeOperationOutboxWorker(pipelineCtx, 2*time.Second); err != nil {
			return closeOnError(err)
		}
	}
	return runtime, nil
}

func startProbeAuthorityGenerationConsumers(
	ctx context.Context,
	runtime *ProbeOperationPipelineRuntime,
	deps ProbeOperationPipelineDeps,
	cfg config.ProbeOperationPipelineConfig,
	readinessStore *api.ProbePipelineReadinessStore,
) error {
	fetcher, err := commonkafka.NewAssignedPartitionFetcher(deps.Kafka.Brokers, deps.Kafka.Security)
	if err != nil {
		return err
	}
	type generationSpec struct {
		enabled bool
		topic   string
		group   string
		role    config.ProbePipelineConsumerRole
		start   func(context.Context, *commonkafka.GenerationConsumer, *commonkafka.GenerationMessageProcessor) error
	}
	ackAdapter, err := consumer.NewProbeAckGenerationAdapter(deps.Handler, deps.Logger)
	if err != nil {
		return err
	}
	lifecycleAdapter, err := consumer.NewProbeOperationEventGenerationAdapter(deps.Handler, deps.Logger)
	if err != nil {
		return err
	}
	readinessAdapter, err := consumer.NewProbeReadinessConsumer(
		readinessStore, "ingest-gateway-probe-control-v2", "probe.control.v2",
	)
	if err != nil {
		return err
	}
	specs := []generationSpec{
		{cfg.AckConsumerEnabled, deps.Kafka.ProbeAckTopic, deps.Kafka.ProbeAckGroup,
			config.ProbeAckAuthorityConsumer, ackAdapter.StartGeneration},
		{cfg.LifecycleConsumerEnabled, deps.Kafka.ProbeEventTopic, deps.Kafka.ProbeEventGroup,
			config.ProbeLifecycleProjectionConsumer, lifecycleAdapter.StartGeneration},
		{cfg.ReadinessConsumerEnabled, deps.Kafka.ProbeGroupReadinessTopic, deps.Kafka.ProbeGroupReadinessGroup,
			"", readinessAdapter.StartGeneration},
	}
	readyChannels := make([]<-chan struct{}, 0, len(specs))
	for _, spec := range specs {
		if !spec.enabled {
			continue
		}
		runner, err := commonkafka.NewGenerationConsumer(commonkafka.GenerationConsumerConfig{
			Brokers: deps.Kafka.Brokers, Topic: spec.topic, GroupID: spec.group,
			StartOffset: segmentkafka.FirstOffset, Security: deps.Kafka.Security,
		}, deps.Logger)
		if err != nil {
			return err
		}
		dlq := commonkafka.NewDLQProducer(commonkafka.DLQConfig{
			Brokers: deps.Kafka.Brokers, TargetTopic: "dlq.v1", MaxRetries: 3,
			Security: deps.Kafka.Security,
		}, "alert-service-"+spec.group, deps.Logger)
		barrier, err := commonkafka.NewPostgresDLQAcknowledgementBarrier(deps.DB, spec.group)
		if err != nil {
			_ = runner.Close()
			_ = dlq.Close()
			return err
		}
		processor, err := commonkafka.NewGenerationMessageProcessor(commonkafka.GenerationMessageProcessorConfig{
			AssignedPartitionFetcher: fetcher, ErrorClassifier: commonkafka.IsPermanent,
			DLQProducer: dlq, DLQAcknowledgementBarrier: barrier,
			CommitOffsets: func(generation *segmentkafka.Generation, offsets map[string]map[int]int64) error {
				return generation.CommitOffsets(offsets)
			},
			CommitObserver: func(*commonkafka.ReceivedMessage, int64) {},
		})
		if err != nil {
			_ = runner.Close()
			_ = dlq.Close()
			return err
		}
		ready := make(chan struct{}, 1)
		if spec.role != "" {
			if err := bindLocalProbeReadiness(runner, readinessStore, spec.role, ready); err != nil {
				_ = runner.Close()
				_ = dlq.Close()
				return err
			}
		} else if err := runner.SetGroupLifecycleObserver(func(
			_ context.Context,
			receipt commonkafka.GroupLifecycleReceipt,
		) error {
			if receipt.State == commonkafka.GroupLifecycleReady {
				select {
				case <-ready:
				default:
					close(ready)
				}
			}
			return nil
		}); err != nil {
			_ = runner.Close()
			_ = dlq.Close()
			return err
		}
		done := make(chan error, 1)
		runtime.generations = append(runtime.generations, runner)
		runtime.dlqs = append(runtime.dlqs, dlq)
		runtime.done = append(runtime.done, done)
		start := spec.start
		go func() { done <- start(ctx, runner, processor) }()
		readyChannels = append(readyChannels, ready)
	}
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	for _, ready := range readyChannels {
		select {
		case <-ready:
		case <-deadline.C:
			return fmt.Errorf("probe authority generation assignment readiness timed out")
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func bindLocalProbeReadiness(
	runner *commonkafka.GenerationConsumer,
	store *api.ProbePipelineReadinessStore,
	role config.ProbePipelineConsumerRole,
	ready chan<- struct{},
) error {
	var mu sync.Mutex
	var renewCancel context.CancelFunc
	var renewDone chan error
	return runner.SetGroupLifecycleObserver(func(ctx context.Context, lifecycle commonkafka.GroupLifecycleReceipt) error {
		if lifecycle.State == commonkafka.GroupLifecycleAssigned {
			return nil
		}
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
		state := config.ProbePipelineRevoked
		expiresAt := time.Time{}
		if lifecycle.State == commonkafka.GroupLifecycleReady {
			state = config.ProbePipelineReady
			expiresAt = time.Now().UTC().Add(time.Minute)
		}
		receipt := config.ProbePipelineReadinessReceipt{
			PipelineID: config.ProbeOperationPipelineID, ConsumerRole: role,
			ConsumerGroup: lifecycle.GroupID, OwnerID: lifecycle.OwnerID + ":" + lifecycle.MemberID,
			OwnerEpoch: lifecycle.OwnerEpoch, State: state,
			ObservedAt: time.Now().UTC(), LeaseExpiresAt: expiresAt,
		}
		if err := store.IssueRenewRevoke(ctx, receipt); err != nil {
			return err
		}
		if state != config.ProbePipelineReady {
			return nil
		}
		select {
		case ready <- struct{}{}:
		default:
		}
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
				case now := <-ticker.C:
					receipt.ObservedAt = now.UTC()
					receipt.LeaseExpiresAt = now.UTC().Add(time.Minute)
					if err := store.IssueRenewRevoke(renewCtx, receipt); err != nil {
						done <- err
						return
					}
				}
			}
		}()
		return nil
	})
}

func (runtime *ProbeOperationPipelineRuntime) Close(ctx context.Context) error {
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
	if runtime.handler != nil {
		runtime.handler.SetProbeDispatcherGate(nil)
		runtime.handler.SetProbeOperationProducer(nil)
		runtime.handler.SetProbeOperationEventProducer(nil)
	}
	var result error
	for _, done := range runtime.done {
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) {
				result = errors.Join(result, err)
			}
		case <-ctx.Done():
			return errors.Join(result, ctx.Err())
		}
	}
	for _, generation := range runtime.generations {
		result = errors.Join(result, generation.Close())
	}
	for _, dlq := range runtime.dlqs {
		result = errors.Join(result, dlq.Close())
	}
	if runtime.command != nil {
		result = errors.Join(result, runtime.command.Close())
	}
	if runtime.lifecycle != nil {
		result = errors.Join(result, runtime.lifecycle.Close())
	}
	return result
}
