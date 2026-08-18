import copy
import json
import sys
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))

from verify_m06_source_quality_receipt import ContractError, validate_contract  # noqa: E402


class M06SourceQualityReceiptTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.contract = json.loads((ROOT / "contracts/data-quality/source-quality-receipt.v1.json").read_text(encoding="utf-8"))
        cls.schema = json.loads((ROOT / "contracts/data-quality/source-quality-receipt.schema.json").read_text(encoding="utf-8"))

    def test_contract_keeps_receipts_inside_m06_boundary(self) -> None:
        result = validate_contract(self.contract, self.schema)
        self.assertEqual("PASS", result["status"])
        self.assertFalse(self.contract["persistence"]["new_table"])
        self.assertFalse(self.contract["persistence"]["new_topic"])
        self.assertFalse(self.contract["rollout"]["enabled"])

    def test_offset_order_scope_and_four_rail_mutations_fail(self) -> None:
        mutations = (
            lambda item: item["offset_barrier"]["order"].reverse(),
            lambda item: item["rails"].remove("device_log"),
            lambda item: item["persistence"].update({"new_table": True}),
            lambda item: item["governance_boundary"]["excluded"].remove("repair"),
        )
        for mutation in mutations:
            candidate = copy.deepcopy(self.contract)
            mutation(candidate)
            with self.subTest(candidate=candidate), self.assertRaises(ContractError):
                validate_contract(candidate, self.schema)

    def test_runtime_repository_enforces_receipt_before_offset(self) -> None:
        repository = (ROOT / self.contract["implementations"]["go_repository"]).read_text(encoding="utf-8")
        reconcile = (ROOT / self.contract["implementations"]["go_reconcile"]).read_text(encoding="utf-8")
        java = (ROOT / self.contract["implementations"]["java_receipt"]).read_text(encoding="utf-8")
        self.assertLess(repository.index("r.Record(ctx, receipt)"), repository.index("commitOffset(ctx, receipt.Source)"))
        self.assertIn("ErrReceiptConflict", repository)
        self.assertIn("MissingOffsets", reconcile)
        self.assertIn("ExtraOffsets", reconcile)
        self.assertIn("BuildMissingReceipts", reconcile)
        self.assertIn("ReconcileAllRails", reconcile)
        self.assertIn("source-quality/v1\\0", java)
        self.assertIn('event.put("object_type", "source_quality_receipt")', java)


if __name__ == "__main__":
    unittest.main()
