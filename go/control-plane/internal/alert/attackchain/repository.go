package attackchain

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/lib/pq"
)

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) (*Repository, error) {
	if db == nil {
		return nil, fmt.Errorf("attack-chain PostgreSQL database is required")
	}
	return &Repository{db: db}, nil
}

func (repository *Repository) VerifySchema(ctx context.Context) error {
	var count int
	if err := repository.db.QueryRowContext(ctx, `
		SELECT count(*) FROM information_schema.tables
		WHERE table_schema='public' AND table_name IN (
		  'gnn_graph_snapshots_v1','attack_chain_snapshots_v1',
		  'attack_chain_snapshot_current_v1','attack_chain_evidence_manifest_v1'
		)`).Scan(&count); err != nil {
		return err
	}
	if count != 4 {
		return fmt.Errorf("attack-chain snapshot schema is incomplete: got %d of 4 tables", count)
	}
	return nil
}

func (repository *Repository) Save(ctx context.Context, snapshot Snapshot) error {
	if err := ValidateSnapshot(snapshot); err != nil {
		return err
	}
	payloadJSON, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	graphNodesJSON, err := json.Marshal(snapshot.GraphSnapshot.Nodes)
	if err != nil {
		return err
	}
	graphEdgesJSON, err := json.Marshal(snapshot.GraphSnapshot.EdgeIDs)
	if err != nil {
		return err
	}
	labelRefsJSON, err := json.Marshal(snapshot.GraphSnapshot.LabelRefs)
	if err != nil {
		return err
	}
	evidenceRefsJSON, err := json.Marshal(snapshot.GraphSnapshot.EvidenceRefs)
	if err != nil {
		return err
	}
	watermarksJSON, err := json.Marshal(snapshot.GraphSnapshot.SourceWatermarks)
	if err != nil {
		return err
	}
	tx, err := repository.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		SELECT pg_advisory_xact_lock(hashtextextended(length($1)::text||':'||$1||length($2)::text||':'||$2,0))`,
		snapshot.TenantID, snapshot.ChainID); err != nil {
		return err
	}
	var currentVersion uint64
	var currentSnapshotID, currentSHA string
	err = tx.QueryRowContext(ctx, `
		SELECT snapshot_version,snapshot_id,snapshot_sha256
		FROM attack_chain_snapshot_current_v1
		WHERE tenant_id=$1 AND chain_id=$2 FOR UPDATE`, snapshot.TenantID, snapshot.ChainID,
	).Scan(&currentVersion, &currentSnapshotID, &currentSHA)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if snapshot.Version != 1 {
			return fmt.Errorf("first attack-chain snapshot version must be 1")
		}
	case err != nil:
		return err
	case currentVersion == snapshot.Version && currentSnapshotID == snapshot.SnapshotID && currentSHA == snapshot.SnapshotSHA256:
		return tx.Commit()
	case snapshot.Version != currentVersion+1:
		return fmt.Errorf("attack-chain snapshot version must advance exactly once: current=%d requested=%d", currentVersion, snapshot.Version)
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO gnn_graph_snapshots_v1 (
		  graph_snapshot_id,tenant_id,chain_id,attack_chain_version,schema_version,as_of,
		  node_count,edge_count,node_sha256,edge_sha256,graph_snapshot_sha256,
		  nodes_json,edge_ids_json,label_refs_json,evidence_refs_json,source_watermarks_json
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::jsonb,$13::jsonb,$14::jsonb,$15::jsonb,$16::jsonb)
		ON CONFLICT (graph_snapshot_id) DO NOTHING`,
		snapshot.GraphSnapshot.SnapshotID, snapshot.TenantID, snapshot.ChainID, snapshot.Version,
		snapshot.GraphSnapshot.SchemaVersion, snapshot.AsOf, snapshot.GraphSnapshot.NodeCount,
		snapshot.GraphSnapshot.EdgeCount, snapshot.GraphSnapshot.NodeSHA256, snapshot.GraphSnapshot.EdgeSHA256,
		snapshot.GraphSnapshot.SnapshotSHA256, graphNodesJSON, graphEdgesJSON, labelRefsJSON, evidenceRefsJSON, watermarksJSON)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected == 0 {
		var storedTenant, storedChain, storedSchema, storedSHA string
		var storedVersion uint64
		if err := tx.QueryRowContext(ctx, `
			SELECT tenant_id,chain_id,attack_chain_version,schema_version,graph_snapshot_sha256
			FROM gnn_graph_snapshots_v1 WHERE graph_snapshot_id=$1`, snapshot.GraphSnapshot.SnapshotID,
		).Scan(&storedTenant, &storedChain, &storedVersion, &storedSchema, &storedSHA); err != nil {
			return err
		}
		if storedTenant != snapshot.TenantID || storedChain != snapshot.ChainID || storedVersion != snapshot.Version ||
			storedSchema != snapshot.GraphSnapshot.SchemaVersion || storedSHA != snapshot.GraphSnapshot.SnapshotSHA256 {
			return fmt.Errorf("graph snapshot ID already has different bytes")
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO attack_chain_snapshots_v1 (
		  snapshot_id,tenant_id,chain_id,snapshot_version,as_of,source_vertex_id,target_vertex_id,
		  candidate_path_sha256,alternative_path_count,graph_snapshot_id,graph_snapshot_sha256,
		  snapshot_sha256,partial,partial_reasons,truncated,truncation_reason,continuation_boundary,state,payload_json
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,'active',$18::jsonb)`,
		snapshot.SnapshotID, snapshot.TenantID, snapshot.ChainID, snapshot.Version, snapshot.AsOf,
		snapshot.Source.VertexID, snapshot.Target.VertexID, snapshot.CandidatePath.PathSHA256,
		len(snapshot.AlternativePaths), snapshot.GraphSnapshot.SnapshotID, snapshot.GraphSnapshot.SnapshotSHA256,
		snapshot.SnapshotSHA256, snapshot.Partial, pq.Array(snapshot.PartialReasons), snapshot.Truncated,
		snapshot.TruncationReason, snapshot.ContinuationBoundary, payloadJSON); err != nil {
		return err
	}
	for _, evidence := range snapshotEvidence(snapshot) {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO attack_chain_evidence_manifest_v1 (
			  snapshot_id,tenant_id,evidence_id,evidence_kind,immutable_uri,evidence_sha256,
			  source_event_id,occurred_at_ms,available
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			snapshot.SnapshotID, snapshot.TenantID, evidence.EvidenceID, evidence.Kind,
			evidence.ImmutableURI, evidence.SHA256, evidence.SourceEventID, evidence.OccurredAt, evidence.Available); err != nil {
			return err
		}
	}
	result, err = tx.ExecContext(ctx, `
		INSERT INTO attack_chain_snapshot_current_v1 (
		  tenant_id,chain_id,snapshot_id,snapshot_version,snapshot_sha256
		) VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (tenant_id,chain_id) DO UPDATE SET
		  snapshot_id=EXCLUDED.snapshot_id,snapshot_version=EXCLUDED.snapshot_version,
		  snapshot_sha256=EXCLUDED.snapshot_sha256,updated_at=now()
		WHERE attack_chain_snapshot_current_v1.snapshot_version<EXCLUDED.snapshot_version`,
		snapshot.TenantID, snapshot.ChainID, snapshot.SnapshotID, snapshot.Version, snapshot.SnapshotSHA256)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return fmt.Errorf("advance attack-chain current snapshot affected %d rows: %w", affected, err)
	}
	return tx.Commit()
}

func (repository *Repository) LoadCurrent(ctx context.Context, tenantID, chainID string) (Snapshot, error) {
	var payload []byte
	if err := repository.db.QueryRowContext(ctx, `
		SELECT snapshot.payload_json
		FROM attack_chain_snapshot_current_v1 current
		JOIN attack_chain_snapshots_v1 snapshot ON snapshot.snapshot_id=current.snapshot_id
		WHERE current.tenant_id=$1 AND current.chain_id=$2 AND snapshot.state='active'`, tenantID, chainID).Scan(&payload); err != nil {
		return Snapshot{}, err
	}
	return decodeStoredSnapshot(payload)
}

func (repository *Repository) ListCurrent(ctx context.Context, tenantID string, limit, offset int) ([]Snapshot, int, error) {
	if limit <= 0 || limit > 100 || offset < 0 {
		return nil, 0, fmt.Errorf("attack-chain list budget is invalid")
	}
	var total int
	if err := repository.db.QueryRowContext(ctx, `SELECT count(*) FROM attack_chain_snapshot_current_v1 WHERE tenant_id=$1`, tenantID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := repository.db.QueryContext(ctx, `
		SELECT snapshot.payload_json
		FROM attack_chain_snapshot_current_v1 current
		JOIN attack_chain_snapshots_v1 snapshot ON snapshot.snapshot_id=current.snapshot_id
		WHERE current.tenant_id=$1 AND snapshot.state='active'
		ORDER BY snapshot.as_of DESC,snapshot.snapshot_id
		LIMIT $2 OFFSET $3`, tenantID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]Snapshot, 0)
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, 0, err
		}
		item, err := decodeStoredSnapshot(payload)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func decodeStoredSnapshot(payload []byte) (Snapshot, error) {
	var snapshot Snapshot
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return Snapshot{}, err
	}
	if err := ValidateSnapshot(snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("stored attack-chain snapshot failed validation: %w", err)
	}
	return snapshot, nil
}

func snapshotEvidence(snapshot Snapshot) []EvidenceAnchor {
	byID := make(map[string]EvidenceAnchor)
	paths := append([]Path{snapshot.CandidatePath}, snapshot.AlternativePaths...)
	for _, path := range paths {
		for _, edge := range path.Edges {
			for _, evidence := range edge.Evidence {
				byID[evidence.EvidenceID] = evidence
			}
		}
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	values := make([]EvidenceAnchor, 0, len(ids))
	for _, id := range ids {
		values = append(values, byID[id])
	}
	return values
}
