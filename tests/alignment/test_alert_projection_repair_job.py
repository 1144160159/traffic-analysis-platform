#!/usr/bin/env python3

from __future__ import annotations

import copy
import sys
import unittest
from datetime import datetime, timedelta, timezone
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from render_alert_projection_repair_job import render_documents, validate_inputs  # noqa: E402


CONTENT_SHA = "b" * 64
REVIEW_SHA = "c" * 64
APPROVAL_SHA = "d" * 64
IMAGE_DIGEST = "sha256:" + "e" * 64
IMAGE = "registry.example.test/traffic/alert-projection-tools@" + IMAGE_DIGEST


def review() -> dict:
    return {
        "schema_version": 1,
        "mode": "REPAIR_REVIEW_PACKAGE",
        "execution_authorized": False,
        "production_applied": False,
        "production_mutations": [],
        "bindings": {
            "g0_candidate_content_sha256": CONTENT_SHA,
            "shadow_binding_sha256": "f" * 64,
            "shadow_captured_at": "2026-08-08T01:55:00+00:00",
            "immutable_tool_image_digest": IMAGE_DIGEST,
            "tenant_id": "tenant-a",
            "start_time": "2026-08-08T00:00:00Z",
            "end_time": "2026-08-08T00:30:00Z",
            "cluster_uuid": "cluster-a",
            "read_target": "alerts-v2-read",
            "write_alias": "alerts-v2-write",
            "write_index": "alerts-v2-000001",
            "missing_count": 1,
            "stale_count": 1,
            "repair_ids": ["alert-a", "alert-b"],
        },
        "proposed_execution": {
            "argv": [
                "alert-projection-reconcile", "--mode", "repair", "--confirm-repair",
                "--tenant", "tenant-a", "--requested-by", "APPROVED_OPERATOR_REQUIRED",
                "--trace-id", "trace-a", "--start", "2026-08-08T00:00:00Z",
                "--end", "2026-08-08T00:30:00Z", "--alert-ids", "alert-a,alert-b",
                "--target-index-version", "alerts-v2-write", "--expected-cluster-uuid", "cluster-a",
                "--expected-read-target", "alerts-v2-read", "--expected-write-alias", "alerts-v2-write",
                "--expected-write-index", "alerts-v2-000001", "--max-documents", "2",
            ],
            "shell": None,
        },
    }


def approval(now: datetime) -> dict:
    not_before = now - timedelta(minutes=10)
    expires_at = now + timedelta(minutes=10)
    return {
        "schema_version": 1,
        "mode": "AUTHORIZED_BOUNDED_REPAIR",
        "execution_authorized": True,
        "review_package_sha256": REVIEW_SHA,
        "immutable_tool_image": IMAGE,
        "approval_nonce": "change-20260808-a",
        "not_before": not_before.isoformat(),
        "expires_at": expires_at.isoformat(),
        "requested_by": "operator-a",
        "approvals": {
            role: {"status": "APPROVED", "approved_by": f"{role}-approver", "approved_at": now.isoformat()}
            for role in ("sre", "qa", "security", "domain_accountable")
        },
    }


class AlertProjectionRepairJobTests(unittest.TestCase):
    def setUp(self) -> None:
        self.now = datetime(2026, 8, 8, 2, 0, tzinfo=timezone.utc)

    def validate(self, review_value: dict | None = None, approval_value: dict | None = None):
        return validate_inputs(
            review_value or review(), REVIEW_SHA, approval_value or approval(self.now), APPROVAL_SHA,
            now=self.now, current_content_sha256=CONTENT_SHA,
        )

    def test_valid_approval_renders_only_a_suspended_hardened_job(self) -> None:
        image, requested_by, argv = self.validate()
        docs = render_documents(
            review=review(), review_text="{}\n", review_sha=REVIEW_SHA,
            approval=approval(self.now), approval_text="{}\n", approval_sha=APPROVAL_SHA,
            image=image, requested_by=requested_by, proposed_argv=argv, run_id="repair-a",
        )
        self.assertEqual([item["kind"] for item in docs], ["ServiceAccount", "ConfigMap", "NetworkPolicy", "Job"])
        self.assertTrue(docs[1]["immutable"])
        job = docs[-1]
        self.assertTrue(job["spec"]["suspend"])
        self.assertEqual(job["spec"]["backoffLimit"], 0)
        pod = job["spec"]["template"]["spec"]
        self.assertFalse(pod["automountServiceAccountToken"])
        container = pod["containers"][0]
        self.assertEqual(container["image"], IMAGE)
        self.assertEqual(container["command"], ["/usr/local/bin/alert-projection-reconcile"])
        self.assertNotIn("sh", container["command"])
        self.assertIn("operator-a", container["args"])
        self.assertIn("--expected-approval-sha256", container["args"])
        self.assertTrue(container["securityContext"]["readOnlyRootFilesystem"])
        secret_keys = {
            item["valueFrom"]["secretKeyRef"]["key"]
            for item in container["env"] if "valueFrom" in item
        }
        self.assertEqual(secret_keys, {"CLICKHOUSE_PASSWORD", "OPENSEARCH_ADMIN_PASSWORD", "PG_PASSWORD"})

    def test_rejects_pending_self_approved_duplicate_or_expired_approval(self) -> None:
        cases = {}
        pending = approval(self.now)
        pending["approvals"]["qa"]["status"] = "PENDING"
        cases["pending"] = pending
        self_approved = approval(self.now)
        self_approved["approvals"]["sre"]["approved_by"] = "operator-a"
        cases["self approved"] = self_approved
        duplicate = approval(self.now)
        duplicate["approvals"]["qa"]["approved_by"] = duplicate["approvals"]["sre"]["approved_by"]
        cases["duplicate"] = duplicate
        expired = approval(self.now)
        expired["not_before"] = (self.now - timedelta(hours=2)).isoformat()
        expired["expires_at"] = (self.now - timedelta(hours=1)).isoformat()
        cases["expired"] = expired
        for name, value in cases.items():
            with self.subTest(name=name), self.assertRaises(ValueError):
                self.validate(approval_value=value)

    def test_rejects_source_image_review_and_scope_drift(self) -> None:
        with self.assertRaises(ValueError):
            validate_inputs(review(), REVIEW_SHA, approval(self.now), APPROVAL_SHA, now=self.now, current_content_sha256="0" * 64)
        image_drift = approval(self.now)
        image_drift["immutable_tool_image"] = "registry.example.test/tools@sha256:" + "0" * 64
        with self.assertRaises(ValueError):
            self.validate(approval_value=image_drift)
        mutating = review()
        mutating["production_mutations"] = ["opensearch_bulk"]
        with self.assertRaises(ValueError):
            self.validate(review_value=mutating)
        wildcard = review()
        wildcard["proposed_execution"]["argv"] = copy.deepcopy(wildcard["proposed_execution"]["argv"])
        index = wildcard["proposed_execution"]["argv"].index("--tenant") + 1
        wildcard["proposed_execution"]["argv"][index] = "*"
        with self.assertRaises(ValueError):
            self.validate(review_value=wildcard)
        stale = review()
        stale["bindings"]["shadow_captured_at"] = "2026-08-08T01:40:00Z"
        with self.assertRaises(ValueError):
            self.validate(review_value=stale)

    def test_dockerfile_and_template_are_immutable_and_non_executing(self) -> None:
        dockerfile = (ROOT / "go/control-plane/deployments/docker/Dockerfile.alert-projection-tools").read_text()
        self.assertIn("golang@sha256:", dockerfile)
        self.assertIn("alpine@sha256:", dockerfile)
        self.assertIn("FROM scratch", dockerfile)
        self.assertIn("SOURCE_CONTENT_SHA256", dockerfile)
        self.assertIn("/usr/local/bin/alert-projection-shadow", dockerfile)
        self.assertIn("/usr/local/bin/alert-projection-reconcile", dockerfile)
        template = (ROOT / "deployments/kubernetes/migrations/opensearch/T-OS-004-alert-projection-repair.template.yaml").read_text()
        self.assertIn("suspend: true", template)
        self.assertIn("backoffLimit: 0", template)
        self.assertIn("REQUIRED-REPOSITORY@sha256:", template)


if __name__ == "__main__":
    unittest.main()
