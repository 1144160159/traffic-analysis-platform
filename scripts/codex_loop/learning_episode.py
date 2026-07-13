#!/usr/bin/env python3
"""Validate and evaluate constrained learning episodes for Codex Loop.

The evaluator is deliberately small and deterministic. It validates the
repository-owned JSON Schema without adding a runtime dependency, applies hard
gates before reward, and never mutates implementation or production state.
"""

from __future__ import annotations

import argparse
import copy
import json
from datetime import datetime
from pathlib import Path
from typing import Any

from lib import REPO_ROOT, rel_path, repo_path, sha256_file, write_json, write_text


DEFAULT_SPEC_ROOT = Path("doc/04_assets/ui_suite_gpt_v1/specs")
DEFAULT_POLICY = DEFAULT_SPEC_ROOT / "reinforcement-learning-policy.json"
DEFAULT_SCHEMA = DEFAULT_SPEC_ROOT / "learning-episode.schema.json"


class EpisodeValidationError(ValueError):
    """Raised when an episode or policy violates its frozen contract."""


def load_json(path: str | Path) -> dict[str, Any]:
    target = repo_path(path)
    data = json.loads(target.read_text(encoding="utf-8"))
    if not isinstance(data, dict):
        raise EpisodeValidationError(f"Expected JSON object: {target}")
    return data


def resolve_ref(root_schema: dict[str, Any], ref: str) -> dict[str, Any]:
    if not ref.startswith("#/"):
        raise EpisodeValidationError(f"Only local JSON Schema refs are supported: {ref}")
    current: Any = root_schema
    for part in ref[2:].split("/"):
        key = part.replace("~1", "/").replace("~0", "~")
        if not isinstance(current, dict) or key not in current:
            raise EpisodeValidationError(f"Unresolvable JSON Schema ref: {ref}")
        current = current[key]
    if not isinstance(current, dict):
        raise EpisodeValidationError(f"JSON Schema ref must resolve to an object: {ref}")
    return current


def type_matches(value: Any, expected: str) -> bool:
    if expected == "null":
        return value is None
    if expected == "boolean":
        return isinstance(value, bool)
    if expected == "number":
        return isinstance(value, (int, float)) and not isinstance(value, bool)
    if expected == "integer":
        return isinstance(value, int) and not isinstance(value, bool)
    if expected == "string":
        return isinstance(value, str)
    if expected == "array":
        return isinstance(value, list)
    if expected == "object":
        return isinstance(value, dict)
    return False


def validate_node(
    value: Any,
    schema: dict[str, Any],
    root_schema: dict[str, Any],
    path: str = "$",
) -> list[str]:
    if "$ref" in schema:
        return validate_node(value, resolve_ref(root_schema, str(schema["$ref"])), root_schema, path)

    errors: list[str] = []
    expected_type = schema.get("type")
    if expected_type:
        expected_types = expected_type if isinstance(expected_type, list) else [expected_type]
        if not any(type_matches(value, str(item)) for item in expected_types):
            return [f"{path}: expected type {expected_types}, got {type(value).__name__}"]

    if "const" in schema and value != schema["const"]:
        errors.append(f"{path}: expected constant {schema['const']!r}")
    if "enum" in schema and value not in schema["enum"]:
        errors.append(f"{path}: value {value!r} is not in {schema['enum']!r}")

    if isinstance(value, str):
        if len(value) < int(schema.get("minLength", 0)):
            errors.append(f"{path}: string is shorter than minLength")

    if isinstance(value, (int, float)) and not isinstance(value, bool):
        if "minimum" in schema and value < schema["minimum"]:
            errors.append(f"{path}: value {value} is below minimum {schema['minimum']}")
        if "maximum" in schema and value > schema["maximum"]:
            errors.append(f"{path}: value {value} is above maximum {schema['maximum']}")

    if isinstance(value, list):
        if len(value) < int(schema.get("minItems", 0)):
            errors.append(f"{path}: array is shorter than minItems")
        if "maxItems" in schema and len(value) > int(schema["maxItems"]):
            errors.append(f"{path}: array is longer than maxItems")
        item_schema = schema.get("items")
        if isinstance(item_schema, dict):
            for index, item in enumerate(value):
                errors.extend(validate_node(item, item_schema, root_schema, f"{path}[{index}]"))

    if isinstance(value, dict):
        required = schema.get("required") or []
        for key in required:
            if key not in value:
                errors.append(f"{path}: missing required property {key!r}")
        properties = schema.get("properties") or {}
        if schema.get("additionalProperties") is False:
            extras = sorted(set(value) - set(properties))
            for key in extras:
                errors.append(f"{path}: unexpected property {key!r}")
        for key, child_schema in properties.items():
            if key in value and isinstance(child_schema, dict):
                errors.extend(validate_node(value[key], child_schema, root_schema, f"{path}.{key}"))

    return errors


def validate_policy(policy: dict[str, Any]) -> None:
    weights = ((policy.get("reward") or {}).get("weights") or {})
    required_dimensions = {"business", "function", "quality", "visual", "performance", "maintainability"}
    if set(weights) != required_dimensions:
        raise EpisodeValidationError("Policy reward dimensions do not match the episode contract")
    total = sum(float(value) for value in weights.values())
    if abs(total - 1.0) > 1e-9:
        raise EpisodeValidationError(f"Policy reward weights must sum to 1, got {total}")
    hard_gates = policy.get("hardGates") or []
    if len(hard_gates) != len(set(hard_gates)) or not hard_gates:
        raise EpisodeValidationError("Policy hard gates must be non-empty and unique")


def validate_episode(episode: dict[str, Any], schema: dict[str, Any], policy: dict[str, Any]) -> None:
    errors = validate_node(episode, schema, schema)
    if errors:
        raise EpisodeValidationError("Episode schema validation failed:\n- " + "\n- ".join(errors))
    if episode["versions"]["policy"] != policy["version"]:
        raise EpisodeValidationError(
            f"Episode policy version {episode['versions']['policy']!r} does not match {policy['version']!r}"
        )
    gate_ids = [str(item["id"]) for item in episode["gates"]["results"]]
    if len(gate_ids) != len(set(gate_ids)):
        raise EpisodeValidationError("Episode gate IDs must be unique")
    unknown_gates = sorted(set(gate_ids) - set(policy["hardGates"]))
    if unknown_gates:
        raise EpisodeValidationError(f"Episode contains unknown hard gates: {unknown_gates}")
    validate_evidence_integrity(episode["evidence"])


def validate_evidence_integrity(evidence: list[dict[str, Any]]) -> None:
    for index, item in enumerate(evidence):
        raw_path = Path(str(item["path"]))
        if raw_path.is_absolute():
            raise EpisodeValidationError(f"Evidence path must be repository-relative at index {index}")
        target = repo_path(raw_path).resolve()
        try:
            target.relative_to(REPO_ROOT)
        except ValueError as exc:
            raise EpisodeValidationError(f"Evidence path escapes repository at index {index}: {raw_path}") from exc
        if not target.is_file():
            raise EpisodeValidationError(f"Evidence file does not exist at index {index}: {raw_path}")
        expected = str(item["checksum"]).lower()
        if len(expected) != 64 or any(character not in "0123456789abcdef" for character in expected):
            raise EpisodeValidationError(f"Evidence checksum must be a SHA-256 hex digest at index {index}")
        actual = sha256_file(target)
        if actual != expected:
            raise EpisodeValidationError(
                f"Evidence checksum mismatch at index {index}: {raw_path} expected={expected} actual={actual}"
            )


def evaluate_episode(episode: dict[str, Any], policy: dict[str, Any]) -> dict[str, Any]:
    evaluated = copy.deepcopy(episode)
    gate_rows = evaluated["gates"]["results"]
    gate_by_id = {str(item["id"]): item for item in gate_rows}
    required_gate_ids = [str(item) for item in policy["hardGates"]]
    missing_gates = [gate_id for gate_id in required_gate_ids if gate_id not in gate_by_id]
    non_pass_gates = [
        gate_id
        for gate_id in required_gate_ids
        if gate_id in gate_by_id and gate_by_id[gate_id]["status"] != "pass"
    ]
    hard_gates_passed = not missing_gates and not non_pass_gates and bool(evaluated["gates"]["all_passed"])

    dimensions = evaluated["reward"]["dimensions"]
    weights = policy["reward"]["weights"]
    penalties = evaluated["reward"]["penalties"]
    weighted = sum(float(dimensions[key]) * float(weights[key]) for key in weights)
    penalty_total = sum(float(penalties[key]) for key in policy["reward"]["penalties"])
    total = max(0.0, min(100.0, weighted - penalty_total)) if hard_gates_passed else None

    evaluated["reward"]["eligible"] = hard_gates_passed
    evaluated["reward"]["total"] = round(total, 4) if total is not None else None
    evaluated["gates"]["all_passed"] = hard_gates_passed

    critic_verdicts = [str(item["verdict"]) for item in evaluated["critics"].values()]
    main_decision = evaluated["main_thread_decision"]["decision"]
    production = evaluated["production"]
    positive = (
        hard_gates_passed
        and main_decision == "accept"
        and production["status"] == "stable"
        and production["rollback_verified"] is True
        and all(verdict == "pass" for verdict in critic_verdicts)
        and bool(evaluated["evidence"])
    )
    negative = bool(non_pass_gates) or main_decision in {"repair", "rollback"} or "fail" in critic_verdicts
    evaluated["learning_status"] = "positive" if positive else "negative" if negative else "neutral"
    evaluated["evaluation"] = {
        "evaluated_at": datetime.now().isoformat(timespec="seconds"),
        "policy_version": policy["version"],
        "hard_gates_passed": hard_gates_passed,
        "missing_hard_gates": missing_gates,
        "non_pass_hard_gates": non_pass_gates,
        "weighted_reward_before_penalties": round(weighted, 4),
        "penalty_total": round(penalty_total, 4),
    }
    return evaluated


def render_report(episode: dict[str, Any]) -> str:
    evaluation = episode.get("evaluation") or {}
    lines = [
        "# Learning Episode Evaluation",
        "",
        f"- run_id: `{episode['run_id']}`",
        f"- task_id: `{episode['task_id']}`",
        f"- subject: `{episode['subject']['type']}:{episode['subject']['id']}`",
        f"- hard_gates_passed: `{evaluation.get('hard_gates_passed')}`",
        f"- reward_eligible: `{episode['reward']['eligible']}`",
        f"- reward_total: `{episode['reward']['total']}`",
        f"- learning_status: `{episode['learning_status']}`",
        f"- main_thread_decision: `{episode['main_thread_decision']['decision']}`",
        "",
        "## Hard Gates",
    ]
    for item in episode["gates"]["results"]:
        lines.append(f"- `{item['id']}`: `{item['status']}`")
    lines.extend(["", "## Reward Dimensions"])
    for key, value in episode["reward"]["dimensions"].items():
        lines.append(f"- `{key}`: `{value}`")
    lines.extend(
        [
            "",
            "## Guardrail",
            "- A reward is computed only after every frozen hard gate passes.",
            "- This report does not deploy, mutate task status, or authorize production changes.",
            "",
        ]
    )
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--episode", required=True)
    parser.add_argument("--policy", default=str(DEFAULT_POLICY))
    parser.add_argument("--schema", default=str(DEFAULT_SCHEMA))
    parser.add_argument("--output-dir", default=None)
    parser.add_argument("--validate-only", action="store_true")
    args = parser.parse_args()

    policy = load_json(args.policy)
    schema = load_json(args.schema)
    episode = load_json(args.episode)
    validate_policy(policy)
    validate_episode(episode, schema, policy)

    if args.validate_only:
        print(f"episode-valid run_id={episode['run_id']} policy={policy['version']}")
        return 0

    evaluated = evaluate_episode(episode, policy)
    if args.output_dir:
        output_dir = repo_path(args.output_dir)
    else:
        output_dir = repo_path(args.episode).parent / "evaluation"
    output_dir.mkdir(parents=True, exist_ok=True)
    json_path = write_json(output_dir / "episode.evaluated.json", evaluated)
    report_path = write_text(output_dir / "episode-report.md", render_report(evaluated))
    print(
        f"episode-evaluated status={evaluated['learning_status']} "
        f"reward={evaluated['reward']['total']} output={rel_path(json_path)} report={rel_path(report_path)}"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
