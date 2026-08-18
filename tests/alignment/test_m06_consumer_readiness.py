from __future__ import annotations

import importlib.util
import sys
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "scripts/alignment/capture_m06_consumer_readiness.py"
sys.path.insert(0, str(SCRIPT.parent))
SPEC = importlib.util.spec_from_file_location("capture_m06_consumer_readiness", SCRIPT)
MODULE = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(MODULE)


def deployment() -> dict:
    return {
        "kind": "Deployment",
        "metadata": {"name": "consumer", "uid": "uid-1", "resourceVersion": "9", "generation": 3},
        "spec": {
            "replicas": 2,
            "template": {"spec": {"containers": [{
                "name": "consumer",
                "image": "registry/consumer@sha256:" + "a" * 64,
                "env": [{"name": "ENABLED", "value": "true"}],
            }]}},
        },
        "status": {"observedGeneration": 3, "readyReplicas": 2, "updatedReplicas": 2, "availableReplicas": 2},
    }


class M06ConsumerReadinessTest(unittest.TestCase):
    def assertBlocked(self, code: str, fn) -> None:
        with self.assertRaises(MODULE.CanaryBlocked) as caught:
            fn()
        self.assertEqual(code, caught.exception.code)

    def test_default_plan_has_exact_consumer_and_producer_guard_sets(self) -> None:
        plan = MODULE.load_plan(MODULE.DEFAULT_PLAN)
        self.assertEqual(set(MODULE.PHASES), set(plan["consumer_observation"]["consumers"]))
        for phase in MODULE.PHASES:
            binding = plan["consumer_observation"]["consumers"][phase]
            self.assertEqual(plan["consumer_readiness"][phase]["consumer"], binding["consumer"])
            self.assertTrue(binding["producer_guards"])

    def test_workload_requires_all_replicas_current_and_ready(self) -> None:
        body = deployment()
        receipt = MODULE.ready_workload(body, "deployment/consumer")
        self.assertEqual(2, receipt["desired"])
        body["status"]["updatedReplicas"] = 1
        self.assertBlocked("BLOCK_READINESS_REPLICAS", lambda: MODULE.ready_workload(body, "deployment/consumer"))

    def test_consumer_env_is_exact_and_value_from_is_rejected(self) -> None:
        selected = MODULE.container(deployment(), "consumer")
        self.assertEqual({"ENABLED": "true"}, MODULE.exact_env(selected, {"ENABLED": "true"}))
        selected["env"][0] = {"name": "ENABLED", "valueFrom": {"secretKeyRef": {"name": "x", "key": "y"}}}
        self.assertBlocked("BLOCK_READINESS_ENV_VALUE_FROM", lambda: MODULE.exact_env(selected, {"ENABLED": "true"}))

    def test_kafka_group_requires_every_partition_to_have_a_member(self) -> None:
        output = "\n".join([
            "GROUP TOPIC PARTITION CURRENT-OFFSET LOG-END-OFFSET LAG CONSUMER-ID HOST CLIENT-ID",
            "group-a topic-a 0 4 4 0 consumer-1 /10.0.0.1 client",
            "group-a topic-a 1 - 0 - consumer-1 /10.0.0.1 client",
        ])
        result = MODULE.parse_group_rows(output, group="group-a", topic="topic-a", partitions=2)
        self.assertEqual(["consumer-1"], result["members"])
        self.assertEqual([0, 1], [item["partition"] for item in result["rows"]])

        unassigned = output.replace("consumer-1 /10.0.0.1 client", "- - -", 1)
        self.assertBlocked(
            "BLOCK_READINESS_KAFKA_UNASSIGNED",
            lambda: MODULE.parse_group_rows(unassigned, group="group-a", topic="topic-a", partitions=2),
        )

    def test_kafka_group_partition_set_is_exact(self) -> None:
        output = "group-a topic-a 0 0 0 0 consumer-1 /10.0.0.1 client"
        self.assertBlocked(
            "BLOCK_READINESS_KAFKA_PARTITION_SET",
            lambda: MODULE.parse_group_rows(output, group="group-a", topic="topic-a", partitions=2),
        )

    def test_repository_plan_cannot_issue_a_live_receipt(self) -> None:
        plan = MODULE.load_plan(MODULE.DEFAULT_PLAN)
        self.assertBlocked("BLOCK_CANDIDATE_ID", lambda: MODULE.capture(plan, "asset-events"))


if __name__ == "__main__":
    unittest.main()
