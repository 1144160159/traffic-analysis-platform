#!/usr/bin/env python3
"""Contract tests for metadata-only governed model registration."""

import hashlib
import json
import os
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parent))

import register_model


class _Response:
    status_code = 201
    text = '{"success":true}'

    def raise_for_status(self):
        return None

    def json(self):
        return {"success": True, "data": {
            "status": "registered", "revision": 1,
            "model_id": "11111111-1111-4111-8111-111111111111",
            "model_version": "v1",
            "registration_request_sha256": "e" * 64,
        }}


class _ShadowResponse:
    status_code = 202
    text = '{"success":true}'

    def raise_for_status(self):
        return None

    def json(self):
        return {"success": True, "data": {
            "request_id": "44444444-4444-4444-8444-444444444444",
            "event_id": "55555555-5555-4555-8555-555555555555",
            "tenant_id": "tenant-a",
            "model_id": "11111111-1111-4111-8111-111111111111",
            "model_version": "v1",
            "package_id": "package-1",
            "package_sha256": "a" * 64,
            "expected_revision": 0,
            "aggregate_revision": 1,
            "request_sha256": "b" * 64,
            "state": "outbox_pending",
            "serving_activated": False,
        }}


class GovernedModelRegistrationTest(unittest.TestCase):
    def test_registry_call_has_idempotency_key_and_no_activation_surface(self):
        with mock.patch.dict(os.environ, {
            "MODEL_REGISTRY_URL": "http://registry",
            "API_TOKEN": "test-token-required",
        }, clear=True), \
                mock.patch.object(register_model.requests, "post", return_value=_Response()) as post:
            result = register_model.register_model_to_registry(
                {"model_id": "model-a", "version": "v1"}, "registration-command-0001"
            )
        self.assertTrue(result["success"])
        _, kwargs = post.call_args
        self.assertEqual(kwargs["headers"]["Idempotency-Key"], "registration-command-0001")
        self.assertEqual(kwargs["headers"]["Authorization"], "Bearer test-token-required")
        self.assertEqual(kwargs["timeout"], 30)
        self.assertFalse(hasattr(register_model, "activate_model_version"))
        self.assertFalse(hasattr(register_model, "notify_flink_reload"))
        self.assertFalse(hasattr(register_model, "notify_via_kafka"))

    def test_registry_call_fails_closed_without_api_token(self):
        with mock.patch.dict(os.environ, {"MODEL_REGISTRY_URL": "http://registry"}, clear=True), \
                mock.patch.object(register_model.requests, "post", return_value=_Response()):
            with self.assertRaisesRegex(RuntimeError, "API_TOKEN is required"):
                register_model.register_model_to_registry(
                    {"model_id": "model-a", "version": "v1"}, "registration-command-0001"
                )

    def test_signed_storage_receipt_maps_to_metadata_only_contract(self):
        digest = "a" * 64
        manifest = {
            "package_id": "76c8debb-d938-596c-b14d-183d20799ef7",
            "package_sha256": digest,
            "tenant_id": "tenant-a",
            "model_id": "behavior-classifier",
            "model_version": "v1.2.3",
            "evaluation_sha256": "b" * 64,
            "explanation_sha256": "c" * 64,
            "graph_snapshot": {"snapshot_id": "graph-1", "manifest_sha256": "d" * 64},
            "signature": {"key_id": "kms/model/key-1"},
            "compatibility": {"feature_set_id": "feature-v1"},
            "artifacts": {
                "baseline-model.onnx": {"sha256": "1" * 64},
                "gnn-full-model.npz": {"sha256": "2" * 64},
                "inference-graph-schema.json": {"sha256": "3" * 64},
                "compatibility-metadata.json": {"sha256": "4" * 64},
            },
        }
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            manifest_bytes = json.dumps(manifest, sort_keys=True).encode()
            (root / "model-artifact-manifest.json").write_bytes(manifest_bytes)
            objects = {
                name: {"uri": f"s3://traffic-models/package/{name}", "sha256": value["sha256"]}
                for name, value in manifest["artifacts"].items()
            }
            objects["model-artifact-manifest.json"] = {
                "uri": "s3://traffic-models/package/model-artifact-manifest.json",
                "sha256": hashlib.sha256(manifest_bytes).hexdigest(),
            }
            (root / "model-package-storage-receipt.json").write_text(json.dumps({
                "schema_version": 1, "state": "stored",
                "package_id": manifest["package_id"], "package_sha256": digest,
                "objects": objects, "activation_authorized": False,
            }), encoding="utf-8")
            with mock.patch(
                "model_artifact_governance.verify_export_package", return_value=manifest
            ):
                payload = register_model.build_governed_registration_payload(
                    str(root), {"f1_score": 0.91}, "/trusted/public.pem"
                )
        self.assertEqual(payload["expected_revision"], 0)
        self.assertEqual(payload["status"], "registered")
        self.assertEqual(payload["governance_version"], "model-registration.v1")
        self.assertNotIn("activate", payload)
        self.assertNotIn("activation_authorized", payload)

    def test_receipt_cannot_authorize_activation(self):
        with tempfile.TemporaryDirectory() as temporary, mock.patch(
            "model_artifact_governance.verify_export_package",
            return_value={"package_id": "p", "package_sha256": "a" * 64},
        ):
            Path(temporary, "model-package-storage-receipt.json").write_text(json.dumps({
                "schema_version": 1, "state": "stored", "package_id": "p",
                "package_sha256": "a" * 64, "activation_authorized": True, "objects": {},
            }), encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "must not authorize activation"):
                register_model.build_governed_registration_payload(
                    temporary, {}, "/trusted/public.pem"
                )

    def test_registration_receipt_is_exclusive_and_never_authorizes_activation(self):
        metadata = {
            "tenant_id": "tenant-a", "model_id": "behavior-classifier", "version": "v1",
            "package_id": "76c8debb-d938-596c-b14d-183d20799ef7", "package_sha256": "a" * 64,
        }
        result = _Response().json()
        with tempfile.TemporaryDirectory() as temporary:
            target = Path(temporary, "receipt.json")
            receipt = register_model.write_registration_receipt(str(target), metadata, result)
            self.assertFalse(receipt["activation_event_created"])
            self.assertFalse(receipt["activation_authorized"])
            with self.assertRaises(FileExistsError):
                register_model.write_registration_receipt(str(target), metadata, result)

    def test_shadow_command_uses_separate_revision_guarded_endpoint(self):
        with mock.patch.dict(os.environ, {
            "MODEL_REGISTRY_URL": "http://registry",
            "API_TOKEN": "test-token-required",
        }, clear=True), \
                mock.patch.object(register_model.requests, "post", return_value=_ShadowResponse()) as post:
            result = register_model.prepare_model_shadow_activation(
                "11111111-1111-4111-8111-111111111111", "v1", 0,
                "22222222-2222-4222-8222-222222222222",
                "independent approval for isolated shadow loading",
                "shadow-activation-command-0001",
            )
        self.assertFalse(result["data"]["serving_activated"])
        args, kwargs = post.call_args
        self.assertEqual(
            args[0],
            "http://registry/api/v1/models/11111111-1111-4111-8111-111111111111/versions/v1/shadow-activation",
        )
        self.assertEqual(kwargs["headers"]["Idempotency-Key"], "shadow-activation-command-0001")
        self.assertEqual(kwargs["json"]["expected_revision"], 0)
        self.assertNotIn("gray_percent", kwargs["json"])

    def test_shadow_receipt_is_exclusive_and_never_claims_serving(self):
        result = _ShadowResponse().json()
        with tempfile.TemporaryDirectory() as temporary:
            target = Path(temporary, "shadow-receipt.json")
            receipt = register_model.write_shadow_activation_receipt(str(target), result)
            self.assertEqual(receipt["state"], "outbox_pending")
            self.assertFalse(receipt["serving_activated"])
            with self.assertRaises(FileExistsError):
                register_model.write_shadow_activation_receipt(str(target), result)

        bad = _ShadowResponse().json()
        bad["data"]["serving_activated"] = True
        with tempfile.TemporaryDirectory() as temporary:
            with self.assertRaisesRegex(ValueError, "cannot mark serving active"):
                register_model.write_shadow_activation_receipt(
                    str(Path(temporary, "bad.json")), bad
                )

    def test_shadow_ops_waits_for_durable_ready_receipt_without_serving_claim(self):
        pending = _ShadowResponse().json()
        published = _ShadowResponse().json()
        published["data"]["state"] = "published"
        ready = _ShadowResponse().json()
        ready["data"]["state"] = "shadow_ready"
        ready["data"]["shadow_ready_expires_at"] = "2026-08-15T12:35:00Z"

        def response(payload):
            item = mock.Mock()
            item.raise_for_status.return_value = None
            item.json.return_value = payload
            return item

        with mock.patch.dict(os.environ, {
            "MODEL_REGISTRY_URL": "http://registry",
            "API_TOKEN": "test-token-required",
        }, clear=True), \
                mock.patch.object(
                    register_model.requests, "get",
                    side_effect=[response(pending), response(published), response(ready)],
                ) as get, mock.patch.object(register_model.time, "sleep"):
            result = register_model.wait_for_model_shadow_ready(
                "11111111-1111-4111-8111-111111111111", "v1",
                "44444444-4444-4444-8444-444444444444", 5, 0.01,
            )
        self.assertEqual(result["data"]["state"], "shadow_ready")
        self.assertFalse(result["data"]["serving_activated"])
        self.assertEqual(get.call_count, 3)
        self.assertIn("/shadow-activation/44444444-4444-4444-8444-444444444444", get.call_args.args[0])


if __name__ == "__main__":
    unittest.main()
