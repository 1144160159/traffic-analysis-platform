#!/usr/bin/env python3

from __future__ import annotations

import importlib.util
import subprocess
import unittest
from pathlib import Path
from unittest import mock


SCRIPT = Path(__file__).with_name("run_m07_campaign_rail_k8s_canary.py")
SPEC = importlib.util.spec_from_file_location("run_m07_campaign_rail_k8s_canary", SCRIPT)
assert SPEC and SPEC.loader
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class CampaignRailK8sCanaryTest(unittest.TestCase):
    run_id = "11111111-1111-4111-8111-111111111111"
    candidate = "a" * 64
    tenant = "canary-m07-111111111111"

    def test_validation_binds_contract_and_canary_tenant(self) -> None:
        suffix, tenant = MODULE.validate_inputs(
            "docker.io/traffic/alert-service:m07-test", self.candidate, self.run_id, "8-2tb"
        )
        self.assertEqual(suffix, "11111111")
        self.assertEqual(tenant, self.tenant)
        with self.assertRaises(MODULE.CanaryError):
            MODULE.validate_inputs(
                "docker.io/traffic/alert-service:latest", self.candidate, self.run_id, "8-2tb"
            )

    def test_each_runtime_stage_has_one_unique_enabled_switch(self) -> None:
        names = MODULE.resource_names("11111111")
        switches = {
            "proto-consumer": "CAMPAIGNS_PROTO_CONSUMER_V1_ENABLED",
            "json-consumer": "CAMPAIGN_EVENT_CONSUMER_V2_ENABLED",
            "json-dispatcher": "CAMPAIGN_EVENT_DISPATCHER_V2_ENABLED",
            "correlation": "CAMPAIGN_RAIL_CORRELATION_V1_ENABLED",
        }
        for stage, expected in switches.items():
            deployment = MODULE.stage_deployment(
                names[stage.split("-")[0] if stage != "json-dispatcher" else "dispatcher"],
                stage,
                "docker.io/traffic/alert-service:m07-test",
                self.candidate,
                self.run_id,
                self.tenant,
                "8-2tb",
                names,
            )
            env = deployment["spec"]["template"]["spec"]["containers"][0]["env"]
            names_in_env = [item["name"] for item in env]
            self.assertEqual(len(names_in_env), len(set(names_in_env)))
            enabled = [item["name"] for item in env if item["name"] in switches.values() and item["value"] == "true"]
            self.assertEqual(enabled, [expected])
            self.assertNotEqual(deployment["metadata"]["name"], "alert-service")
            self.assertEqual(deployment["spec"]["template"]["spec"]["containers"][0]["imagePullPolicy"], "Never")

    def test_postgres_is_run_scoped_and_uses_real_migrations(self) -> None:
        names = MODULE.resource_names("11111111")
        objects = MODULE.postgres_objects(names, "ephemeral-password", self.run_id, self.candidate, "8-2tb")
        config_map = objects[0]
        self.assertEqual(config_map["metadata"]["namespace"], MODULE.DB_NAMESPACE)
        self.assertEqual(set(config_map["data"]), {
            "00-prerequisites.sql", "10-alert-links.sql", "20-campaign-aggregate.sql",
            "30-campaign-membership.sql", "40-campaign-delivery.sql",
            "50-campaign-correlation.sql", "90-canary-sentinel.sql",
        })
        self.assertIn("as_of", config_map["data"]["50-campaign-correlation.sql"])
        self.assertIn(self.candidate, config_map["data"]["90-canary-sentinel.sql"])
        pod = next(item for item in objects if item["kind"] == "Pod")
        self.assertEqual(pod["spec"]["restartPolicy"], "Never")
        self.assertEqual(pod["spec"]["volumes"][0], {"name": "data", "emptyDir": {}})

    def test_psql_streams_multiline_query_without_shell_escaping(self) -> None:
        names = MODULE.resource_names("11111111")
        query = "SELECT json_build_object(\n  'ready', true\n);"
        completed = subprocess.CompletedProcess([], 0, stdout='{"ready":true}\n', stderr="")
        with mock.patch.object(MODULE, "kubectl", return_value=completed) as kubectl:
            result = MODULE.psql(names, query)

        self.assertEqual(result, '{"ready":true}')
        args, kwargs = kubectl.call_args
        self.assertIn("-i", args)
        self.assertNotIn("-c", args)
        self.assertEqual(kwargs["input_text"], query)


if __name__ == "__main__":
    unittest.main()
