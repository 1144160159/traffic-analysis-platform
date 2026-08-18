#!/usr/bin/env python3
"""Generate the design-application row for every Topic One atomic PR.

The output records where pattern application is relevant, where it is exempt,
and where trusted target binding must happen first.  It never fabricates a
function signature and never grants implementation or execution authority.
"""

from __future__ import annotations

import argparse
import hashlib
import json
from collections import Counter
from pathlib import Path
from typing import Any

from build_topic1_task_registry import validate_against_schema


REPO_ROOT = Path(__file__).resolve().parents[2]
SOURCE = REPO_ROOT / "contracts/alignment/developer-claim-package-catalog.v1.json"
OUTPUT = REPO_ROOT / "contracts/alignment/pr-design-application-catalog.v1.json"
SCHEMA = REPO_ROOT / "contracts/alignment/pr-design-application-catalog.schema.json"
DOC_OUTPUT = REPO_ROOT / "doc/07_alignment/generated/PR模式应用与函数展开目录.md"

SOURCE_LOCATORS = {"go_symbol", "rust_symbol", "java_symbol", "ts_symbol", "python_symbol"}
SPECIALIZED_APPLICATIONS = {
    "T1-M06-P007-WRT-n004-s1": {
        "selected_form": "DIRECT",
        "application_stage": "SPECIALIZED_PROPOSAL_PENDING_DEBATE",
        "mode_decision": "PROPOSE_DIRECT_WITH_DISTRIBUTED_CONSTRAINT",
        "non_binding_pattern_hints": ["PROJECT-TRANSACTIONAL-OUTBOX"],
        "function_expansion_status": "CANDIDATE_BOUND_CODE_UNIT_DRAFT",
        "next_design_action": "完成DIRECT+事务outbox方案的多角色答辩、最终ADR和函数审查；Command候选已因缺少真实Invoker与多态变化轴而拒绝。",
        "blockers": ["pattern debate and final ADR missing", "function review receipt missing"],
        "application_plan_patch": {
            "direct_baseline": "直接保留1个事务协调方法、0个新多态接口、0个新增调用跳；仅使用既有PG事务与outbox表，不新增锁、序列化格式或运行时分配类别。",
            "rejection_proof": "GoF Command至少增加Invoker/Command合同和一次间接调用，却没有第二ConcreteCommand、运行时替换、排队或undo变化轴；因此DIRECT+事务outbox成本更低且失败语义更显式。",
            "removal_rule": "outbox由独立可靠发布机制取代时移除PROJECT-TRANSACTIONAL-OUTBOX约束；只有出现至少两个可替换命令实现和真实Invoker调度需求才重开Command。",
        },
    },
    "T1-M06-P902-WRT-n004-http-commit-unknown-mapping": {
        "selected_form": "DIRECT",
        "application_stage": "DIRECT_FUNCTION_DESIGN_CANDIDATE",
        "mode_decision": "DIRECT_TYPED_HTTP_ERROR_MAPPING",
        "non_binding_pattern_hints": [],
        "function_expansion_status": "CANDIDATE_BOUND_PRIMARY_AST",
        "next_design_action": "按冻结的503安全envelope实现H01-H08，并由P909/P910的exact run+pass门验证零泄漏和同key恢复动作。",
        "blockers": ["function review receipt missing", "signed execution overlay missing"],
        "application_plan_patch": {
            "direct_baseline": "在既有HTTP handler增加1个typed error分支；0新文件、0新类型、0新调用跳、0额外锁/分配/序列化格式。",
            "rejection_proof": "Facade/Command/State都会为单一transport映射增加类型与跳转，却没有第二实现或生命周期状态机；直接分支已完整表达503安全envelope和恢复动作。",
            "removal_rule": "typed commit-unknown合同退役时删除该分支；若三个以上transport共享同一稳定映射，再独立研判共享adapter。",
        },
    },
    "T1-M06-P903-WRT-n004-grpc-commit-unknown-mapping": {
        "selected_form": "DIRECT",
        "application_stage": "DIRECT_FUNCTION_DESIGN_CANDIDATE",
        "mode_decision": "DIRECT_TYPED_GRPC_ERROR_MAPPING",
        "non_binding_pattern_hints": [],
        "function_expansion_status": "CANDIDATE_BOUND_PRIMARY_AST",
        "next_design_action": "按冻结的codes.Unavailable和安全消息实现G01-G08，并由P911/P912的exact run+pass门验证。",
        "blockers": ["function review receipt missing", "signed execution overlay missing"],
        "application_plan_patch": {
            "direct_baseline": "在既有gRPC handler增加1个typed error分支；0新文件、0新类型、0新调用跳、0额外锁/分配，复用标准status编码。",
            "rejection_proof": "Strategy/Command对单一codes.Unavailable映射没有替换轴，只增加接口和认知成本；直接映射能冻结message、metadata和same-key恢复。",
            "removal_rule": "typed ambiguity合同退役时删除分支；出现多个可替换transport error policy且需运行时选择时才重开Strategy。",
        },
    },
    "T1-M06-P904-WRT-n004-asset-event-topic-rail": {
        "selected_form": "DIRECT",
        "application_stage": "DIRECT_FUNCTION_DESIGN_CANDIDATE",
        "mode_decision": "DIRECT_STARTUP_PREDICATE",
        "non_binding_pattern_hints": [],
        "function_expansion_status": "CANDIDATE_BOUND_PRIMARY_AST",
        "next_design_action": "按Kafka.Enabled/EventOutboxEnabled/ProjectionEnabled精确谓词实现C01-C06，并由P913/P914 exact run+pass门验证。",
        "blockers": ["function review receipt missing", "signed execution overlay missing"],
        "application_plan_patch": {
            "direct_baseline": "在既有Config.validate增加1组纯布尔predicate和3条稳定错误；0新文件/类型/调用跳/分配/锁/序列化。",
            "rejection_proof": "State/Chain of Responsibility对四种启动组合没有运行态迁移或动态责任链，只会分散一个可穷尽的fail-closed谓词。",
            "removal_rule": "rail catalog版本化后由catalog validator替换本地常量谓词；在此之前保持直接表驱动验证。",
        },
    },
    "T1-M06-P917-IDX-n004-source-precedence-approval": {
        "selected_form": "NOT_APPLICABLE",
        "application_stage": "SPECIALIZED_APPROVAL_CONTRACT_DEFINED",
        "mode_decision": "DOMAIN_QUORUM_INDEX_NOT_OBJECT_COLLABORATION",
        "non_binding_pattern_hints": [],
        "function_expansion_status": "CANDIDATE_BOUND_APPROVAL_INDEX_PENDING_TRUST",
        "next_design_action": "生成同candidate/profile/environment的P916 typed result和P917 current-index，加载来源合同、两张不同owner签名receipt并逐张调用M01受保护验签器；受保护验签器未安装时保持BLOCKED。",
        "blockers": ["candidate-bound approval current-index missing", "two trusted owner signature receipts missing", "protected M01 verifier unavailable"],
    },
    "T1-M06-P918-REF-n004-asset-authority-live-reconcile-runner": {
        "selected_form": "DIRECT",
        "application_stage": "SPECIALIZED_RECONCILIATION_FUNCTION_DESIGN",
        "mode_decision": "DIRECT_TYPED_RECONCILIATION",
        "non_binding_pattern_hints": [],
        "function_expansion_status": "PLANNED_EXACT_PYTHON_FUNCTION",
        "next_design_action": "按R01-R14实现reconcile_receipts、write_reconciliation_outputs、main和测试main，运行positive+10个恶意负例；REF只关闭实现，不宣称G2/G3。",
        "blockers": ["planned function implementation missing", "signed execution overlay missing"],
        "application_plan_patch": {
            "direct_baseline": "使用3个明确函数：1个pure reconcile、1个immutable writer、1个CLI shell；1个生产脚本+1个测试脚本，函数间2次显式调用，0类/接口/锁，规范JSON仅用于final-fact hash。",
            "rejection_proof": "Template Method/Observer没有继承或一对多通知轴，Strategy没有第二对账实现；直接functional-core/imperative-shell以更少类型覆盖同一失败矩阵。",
            "removal_rule": "runner退役时整叶删除；出现至少两个独立、可替换且受同合同约束的reconcile算法时才引入Strategy。",
        },
    },
    "T1-M06-P919-TST-POST-n004-asset-authority-live-reconcile": {
        "selected_form": "NOT_APPLICABLE",
        "application_stage": "SPECIALIZED_G2_G3_EVIDENCE_CONTRACT_DEFINED",
        "mode_decision": "EVIDENCE_RECONCILIATION_NOT_OBJECT_COLLABORATION",
        "non_binding_pattern_hints": [],
        "function_expansion_status": "AUTHORIZED_REAL_DEPENDENCY_RUN_PENDING",
        "next_design_action": "在授权真实依赖环境运行P918 runner，输入同candidate的authority/broker/projection受信回执，产出exact G2与G3 manifests；缺一回执、验签或零差异对账均BLOCKED。",
        "blockers": ["authorized real-dependency environment missing", "trusted receipt set missing", "signed execution overlay missing"],
    },
}


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def classify(package: dict[str, Any]) -> dict[str, Any]:
    targets = package["change_targets"]
    if package["claim_mode"] == "TARGET_BINDING":
        mapping_target: dict[str, Any] | None = None
        scope = "TARGET_BINDING_ONLY"
        stage = "WAITING_TRUSTED_LOCATOR"
        selected_form = "PENDING_REVIEW"
        decision = "DEFER_UNTIL_TARGET_BINDING"
        candidates: list[str] = []
        rationale = "未建立受信exact locator；模式方案必须等待目标类型、完整签名和影响调用图冻结。"
        expansion = "BLOCKED_ON_TARGET_BINDING"
        next_action = "领取TARGET_BINDING，只解析候选目标、歧义、兼容入口和default-off guard。"
        blockers = [*package["implementation_blockers"], "trusted exact locator and code-unit kind unresolved"]
        allowed = "PR_DESIGN_CLASSIFIED"
        required_units: list[str] = []
    else:
        source_targets = [target for target in targets if target["locator_kind"] in SOURCE_LOCATORS]
        if source_targets:
            mapping_target = source_targets[0]
            scope = "FUNCTION_SET"
            stage = "APPLICATION_CANDIDATE_READY_FOR_REVIEW"
            selected_form = "PENDING_REVIEW"
            decision = "REVIEW_0_TO_3_CANDIDATES_AGAINST_DIRECT"
            # Empty is the safe default. Pattern candidates are introduced
            # only after a leaf-specific change-axis review; PR type is not a
            # pattern quota and must never fabricate participants.
            candidates = []
            rationale = "源码叶先形成DIRECT基线，再研判至多3个与唯一变化轴相关的语言原生模式候选。"
            expansion = "REQUIRES_TRUSTED_AST_EXPANSION"
            next_action = "用语言AST解析当前locator、caller/callee和完整签名，再建立PR级应用方案与逐函数合同。"
            blockers = ["pattern candidate is not adjudicated", "trusted AST locator receipt and function exact-set missing"]
            allowed = "PATTERN_APPLICATION_CANDIDATE_ONLY"
            required_units = [
                f"expand:{target['path']}#{target['symbol_or_pointer']}"
                for target in source_targets
            ]
        else:
            mapping_target = None
            scope = "NON_FUNCTION_CODE_UNIT"
            stage = "APPLICATION_EXEMPTION_RECORDED"
            selected_form = "NOT_APPLICABLE"
            decision = "EXEMPT_NON_OBJECT_COLLABORATION"
            candidates = []
            surfaces = sorted({target["surface_kind"] for target in targets})
            rationale = f"当前叶只修改{','.join(surfaces)}，不以对象协作变化为交付面；使用专用合同而不强塞GoF模式。"
            expansion = "NON_FUNCTION_CONTRACT"
            next_action = "按合同、迁移、部署、测试或证据profile展开code-unit/statement/artifact步骤与验证。"
            blockers = ["specialized non-function design contract instance not generated"]
            allowed = "NON_FUNCTION_EXEMPTION_ONLY"
            required_units = [f"{target['surface_kind']}:{target['path']}#{target['symbol_or_pointer']}" for target in targets]
    entry = {
        "atomic_pr_id": package["atomic_pr_id"],
        "parent_work_id": package["parent_work_id"],
        "milestone_id": package["milestone_id"],
        "pr_type": package["pr_type"],
        "claim_mode": package["claim_mode"],
        "formal_execution_status": package["formal_execution_status"],
        "design_scope": scope,
        "application_stage": stage,
        "selected_form": selected_form,
        "mode_decision": decision,
        "non_binding_pattern_hints": candidates,
        "application_plan": {
            "direct_baseline": (
                "以单一函数或具体类型完成相同结果，保持输入输出、错误、副作用和兼容合同显式。"
                if scope == "FUNCTION_SET" else
                "使用该制品类型的直接、版本化、可验证合同，不建立对象模式抽象。"
            ),
            "candidate_role_mapping": [
                {
                    "candidate_id": candidate_id,
                    "participant_target": mapping_target["symbol_or_pointer"],
                    "target_path": mapping_target["path"],
                    "status": "PENDING_TRUSTED_AST_EXPANSION",
                }
                for candidate_id in candidates
                if mapping_target is not None
            ],
            "cost_dimensions": ["new_files", "new_types", "call_hops", "allocations", "locks", "serialization", "cognitive_cost"],
            "rejection_proof": (
                "若直接方案以更少间接层满足同一变化轴和失败不变量，则研判必须选择DIRECT。"
                if scope == "FUNCTION_SET" else
                "本叶不改变对象协作，模式采用没有可证明收益。"
            ),
            "removal_rule": (
                "候选变化轴或第二实现消失时移除抽象并回归直接方案。"
                if scope == "FUNCTION_SET" else
                "不产生模式兼容层；仅按专用合同完成制品生命周期。"
            ),
            "review_questions": (
                ["候选是否优于DIRECT并保持失败与资源语义？", "参与者是否可绑定受信exact symbol？", "负例是否证明没有tenant、事务或生命周期越界？"]
                if scope == "FUNCTION_SET" else
                ["专用合同是否覆盖该制品的版本、兼容、验证和回滚？"]
            ),
        },
        "pattern_rationale": rationale,
        "target_locators": targets,
        "required_code_units": required_units,
        "function_expansion_status": expansion,
        "next_design_action": next_action,
        "blockers": sorted(set(blockers)),
        "allowed_claim": allowed,
        "forbidden_claims": ["FUNCTION_DESIGN_COMPLETE", "READY_BINDING", "EXECUTION_AUTHORIZED", "IMPLEMENTATION_COMPLETE", "PRODUCTION_ACCEPTED"],
    }
    specialized = SPECIALIZED_APPLICATIONS.get(package["atomic_pr_id"])
    if specialized is not None:
        entry.update({
            key: value for key, value in specialized.items()
            if key not in {"blockers", "application_plan_patch"}
        })
        entry["blockers"] = specialized["blockers"]
        entry["application_plan"]["candidate_role_mapping"] = []
        entry["application_plan"].update(specialized.get("application_plan_patch", {}))
    return entry


def build() -> dict[str, Any]:
    source = json.loads(SOURCE.read_text(encoding="utf-8"))
    entries = [classify(package) for package in source["packages"]]
    ids = [entry["atomic_pr_id"] for entry in entries]
    source_ids = [package["atomic_pr_id"] for package in source["packages"]]
    if len(entries) != source["package_count"] or ids != source_ids or len(ids) != len(set(ids)):
        raise ValueError("per-PR design application catalog must exactly cover the source claim catalog")
    scope_counts = Counter(entry["design_scope"] for entry in entries)
    stage_counts = Counter(entry["application_stage"] for entry in entries)
    milestone_counts = Counter(entry["milestone_id"] for entry in entries)
    return {
        "schema_version": "1.0.0",
        "artifact_status": "GENERATED_DESIGN_INVENTORY",
        "source_claim_catalog_sha256": sha256(SOURCE),
        "entry_count": len(entries),
        "entries": entries,
        "summary": {
            "design_scope_counts": dict(sorted(scope_counts.items())),
            "application_stage_counts": dict(sorted(stage_counts.items())),
            "milestone_counts": dict(sorted(milestone_counts.items())),
        },
        "proof_ceiling": "PR_DESIGN_CLASSIFICATION_ONLY_NOT_FUNCTION_DESIGN_OR_EXECUTION_AUTHORIZATION",
    }


def write_json(path: Path, payload: dict[str, Any]) -> None:
    path.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def render_markdown(payload: dict[str, Any]) -> str:
    lines = [
        "# 课题一逐PR模式应用与函数展开目录",
        "",
        "状态：`GENERATED_DESIGN_INVENTORY / NOT_FUNCTION_DESIGN_COMPLETE / EXECUTION_NO_GO`",
        "",
        f"本目录由当前developer claim catalog确定性生成，覆盖{payload['entry_count']}张原子PR。"
        "每行只回答先应用什么方案、是否需要模式研判、下一步展开什么；它不授权改代码。",
        "",
        "## 汇总",
        "",
        "| 范围 | 数量 | 下一设计动作 |",
        "|---|---:|---|",
    ]
    for scope, count in payload["summary"]["design_scope_counts"].items():
        action = {
            "FUNCTION_SET": "建立DIRECT基线和0–3个候选，专家研判后逐函数展开",
            "NON_FUNCTION_CODE_UNIT": "按合同/SQL/部署/测试/证据专用profile展开",
            "TARGET_BINDING_ONLY": "先完成受信locator，禁止虚构函数或模式",
        }[scope]
        lines.append(f"| `{scope}` | {count} | {action} |")
    entries_by_milestone: dict[str, list[dict[str, Any]]] = {}
    for entry in payload["entries"]:
        entries_by_milestone.setdefault(entry["milestone_id"], []).append(entry)
    for milestone in sorted(entries_by_milestone):
        entries = entries_by_milestone[milestone]
        counts = Counter(entry["design_scope"] for entry in entries)
        lines.extend([
            "",
            f"## {milestone} 里程碑清单",
            "",
            f"共{len(entries)}张：FUNCTION_SET={counts['FUNCTION_SET']}，NON_FUNCTION_CODE_UNIT={counts['NON_FUNCTION_CODE_UNIT']}，TARGET_BINDING_ONLY={counts['TARGET_BINDING_ONLY']}。",
            "",
            "- [ ] 本里程碑每张PR分类无遗漏、无重复；",
            "- [ ] TARGET_BINDING_ONLY均完成受信locator后才进入方案应用；",
            "- [ ] FUNCTION_SET均先写DIRECT基线、候选角色/成本/反证，再完成专家研判；",
            "- [ ] 研判通过的函数叶完成signature、caller/callee、body steps和step-test-oracle；",
            "- [ ] NON_FUNCTION_CODE_UNIT均完成专用合同，不虚构GoF或函数；",
            "- [ ] 所有设计状态保持与执行授权正交，TASK-IDX不得因本目录提前PASS。",
            "",
            "| Atomic PR | 父任务 | 类型 | 设计范围/阶段 | 非绑定提示/豁免 | 当前目标 | 下一步 |",
            "|---|---|---|---|---|---|---|",
        ])
        for entry in entries:
            candidates = ", ".join(entry["non_binding_pattern_hints"]) or entry["selected_form"]
            targets = "<br>".join(
                f"`{target['path']}#{target.get('symbol_or_pointer') or '<planned>'}`"
                for target in entry["target_locators"][:3]
            ) or "未绑定"
            if len(entry["target_locators"]) > 3:
                targets += f"<br>另{len(entry['target_locators']) - 3}项"
            next_action = entry["next_design_action"].replace("|", "\\|")
            lines.append(
                f"| `{entry['atomic_pr_id']}` | `{entry['parent_work_id']}` | `{entry['pr_type']}` | "
                f"`{entry['design_scope']}`<br>`{entry['application_stage']}` | {candidates} | {targets} | {next_action} |"
            )
    lines.extend([
        "",
        "## 证明边界",
        "",
        "本目录只证明逐PR设计分类和下一设计动作。`PENDING_REVIEW`不代表模式已采用；"
        "`FUNCTION_SET`不代表函数已解析；任何行均不产生`READY_BINDING`、执行授权、实现完成、部署或验收结论。",
        "",
    ])
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--write", action="store_true")
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    if not args.write and not args.check:
        parser.error("choose --write and/or --check")
    payload = build()
    validate_against_schema(payload, SCHEMA)
    if args.write:
        write_json(OUTPUT, payload)
        DOC_OUTPUT.parent.mkdir(parents=True, exist_ok=True)
        DOC_OUTPUT.write_text(render_markdown(payload), encoding="utf-8")
        print(f"WROTE {OUTPUT.relative_to(REPO_ROOT)} entries={payload['entry_count']}")
        print(f"WROTE {DOC_OUTPUT.relative_to(REPO_ROOT)}")
    if args.check:
        if not OUTPUT.is_file():
            raise ValueError("generated PR design application catalog is missing")
        current = json.loads(OUTPUT.read_text(encoding="utf-8"))
        if current != payload:
            raise ValueError("generated PR design application catalog is stale")
        if not DOC_OUTPUT.is_file() or DOC_OUTPUT.read_text(encoding="utf-8") != render_markdown(payload):
            raise ValueError("generated PR design application markdown is stale")
        print(f"PASS entries={payload['entry_count']} scopes={payload['summary']['design_scope_counts']}")
    print("PROOF_CEILING PR_DESIGN_CLASSIFICATION_ONLY_NOT_FUNCTION_DESIGN_OR_EXECUTION_AUTHORIZATION")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
