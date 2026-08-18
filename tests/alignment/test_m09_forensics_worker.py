import json
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]


def source(path: str) -> str:
    return (ROOT / path).read_text(encoding="utf-8")


def test_versioned_worker_is_idle_and_writer_is_default_off():
    config = source("go/control-plane/internal/forensics/config/config.go")
    main = source("go/control-plane/cmd/forensics-service/main.go")
    worker = source("go/control-plane/internal/forensics/task/async_cutter.go")
    manifests = source("go/control-plane/deployments/kubernetes/forensics-service.yaml") + source(
        "deployments/kubernetes/applications/go-services.yaml"
    )
    for marker in (
        'env:"FORENSICS_WORKER_ENABLED" envDefault:"false"',
        'env:"FORENSICS_WORKER_COMPATIBLE_READY" envDefault:"false"',
        'env:"FORENSICS_PIPELINE_V1_ENABLED" envDefault:"false"',
    ):
        assert marker in config
    assert "CompatibleWorkerReady()" in main
    assert "SetVersionedPipeline(versionedPipeline)" in main
    assert "if a.config.ConsumerEnabled {" in worker
    assert manifests.count("FORENSICS_WORKER_ENABLED") >= 2
    assert manifests.count('value: "false"') >= 2


def test_worker_uses_m02_authority_and_m03_processor_without_second_reassembly():
    index = source("go/control-plane/internal/forensics/index/verified_cut_source.go")
    cutter = source("go/control-plane/internal/forensics/cutter/pcap_cutter.go")
    pipeline = source("go/control-plane/internal/forensics/task/versioned_pipeline.go")
    assert "traffic.pcap_index_v2" in index
    for marker in ("object_version", "etag", "sha256", "manifest_version >= 2"):
        assert marker in index
    assert "LookupVerifiedCutSources" in cutter
    assert "ReadVerifiedObject" in cutter
    assert "SourceRetention" in cutter
    assert "pipeline.restoration.Process" in pipeline
    assert "reassembly." not in pipeline


def test_checkpoint_manifest_and_result_objects_are_fenced_and_immutable():
    repository = source("go/control-plane/internal/forensics/repository/task_execution.go")
    result_store = source("go/control-plane/internal/forensics/s3client/forensics_result.go")
    migration = source("deployments/postgres/migrations/202608151930_m09_forensics_worker_checkpoint_manifest.sql")
    for marker in (
        "ClaimVersionedExecution",
        "AdvanceVersionedExecution",
        "CompleteVersionedExecution",
        "lease_token",
        "checkpoint_revision",
        "LevelSerializable",
    ):
        assert marker in repository
    for marker in ("SetMatchETagExcept", "VersionID", "RetentionUntil", "ErrForensicsResultConflict"):
        assert marker in result_store
    for table in ("forensics_task_checkpoints", "forensics_job_manifests"):
        assert f"CREATE TABLE IF NOT EXISTS {table}" in migration
    assert "CHECK (NOT executable)" in migration
    assert "CHECK (NOT automatic_open)" in migration


def test_openapi_exposes_restoration_contract_and_partial_terminal_state():
    document = json.loads(source("contracts/openapi/alignment-v1.openapi.json"))
    schemas = document["components"]["schemas"]
    create = schemas["ForensicsTaskCreate"]
    assert create["properties"]["restorations"]["items"]["$ref"] == "#/components/schemas/ForensicsRestorationTaskSpec"
    restoration = schemas["ForensicsRestorationTaskSpec"]
    assert {"request_id", "protocol_profile_id", "session_id", "community_id"}.issubset(restoration["required"])
    assert "partial" in schemas["ForensicsTaskCommandReceipt"]["properties"]["status"]["enum"]


def test_k8s_worker_evidence_is_candidate_bound_and_cleaned():
    evidence = json.loads(source("doc/02_acceptance/topic1/work-orders/t1-m09-w9-forensics-worker/test-result.json"))
    assert evidence["artifact_kind"] == "M09_FORENSICS_WORKER_TEST_RESULT"
    assert evidence["task_id"] == "T1-M09-N010"
    assert evidence["status"] == "PASS"
    assert evidence["production_applied"] is False
    assert evidence["run_scoped_resources_removed"] is True
    assert evidence["postgres_oracle"] == {
        "checkpoints": 0,
        "manifests": 0,
        "sentinel": "ephemeral-only",
        "task_migration": 1,
        "tasks": 0,
        "worker_migration": 1,
    }
    assert evidence["clickhouse_authority_rows"] == 1
    assert len(evidence["kubernetes"]) == 4
    assert all(item["pod_uid"] and item["image_id"] for item in evidence["kubernetes"])
