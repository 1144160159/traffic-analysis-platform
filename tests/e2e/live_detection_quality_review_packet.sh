#!/usr/bin/env bash
set -euo pipefail

RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-detection-quality-review}"
LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/$RUN_ID}"
INPUT_BOOTSTRAP_DIR="${INPUT_BOOTSTRAP_DIR:-doc/02_acceptance/04-detection-quality/bootstrap/latest}"
STABLE_DIR="${STABLE_DIR:-doc/02_acceptance/04-detection-quality/review}"

REPORT="$LOG_DIR/detection-quality-review-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/detection-quality-review-$RUN_ID-summary.json"
LOCAL_REPORT="$LOG_DIR/local-report.md"
PACKET_DIR="$LOG_DIR/detection-quality-review.packet"
STABLE_PACKET_DIR="$STABLE_DIR/latest"
STABLE_JSON="$STABLE_DIR/detection-quality-review-latest.json"
STABLE_MD="$STABLE_DIR/detection-quality-review-latest.md"

mkdir -p "$LOG_DIR" "$PACKET_DIR" "$STABLE_DIR"
: >"$REPORT"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 2
  fi
}

json_log() {
  local phase="$1" name="$2" severity="$3" passed="$4" status="$5" detail="${6:-}" artifact="${7:-}"
  jq -nc \
    --arg ts "$(date -Iseconds)" \
    --arg phase "$phase" \
    --arg name "$name" \
    --arg severity "$severity" \
    --argjson passed "$passed" \
    --arg status "$status" \
    --arg detail "$detail" \
    --arg artifact "$artifact" \
    '{ts:$ts, phase:$phase, name:$name, severity:$severity, passed:$passed, status:$status, detail:$detail, artifact:$artifact}' >>"$REPORT"
}

finalize() {
  local passed total blockers warnings result sample_count duplicate_count template_count
  passed="$(jq -s '[.[] | select(.passed == true)] | length' "$REPORT")"
  total="$(jq -s 'length' "$REPORT")"
  blockers="$(jq -s '[.[] | select(.passed != true and .severity == "blocker")] | length' "$REPORT")"
  warnings="$(jq -s '[.[] | select(.passed != true and ((.severity == "warn") or (.severity == "warning")))] | length' "$REPORT")"
  result="pass"
  if [[ "$blockers" -gt 0 ]]; then
    result="blocked"
  elif [[ "$warnings" -gt 0 ]]; then
    result="warn"
  fi

  sample_count="$(jq -r '.sample_count // 0' "$PACKET_DIR/review-summary.json" 2>/dev/null || echo 0)"
  duplicate_count="$(jq -r '.duplicate_sample_id_count // 0' "$PACKET_DIR/review-summary.json" 2>/dev/null || echo 0)"
  template_count="$(jq -r '.review_template_file_count // 0' "$PACKET_DIR/review-summary.json" 2>/dev/null || echo 0)"

  jq -n \
    --arg run_id "$RUN_ID" \
    --arg result "$result" \
    --arg generated_at "$(date -Iseconds)" \
    --arg input_bootstrap_dir "$INPUT_BOOTSTRAP_DIR" \
    --arg packet_dir "$PACKET_DIR" \
    --arg stable_packet_dir "$STABLE_PACKET_DIR" \
    --argjson sample_count "$sample_count" \
    --argjson duplicate_sample_id_count "$duplicate_count" \
    --argjson review_template_file_count "$template_count" \
    --argjson passed "$passed" \
    --argjson total "$total" \
    --argjson blockers "$blockers" \
    --argjson warnings "$warnings" \
    --slurpfile checks "$REPORT" \
    --slurpfile review_summary "$PACKET_DIR/review-summary.json" \
    '{
      run_id:$run_id,
      result:$result,
      generated_at:$generated_at,
      input_bootstrap_dir:$input_bootstrap_dir,
      packet_dir:$packet_dir,
      stable_packet_dir:$stable_packet_dir,
      review_required:true,
      formal_gate_note:"blind detection-quality review packet only; does not close GATE-P0-06",
      sample_count:$sample_count,
      duplicate_sample_id_count:$duplicate_sample_id_count,
      review_template_file_count:$review_template_file_count,
      passed:$passed,
      total:$total,
      blockers:$blockers,
      warnings:$warnings,
      review_summary:($review_summary[0] // {}),
      checks:$checks
    }' >"$SUMMARY"

  {
    echo "# Detection Quality Review Packet"
    echo
    echo "- Run ID: \`$RUN_ID\`"
    echo "- Result: \`$result\`"
    echo "- Input bootstrap: \`$INPUT_BOOTSTRAP_DIR\`"
    echo "- Candidate samples: $sample_count"
    echo "- Duplicate sample IDs: $duplicate_count"
    echo "- Stable packet: \`$STABLE_PACKET_DIR\`"
    echo
    echo "This package turns live alert candidates into a review board for blind labels, prediction execution, threshold locking, and third-party attestation. It is not a frozen blind package and cannot close GATE-P0-06."
    echo
    echo "## Files"
    echo
    echo "- \`sample-review.csv\`: row-level sample review board"
    echo "- \`labeling-worklist.csv\`: evaluator label worklist"
    echo "- \`prediction-worklist.csv\`: no-label model prediction worklist"
    echo "- \`formal-package-manifest.template.yaml\`: manifest template for the frozen package"
    echo "- \`threshold-lock.template.json\`: threshold lock template"
    echo "- \`third-party-attestation.template.yaml\`: attestation template"
    echo "- \`review-checklist.md\`: freeze and rerun checklist"
    echo "- \`review-summary.json\`: package metadata"
  } >"$LOCAL_REPORT"

  rm -rf "$STABLE_PACKET_DIR"
  mkdir -p "$STABLE_PACKET_DIR"
  cp -R "$PACKET_DIR/." "$STABLE_PACKET_DIR/"
  cp "$SUMMARY" "$STABLE_JSON"
  cp "$LOCAL_REPORT" "$STABLE_MD"

  echo "detection quality review packet result=$result summary=$SUMMARY"
  if [[ "$result" == "blocked" ]]; then
    exit 1
  fi
}

need_cmd git
need_cmd jq
need_cmd python3

git rev-parse HEAD >"$LOG_DIR/commit-sha.txt"
git branch --show-current >"$LOG_DIR/git-branch.txt"
git status --short >"$LOG_DIR/git-status.txt"

if [[ -d "$INPUT_BOOTSTRAP_DIR" ]]; then
  json_log "input" "bootstrap directory exists" "info" true "ok" "$INPUT_BOOTSTRAP_DIR" "$INPUT_BOOTSTRAP_DIR"
else
  json_log "input" "bootstrap directory exists" "blocker" false "missing" "$INPUT_BOOTSTRAP_DIR" "$INPUT_BOOTSTRAP_DIR"
  finalize
fi

python3 - "$INPUT_BOOTSTRAP_DIR" "$PACKET_DIR" "$RUN_ID" <<'PY'
import csv
import json
import sys
from collections import Counter
from datetime import datetime, timezone
from pathlib import Path

bootstrap_dir = Path(sys.argv[1])
packet_dir = Path(sys.argv[2])
run_id = sys.argv[3]
packet_dir.mkdir(parents=True, exist_ok=True)

sample_index = bootstrap_dir / "sample-index.bootstrap.csv"
labels_template = bootstrap_dir / "labels" / "labels.review-template.csv"
predictions_template = bootstrap_dir / "predictions" / "predictions.review-template.csv"
threshold_template = bootstrap_dir / "threshold-lock.review-template.json"
manifest_bootstrap = bootstrap_dir / "dataset-manifest.bootstrap.json"
attestation_template = bootstrap_dir / "reports" / "third-party-attestation.review-template.yaml"

required_files = [
    sample_index,
    labels_template,
    predictions_template,
    threshold_template,
    manifest_bootstrap,
    attestation_template,
]
missing_files = [str(path.relative_to(bootstrap_dir)) for path in required_files if not path.is_file()]
if missing_files:
    raise SystemExit("missing bootstrap files: " + ", ".join(missing_files))

def read_csv(path):
    with path.open("r", encoding="utf-8-sig", newline="") as handle:
        return [{key: (value or "").strip() for key, value in row.items()} for row in csv.DictReader(handle)]

samples = read_csv(sample_index)
labels = {row.get("sample_id", ""): row for row in read_csv(labels_template)}
predictions = {row.get("sample_id", ""): row for row in read_csv(predictions_template)}
sample_ids = [row.get("sample_id", "") for row in samples if row.get("sample_id", "")]
duplicate_ids = sorted(sample_id for sample_id, count in Counter(sample_ids).items() if count > 1)

manifest = json.loads(manifest_bootstrap.read_text(encoding="utf-8"))
threshold = json.loads(threshold_template.read_text(encoding="utf-8"))

review_rows = []
for index, row in enumerate(samples, start=1):
    sample_id = row.get("sample_id", "")
    label_row = labels.get(sample_id, {})
    prediction_row = predictions.get(sample_id, {})
    review_rows.append({
        "review_index": index,
        "sample_id": sample_id,
        "source_type": row.get("source_type", ""),
        "source_id": row.get("source_id", ""),
        "severity": row.get("severity", ""),
        "status": row.get("status", ""),
        "first_seen": row.get("first_seen", ""),
        "candidate_ground_truth_hint": row.get("candidate_ground_truth_hint", ""),
        "label_review_status": row.get("review_status", "needs_blind_label_review"),
        "label_ground_truth": label_row.get("ground_truth", "TBD"),
        "label_attack_family": label_row.get("attack_family", "TBD"),
        "label_is_unknown": label_row.get("is_unknown", "TBD"),
        "label_is_encrypted": label_row.get("is_encrypted", "TBD"),
        "prediction_review_status": "needs_no_label_prediction_run",
        "prediction": prediction_row.get("prediction", "TBD"),
        "prediction_score": prediction_row.get("score", "TBD"),
        "threshold": threshold.get("threshold") if threshold.get("threshold") is not None else "TBD",
        "third_party_decision": "TBD: include | exclude | request_more_context",
        "review_note": "TBD",
    })

def write_csv(path, rows, fieldnames):
    with path.open("w", encoding="utf-8", newline="") as handle:
        writer = csv.DictWriter(handle, fieldnames=fieldnames)
        writer.writeheader()
        for item in rows:
            writer.writerow({field: item.get(field, "") for field in fieldnames})

sample_review_fields = [
    "review_index",
    "sample_id",
    "source_type",
    "source_id",
    "severity",
    "status",
    "first_seen",
    "candidate_ground_truth_hint",
    "label_review_status",
    "label_ground_truth",
    "label_attack_family",
    "label_is_unknown",
    "label_is_encrypted",
    "prediction_review_status",
    "prediction",
    "prediction_score",
    "threshold",
    "third_party_decision",
    "review_note",
]
write_csv(packet_dir / "sample-review.csv", review_rows, sample_review_fields)

label_rows = [
    {
        "sample_id": row["sample_id"],
        "ground_truth": "TBD",
        "attack_family": "TBD",
        "is_unknown": "TBD",
        "is_encrypted": "TBD",
        "stratum": "TBD",
        "labeler": "TBD",
        "label_timestamp": "TBD",
        "review_note": "TBD",
    }
    for row in review_rows
]
write_csv(
    packet_dir / "labeling-worklist.csv",
    label_rows,
    ["sample_id", "ground_truth", "attack_family", "is_unknown", "is_encrypted", "stratum", "labeler", "label_timestamp", "review_note"],
)

prediction_rows = [
    {
        "sample_id": row["sample_id"],
        "prediction": "TBD",
        "score": "TBD",
        "threshold": "TBD",
        "model_id": "behavior-classifier",
        "model_version": "TBD",
        "feature_set_id": "v1",
        "prediction_timestamp": "TBD",
        "review_note": "TBD",
    }
    for row in review_rows
]
write_csv(
    packet_dir / "prediction-worklist.csv",
    prediction_rows,
    ["sample_id", "prediction", "score", "threshold", "model_id", "model_version", "feature_set_id", "prediction_timestamp", "review_note"],
)

formal_manifest_template = {
    "package_id": "topic1_blind",
    "version": 1,
    "status": "review-template",
    "generated_from_review_packet": run_id,
    "generated_at": datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z"),
    "review_required": True,
    "freeze": {
        "frozen_at": "TBD",
        "frozen_by": "TBD",
        "immutable_storage_uri": "TBD",
        "package_sha256_manifest": "TBD",
    },
    "model_scope": {
        "model_id": "behavior-classifier",
        "feature_set_id": "v1",
        "tenant_scope": manifest.get("tenant_scope", "default"),
    },
    "sample_contract": {
        "primary_key": "sample_id",
        "label_file": "labels/labels.csv",
        "prediction_file": "predictions/predictions.csv",
        "required_strata": ["normal", "known_attack", "unknown_attack", "encrypted"],
    },
    "candidate_sample_count": len(samples),
    "strata_counts": {
        "normal": "TBD",
        "known_attack": "TBD",
        "unknown_attack": "TBD",
        "encrypted": "TBD",
    },
    "review": {
        "third_party_attestation": "third-party-attestation.yaml",
        "cnas_lab": "TBD",
        "reviewer": "TBD",
    },
    "notes": [
        "This is a review template, not dataset-manifest.yaml.",
        "Replace all TBD fields and remove review_required before copying into the formal package.",
    ],
}
(packet_dir / "formal-package-manifest.template.yaml").write_text(
    json.dumps(formal_manifest_template, ensure_ascii=False, indent=2) + "\n",
    encoding="utf-8",
)

threshold_lock_template = {
    "status": "review-template",
    "review_required": True,
    "generated_from_review_packet": run_id,
    "model_id": "behavior-classifier",
    "feature_set_id": "v1",
    "threshold": "TBD",
    "locked_by": "TBD",
    "locked_at": "TBD",
    "lock_evidence": "TBD",
}
(packet_dir / "threshold-lock.template.json").write_text(
    json.dumps(threshold_lock_template, ensure_ascii=False, indent=2) + "\n",
    encoding="utf-8",
)

(packet_dir / "third-party-attestation.template.yaml").write_text(
    "\n".join([
        "status: review-template",
        "review_required: true",
        "package_id: topic1_blind",
        "reviewer:",
        "  organization: TBD",
        "  lab_accreditation: TBD",
        "  contact: TBD",
        "evidence:",
        "  dataset_manifest_sha256: TBD",
        "  labels_sha256: TBD",
        "  predictions_sha256: TBD",
        "  threshold_lock_sha256: TBD",
        "  evaluation_summary_sha256: TBD",
        "signature:",
        "  signed_by: TBD",
        "  signed_at: TBD",
        "  signature_uri: TBD",
        "notes:",
        "  - This is a review template, not an attestation.",
        "",
    ]),
    encoding="utf-8",
)

summary = {
    "package_id": "detection_quality_review_packet",
    "run_id": run_id,
    "generated_at": datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z"),
    "input_bootstrap_dir": str(bootstrap_dir),
    "review_required": True,
    "formal_gate_note": "review packet only; does not close GATE-P0-06",
    "sample_count": len(samples),
    "duplicate_sample_ids": duplicate_ids,
    "duplicate_sample_id_count": len(duplicate_ids),
    "review_template_file_count": 6,
    "required_formal_artifacts": [
        "mlops/eval_packages/topic1_blind/dataset-manifest.yaml",
        "mlops/eval_packages/topic1_blind/threshold-lock.json",
        "mlops/eval_packages/topic1_blind/labels/labels.csv",
        "mlops/eval_packages/topic1_blind/predictions/predictions.csv",
        "mlops/eval_packages/topic1_blind/third-party-attestation.yaml",
    ],
    "review_outputs": [
        "sample-review.csv",
        "labeling-worklist.csv",
        "prediction-worklist.csv",
        "formal-package-manifest.template.yaml",
        "threshold-lock.template.json",
        "third-party-attestation.template.yaml",
        "review-checklist.md",
    ],
}
(packet_dir / "review-summary.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

(packet_dir / "review-checklist.md").write_text(
    "\n".join([
        "# Detection Quality Review Checklist",
        "",
        "1. Freeze the sample set and immutable source artifacts.",
        "2. Fill `labeling-worklist.csv` without exposing labels to the prediction runner.",
        "3. Lock threshold in `threshold-lock.template.json` before generating predictions.",
        "4. Fill `prediction-worklist.csv` from a no-label model run.",
        "5. Produce `dataset-manifest.yaml`, `threshold-lock.json`, `labels/labels.csv`, `predictions/predictions.csv`, and `third-party-attestation.yaml` in `mlops/eval_packages/topic1_blind/` only after all TBD/review-template markers are removed.",
        "6. Rerun `ALLOW_BLOCKERS=false tests/e2e/live_detection_quality_preflight.sh`.",
        "",
        "Do not copy this review packet into the formal package unchanged.",
        "",
    ]),
    encoding="utf-8",
)
PY

if [[ -s "$PACKET_DIR/review-summary.json" ]]; then
  sample_count="$(jq -r '.sample_count' "$PACKET_DIR/review-summary.json")"
  duplicate_count="$(jq -r '.duplicate_sample_id_count' "$PACKET_DIR/review-summary.json")"
  json_log "packet" "review packet generated" "info" true "ok" "samples=$sample_count duplicates=$duplicate_count" "review-summary.json"
else
  json_log "packet" "review packet generated" "blocker" false "missing" "review-summary.json" "review-summary.json"
fi

if jq -e '.sample_count > 0' "$PACKET_DIR/review-summary.json" >/dev/null 2>&1; then
  json_log "packet" "candidate samples present" "info" true "ok" "$(jq -r '.sample_count' "$PACKET_DIR/review-summary.json") samples" "sample-review.csv"
else
  json_log "packet" "candidate samples present" "blocker" false "empty" "sample_count=0" "sample-review.csv"
fi

if jq -e '.duplicate_sample_id_count == 0' "$PACKET_DIR/review-summary.json" >/dev/null 2>&1; then
  json_log "packet" "sample IDs are unique" "info" true "ok" "duplicate_sample_id_count=0" "sample-review.csv"
else
  json_log "packet" "sample IDs are unique" "blocker" false "duplicate" "duplicate sample IDs present" "review-summary.json"
fi

if [[ -s "$PACKET_DIR/review-checklist.md" && -s "$PACKET_DIR/labeling-worklist.csv" && -s "$PACKET_DIR/prediction-worklist.csv" ]]; then
  json_log "packet" "review worklists and checklist present" "info" true "ok" "labeling/prediction/checklist" "$PACKET_DIR"
else
  json_log "packet" "review worklists and checklist present" "blocker" false "missing" "required review worklists missing" "$PACKET_DIR"
fi

if [[ ! -e "mlops/eval_packages/topic1_blind/dataset-manifest.yaml" && ! -e "mlops/eval_packages/topic1_blind/threshold-lock.json" && ! -e "mlops/eval_packages/topic1_blind/third-party-attestation.yaml" ]]; then
  json_log "safety" "formal blind package remains untouched" "info" true "ok" "review packet did not create formal artifacts" "mlops/eval_packages/topic1_blind"
else
  json_log "safety" "formal blind package remains untouched" "blocker" false "formal_artifact_present" "formal artifacts already exist or were created" "mlops/eval_packages/topic1_blind"
fi

finalize
