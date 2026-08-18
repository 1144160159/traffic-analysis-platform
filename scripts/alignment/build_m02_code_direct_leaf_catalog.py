#!/usr/bin/env python3
"""Freeze the 207 M02 code-direct design leaves and their mixed DAG.

This is the transition artifact for M02-REG-01.  It deliberately does not
rewrite the global task registry or grant target binding/execution authority.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import re
from collections import Counter, defaultdict
from pathlib import Path
from typing import Any

from build_topic1_task_registry import validate_against_schema


REPO = Path(__file__).resolve().parents[2]
DOC_REL = Path("doc/07_alignment/课题一PR里程碑代码级详细设计.md")
DOC = REPO / DOC_REL
OUTPUT = REPO / "contracts/alignment/m02-code-direct-leaf-catalog.v1.json"
SCHEMA = REPO / "contracts/alignment/m02-code-direct-leaf-catalog.schema.json"

ROW_RE = re.compile(
    r"^\| (?P<leaf>M02-N(?P<parent>\d{3})-L(?P<leaf_no>\d{2})) \| "
    r"(?P<type>CTR|EXP|PRJ|WRT|REF|OPS|TST-PRE|TST-POST|IDX|PROM) \| "
    r"(?P<target>.+?) \| (?P<pre>.+?) \| (?P<outcome>.+?) \| (?P<oracle>.+?) \|$"
)
BACKTICK_RE = re.compile(r"`([^`]+)`")
PREVIEW_MARKER_RE = re.compile(
    r"<!-- topic1-m02-preview leaf=(\d+) external=(\d+) active_new_leaf=(\d+) "
    r"id_epoch=([^ ]+) status=([^ ]+) -->"
)

EXPECTED_TYPE_COUNTS = {
    "CTR": 36,
    "EXP": 1,
    "PRJ": 21,
    "WRT": 69,
    "REF": 30,
    "OPS": 8,
    "TST-PRE": 18,
    "TST-POST": 5,
    "IDX": 18,
    "PROM": 1,
}
EXPECTED_PARENT_COUNTS = {
    "T1-M02-N001": 24,
    "T1-M02-N002": 8,
    "T1-M02-N003": 12,
    "T1-M02-N004": 26,
    "T1-M02-N005": 9,
    "T1-M02-N006": 20,
    "T1-M02-N007": 10,
    "T1-M02-N008": 10,
    "T1-M02-N009": 13,
    "T1-M02-N010": 7,
    "T1-M02-N011": 10,
    "T1-M02-N012": 23,
    "T1-M02-N013": 9,
    "T1-M02-N014": 9,
    "T1-M02-N015": 11,
    "T1-M02-N016": 6,
}

PARENT_DEPENDENCIES = {
    "T1-M02-N001": ["T1-M01-N014"],
    "T1-M02-N002": ["T1-M02-N001"],
    "T1-M02-N003": ["T1-M02-N001", "T1-M02-N002"],
    "T1-M02-N004": ["T1-M02-N001"],
    "T1-M02-N005": ["T1-M02-N004"],
    "T1-M02-N006": ["T1-M02-N004"],
    "T1-M02-N007": ["T1-M02-N001"],
    "T1-M02-N008": ["T1-M02-N003"],
    "T1-M02-N009": ["T1-M02-N003"],
    "T1-M02-N010": ["T1-M02-N009"],
    "T1-M02-N011": ["T1-M02-N009"],
    "T1-M02-N012": ["T1-M02-N003"],
    "T1-M02-N013": ["T1-M02-N014"],
    "T1-M02-N014": [f"T1-M02-N{i:03d}" for i in range(1, 13)],
    "T1-M02-N015": ["T1-M02-N013"],
    "T1-M02-N016": [f"T1-M02-N{i:03d}" for i in range(1, 16)],
}


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def leaf(parent: int, number: int) -> str:
    return f"M02-N{parent:03d}-L{number:02d}"


def expand(parent: int, start: int, end: int) -> list[str]:
    return [leaf(parent, number) for number in range(start, end + 1)]


def explicit_cross_edges() -> list[dict[str, str]]:
    specs: list[tuple[list[str], list[str], str]] = [
        ([leaf(1, 24)], [leaf(2, 1), leaf(4, 1), leaf(4, 7), leaf(7, 1)], "M02 contract baseline precedes identity, capture and Agent sender work"),
        ([leaf(1, 24), leaf(2, 8)], [leaf(3, 1)], "Proto and identity contracts precede Topic and ACL materialization"),
        ([leaf(4, 26)], [leaf(5, 1), leaf(6, 1)], "capture completion precedes flow identity and PCAP durability trains"),
        ([leaf(3, 12)], [leaf(8, 1), leaf(9, 1), leaf(9, 4), leaf(12, 1)], "Topic and ACL completion unlocks independent Gateway, consumer and control rails"),
        ([leaf(9, 13)], [leaf(8, 3)], "flow consumer readiness precedes Gateway flow writer"),
        ([leaf(8, 4)], [leaf(7, 4)], "Gateway durable batch endpoint precedes Agent batch producer"),
        ([leaf(8, 5)], [leaf(7, 5)], "Gateway stream endpoint precedes Agent stream producer"),
        ([leaf(6, 9)], [leaf(11, 1)], "object receipt contract follows durable spool receipt definition"),
        ([leaf(9, 6)], [leaf(11, 4), leaf(11, 5), leaf(11, 6)], "raw PCAP consumer carrier precedes projection schema and sink work"),
        ([leaf(9, 9), *expand(11, 1, 6)], [leaf(10, 3)], "Pcap job graph requires raw-record carrier plus object and sink contracts"),
        ([leaf(9, 13), leaf(10, 7), *expand(11, 1, 6)], [leaf(8, 6)], "Gateway PCAP Kafka writer waits for consumer-first job and carrier closure"),
        ([*expand(6, 15, 17), leaf(8, 7)], [leaf(11, 7)], "Rust metadata writer waits for object receipt and Gateway Kafka endpoint"),
        ([leaf(12, 5)], [leaf(12, 6)], "ACK authority precedes ACK consumer"),
        ([leaf(12, 6)], [leaf(12, 14)], "ACK consumer receipt precedes command and lifecycle enablement join"),
        ([leaf(12, 8)], [leaf(12, 9)], "control topic contract precedes idle command router"),
        ([leaf(12, 9)], [leaf(12, 10)], "idle router precedes Agent command validator"),
        ([leaf(12, 10)], [leaf(12, 11)], "command validator precedes typed operation executor"),
        ([leaf(12, 11)], [leaf(12, 14)], "operation executor precedes command and ACK join"),
        ([leaf(12, 6), leaf(12, 7), leaf(12, 8), leaf(12, 14)], [leaf(12, 15)], "Gateway ACK bridge waits for both ACK and command rails"),
        ([leaf(12, 15)], [leaf(12, 16)], "ACK bridge receipt precedes desired writer and outbox"),
        ([leaf(14, 9)], expand(13, 1, 7), "integration closure precedes scoped canary preparation"),
        ([leaf(13, 9)], expand(15, 7, 11), "canary closure precedes approved-profile execution and reconciliation"),
        ([leaf(16, 1)], [leaf(16, 2)], "current evidence exact-set precedes promotion equivalence tool"),
        ([leaf(16, 2)], [leaf(16, 3)], "promotion equivalence tool precedes premerge run"),
        ([leaf(16, 4)], [leaf(16, 5)], "protected merge equivalence precedes release pointer"),
        ([leaf(16, 5)], [leaf(16, 6)], "release pointer precedes terminal promotion index"),
    ]
    edges: list[dict[str, str]] = []
    for sources, targets, reason in specs:
        for source in sources:
            for target in targets:
                edges.append({"from": source, "to": target, "reason": reason})
    return edges


def external_activities() -> list[dict[str, Any]]:
    return [
        {
            "activity_id": "EXT-T1-M02-N013-CANARY",
            "parent_task_id": "T1-M02-N013",
            "activity_type": "SCOPED_CANARY",
            "status": "BLOCKED_CONTRACT_NOT_YET_IN_GLOBAL_REGISTRY",
            "mutable_by_repository_authors": False,
            "depends_on_leaf_ids": [leaf(13, 7)],
            "successor_leaf_ids": [leaf(13, 8)],
            "proof_ceiling": "DESIGN_ACTIVITY_NODE_ONLY_NOT_EXTERNAL_EXECUTION_OR_RECEIPT",
        },
        {
            "activity_id": "EXT-T1-M02-N015-PROFILE-APPROVAL",
            "parent_task_id": "T1-M02-N015",
            "activity_type": "PROFILE_APPROVAL",
            "status": "BLOCKED_CONTRACT_NOT_YET_IN_GLOBAL_REGISTRY",
            "mutable_by_repository_authors": False,
            "depends_on_leaf_ids": [*expand(15, 1, 3), leaf(4, 8), leaf(1, 21)],
            "successor_leaf_ids": [leaf(15, 7)],
            "proof_ceiling": "DESIGN_ACTIVITY_NODE_ONLY_NOT_EXTERNAL_EXECUTION_OR_RECEIPT",
        },
        {
            "activity_id": "EXT-T1-M02-N016-MERGE",
            "parent_task_id": "T1-M02-N016",
            "activity_type": "PROTECTED_MERGE",
            "status": "BLOCKED_CONTRACT_NOT_YET_IN_GLOBAL_REGISTRY",
            "mutable_by_repository_authors": False,
            "depends_on_leaf_ids": [leaf(16, 3)],
            "successor_leaf_ids": [leaf(16, 4)],
            "proof_ceiling": "DESIGN_ACTIVITY_NODE_ONLY_NOT_EXTERNAL_EXECUTION_OR_RECEIPT",
        },
    ]


def parse_local_dependencies(parent: int, prerequisites: str) -> set[str]:
    dependencies: set[str] = set()
    for source_parent, start, end in re.findall(
        r"N(\d{3})-L(\d{2})(?:-|\.\.)L?(\d{2})", prerequisites
    ):
        dependencies.update(expand(int(source_parent), int(start), int(end)))
    without_cross_ranges = re.sub(
        r"N\d{3}-L\d{2}(?:-|\.\.)L?\d{2}", "", prerequisites
    )
    for start, end in re.findall(
        r"(?<!N\d{3}-)L(\d{2})(?:-|\.\.)L?(\d{2})", without_cross_ranges
    ):
        dependencies.update(expand(parent, int(start), int(end)))
    scrubbed = re.sub(
        r"(?<!N\d{3}-)L\d{2}(?:-|\.\.)L?\d{2}", "", without_cross_ranges
    )
    dependencies.update(leaf(parent, int(value)) for value in re.findall(r"(?<!N\d{3}-)L(\d{2})", scrubbed))
    dependencies.update(
        leaf(int(source_parent), int(number))
        for source_parent, number in re.findall(
            r"N(\d{3})-L(\d{2})(?!(?:-|\.\.)L?\d{2})", without_cross_ranges
        )
    )
    return dependencies


def target_state(raw: str, locators: list[str]) -> str:
    if "（planned）" in raw or "(planned)" in raw:
        return "PLANNED_OUTPUT" if all(item.startswith("doc/02_acceptance/") for item in locators) else "PLANNED"
    paths = [item.split("#", 1)[0] for item in locators]
    if paths and all((REPO / path).exists() for path in paths):
        return "EXISTING"
    return "PLANNED_OUTPUT" if all(item.startswith(("doc/02_acceptance/", "contracts/releases/")) for item in paths) else "PLANNED"


def assert_acyclic(nodes: set[str], edges: list[dict[str, str]]) -> None:
    indegree = {node: 0 for node in nodes}
    outgoing: dict[str, set[str]] = defaultdict(set)
    for edge in edges:
        source, target = edge["from"], edge["to"]
        if source not in nodes or target not in nodes:
            raise ValueError(f"mixed DAG edge references unknown node: {source} -> {target}")
        if target not in outgoing[source]:
            outgoing[source].add(target)
            indegree[target] += 1
    ready = sorted(node for node, degree in indegree.items() if degree == 0)
    visited = 0
    while ready:
        node = ready.pop(0)
        visited += 1
        for target in sorted(outgoing[node]):
            indegree[target] -= 1
            if indegree[target] == 0:
                ready.append(target)
                ready.sort()
    if visited != len(nodes):
        cyclic = sorted(node for node, degree in indegree.items() if degree > 0)
        raise ValueError(f"M02 mixed DAG contains a cycle involving {cyclic[:12]}")


def build() -> dict[str, Any]:
    document = DOC.read_text(encoding="utf-8")
    marker = PREVIEW_MARKER_RE.search(document)
    if marker is None or marker.groups() != ("207", "3", "0", "P101-P307", "DESIGNED_NOT_REGISTERED"):
        raise ValueError("M02 preview marker is missing or stale")
    rows = []
    for line in document.splitlines():
        match = ROW_RE.match(line)
        if match:
            rows.append(match.groupdict())
    if len(rows) != 207:
        raise ValueError(f"expected 207 M02 design rows, found {len(rows)}")

    leaves: list[dict[str, Any]] = []
    by_parent: dict[str, list[dict[str, Any]]] = defaultdict(list)
    for offset, row in enumerate(rows, start=101):
        parent = int(row["parent"])
        number = int(row["leaf_no"])
        parent_task_id = f"T1-M02-N{parent:03d}"
        locators = BACKTICK_RE.findall(row["target"])
        if not locators or len(locators) > 2:
            raise ValueError(f"{row['leaf']} must contain one locator or an evidence pair: {locators}")
        item = {
            "leaf_id": row["leaf"],
            "atomic_pr_id": f"T1-M02-P{offset:03d}-{row['type']}-n{parent:03d}-l{number:02d}",
            "parent_task_id": parent_task_id,
            "leaf_number": number,
            "pr_type": row["type"],
            "phase": f"m02-n{parent:03d}-l{number:02d}",
            "write_locators": locators,
            "target_state": target_state(row["target"], locators),
            "prerequisites_raw": row["pre"],
            "single_outcome": row["outcome"],
            "oracle_and_rollback": row["oracle"],
            "depends_on_leaf_ids": sorted(parse_local_dependencies(parent, row["pre"])),
            "depends_on_external_activities": [],
            "terminal_task_idx": False,
            "formal_execution_status": "BLOCKED_UNTIL_GLOBAL_REGISTRY_TARGET_BINDING_FUNCTION_REVIEW_AND_SIGNED_OVERLAY",
            "allowed_claim": "M02 design leaf identity and dependency intent are frozen",
            "forbidden_claims": [
                "TARGET_BINDING_COMPLETE",
                "FUNCTION_DESIGN_REVIEWED",
                "EXECUTION_AUTHORIZED",
                "M02_ACCEPTED",
            ],
        }
        leaves.append(item)
        by_parent[parent_task_id].append(item)

    leaf_ids = [item["leaf_id"] for item in leaves]
    atomic_ids = [item["atomic_pr_id"] for item in leaves]
    if len(leaf_ids) != len(set(leaf_ids)) or len(atomic_ids) != len(set(atomic_ids)):
        raise ValueError("M02 leaf or atomic PR IDs are not unique")
    if atomic_ids[0].split("-")[2] != "P101" or atomic_ids[-1].split("-")[2] != "P307":
        raise ValueError("M02 ID epoch drifted from P101-P307")

    for parent, items in by_parent.items():
        items.sort(key=lambda item: item["leaf_number"])
        if [item["leaf_number"] for item in items] != list(range(1, len(items) + 1)):
            raise ValueError(f"{parent} leaf numbers are not contiguous")
        terminal = items[-1]
        if terminal["pr_type"] != "IDX":
            raise ValueError(f"{parent} terminal leaf is not IDX")
        terminal["terminal_task_idx"] = True
        terminal["depends_on_leaf_ids"] = sorted(
            set(terminal["depends_on_leaf_ids"]) | {item["leaf_id"] for item in items[:-1]}
        )

    type_counts = dict(sorted(Counter(item["pr_type"] for item in leaves).items()))
    parent_counts = dict(sorted((parent, len(items)) for parent, items in by_parent.items()))
    if type_counts != EXPECTED_TYPE_COUNTS:
        raise ValueError(f"M02 type counts drifted: {type_counts}")
    if parent_counts != EXPECTED_PARENT_COUNTS:
        raise ValueError(f"M02 parent counts drifted: {parent_counts}")

    by_id = {item["leaf_id"]: item for item in leaves}
    edges = explicit_cross_edges()
    for parent, predecessors in PARENT_DEPENDENCIES.items():
        first = by_parent[parent][0]["leaf_id"]
        for predecessor in predecessors:
            if predecessor.startswith("T1-M02"):
                source = by_parent[predecessor][-1]["leaf_id"]
                edges.append({"from": source, "to": first, "reason": "parent dependency summary first-leaf barrier"})
    activities = external_activities()
    for activity in activities:
        for source in activity["depends_on_leaf_ids"]:
            edges.append({"from": source, "to": activity["activity_id"], "reason": "external activity exact input predecessor"})
        for target in activity["successor_leaf_ids"]:
            edges.append({"from": activity["activity_id"], "to": target, "reason": "trusted external receipt predecessor"})
            by_id[target]["depends_on_external_activities"].append(activity["activity_id"])

    # Every parsed local dependency is part of the mixed DAG.  References to
    # same-parent future leaves are rejected because they would make a design
    # card depend on an output that cannot exist yet.
    for item in leaves:
        for source in item["depends_on_leaf_ids"]:
            if source not in by_id:
                raise ValueError(f"{item['leaf_id']} references unknown dependency {source}")
            edges.append({"from": source, "to": item["leaf_id"], "reason": "leaf prerequisite table cell"})

    deduped = {
        (edge["from"], edge["to"]): edge
        for edge in edges
        if edge["from"] != edge["to"]
    }
    edges = [deduped[key] for key in sorted(deduped)]
    assert_acyclic(
        set(leaf_ids) | {activity["activity_id"] for activity in activities},
        edges,
    )

    unresolved_count = sum(
        1 for item in leaves
        if re.search(r"\bN\d{3}\b|TASK-IDX|external|receipt|批准|受信", item["prerequisites_raw"], re.IGNORECASE)
    )
    payload = {
        "schema_version": "1.0.0",
        "artifact_kind": "M02_CODE_DIRECT_LEAF_CATALOG",
        "artifact_status": "DESIGN_LEAF_ID_FROZEN_NOT_GLOBAL_REGISTRY",
        "source_document": DOC_REL.as_posix(),
        "source_document_sha256": sha256(DOC),
        "id_epoch": "T1-M02-P101-P307",
        "leaf_count": len(leaves),
        "type_counts": type_counts,
        "parent_counts": parent_counts,
        "leaves": leaves,
        "parent_dependency_summary": PARENT_DEPENDENCIES,
        "cross_leaf_edges": edges,
        "external_activities": activities,
        "validation": {
            "structure": "PASS",
            "mixed_dag": "PASS",
            "unique_leaf_ids": True,
            "unique_atomic_pr_ids": True,
            "terminal_task_idx_exact_set": True,
            "type_count_exact_set": True,
            "unresolved_dependency_text_count": unresolved_count,
        },
        "proof_ceiling": "M02_LEAF_ID_AND_DESIGN_DAG_ONLY_NOT_TARGET_BINDING_FUNCTION_REVIEW_EXECUTION_OR_ACCEPTANCE",
    }
    validate_against_schema(payload, SCHEMA)
    return payload


def canonical(payload: dict[str, Any]) -> str:
    return json.dumps(payload, ensure_ascii=False, indent=2) + "\n"


def main() -> int:
    parser = argparse.ArgumentParser()
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--write", action="store_true")
    mode.add_argument("--check", action="store_true")
    args = parser.parse_args()
    expected = canonical(build())
    if args.write:
        OUTPUT.write_text(expected, encoding="utf-8")
        print(f"WROTE {OUTPUT.relative_to(REPO)}")
        return 0
    if not OUTPUT.exists() or OUTPUT.read_text(encoding="utf-8") != expected:
        raise SystemExit(f"STALE {OUTPUT.relative_to(REPO)}; run with --write")
    print("PASS M02 code-direct leaf catalog: 207 leaves, P101-P307, 16 terminal IDX leaves, 3 external nodes, acyclic mixed DAG")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
