import json
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]


def source(path: str) -> str:
    return (ROOT / path).read_text(encoding="utf-8")


def test_workbench_uses_versioned_task_client_and_refresh_identity():
    page = source("web/ui/src/pages/ForensicsWorkbenchPage.tsx")
    client = source("web/ui/src/services/forensicsApi.ts")
    for marker in (
        "searchParams.get('job_id')",
        "getForensicsJob(selectedJobId)",
        "refetchInterval",
        "任务已受理，但尚未完成",
        "部分完成不等于完整证据",
        "source_object_receipts",
        "restoration_receipts",
        "result_object.object_version",
    ):
        assert marker in page
    for marker in (
        "Idempotency-Key",
        "X-Action-Reason",
        "If-Match",
        "restoration_contract_version: 1",
        "purpose: purpose.trim()",
        "expiry_seconds: expirySeconds",
    ):
        assert marker in client


def test_openapi_separates_admission_from_completion_and_binds_exact_result_version():
    document = json.loads(source("contracts/openapi/alignment-v1.openapi.json"))
    create = document["paths"]["/v1/pcap/jobs"]["post"]
    assert "not a completion receipt" in create["responses"]["202"]["description"].lower()
    assert document["components"]["schemas"]["ForensicsJobStatus"]["enum"] == [
        "queued", "processing", "partial", "completed", "failed", "cancelled",
    ]
    assert document["paths"]["/v1/pcap/presign"]["post"]["x-required-scope"] == "pcap:download"
    assert document["paths"]["/v1/pcap/verify"]["post"]["x-required-scope"] == "pcap:read"
    download = document["paths"]["/v1/pcap/download/{key}"]["get"]
    assert download["x-required-scope"] == "pcap:download"
    purpose = next(parameter for parameter in download["parameters"] if parameter["name"] == "purpose")
    assert not purpose["required"]
    assert "Required for versioned result keys" in purpose["description"]


def test_page_action_plan_keeps_retry_and_exact_version_guardrails():
    plan = source("web/ui/src/services/pageApiPlans.ts")
    for marker in (
        'id: "forensics-retry-job"',
        'endpoint: "/v1/pcap/jobs/{id}/retry"',
        "retry preserves the original frozen request and task identity",
        "the signed URL must remain bound to the exact manifest object version",
    ):
        assert marker in plan
