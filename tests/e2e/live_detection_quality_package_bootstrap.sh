#!/usr/bin/env bash
set -euo pipefail

APISIX="${APISIX:-http://10.0.5.8:30180}"
TENANT="${TENANT:-default}"
LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/$(date +%Y%m%d%H%M%S)-detection-quality-bootstrap}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-detection-quality-bootstrap}"
STABLE_DIR="${STABLE_DIR:-doc/02_acceptance/04-detection-quality/bootstrap}"
KUBECTL="${KUBECTL:-kubectl}"
JWT_SECRET_NAMESPACE="${JWT_SECRET_NAMESPACE:-traffic-analysis}"
JWT_SECRET_NAME="${JWT_SECRET_NAME:-traffic-credentials}"
JWT_SECRET_KEY="${JWT_SECRET_KEY:-JWT_SECRET}"

REPORT="$LOG_DIR/live-detection-quality-bootstrap-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-detection-quality-bootstrap-$RUN_ID-summary.json"
LOCAL_REPORT="$LOG_DIR/local-report.md"
BOOTSTRAP_DIR="$LOG_DIR/blind-package.bootstrap"
STABLE_BOOTSTRAP_DIR="$STABLE_DIR/latest"

mkdir -p "$LOG_DIR" "$BOOTSTRAP_DIR" "$STABLE_DIR"
: >"$REPORT"

JWT_SECRET=""

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 2
  fi
}

kctl() {
  env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy "$KUBECTL" "$@"
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

trim_file() {
  local file="$1"
  if [[ -s "$file" ]]; then
    head -c 1200 "$file" | tr '\n' ' ' | sed -E 's/Bearer [A-Za-z0-9._-]+/Bearer <redacted>/g'
  fi
}

make_token() {
  JWT_SECRET="$JWT_SECRET" TENANT="$TENANT" RUN_ID="$RUN_ID" python3 - <<'PY'
import base64
import hashlib
import hmac
import json
import os
import time
import uuid

def b64url(raw: bytes) -> str:
    return base64.urlsafe_b64encode(raw).rstrip(b"=").decode("ascii")

now = int(time.time())
claims = {
    "iss": "traffic-auth-service",
    "sub": str(uuid.uuid4()),
    "iat": now,
    "exp": now + 1800,
    "jti": str(uuid.uuid4()),
    "user_id": str(uuid.uuid4()),
    "tenant_id": os.environ["TENANT"],
    "username": "codex-detection-quality-bootstrap",
    "roles": ["admin"],
    "permissions": ["*", "admin:*", "alert:read", "fusion:read", "audit:read"],
    "token_type": "access",
    "session_id": "codex-detection-quality-bootstrap-" + os.environ["RUN_ID"],
}
header = {"alg": "HS256", "typ": "JWT"}
signing_input = b".".join([
    b64url(json.dumps(header, separators=(",", ":")).encode()).encode(),
    b64url(json.dumps(claims, separators=(",", ":")).encode()).encode(),
])
signature = hmac.new(os.environ["JWT_SECRET"].encode(), signing_input, hashlib.sha256).digest()
print(signing_input.decode() + "." + b64url(signature))
PY
}

api_get() {
  local path="$1" out="$2" token="$3"
  local code rc err_file
  err_file="$out.err"
  set +e
  code="$(curl --noproxy '*' -sS -m 20 "$APISIX$path" \
    -H "Authorization: Bearer $token" \
    -H "X-Tenant-ID: $TENANT" \
    -o "$out" \
    -w '%{http_code}' 2>"$err_file")"
  rc=$?
  set -e
  if [[ "$rc" -ne 0 ]]; then
    json_log "api" "GET $path" "blocker" false "curl_rc_$rc" "$(trim_file "$err_file")" "$(basename "$err_file")"
    return 1
  fi
  if [[ "$code" != 2* ]]; then
    json_log "api" "GET $path" "blocker" false "http_$code" "$(trim_file "$out")" "$(basename "$out")"
    return 1
  fi
  json_log "api" "GET $path" "info" true "$code" "$path" "$(basename "$out")"
}

finalize() {
  local passed total blockers warnings result sample_count
  passed="$(jq -s '[.[] | select(.passed == true)] | length' "$REPORT")"
  total="$(jq -s 'length' "$REPORT")"
  blockers="$(jq -s '[.[] | select(.passed != true and .severity == "blocker")] | length' "$REPORT")"
  warnings="$(jq -s '[.[] | select(.passed != true and ((.severity == "warn") or (.severity == "warning")))] | length' "$REPORT")"
  result="pass"
  if [[ "$blockers" -gt 0 ]]; then
    result="blocked"
  fi
  sample_count=0
  if [[ -s "$BOOTSTRAP_DIR/sample-index.bootstrap.csv" ]]; then
    sample_count="$(tail -n +2 "$BOOTSTRAP_DIR/sample-index.bootstrap.csv" | wc -l | tr -d ' ')"
  fi

  jq -n \
    --arg run_id "$RUN_ID" \
    --arg result "$result" \
    --arg apisix "$APISIX" \
    --arg tenant "$TENANT" \
    --arg bootstrap_dir "$BOOTSTRAP_DIR" \
    --arg stable_bootstrap_dir "$STABLE_BOOTSTRAP_DIR" \
    --arg generated_at "$(date -Iseconds)" \
    --argjson passed "$passed" \
    --argjson total "$total" \
    --argjson blockers "$blockers" \
    --argjson warnings "$warnings" \
    --argjson sample_count "$sample_count" \
    --slurpfile checks "$REPORT" \
    '{run_id:$run_id, result:$result, generated_at:$generated_at, apisix:$apisix, tenant_id:$tenant, bootstrap_dir:$bootstrap_dir, stable_bootstrap_dir:$stable_bootstrap_dir, sample_count:$sample_count, passed:$passed, total:$total, blockers:$blockers, warnings:$warnings, checks:$checks}' >"$SUMMARY"

  {
    echo "# Detection quality bootstrap"
    echo
    echo "- Run ID: \`$RUN_ID\`"
    echo "- Result: \`$result\`"
    echo "- Candidate samples: $sample_count"
    echo "- Bootstrap dir: \`$BOOTSTRAP_DIR\`"
    echo "- Stable bootstrap dir: \`$STABLE_BOOTSTRAP_DIR\`"
    echo "- Summary: \`$SUMMARY\`"
    echo
    echo "This bootstrap is generated from live observed alert candidates and is review-required. It deliberately writes review-template files, not formal labels.csv or predictions.csv, so it cannot satisfy GATE-P0-06 without a frozen blind dataset, locked threshold, real predictions, and third-party attestation."
  } >"$LOCAL_REPORT"

  rm -rf "$STABLE_BOOTSTRAP_DIR"
  mkdir -p "$STABLE_BOOTSTRAP_DIR"
  cp -R "$BOOTSTRAP_DIR/." "$STABLE_BOOTSTRAP_DIR/"
  cp "$LOCAL_REPORT" "$STABLE_DIR/detection-quality-bootstrap-latest.md"
  cp "$SUMMARY" "$STABLE_DIR/detection-quality-bootstrap-latest.json"

  echo "detection quality bootstrap result: $result"
  echo "summary: $SUMMARY"
  if [[ "$result" != "pass" ]]; then
    exit 1
  fi
}

need_cmd curl
need_cmd git
need_cmd jq
need_cmd python3
need_cmd "$KUBECTL"

git rev-parse HEAD >"$LOG_DIR/commit-sha.txt"
git status --short >"$LOG_DIR/git-status.txt"

JWT_SECRET="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$JWT_SECRET_KEY}" | base64 -d)"
ADMIN_TOKEN="$(make_token)"

api_get "/api/v1/alerts?tenant_id=$TENANT&limit=100" "$LOG_DIR/alerts-response.json" "$ADMIN_TOKEN"
api_get "/api/v1/fusion/value-report?window_hours=168" "$LOG_DIR/fusion-value-report-response.json" "$ADMIN_TOKEN" || true

python3 - "$LOG_DIR/alerts-response.json" "$LOG_DIR/fusion-value-report-response.json" "$BOOTSTRAP_DIR" "$RUN_ID" "$TENANT" <<'PY'
import csv
import hashlib
import json
import sys
from datetime import datetime, timezone
from pathlib import Path

alerts_path = Path(sys.argv[1])
fusion_path = Path(sys.argv[2])
out_dir = Path(sys.argv[3])
run_id = sys.argv[4]
tenant = sys.argv[5]
out_dir.mkdir(parents=True, exist_ok=True)
(out_dir / "labels").mkdir(exist_ok=True)
(out_dir / "predictions").mkdir(exist_ok=True)
(out_dir / "reports").mkdir(exist_ok=True)

def extract_items(payload):
    data = payload.get("data", payload)
    if isinstance(data, list):
        return data
    if isinstance(data, dict):
        for key in ("items", "alerts", "records", "rows", "data"):
            value = data.get(key)
            if isinstance(value, list):
                return value
    return []

def first_value(item, keys):
    for key in keys:
        value = item.get(key)
        if value not in (None, ""):
            return value
    return ""

alerts_payload = json.loads(alerts_path.read_text(encoding="utf-8"))
alerts = extract_items(alerts_payload)
sample_rows = []
seen = set()
for index, item in enumerate(alerts):
    if not isinstance(item, dict):
        continue
    alert_id = str(first_value(item, ["id", "alert_id", "alertId"]) or f"alert-{index}")
    sample_id = "alert-" + hashlib.sha256(alert_id.encode("utf-8")).hexdigest()[:16]
    if sample_id in seen:
        continue
    seen.add(sample_id)
    sample_rows.append({
        "sample_id": sample_id,
        "source_type": "alert",
        "source_id": alert_id,
        "candidate_ground_truth_hint": first_value(item, ["category", "attack_type", "rule_name", "ruleName", "title", "name"]),
        "severity": first_value(item, ["severity", "risk_level", "riskLevel"]),
        "status": first_value(item, ["status", "state"]),
        "first_seen": first_value(item, ["first_seen", "firstSeen", "created_at", "createdAt", "timestamp"]),
        "review_status": "needs_blind_label_review",
    })

fusion_meta = {}
if fusion_path.exists() and fusion_path.stat().st_size:
    try:
        fusion_payload = json.loads(fusion_path.read_text(encoding="utf-8"))
        fusion_data = fusion_payload.get("data", {})
        if isinstance(fusion_data, dict):
            fusion_meta = {
                "model": fusion_data.get("model") or fusion_data.get("report_model") or "fusion-value-ablation-v1",
                "quality_gates": fusion_data.get("quality_gates", []),
            }
    except json.JSONDecodeError:
        fusion_meta = {}

fieldnames = [
    "sample_id",
    "source_type",
    "source_id",
    "candidate_ground_truth_hint",
    "severity",
    "status",
    "first_seen",
    "review_status",
]
with (out_dir / "sample-index.bootstrap.csv").open("w", encoding="utf-8", newline="") as handle:
    writer = csv.DictWriter(handle, fieldnames=fieldnames)
    writer.writeheader()
    writer.writerows(sample_rows)

label_fields = ["sample_id", "ground_truth", "attack_family", "is_unknown", "is_encrypted", "stratum", "labeler", "label_timestamp", "review_note"]
with (out_dir / "labels" / "labels.review-template.csv").open("w", encoding="utf-8", newline="") as handle:
    writer = csv.DictWriter(handle, fieldnames=label_fields)
    writer.writeheader()
    for row in sample_rows:
        writer.writerow({
            "sample_id": row["sample_id"],
            "ground_truth": "",
            "attack_family": "",
            "is_unknown": "",
            "is_encrypted": "",
            "stratum": "",
            "labeler": "",
            "label_timestamp": "",
            "review_note": "third-party evaluator must fill this before freezing labels.csv",
        })

prediction_fields = ["sample_id", "prediction", "score", "threshold", "model_id", "model_version", "feature_set_id", "prediction_timestamp", "review_note"]
with (out_dir / "predictions" / "predictions.review-template.csv").open("w", encoding="utf-8", newline="") as handle:
    writer = csv.DictWriter(handle, fieldnames=prediction_fields)
    writer.writeheader()
    for row in sample_rows:
        writer.writerow({
            "sample_id": row["sample_id"],
            "prediction": "",
            "score": "",
            "threshold": "",
            "model_id": "behavior-classifier",
            "model_version": "",
            "feature_set_id": "v1",
            "prediction_timestamp": "",
            "review_note": "model runner must fill this without access to labels.csv",
        })

manifest = {
    "package_id": "topic1_blind_bootstrap",
    "version": 1,
    "status": "bootstrap_review_required",
    "run_id": run_id,
    "tenant_scope": tenant,
    "generated_at": datetime.now(timezone.utc).isoformat(),
    "review_required": True,
    "formal_gate_note": "Rename/freeze this package only after third-party label review, threshold lock, prediction run, and attestation.",
    "sample_contract": {
        "primary_key": "sample_id",
        "sample_index": "sample-index.bootstrap.csv",
        "label_template": "labels/labels.review-template.csv",
        "prediction_template": "predictions/predictions.review-template.csv",
        "required_strata": ["normal", "known_attack", "unknown_attack", "encrypted"],
    },
    "observed_candidate_counts": {
        "alerts": len(sample_rows),
    },
    "fusion_value_report_meta": fusion_meta,
}
(out_dir / "dataset-manifest.bootstrap.json").write_text(json.dumps(manifest, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")

threshold = {
    "status": "bootstrap_review_required",
    "review_required": True,
    "threshold": None,
    "model_id": "behavior-classifier",
    "feature_set_id": "v1",
    "lock_note": "Set numeric threshold before prediction generation; do not use this bootstrap file as threshold-lock.json.",
}
(out_dir / "threshold-lock.review-template.json").write_text(json.dumps(threshold, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")

attestation = """status: template_review_required
review_required: true
package_id: topic1_blind
reviewer:
  organization: ""
  lab_accreditation: ""
  contact: ""
evidence:
  dataset_manifest_sha256: ""
  labels_sha256: ""
  predictions_sha256: ""
  threshold_lock_sha256: ""
  evaluation_summary_sha256: ""
signature:
  signed_by: ""
  signed_at: ""
  signature_uri: ""
notes:
  - This template is not an attestation.
  - GATE-P0-06 remains blocked until the reviewer fills and signs the final attestation.
"""
(out_dir / "reports" / "third-party-attestation.review-template.yaml").write_text(attestation, encoding="utf-8")

readme = f"""# Detection Quality Bootstrap Package

Run ID: `{run_id}`

This directory contains review-required candidate material generated from live observed alerts. It is intentionally not a formal blind package.

## Files

- `sample-index.bootstrap.csv`: candidate sample IDs derived from live alerts.
- `labels/labels.review-template.csv`: blank label template for third-party evaluator review.
- `predictions/predictions.review-template.csv`: blank prediction template for a no-label prediction run.
- `dataset-manifest.bootstrap.json`: package draft metadata.
- `threshold-lock.review-template.json`: threshold lock template; threshold is intentionally null.
- `reports/third-party-attestation.review-template.yaml`: unsigned attestation template.

## Non-Closure Rule

Do not rename these files into the formal `topic1_blind` package until sample freezing, blind labels, locked threshold, predictions, and third-party attestation are complete.
"""
(out_dir / "README.md").write_text(readme, encoding="utf-8")
PY

sample_count="$(tail -n +2 "$BOOTSTRAP_DIR/sample-index.bootstrap.csv" | wc -l | tr -d ' ')"
if [[ "$sample_count" -gt 0 ]]; then
  json_log "bootstrap" "Live alert candidates exported" "info" true "ok" "samples=$sample_count" "sample-index.bootstrap.csv"
else
  json_log "bootstrap" "Live alert candidates exported" "blocker" false "empty" "samples=0" "sample-index.bootstrap.csv"
fi

if jq -e '.review_required == true and .status == "bootstrap_review_required"' "$BOOTSTRAP_DIR/dataset-manifest.bootstrap.json" >/dev/null; then
  json_log "bootstrap" "Bootstrap manifest is review-required" "info" true "ok" "dataset-manifest.bootstrap.json" "dataset-manifest.bootstrap.json"
else
  json_log "bootstrap" "Bootstrap manifest is review-required" "blocker" false "invalid" "$(trim_file "$BOOTSTRAP_DIR/dataset-manifest.bootstrap.json")" "dataset-manifest.bootstrap.json"
fi

if [[ -s "$BOOTSTRAP_DIR/labels/labels.review-template.csv" && -s "$BOOTSTRAP_DIR/predictions/predictions.review-template.csv" && -s "$BOOTSTRAP_DIR/reports/third-party-attestation.review-template.yaml" ]]; then
  json_log "bootstrap" "Review templates are present" "info" true "ok" "labels/predictions/attestation templates" "blind-package.bootstrap"
else
  json_log "bootstrap" "Review templates are present" "blocker" false "missing" "expected review templates" "blind-package.bootstrap"
fi

if [[ -e "$BOOTSTRAP_DIR/labels/labels.csv" || -e "$BOOTSTRAP_DIR/predictions/predictions.csv" || -e "$BOOTSTRAP_DIR/third-party-attestation.yaml" ]]; then
  json_log "safety" "Bootstrap does not create formal gate artifacts" "blocker" false "formal_artifact_found" "formal artifact names must not be generated by bootstrap" "blind-package.bootstrap"
else
  json_log "safety" "Bootstrap does not create formal gate artifacts" "info" true "ok" "formal artifacts absent" "blind-package.bootstrap"
fi

finalize
