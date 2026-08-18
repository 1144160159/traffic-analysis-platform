package dataquality

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

func (m *Monitor) CollectAndPersistHandoffSignals(ctx context.Context, tenantID, traceID string, collector *CompositeSignalCollector) ([]HandoffSignal, error) {
	if m == nil || m.controlDB == nil {
		return nil, fmt.Errorf("PostgreSQL data quality control plane not available")
	}
	if collector == nil || tenantID == "" || traceID == "" {
		return nil, fmt.Errorf("tenant_id, trace_id and signal collector are required")
	}
	request := SignalCollectionRequest{TenantID: tenantID, DatasetID: collector.Contract().DatasetID, TraceID: traceID, CollectedAt: time.Now().UTC()}
	signals := collector.Collect(ctx, request)
	if err := m.persistHandoffSignals(ctx, collector.Contract(), signals); err != nil {
		return nil, err
	}
	return signals, nil
}

func (m *Monitor) persistHandoffSignals(ctx context.Context, contract DatasetSignalContract, signals []HandoffSignal) error {
	if err := validateDatasetSignalContract(contract); err != nil {
		return err
	}
	if len(signals) != len(contract.Signals) {
		return fmt.Errorf("dataset %s collected %d signals, want %d", contract.DatasetID, len(signals), len(contract.Signals))
	}
	tenantID := signals[0].TenantID
	traceID := signals[0].TraceID
	if tenantID == "" || traceID == "" {
		return fmt.Errorf("signal tenant_id and trace_id are required")
	}
	keysJSON, err := json.Marshal(contract.BusinessKeys)
	if err != nil {
		return fmt.Errorf("marshal dataset business keys: %w", err)
	}
	upstreamsJSON, err := json.Marshal(contract.Upstreams)
	if err != nil {
		return fmt.Errorf("marshal dataset upstreams: %w", err)
	}
	downstreamsJSON, err := json.Marshal(contract.Downstreams)
	if err != nil {
		return fmt.Errorf("marshal dataset downstreams: %w", err)
	}
	tx, err := m.controlDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin hand-off signal transaction: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO data_quality_datasets (
			tenant_id,dataset_id,display_name,owner,schema_version,signal_contract_version,
			business_keys,allowed_lateness_seconds,retention_seconds,upstreams,downstreams,
			slo_target,status,revision,trace_id,created_at,updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8,$9,$10::jsonb,$11::jsonb,$12,'active',1,$13,now(),now())
		ON CONFLICT (tenant_id,dataset_id) DO UPDATE SET
			display_name=EXCLUDED.display_name,owner=EXCLUDED.owner,schema_version=EXCLUDED.schema_version,
			signal_contract_version=EXCLUDED.signal_contract_version,business_keys=EXCLUDED.business_keys,
			allowed_lateness_seconds=EXCLUDED.allowed_lateness_seconds,retention_seconds=EXCLUDED.retention_seconds,
			upstreams=EXCLUDED.upstreams,downstreams=EXCLUDED.downstreams,slo_target=EXCLUDED.slo_target,
			status='active',trace_id=EXCLUDED.trace_id,updated_at=now()
	`, tenantID, contract.DatasetID, contract.DisplayName, contract.Owner, contract.SchemaVersion, contract.ContractVersion,
		string(keysJSON), contract.AllowedLatenessSecond, contract.RetentionSeconds, string(upstreamsJSON), string(downstreamsJSON), contract.SLOTarget, traceID); err != nil {
		return fmt.Errorf("upsert data quality dataset contract: %w", err)
	}
	seen := make(map[string]bool, len(signals))
	for _, signal := range signals {
		if signal.TenantID != tenantID || signal.DatasetID != contract.DatasetID || signal.TraceID != traceID {
			return fmt.Errorf("signal identity does not match collection transaction")
		}
		if err := validateHandoffSignal(signal); err != nil {
			return err
		}
		if seen[signal.SourceKind] {
			return fmt.Errorf("duplicate signal kind %s", signal.SourceKind)
		}
		seen[signal.SourceKind] = true
		metadataJSON, err := json.Marshal(signal.Metadata)
		if err != nil {
			return fmt.Errorf("marshal %s signal metadata: %w", signal.SourceKind, err)
		}
		var watermarkValue interface{}
		if signal.WatermarkValue != nil {
			watermarkValue = *signal.WatermarkValue
		}
		var observedAt interface{}
		if signal.ObservedAt != nil {
			observedAt = *signal.ObservedAt
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO data_quality_watermarks (
				tenant_id,dataset_id,source_kind,source_id,partition_id,measurement_status,
				watermark_value,observed_at,collected_at,trace_id,measurement_error,metadata,updated_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::jsonb,now())
			ON CONFLICT (tenant_id,dataset_id,source_kind,source_id,partition_id) DO UPDATE SET
				measurement_status=EXCLUDED.measurement_status,watermark_value=EXCLUDED.watermark_value,
				observed_at=EXCLUDED.observed_at,collected_at=EXCLUDED.collected_at,trace_id=EXCLUDED.trace_id,
				measurement_error=EXCLUDED.measurement_error,metadata=EXCLUDED.metadata,updated_at=now()
		`, signal.TenantID, signal.DatasetID, signal.SourceKind, signal.SourceID, signal.PartitionID,
			signal.MeasurementState, watermarkValue, observedAt, signal.CollectedAt, signal.TraceID,
			signal.MeasurementError, string(metadataJSON)); err != nil {
			return fmt.Errorf("upsert %s hand-off signal: %w", signal.SourceKind, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit hand-off signals: %w", err)
	}
	return nil
}

func (m *Monitor) LoadHandoffSignals(ctx context.Context, tenantID, datasetID string) ([]HandoffSignal, error) {
	if m == nil || m.controlDB == nil {
		return nil, nil
	}
	rows, err := m.controlDB.QueryContext(ctx, `
		SELECT tenant_id,dataset_id,source_kind,source_id,partition_id,measurement_status,
			watermark_value,observed_at,collected_at,trace_id,measurement_error,metadata
		FROM data_quality_watermarks
		WHERE tenant_id=$1 AND dataset_id=$2
		ORDER BY source_kind,source_id,partition_id
	`, tenantID, datasetID)
	if err != nil {
		return nil, fmt.Errorf("load hand-off signals: %w", err)
	}
	defer rows.Close()
	result := make([]HandoffSignal, 0, 5)
	for rows.Next() {
		var signal HandoffSignal
		var value sql.NullString
		var observed sql.NullTime
		var metadataJSON []byte
		if err := rows.Scan(&signal.TenantID, &signal.DatasetID, &signal.SourceKind, &signal.SourceID,
			&signal.PartitionID, &signal.MeasurementState, &value, &observed, &signal.CollectedAt,
			&signal.TraceID, &signal.MeasurementError, &metadataJSON); err != nil {
			return nil, fmt.Errorf("scan hand-off signal: %w", err)
		}
		if value.Valid {
			signal.WatermarkValue = &value.String
		}
		if observed.Valid {
			signal.ObservedAt = &observed.Time
		}
		if err := json.Unmarshal(metadataJSON, &signal.Metadata); err != nil {
			return nil, fmt.Errorf("decode %s signal metadata: %w", signal.SourceKind, err)
		}
		// The four semantic axes are persisted in metadata so the v1 table can
		// remain backward compatible. Hydrate them on every read, including rows
		// written before the typed fields were added to the API model.
		deriveSignalSemantics(&signal)
		result = append(result, signal)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate hand-off signals: %w", err)
	}
	return result, nil
}

func (m *Monitor) activeTenantIDs(ctx context.Context) ([]string, error) {
	if m == nil || m.controlDB == nil {
		return nil, fmt.Errorf("PostgreSQL data quality control plane not available")
	}
	rows, err := m.controlDB.QueryContext(ctx, `SELECT tenant_id FROM tenants WHERE status='active' ORDER BY tenant_id`)
	if err != nil {
		return nil, fmt.Errorf("list active tenants for signal collection: %w", err)
	}
	defer rows.Close()
	result := make([]string, 0)
	for rows.Next() {
		var tenantID string
		if err := rows.Scan(&tenantID); err != nil {
			return nil, err
		}
		result = append(result, tenantID)
	}
	return result, rows.Err()
}

func RunHandoffSignalCollectionLoop(ctx context.Context, monitor *Monitor, collector *CompositeSignalCollector, interval time.Duration, logger *zap.Logger) {
	if monitor == nil || collector == nil {
		return
	}
	if interval <= 0 {
		interval = time.Minute
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	collect := func() {
		runCtx, cancel := context.WithTimeout(ctx, minDuration(interval, 30*time.Second))
		defer cancel()
		tenantIDs, err := monitor.activeTenantIDs(runCtx)
		if err != nil {
			logger.Warn("Data quality signal collection could not list tenants", zap.Error(err))
			return
		}
		for _, tenantID := range tenantIDs {
			traceID := "dq-signals-" + uuid.NewString()
			signals, err := monitor.CollectAndPersistHandoffSignals(runCtx, tenantID, traceID, collector)
			if err != nil {
				logger.Warn("Data quality signal collection failed", zap.String("tenant_id", tenantID), zap.Error(err))
				continue
			}
			states := make(map[string]string, len(signals))
			for _, signal := range signals {
				states[signal.SourceKind] = signal.MeasurementState
			}
			logger.Info("Data quality hand-off signals persisted", zap.String("tenant_id", tenantID), zap.Any("states", states), zap.String("trace_id", traceID))
		}
	}
	collect()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			collect()
		}
	}
}

func validateHandoffSignal(signal HandoffSignal) error {
	allowedKind := map[string]bool{SignalKindKafkaOffset: true, SignalKindFlinkWatermark: true, SignalKindSinkCommit: true, SignalKindBusinessVersion: true, SignalKindObjectManifest: true}
	allowedState := map[string]bool{SignalStatusMeasured: true, SignalStatusUnknown: true, SignalStatusNotApplicable: true, SignalStatusError: true}
	if signal.TenantID == "" || signal.DatasetID == "" || !allowedKind[signal.SourceKind] || signal.SourceID == "" || !allowedState[signal.MeasurementState] || signal.TraceID == "" || signal.CollectedAt.IsZero() {
		return fmt.Errorf("invalid %s hand-off signal identity", signal.SourceKind)
	}
	if signal.Metadata == nil {
		return fmt.Errorf("%s hand-off signal metadata is required", signal.SourceKind)
	}
	deriveSignalSemantics(&signal)
	allowedAvailability := map[string]bool{SignalAvailabilityArrived: true, SignalAvailabilityNotArrived: true, SignalAvailabilityUnavailable: true, SignalAvailabilityNotApplicable: true}
	allowedFreshness := map[string]bool{SignalFreshnessFresh: true, SignalFreshnessStale: true, SignalFreshnessUnknown: true, SignalFreshnessNotApplicable: true}
	allowedCompleteness := map[string]bool{SignalCompletenessComplete: true, SignalCompletenessPartial: true, SignalCompletenessUnknown: true, SignalCompletenessNotApplicable: true}
	allowedValue := map[string]bool{SignalValueZero: true, SignalValueNonzero: true, SignalValueNone: true}
	if !allowedAvailability[signal.AvailabilityState] || !allowedFreshness[signal.FreshnessState] || !allowedCompleteness[signal.CompletenessState] || !allowedValue[signal.ValueState] {
		return fmt.Errorf("%s hand-off signal has invalid semantic axes", signal.SourceKind)
	}
	if signal.MeasurementState == SignalStatusMeasured {
		if signal.WatermarkValue == nil || *signal.WatermarkValue == "" || signal.ObservedAt == nil || signal.MeasurementError != "" {
			return fmt.Errorf("measured %s signal requires value/observed_at and no error", signal.SourceKind)
		}
		if signal.AvailabilityState != SignalAvailabilityArrived ||
			(signal.FreshnessState != SignalFreshnessFresh && signal.FreshnessState != SignalFreshnessStale) ||
			(signal.CompletenessState != SignalCompletenessComplete && signal.CompletenessState != SignalCompletenessPartial) ||
			(signal.ValueState != SignalValueZero && signal.ValueState != SignalValueNonzero) {
			return fmt.Errorf("measured %s signal has inconsistent semantic axes", signal.SourceKind)
		}
		if numeric, err := strconv.ParseFloat(*signal.WatermarkValue, 64); err == nil {
			if (numeric == 0) != (signal.ValueState == SignalValueZero) {
				return fmt.Errorf("measured %s signal value_status does not match watermark_value", signal.SourceKind)
			}
		}
		return nil
	}
	if signal.WatermarkValue != nil || signal.ObservedAt != nil {
		return fmt.Errorf("non-measured %s signal cannot carry a value or observed_at", signal.SourceKind)
	}
	if signal.MeasurementState == SignalStatusError && signal.MeasurementError == "" {
		return fmt.Errorf("error %s signal requires measurement_error", signal.SourceKind)
	}
	if signal.MeasurementState != SignalStatusError && signal.MeasurementError != "" {
		return fmt.Errorf("%s signal state %s cannot carry measurement_error", signal.SourceKind, signal.MeasurementState)
	}
	expectedAxes := map[string][4]string{
		SignalStatusUnknown:       {SignalAvailabilityNotArrived, SignalFreshnessUnknown, SignalCompletenessUnknown, SignalValueNone},
		SignalStatusError:         {SignalAvailabilityUnavailable, SignalFreshnessUnknown, SignalCompletenessUnknown, SignalValueNone},
		SignalStatusNotApplicable: {SignalAvailabilityNotApplicable, SignalFreshnessNotApplicable, SignalCompletenessNotApplicable, SignalValueNone},
	}
	expected := expectedAxes[signal.MeasurementState]
	if signal.AvailabilityState != expected[0] || signal.FreshnessState != expected[1] || signal.CompletenessState != expected[2] || signal.ValueState != expected[3] {
		return fmt.Errorf("%s signal state %s has inconsistent semantic axes", signal.SourceKind, signal.MeasurementState)
	}
	return nil
}

func (m *Monitor) applyHandoffSignals(ctx context.Context, tenantID string, report *DataQualityReport) {
	signals, err := m.LoadHandoffSignals(ctx, tenantID, "flows_raw")
	if err != nil {
		for _, kind := range []string{SignalKindKafkaOffset, SignalKindFlinkWatermark, SignalKindSinkCommit} {
			appendUnavailableHandoffCheck(report, kind, "PostgreSQL hand-off signals unavailable: "+err.Error())
		}
		return
	}
	byKind := make(map[string]HandoffSignal, len(signals))
	for _, signal := range signals {
		deriveSignalSemantics(&signal)
		byKind[signal.SourceKind] = signal
	}
	for _, kind := range []string{SignalKindKafkaOffset, SignalKindFlinkWatermark, SignalKindSinkCommit} {
		signal, exists := byKind[kind]
		if !exists {
			appendNotArrivedHandoffCheck(report, kind, "No persisted measurement exists; signal collection may be disabled or not yet run")
			continue
		}
		name := handoffCheckName(kind)
		if signal.MeasurementState != SignalStatusMeasured || signal.WatermarkValue == nil || signal.ObservedAt == nil {
			message := fmt.Sprintf("%s is %s", kind, signal.MeasurementState)
			if signal.MeasurementError != "" {
				message += ": " + signal.MeasurementError
			}
			report.Checks = append(report.Checks, qualityCheckFromSignal(signal, name, "unknown", message, 0, 0, false))
			continue
		}
		age := report.Timestamp.Sub(signal.CollectedAt)
		status := "pass"
		message := fmt.Sprintf("%s measured at %s", kind, signal.ObservedAt.UTC().Format(time.RFC3339Nano))
		value := 0.0
		threshold := m.config.MaxSignalAge.Seconds()
		if kind == SignalKindKafkaOffset {
			lag, parseErr := strconv.ParseFloat(*signal.WatermarkValue, 64)
			if parseErr != nil {
				report.Checks = append(report.Checks, qualityCheckFromSignal(signal, name, "unknown", "Persisted Kafka lag is not numeric", 0, threshold, false))
				continue
			}
			value = lag
			threshold = float64(m.config.MaxKafkaLag)
			message = fmt.Sprintf("Kafka consumer lag is %.0f records", lag)
			if lag > float64(m.config.MaxKafkaLag) {
				status = "fail"
			}
		}
		if signal.ValueState == SignalValueZero {
			message += "; zero is an observed value"
		}
		if signal.CompletenessState == SignalCompletenessPartial {
			if status == "pass" {
				status = "warn"
			}
			message += "; measurement is partial"
		}
		if signal.FreshnessState == SignalFreshnessStale || (m.config.MaxSignalAge > 0 && age > m.config.MaxSignalAge) {
			signal.FreshnessState = SignalFreshnessStale
			deriveSignalSemantics(&signal)
			if status == "pass" {
				status = "warn"
			}
			message += fmt.Sprintf("; measurement is stale by %s", age.Round(time.Second))
		}
		byKind[kind] = signal
		report.Checks = append(report.Checks, qualityCheckFromSignal(signal, name, status, message, value, threshold, true))
	}
	for _, signal := range byKind {
		report.SourceWatermarks[signal.SourceKind] = signal
	}
}

func qualityCheckFromSignal(signal HandoffSignal, name, status, message string, value, threshold float64, measured bool) QualityCheck {
	return QualityCheck{
		Name: name, Status: status, Message: message, Value: value, Threshold: threshold,
		Measured: measured, Source: signal.SourceID, Availability: signal.AvailabilityState,
		Freshness: signal.FreshnessState, Completeness: signal.CompletenessState, ValueState: signal.ValueState,
	}
}

func appendUnavailableHandoffCheck(report *DataQualityReport, kind, message string) {
	report.Checks = append(report.Checks, QualityCheck{
		Name: handoffCheckName(kind), Status: "unknown", Message: message, Measured: false,
		Source: handoffSourceName(kind), Availability: SignalAvailabilityUnavailable,
		Freshness: SignalFreshnessUnknown, Completeness: SignalCompletenessUnknown, ValueState: SignalValueNone,
	})
}

func appendNotArrivedHandoffCheck(report *DataQualityReport, kind, message string) {
	report.Checks = append(report.Checks, QualityCheck{
		Name: handoffCheckName(kind), Status: "unknown", Message: message, Measured: false,
		Source: handoffSourceName(kind), Availability: SignalAvailabilityNotArrived,
		Freshness: SignalFreshnessUnknown, Completeness: SignalCompletenessUnknown, ValueState: SignalValueNone,
	})
}

func handoffCheckName(kind string) string {
	switch kind {
	case SignalKindKafkaOffset:
		return "kafka_consumer_lag"
	case SignalKindFlinkWatermark:
		return "flink_event_time_watermark"
	case SignalKindSinkCommit:
		return "sink_commit_watermark"
	default:
		return "handoff_" + kind
	}
}

func handoffSourceName(kind string) string {
	switch kind {
	case SignalKindKafkaOffset:
		return "kafka.end_offset_minus_committed_offset"
	case SignalKindFlinkWatermark:
		return "flink.currentOutputWatermark"
	case SignalKindSinkCommit:
		return "clickhouse.traffic.flows_raw.max_ingest_ts"
	default:
		return kind
	}
}

func minDuration(left, right time.Duration) time.Duration {
	if left < right {
		return left
	}
	return right
}
