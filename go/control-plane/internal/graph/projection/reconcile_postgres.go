package projection

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/lib/pq"
	"google.golang.org/protobuf/proto"

	trafficv1 "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

type PostgresReconcileRepository struct{ db *sql.DB }

func NewPostgresReconcileRepository(db *sql.DB) (*PostgresReconcileRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("graph reconcile PostgreSQL database is required")
	}
	return &PostgresReconcileRepository{db: db}, nil
}

func (repository *PostgresReconcileRepository) VerifySchema(ctx context.Context) error {
	var count int
	if err := repository.db.QueryRowContext(ctx, `
		SELECT count(*) FROM information_schema.tables
		WHERE table_schema='public' AND table_name IN (
		  'graph_projection_inbox_v1','graph_projection_current_v1',
		  'graph_projection_watermarks_v1','graph_projection_dead_letters_v1',
		  'graph_projection_reconcile_runs_v1','graph_projection_reconcile_items_v1'
		)`).Scan(&count); err != nil {
		return fmt.Errorf("verify graph reconcile PostgreSQL schema: %w", err)
	}
	if count != 6 {
		return fmt.Errorf("graph reconcile PostgreSQL schema is incomplete: got %d of 6 tables", count)
	}
	return nil
}

// LoadReconcileSnapshot reads the closed-window authority from PostgreSQL. A
// fact intersects the window when its validity began before the close and did
// not end before the window opened.
func (repository *PostgresReconcileRepository) LoadReconcileSnapshot(
	ctx context.Context,
	scope ReconcileScope,
) ([]ProjectionFact, string, error) {
	rows, err := repository.db.QueryContext(ctx, `
		SELECT projection_kind,projection_id,aggregate_version,projection_sha256,revoked
		FROM graph_projection_current_v1
		WHERE tenant_id=$1
		  AND valid_from_ms<=$2
		  AND (valid_to_ms=0 OR valid_to_ms>=$3)
		ORDER BY projection_kind,projection_id
		LIMIT $4`, scope.TenantID, scope.WindowThrough.UnixMilli(), scope.WindowFrom.UnixMilli(), scope.MaxFacts+1)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	facts := make([]ProjectionFact, 0)
	for rows.Next() {
		var fact ProjectionFact
		if err := rows.Scan(&fact.Kind, &fact.ProjectionID, &fact.AggregateVersion, &fact.ProjectionSHA256, &fact.Revoked); err != nil {
			return nil, "", err
		}
		facts = append(facts, fact)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	profile := fmt.Sprintf("postgresql:index=graph_projection_current_v1_pkey;rows=%d;limit=%d;closed_window=true", len(facts), scope.MaxFacts+1)
	return facts, profile, nil
}

func (repository *PostgresReconcileRepository) LoadProjectionEvents(
	ctx context.Context,
	scope ReconcileScope,
	facts []ProjectionFact,
) ([]*trafficv1.GraphProjectionEvent, error) {
	if len(facts) == 0 {
		return []*trafficv1.GraphProjectionEvent{}, nil
	}
	kinds, ids := make([]string, 0, len(facts)), make([]string, 0, len(facts))
	wanted := make(map[string]ProjectionFact, len(facts))
	for _, fact := range facts {
		kinds, ids = append(kinds, fact.Kind), append(ids, fact.ProjectionID)
		wanted[fact.key()] = fact
	}
	rows, err := repository.db.QueryContext(ctx, `
		WITH requested AS (
		  SELECT * FROM unnest($2::text[],$3::text[]) AS item(projection_kind,projection_id)
		)
		SELECT current.projection_kind,current.projection_id,inbox.raw_payload
		FROM requested
		JOIN graph_projection_current_v1 current
		  ON current.tenant_id=$1
		 AND current.projection_kind=requested.projection_kind
		 AND current.projection_id=requested.projection_id
		JOIN graph_projection_inbox_v1 inbox ON inbox.event_id=current.event_id
		WHERE current.valid_from_ms<=$4
		  AND (current.valid_to_ms=0 OR current.valid_to_ms>=$5)
		ORDER BY current.projection_kind,current.projection_id`,
		scope.TenantID, pq.Array(kinds), pq.Array(ids), scope.WindowThrough.UnixMilli(), scope.WindowFrom.UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	eventsByKey := make(map[string]*trafficv1.GraphProjectionEvent, len(facts))
	for rows.Next() {
		var kind, projectionID string
		var payload []byte
		if err := rows.Scan(&kind, &projectionID, &payload); err != nil {
			return nil, err
		}
		event := &trafficv1.GraphProjectionEvent{}
		if err := proto.Unmarshal(payload, event); err != nil {
			return nil, fmt.Errorf("decode authoritative graph projection %s:%s: %w", kind, projectionID, err)
		}
		if err := ValidateEvent(event); err != nil {
			return nil, fmt.Errorf("validate authoritative graph projection %s:%s: %w", kind, projectionID, err)
		}
		metadata, err := metadataOf(event)
		if err != nil {
			return nil, err
		}
		fact, exists := wanted[kind+":"+projectionID]
		if !exists || metadata.aggregateVersion != fact.AggregateVersion ||
			metadata.projectionSHA256 != fact.ProjectionSHA256 || metadata.revoked != fact.Revoked {
			return nil, fmt.Errorf("authoritative graph projection event differs from reconcile fact %s:%s", kind, projectionID)
		}
		eventsByKey[kind+":"+projectionID] = event
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	ordered := make([]*trafficv1.GraphProjectionEvent, 0, len(facts))
	for _, fact := range facts {
		event, exists := eventsByKey[fact.key()]
		if !exists {
			return nil, fmt.Errorf("authoritative graph projection event is missing for %s", fact.key())
		}
		ordered = append(ordered, event)
	}
	return ordered, nil
}

func (repository *PostgresReconcileRepository) RecordReconcileManifest(
	ctx context.Context,
	manifest ReconcileManifest,
) error {
	profileJSON, err := json.Marshal(manifest.Profile)
	if err != nil {
		return err
	}
	tx, err := repository.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if manifest.Phase == "before" {
		state := "compared"
		var completedAt interface{}
		if manifest.Converged {
			state, completedAt = "exact", manifest.CreatedAt
		}
		result, err := tx.ExecContext(ctx, `
			INSERT INTO graph_projection_reconcile_runs_v1 (
			  run_id,tenant_id,window_from,window_through,max_facts,max_duration_ms,state,
			  before_authority_sha256,before_target_sha256,before_manifest_sha256,
			  before_missing_count,before_stale_count,before_extra_count,before_profile_json,
			  started_at,completed_at
			) VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14::jsonb,$15,$16)
			ON CONFLICT (run_id) DO NOTHING`,
			manifest.RunID, manifest.Scope.TenantID, manifest.Scope.WindowFrom, manifest.Scope.WindowThrough,
			manifest.Scope.MaxFacts, manifest.Scope.MaxDuration.Milliseconds(), state,
			manifest.Authority.SHA256, manifest.Target.SHA256, manifest.ManifestSHA256,
			manifest.MissingCount, manifest.StaleCount, manifest.ExtraCount, profileJSON,
			manifest.CreatedAt, completedAt)
		if err != nil {
			return err
		}
		if affected, err := result.RowsAffected(); err != nil {
			return err
		} else if affected == 0 {
			var stored string
			if err := tx.QueryRowContext(ctx, `SELECT before_manifest_sha256 FROM graph_projection_reconcile_runs_v1 WHERE run_id=$1::uuid`, manifest.RunID).Scan(&stored); err != nil {
				return err
			}
			if stored != manifest.ManifestSHA256 {
				return fmt.Errorf("graph reconcile run ID already has a different before manifest")
			}
		}
	} else {
		state := "not_converged"
		if manifest.MissingCount == 0 && manifest.StaleCount == 0 {
			state = "exact"
			if manifest.ExtraCount > 0 {
				state = "extra_preserved"
			}
		}
		result, err := tx.ExecContext(ctx, `
			UPDATE graph_projection_reconcile_runs_v1 SET
			  state=$2,after_authority_sha256=$3,after_target_sha256=$4,after_manifest_sha256=$5,
			  after_missing_count=$6,after_stale_count=$7,after_extra_count=$8,
			  after_profile_json=$9::jsonb,completed_at=$10,updated_at=now()
			WHERE run_id=$1::uuid AND state='repairing'
			  AND (after_manifest_sha256 IS NULL OR after_manifest_sha256=$5)`,
			manifest.RunID, state, manifest.Authority.SHA256, manifest.Target.SHA256, manifest.ManifestSHA256,
			manifest.MissingCount, manifest.StaleCount, manifest.ExtraCount, profileJSON, manifest.CreatedAt)
		if err != nil {
			return err
		}
		if affected, err := result.RowsAffected(); err != nil || affected != 1 {
			return fmt.Errorf("record graph reconcile after manifest affected %d rows: %w", affected, err)
		}
	}
	for _, difference := range manifest.Differences {
		var authorityVersion, targetVersion interface{}
		var authoritySHA, targetSHA interface{}
		var authorityRevoked, targetRevoked interface{}
		if difference.Authority != nil {
			authorityVersion, authoritySHA, authorityRevoked = difference.Authority.AggregateVersion, difference.Authority.ProjectionSHA256, difference.Authority.Revoked
		}
		if difference.Target != nil {
			targetVersion, targetSHA, targetRevoked = difference.Target.AggregateVersion, difference.Target.ProjectionSHA256, difference.Target.Revoked
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO graph_projection_reconcile_items_v1 (
			  run_id,phase,difference_class,projection_kind,projection_id,
			  authority_version,authority_sha256,authority_revoked,
			  target_version,target_sha256,target_revoked,repair_eligible
			) VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
			ON CONFLICT (run_id,phase,projection_kind,projection_id) DO NOTHING`,
			manifest.RunID, manifest.Phase, difference.Class, difference.Kind, difference.ProjectionID,
			authorityVersion, authoritySHA, authorityRevoked, targetVersion, targetSHA, targetRevoked, difference.RepairEligible); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (repository *PostgresReconcileRepository) RecordRepairAuthorization(
	ctx context.Context,
	authorization RepairAuthorization,
) error {
	result, err := repository.db.ExecContext(ctx, `
		UPDATE graph_projection_reconcile_runs_v1 SET
		  state='repairing',requested_by=$2,approved_by=$3,approved_at=$4,repair_max_items=$5,updated_at=now()
		WHERE run_id=$1::uuid
		  AND ((state='compared' AND requested_by IS NULL)
		    OR (state='repairing' AND requested_by=$2 AND approved_by=$3 AND approved_at=$4 AND repair_max_items=$5))`,
		authorization.RunID, authorization.RequestedBy, authorization.ApprovedBy,
		authorization.ApprovedAt.UTC(), authorization.MaxItems)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return fmt.Errorf("record graph repair authorization affected %d rows: %w", affected, err)
	}
	return nil
}
