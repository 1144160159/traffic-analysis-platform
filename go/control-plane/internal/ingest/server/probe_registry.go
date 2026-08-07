package server

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	"github.com/google/uuid"
	"google.golang.org/protobuf/encoding/protojson"
)

const probeRegisteredEventType = "traffic.probe.registry.v1.ProbeRegistered"

// ProbeRegistry persists the authenticated Agent identity in the PG authority.
// Implementations must never move an existing probe_id between tenants.
type ProbeRegistry interface {
	Register(
		ctx context.Context,
		tenantID string,
		probeID string,
		softwareVersion string,
		buildCommit string,
		hardware *pb.HardwareInfo,
	) error
	Heartbeat(ctx context.Context, tenantID string, probeID string) error
}

type PostgresProbeRegistry struct {
	db *sql.DB
}

func NewPostgresProbeRegistry(db *sql.DB) *PostgresProbeRegistry {
	return &PostgresProbeRegistry{db: db}
}

func (registry *PostgresProbeRegistry) Register(
	ctx context.Context,
	tenantID string,
	probeID string,
	softwareVersion string,
	buildCommit string,
	hardware *pb.HardwareInfo,
) error {
	if registry == nil || registry.db == nil {
		return fmt.Errorf("probe registry PostgreSQL connection is unavailable")
	}
	tenantID = strings.TrimSpace(tenantID)
	probeID = strings.TrimSpace(probeID)
	softwareVersion = strings.TrimSpace(softwareVersion)
	buildCommit = strings.TrimSpace(buildCommit)
	if tenantID == "" || probeID == "" {
		return fmt.Errorf("tenant_id and probe_id are required")
	}
	hardwareDocument := map[string]interface{}{}
	if hardware != nil {
		raw, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(hardware)
		if err != nil {
			return fmt.Errorf("marshal probe hardware: %w", err)
		}
		if err := json.Unmarshal(raw, &hardwareDocument); err != nil {
			return fmt.Errorf("normalize probe hardware: %w", err)
		}
	}
	if buildCommit != "" {
		hardwareDocument["build_commit"] = buildCommit
	}
	hardwareJSON, err := json.Marshal(hardwareDocument)
	if err != nil {
		return fmt.Errorf("marshal probe registry document: %w", err)
	}
	requestPayload, err := json.Marshal(map[string]interface{}{
		"tenant_id":        tenantID,
		"probe_id":         probeID,
		"software_version": softwareVersion,
		"build_commit":     buildCommit,
		"hardware":         hardwareDocument,
	})
	if err != nil {
		return fmt.Errorf("marshal probe registration command: %w", err)
	}
	requestDigest := sha256.Sum256(requestPayload)
	requestSHA256 := fmt.Sprintf("%x", requestDigest)
	idempotencyKey := "probe-register:" + probeID + ":" + requestSHA256
	eventID := uuid.NewSHA1(
		uuid.NameSpaceOID,
		[]byte("traffic.probe.registry.v1:"+tenantID+":"+idempotencyKey),
	)

	tx, err := registry.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin probe registration transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1,0))`,
		"probe-registry:"+probeID,
	); err != nil {
		return fmt.Errorf("lock probe registration: %w", err)
	}

	var replaySHA256 string
	err = tx.QueryRowContext(ctx, `
		SELECT request_sha256
		FROM probe_registry_requests
		WHERE tenant_id=$1 AND idempotency_key=$2`,
		tenantID, idempotencyKey,
	).Scan(&replaySHA256)
	if err == nil {
		if replaySHA256 != requestSHA256 {
			return fmt.Errorf("probe registration idempotency conflict")
		}
		if err = tx.Commit(); err != nil {
			return fmt.Errorf("commit probe registration replay: %w", err)
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("load probe registration replay: %w", err)
	}

	var boundTenant string
	var currentRevision int64
	err = tx.QueryRowContext(ctx, `
		SELECT tenant_id,revision
		FROM probes
		WHERE probe_id=$1
		FOR UPDATE`, probeID,
	).Scan(&boundTenant, &currentRevision)
	var revision int64
	switch {
	case errors.Is(err, sql.ErrNoRows):
		revision = 1
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO probes (
			  probe_id,tenant_id,name,status,hardware_info,software_version,
			  last_heartbeat,revision,updated_at
			) VALUES ($1,$2,$3,'active',$4::jsonb,$5,now(),$6,now())`,
			probeID, tenantID, probeID, string(hardwareJSON), softwareVersion, revision,
		); err != nil {
			return fmt.Errorf("insert probe registration: %w", err)
		}
	case err != nil:
		return fmt.Errorf("load probe registration authority: %w", err)
	case boundTenant != tenantID:
		return fmt.Errorf("probe_id is already bound to another tenant")
	default:
		err = tx.QueryRowContext(ctx, `
			UPDATE probes
			SET name=$3,status='active',hardware_info=$4::jsonb,software_version=$5,
			    last_heartbeat=now(),revision=revision+1,updated_at=now()
			WHERE tenant_id=$1 AND probe_id=$2 AND revision=$6
			RETURNING revision`,
			tenantID, probeID, probeID, string(hardwareJSON), softwareVersion, currentRevision,
		).Scan(&revision)
		if err != nil {
			return fmt.Errorf("update probe registration revision: %w", err)
		}
	}

	detailJSON, err := json.Marshal(map[string]interface{}{
		"software_version":  softwareVersion,
		"build_commit":      buildCommit,
		"hardware_sha256":   fmt.Sprintf("%x", sha256.Sum256(hardwareJSON)),
		"resource_revision": revision,
	})
	if err != nil {
		return fmt.Errorf("marshal probe registration evidence: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO probe_registry_history (
		  event_id,tenant_id,probe_id,revision,event_type,request_sha256,detail
		) VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb)`,
		eventID, tenantID, probeID, revision, probeRegisteredEventType, requestSHA256, string(detailJSON),
	); err != nil {
		return fmt.Errorf("insert probe registration history: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO audit_logs (
		  event_id,tenant_id,user_id,action,object_type,object_id,detail,ip_addr,user_agent
		) VALUES ($1,$2,NULL,'register_probe','probe',$3,$4::jsonb,NULL,NULL)`,
		"audit-"+eventID.String(), tenantID, probeID, string(detailJSON),
	); err != nil {
		return fmt.Errorf("insert probe registration audit: %w", err)
	}
	eventPayload, err := json.Marshal(map[string]interface{}{
		"event_id":          eventID.String(),
		"event_type":        probeRegisteredEventType,
		"tenant_id":         tenantID,
		"probe_id":          probeID,
		"aggregate_version": revision,
		"schema_version":    1,
		"software_version":  softwareVersion,
		"build_commit":      buildCommit,
		"hardware":          hardwareDocument,
	})
	if err != nil {
		return fmt.Errorf("marshal probe registration event: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO probe_registry_outbox (
		  event_id,tenant_id,probe_id,event_type,aggregate_version,schema_version,
		  partition_key,payload
		) VALUES ($1,$2,$3,$4,$5,1,$3,$6::jsonb)`,
		eventID, tenantID, probeID, probeRegisteredEventType, revision, string(eventPayload),
	); err != nil {
		return fmt.Errorf("insert probe registration outbox: %w", err)
	}
	resultJSON, err := json.Marshal(map[string]interface{}{
		"success": true, "event_id": eventID.String(), "resource_revision": revision,
	})
	if err != nil {
		return fmt.Errorf("marshal probe registration result: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO probe_registry_requests (
		  tenant_id,idempotency_key,request_sha256,probe_id,event_id,resource_revision,result
		) VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb)`,
		tenantID, idempotencyKey, requestSHA256, probeID, eventID, revision, string(resultJSON),
	); err != nil {
		return fmt.Errorf("insert probe registration replay: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit probe registration transaction: %w", err)
	}
	return nil
}

func (registry *PostgresProbeRegistry) Heartbeat(
	ctx context.Context,
	tenantID string,
	probeID string,
) error {
	if registry == nil || registry.db == nil {
		return fmt.Errorf("probe registry PostgreSQL connection is unavailable")
	}
	// Heartbeat is an authenticated liveness projection, not a business
	// command. Keep it tenant-bound and bounded instead of emitting an audit and
	// outbox row every 30 seconds per probe.
	result, err := registry.db.ExecContext(ctx, `
		UPDATE probes
		SET status='active',last_heartbeat=now(),updated_at=now()
		WHERE tenant_id=$1 AND probe_id=$2`,
		tenantID,
		probeID,
	)
	if err != nil {
		return fmt.Errorf("persist probe heartbeat: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read probe heartbeat result: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("registered probe identity was not found")
	}
	return nil
}
