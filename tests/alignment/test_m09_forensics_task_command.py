import json
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]


def test_forensics_openapi_exposes_versioned_atomic_commands():
    document = json.loads((ROOT / "contracts/openapi/alignment-v1.openapi.json").read_text(encoding="utf-8"))
    paths = document["paths"]
    assert paths["/v1/pcap/jobs"]["post"]["operationId"] == "createPcapForensicsJob"
    assert paths["/v1/pcap/jobs/{id}/cancel"]["post"]["operationId"] == "cancelPcapForensicsJob"
    assert paths["/v1/pcap/jobs/{id}/retry"]["post"]["operationId"] == "retryPcapForensicsJob"
    create = document["components"]["schemas"]["ForensicsTaskCreate"]
    assert {"start_time", "end_time", "purpose"}.issubset(create["required"])
    for field in ("probe_ids", "alert_ids", "case_ids", "retention_policy", "restoration_contract_version"):
        assert field in create["properties"]
    assert "permission_snapshot" not in create["properties"]


def test_forensics_writer_is_default_off_and_worker_gated():
    config = (ROOT / "go/control-plane/internal/forensics/config/config.go").read_text(encoding="utf-8")
    handler = (ROOT / "go/control-plane/internal/forensics/api/handler.go").read_text(encoding="utf-8")
    main = (ROOT / "go/control-plane/cmd/forensics-service/main.go").read_text(encoding="utf-8")
    manifests = "\n".join([
        (ROOT / "go/control-plane/deployments/kubernetes/forensics-service.yaml").read_text(encoding="utf-8"),
        (ROOT / "deployments/kubernetes/applications/go-services.yaml").read_text(encoding="utf-8"),
    ])
    assert 'env:"FORENSICS_PIPELINE_V1_ENABLED" envDefault:"false"' in config
    assert 'env:"FORENSICS_WORKER_COMPATIBLE_READY" envDefault:"false"' in config
    assert "h.taskPipelineEnabled && h.compatibleWorkerReady" in handler
    assert "SetTaskCommandAdmission(cfg.Task.PipelineV1Enabled && cfg.Task.WorkerEnabled, workerReady)" in main
    assert manifests.count("FORENSICS_PIPELINE_V1_ENABLED") >= 2
    assert manifests.count("FORENSICS_WORKER_COMPATIBLE_READY") >= 2


def test_forensics_atomic_repository_freezes_request_and_supports_retry():
    repository = (ROOT / "go/control-plane/internal/forensics/repository/task_command_atomic.go").read_text(encoding="utf-8")
    task = (ROOT / "go/control-plane/internal/forensics/task/async_cutter.go").read_text(encoding="utf-8")
    migration = (ROOT / "deployments/postgres/migrations/202608031600_forensics_task_command_atomic.sql").read_text(encoding="utf-8")
    for marker in (
        "ForensicsTaskRetryAction", "RetryForTenant", 'case "retry"',
        "forensics_task_outbox", "forensics_task_history", "forensics_task_requests",
        '"params_sha256"',
    ):
        assert marker in repository
    for field in ("ProbeIDs", "AlertIDs", "CaseIDs", "PermissionSnapshot", "Purpose", "RetentionPolicy", "RestorationContractVersion"):
        assert field in task
    for table in ("forensics_task_history", "forensics_task_outbox", "forensics_task_requests"):
        assert f"CREATE TABLE IF NOT EXISTS {table}" in migration
