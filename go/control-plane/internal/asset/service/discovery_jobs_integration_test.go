package service_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/service"
)

const discoveryIntegrationTenant = "asset-discovery-jobs-integration"

type deterministicDiscoveryScanner struct {
	observations []config.DiscoveryObservation
	err          error
}

func (s *deterministicDiscoveryScanner) Scan(
	_ context.Context,
	_ *config.ActiveDiscoveryRequest,
	_ *config.DiscoveryCredential,
) ([]config.DiscoveryObservation, error) {
	return s.observations, s.err
}

// This test is guarded by the same explicit DSN and sentinel used by the
// authoritative asset tests. It must never run against shared PostgreSQL.
func TestDiscoveryJobPostgresLifecycleAndCandidateIsolation(t *testing.T) {
	dsn := os.Getenv("ASSET_ATOMIC_EPHEMERAL_PG_DSN")
	if dsn == "" {
		t.Skip("ASSET_ATOMIC_EPHEMERAL_PG_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var sentinel string
	if err := db.QueryRow(`SELECT marker FROM codex_ephemeral_asset_atomic_test_sentinel LIMIT 1`).Scan(&sentinel); err != nil || sentinel != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel database: marker=%q err=%v", sentinel, err)
	}
	if err := cleanupDiscoveryIntegration(db); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := cleanupDiscoveryIntegration(db); err != nil {
			t.Errorf("cleanup discovery integration: %v", err)
		}
	}()
	if _, err := db.Exec(`
		INSERT INTO tenants(tenant_id,name)
		VALUES ($1,'Asset Discovery Jobs Integration')`, discoveryIntegrationTenant); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO asset_discovery_credentials(
			credential_id,tenant_id,name,protocol,endpoint,secret_ref,created_by
		) VALUES (
			'discovery-integration-credential',$1,'integration-snmp','snmp_lldp',
			'198.51.100.0/30','secret://integration','integration-setup'
		)`, discoveryIntegrationTenant); err != nil {
		t.Fatal(err)
	}
	repo, err := repository.NewAssetRepository(db, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	svc := service.New(&config.Config{
		Discovery: config.DiscoveryConfig{
			JobsV2Enabled: true,
			WorkerLease:   30 * time.Second,
		},
	}, repo, zap.NewNop()).WithDiscoveryScanner(&deterministicDiscoveryScanner{
		observations: []config.DiscoveryObservation{{
			IPAddress:  "198.51.100.1",
			MACAddress: "02:10:20:30:40:50",
			Hostname:   "candidate-only",
			Vendor:     "Integration",
			Neighbors: []config.DiscoveryNeighbor{{
				MACAddress: "02:10:20:30:40:51",
				IPAddress:  "198.51.100.2",
				Protocol:   "lldp",
			}},
		}},
	})
	command := config.DiscoveryJobCommand{
		IdempotencyKey: "discovery-integration-create",
		Actor:          "integration-operator",
		TraceID:        "trace-discovery-integration",
		RequestID:      "request-discovery-integration",
		ClientIP:       "127.0.0.1",
		UserAgent:      "integration-test",
	}
	request := &config.ActiveDiscoveryRequest{
		TenantID:     discoveryIntegrationTenant,
		ActionID:     "asset-active-discovery-run",
		Mode:         "snmp_lldp",
		TargetCIDR:   "198.51.100.0/30",
		CredentialID: "discovery-integration-credential",
		Reason:       "authorized integration discovery",
		RateLimit:    20,
		ApprovedBy:   "integration-approver",
	}
	accepted, err := svc.SubmitActiveDiscovery(context.Background(), request, command)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Status != config.DiscoveryStatusQueued || accepted.Revision != 1 {
		t.Fatalf("accepted=%+v", accepted)
	}
	replay, err := svc.SubmitActiveDiscovery(context.Background(), request, command)
	if err != nil {
		t.Fatal(err)
	}
	if replay.RunID != accepted.RunID || !replay.IdempotentReplay {
		t.Fatalf("idempotent replay diverged: first=%+v replay=%+v", accepted, replay)
	}
	conflicting := *request
	conflicting.Reason = "different immutable reason"
	if _, err := svc.SubmitActiveDiscovery(context.Background(), &conflicting, command); !errors.Is(err, repository.ErrDiscoveryIdempotencyConflict) {
		t.Fatalf("idempotency conflict err=%v", err)
	}
	overlapCommand := command
	overlapCommand.IdempotencyKey = "discovery-integration-overlap"
	overlap := *request
	overlap.TargetCIDR = "198.51.100.0/31"
	if _, err := svc.SubmitActiveDiscovery(context.Background(), &overlap, overlapCommand); !errors.Is(err, repository.ErrDiscoveryOverlapConflict) {
		t.Fatalf("overlap conflict err=%v", err)
	}

	found, err := svc.ProcessNextDiscoveryJob(context.Background(), "integration-worker")
	if err != nil || !found {
		t.Fatalf("worker found=%v err=%v", found, err)
	}
	completed, err := svc.GetDiscoveryJob(context.Background(), discoveryIntegrationTenant, accepted.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != config.DiscoveryStatusSucceeded || completed.Revision != 3 || completed.CandidateCount != 1 {
		t.Fatalf("completed=%+v", completed)
	}
	candidates, err := svc.ListDiscoveryCandidates(context.Background(), discoveryIntegrationTenant, accepted.RunID, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].Status != "pending" ||
		candidates[0].Observation.MACAddress != "02:10:20:30:40:50" {
		t.Fatalf("candidates=%+v", candidates)
	}
	history, err := svc.ListDiscoveryJobHistory(context.Background(), discoveryIntegrationTenant, accepted.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 3 || history[0].ToStatus != "queued" ||
		history[1].ToStatus != "running" || history[2].ToStatus != "succeeded" {
		t.Fatalf("history=%+v", history)
	}
	var assets, audits, outbox, candidateRows int
	if err := db.QueryRow(`
		SELECT
		  (SELECT count(*) FROM assets WHERE tenant_id=$1),
		  (SELECT count(*) FROM audit_logs WHERE tenant_id=$1 AND action='ASSET_DISCOVERY_JOB_ACCEPTED'),
		  (SELECT count(*) FROM asset_discovery_outbox WHERE tenant_id=$1),
		  (SELECT count(*) FROM asset_discovery_candidates WHERE tenant_id=$1)`,
		discoveryIntegrationTenant,
	).Scan(&assets, &audits, &outbox, &candidateRows); err != nil {
		t.Fatal(err)
	}
	if assets != 0 || audits != 1 || outbox != 3 || candidateRows != 1 {
		t.Fatalf("reconcile assets=%d audits=%d outbox=%d candidates=%d", assets, audits, outbox, candidateRows)
	}
	mergeCommand := config.DiscoveryCandidateMergeCommand{
		ExpectedCandidateRevision: 1,
		ExpectedAssetRevision:     0,
		MergeMode:                 "manual",
		Reason:                    "reviewed candidate evidence",
		IdempotencyKey:            "discovery-integration-candidate-merge",
		Actor:                     "integration-reviewer",
		TraceID:                   "trace-discovery-candidate-merge",
		RequestID:                 "request-discovery-candidate-merge",
		ClientIP:                  "127.0.0.1",
		UserAgent:                 "integration-test",
	}
	merged, err := svc.MergeDiscoveryCandidate(
		context.Background(), discoveryIntegrationTenant, accepted.RunID,
		candidates[0].CandidateID, mergeCommand,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !merged.AssetCreated || merged.AssetRevision != 1 ||
		merged.Candidate.Status != "merged" || merged.Candidate.Revision != 2 ||
		merged.Candidate.SourceAssetID != merged.AssetID ||
		merged.EventID == "" || merged.OutboxID < 1 {
		t.Fatalf("merged=%+v", merged)
	}
	replayedMerge, err := svc.MergeDiscoveryCandidate(
		context.Background(), discoveryIntegrationTenant, accepted.RunID,
		candidates[0].CandidateID, mergeCommand,
	)
	if err != nil || !replayedMerge.IdempotentReplay ||
		replayedMerge.AssetID != merged.AssetID ||
		replayedMerge.AssetRevision != merged.AssetRevision ||
		replayedMerge.OutboxID != merged.OutboxID {
		t.Fatalf("merge replay=%+v err=%v", replayedMerge, err)
	}
	changedMerge := mergeCommand
	changedMerge.Reason = "changed immutable decision"
	if _, err := svc.MergeDiscoveryCandidate(
		context.Background(), discoveryIntegrationTenant, accepted.RunID,
		candidates[0].CandidateID, changedMerge,
	); !errors.Is(err, repository.ErrDiscoveryIdempotencyConflict) {
		t.Fatalf("merge idempotency conflict err=%v", err)
	}
	staleMerge := mergeCommand
	staleMerge.IdempotencyKey = "discovery-integration-stale-candidate"
	if _, err := svc.MergeDiscoveryCandidate(
		context.Background(), discoveryIntegrationTenant, accepted.RunID,
		candidates[0].CandidateID, staleMerge,
	); !errors.Is(err, repository.ErrDiscoveryRevisionConflict) {
		t.Fatalf("stale candidate merge err=%v", err)
	}
	if _, err := svc.MergeDiscoveryCandidate(
		context.Background(), "another-tenant", accepted.RunID,
		candidates[0].CandidateID, mergeCommand,
	); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cross-tenant merge err=%v", err)
	}
	var mergedAssets, assetEvents, mergeAudits, assetOutbox, mergeControls, discoveredAssets int
	if err := db.QueryRow(`
		SELECT
		  (SELECT count(*) FROM assets WHERE tenant_id=$1),
		  (SELECT count(*) FROM asset_events WHERE tenant_id=$1),
		  (SELECT count(*) FROM audit_logs WHERE tenant_id=$1 AND action='ASSET_DISCOVERY_CANDIDATE_MERGED'),
		  (SELECT count(*) FROM asset_event_outbox WHERE tenant_id=$1),
		  (SELECT count(*) FROM asset_discovery_control_requests WHERE tenant_id=$1 AND operation='merge_candidate'),
		  (SELECT discovered_assets FROM asset_discovery_runs WHERE tenant_id=$1 AND run_id=$2)`,
		discoveryIntegrationTenant, accepted.RunID,
	).Scan(
		&mergedAssets, &assetEvents, &mergeAudits, &assetOutbox,
		&mergeControls, &discoveredAssets,
	); err != nil {
		t.Fatal(err)
	}
	if mergedAssets != 1 || assetEvents != 1 || mergeAudits != 1 ||
		assetOutbox != 1 || mergeControls != 1 || discoveredAssets != 1 {
		t.Fatalf(
			"merge reconcile assets=%d events=%d audits=%d outbox=%d controls=%d run_assets=%d",
			mergedAssets, assetEvents, mergeAudits, assetOutbox, mergeControls, discoveredAssets,
		)
	}
	existingAssetRunCommand := command
	existingAssetRunCommand.IdempotencyKey = "discovery-integration-existing-asset-run"
	existingAssetRunRequest := *request
	existingAssetRunRequest.TargetCIDR = "203.0.113.0/30"
	existingAssetRun, err := svc.SubmitActiveDiscovery(
		context.Background(), &existingAssetRunRequest, existingAssetRunCommand,
	)
	if err != nil {
		t.Fatal(err)
	}
	found, err = svc.ProcessNextDiscoveryJob(context.Background(), "existing-asset-worker")
	if err != nil || !found {
		t.Fatalf("existing asset worker found=%v err=%v", found, err)
	}
	existingAssetCandidates, err := svc.ListDiscoveryCandidates(
		context.Background(), discoveryIntegrationTenant, existingAssetRun.RunID, 50,
	)
	if err != nil || len(existingAssetCandidates) != 1 {
		t.Fatalf("existing asset candidates=%+v err=%v", existingAssetCandidates, err)
	}
	staleAssetMerge := mergeCommand
	staleAssetMerge.IdempotencyKey = "discovery-integration-stale-asset"
	if _, err := svc.MergeDiscoveryCandidate(
		context.Background(), discoveryIntegrationTenant, existingAssetRun.RunID,
		existingAssetCandidates[0].CandidateID, staleAssetMerge,
	); !errors.Is(err, repository.ErrAssetRevisionConflict) {
		t.Fatalf("stale asset merge err=%v", err)
	}
	updateAssetMerge := mergeCommand
	updateAssetMerge.ExpectedAssetRevision = 1
	updateAssetMerge.IdempotencyKey = "discovery-integration-existing-asset-merge"
	updatedAsset, err := svc.MergeDiscoveryCandidate(
		context.Background(), discoveryIntegrationTenant, existingAssetRun.RunID,
		existingAssetCandidates[0].CandidateID, updateAssetMerge,
	)
	if err != nil {
		t.Fatal(err)
	}
	if updatedAsset.AssetCreated || updatedAsset.AssetID != merged.AssetID ||
		updatedAsset.AssetRevision != 2 ||
		updatedAsset.Candidate.Status != "merged" {
		t.Fatalf("updated asset merge=%+v", updatedAsset)
	}
	if _, err := svc.GetDiscoveryJob(context.Background(), "another-tenant", accepted.RunID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cross-tenant read err=%v", err)
	}

	cancelCommand := command
	cancelCommand.IdempotencyKey = "discovery-integration-cancel"
	cancelRequest := *request
	cancelRequest.TargetCIDR = "203.0.113.0/30"
	cancelledJob, err := svc.SubmitActiveDiscovery(context.Background(), &cancelRequest, cancelCommand)
	if err != nil {
		t.Fatal(err)
	}
	cancelledJob, err = svc.CancelDiscoveryJob(
		context.Background(), discoveryIntegrationTenant, cancelledJob.RunID,
		"window revoked", config.DiscoveryJobCommand{
			IdempotencyKey: "discovery-integration-cancel-control",
			Actor:          "integration-operator",
			TraceID:        "trace-cancel",
		}, 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if cancelledJob.Status != config.DiscoveryStatusCancelled || cancelledJob.Revision != 2 {
		t.Fatalf("cancelled=%+v", cancelledJob)
	}
	replayedCancel, err := svc.CancelDiscoveryJob(
		context.Background(), discoveryIntegrationTenant, cancelledJob.RunID,
		"window revoked", config.DiscoveryJobCommand{
			IdempotencyKey: "discovery-integration-cancel-control",
			Actor:          "integration-operator",
			TraceID:        "trace-cancel",
		}, 1,
	)
	if err != nil || !replayedCancel.IdempotentReplay {
		t.Fatalf("cancel replay=%+v err=%v", replayedCancel, err)
	}

	reclaimCommand := command
	reclaimCommand.IdempotencyKey = "discovery-integration-reclaim"
	reclaimRequest := *request
	reclaimRequest.TargetCIDR = "198.18.0.0/30"
	reclaimJob, err := svc.SubmitActiveDiscovery(context.Background(), &reclaimRequest, reclaimCommand)
	if err != nil {
		t.Fatal(err)
	}
	firstLease, err := repo.ClaimDiscoveryJob(context.Background(), "failed-worker", time.Minute)
	if err != nil || firstLease.RunID != reclaimJob.RunID || firstLease.Revision != 2 {
		t.Fatalf("first lease=%+v err=%v", firstLease, err)
	}
	if _, err := db.Exec(`
		UPDATE asset_discovery_runs
		   SET locked_until=now()-interval '1 second'
		 WHERE tenant_id=$1 AND run_id=$2`,
		discoveryIntegrationTenant, reclaimJob.RunID,
	); err != nil {
		t.Fatal(err)
	}
	reclaimed, err := repo.ClaimDiscoveryJob(context.Background(), "recovery-worker", time.Minute)
	if err != nil || reclaimed.RunID != reclaimJob.RunID || reclaimed.Revision != 3 {
		t.Fatalf("reclaimed=%+v err=%v", reclaimed, err)
	}
	cancelRequested, err := svc.CancelDiscoveryJob(
		context.Background(), discoveryIntegrationTenant, reclaimJob.RunID,
		"cancel recovered lease", config.DiscoveryJobCommand{
			IdempotencyKey: "discovery-integration-reclaim-cancel",
			Actor:          "integration-operator",
			TraceID:        "trace-recovered-cancel",
		}, 3,
	)
	if err != nil || cancelRequested.Status != config.DiscoveryStatusCancelRequested {
		t.Fatalf("cancel requested=%+v err=%v", cancelRequested, err)
	}
	recoveredTerminal, err := repo.CompleteDiscoveryJob(
		context.Background(), reclaimed,
		[]config.DiscoveryObservation{{MACAddress: "02:99:99:99:99:99"}},
		0, nil, "recovery-worker",
	)
	if err != nil {
		t.Fatal(err)
	}
	if recoveredTerminal.Status != config.DiscoveryStatusCancelled ||
		recoveredTerminal.Revision != 5 || recoveredTerminal.CandidateCount != 0 {
		t.Fatalf("recovered terminal=%+v", recoveredTerminal)
	}
	recoveredCandidates, err := svc.ListDiscoveryCandidates(
		context.Background(), discoveryIntegrationTenant, reclaimJob.RunID, 50,
	)
	if err != nil || len(recoveredCandidates) != 0 {
		t.Fatalf("cancelled in-flight candidates=%+v err=%v", recoveredCandidates, err)
	}

	expiredCommand := command
	expiredCommand.IdempotencyKey = "discovery-integration-expired-window"
	expiredRequest := *request
	expiredRequest.TargetCIDR = "198.19.0.0/30"
	expiredJob, err := svc.SubmitActiveDiscovery(context.Background(), &expiredRequest, expiredCommand)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		UPDATE asset_discovery_runs
		   SET security_window_start=now()-interval '2 hours',
		       security_window_end=now()-interval '1 hour'
		 WHERE tenant_id=$1 AND run_id=$2`,
		discoveryIntegrationTenant, expiredJob.RunID,
	); err != nil {
		t.Fatal(err)
	}
	found, err = svc.ProcessNextDiscoveryJob(context.Background(), "window-guard-worker")
	if err != nil || !found {
		t.Fatalf("expired window worker found=%v err=%v", found, err)
	}
	expiredTerminal, err := svc.GetDiscoveryJob(
		context.Background(), discoveryIntegrationTenant, expiredJob.RunID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if expiredTerminal.Status != config.DiscoveryStatusBlocked ||
		expiredTerminal.Revision != 2 ||
		expiredTerminal.ErrorMessage != "approved security window expired before worker lease" {
		t.Fatalf("expired terminal=%+v", expiredTerminal)
	}
}

func TestSubmitActiveDiscoveryRejectsInvalidScopeAndClientObservations(t *testing.T) {
	svc := service.New(&config.Config{}, nil, zap.NewNop())
	base := config.ActiveDiscoveryRequest{
		TenantID:   "tenant-a",
		ActionID:   "asset-active-discovery-run",
		Mode:       "snmp",
		TargetCIDR: "not-a-cidr",
		Reason:     "authorized test",
		RateLimit:  10,
		ApprovedBy: "approver",
	}
	command := config.DiscoveryJobCommand{
		IdempotencyKey: "discovery-validation-test",
		Actor:          "operator",
		TraceID:        "trace-validation",
	}
	if _, err := svc.SubmitActiveDiscovery(context.Background(), &base, command); err == nil {
		t.Fatal("invalid CIDR was accepted")
	}
	base.TargetCIDR = "192.0.2.0/24"
	base.Observations = []config.DiscoveryObservation{{MACAddress: "02:00:00:00:00:01"}}
	if _, err := svc.SubmitActiveDiscovery(context.Background(), &base, command); err == nil {
		t.Fatal("client-supplied observation was accepted")
	}
	base.Observations = nil
	base.ApprovedBy = command.Actor
	if _, err := svc.SubmitActiveDiscovery(context.Background(), &base, command); err == nil {
		t.Fatal("self-approved discovery was accepted")
	}
}

func cleanupDiscoveryIntegration(db *sql.DB) error {
	for _, table := range []string{
		"asset_discovery_candidates",
		"asset_discovery_run_history",
		"asset_discovery_control_requests",
		"asset_discovery_outbox",
		"asset_discovery_runs",
		"asset_discovery_credentials",
		"asset_event_outbox",
		"asset_events",
		"audit_logs",
		"assets",
		"tenants",
	} {
		if _, err := db.Exec(
			fmt.Sprintf("DELETE FROM %s WHERE tenant_id=$1", table),
			discoveryIntegrationTenant,
		); err != nil {
			return err
		}
	}
	return nil
}
