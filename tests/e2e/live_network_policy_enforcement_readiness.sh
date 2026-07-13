#!/usr/bin/env bash
set -euo pipefail

RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-network-policy-enforcement-readiness}"
LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/$RUN_ID}"
SECURITY_DIR="${SECURITY_DIR:-doc/02_acceptance/05-security}"
STABLE_DIR="${STABLE_DIR:-$SECURITY_DIR/network-policy-readiness}"
NETWORK_POLICY_FILE="${NETWORK_POLICY_FILE:-deployments/kubernetes/security/00-network-policies.yaml}"
PREFLIGHT_JSON="${PREFLIGHT_JSON:-$SECURITY_DIR/network-policy-enforcement-preflight-latest.json}"
CNI_SUMMARY_JSON="${CNI_SUMMARY_JSON:-$SECURITY_DIR/live-cni-policy-capability-summary-latest.json}"
LIVE_NETWORK_POLICY_JSON="${LIVE_NETWORK_POLICY_JSON:-$SECURITY_DIR/network-policy-live-latest.json}"
KUBECTL="${KUBECTL:-kubectl}"

REPORT="$LOG_DIR/network-policy-enforcement-readiness-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/network-policy-enforcement-readiness-$RUN_ID-summary.json"
LOCAL_REPORT="$LOG_DIR/local-report.md"
BOOTSTRAP_DIR="$LOG_DIR/network-policy-enforcement-readiness.bootstrap"
STABLE_BOOTSTRAP_DIR="$STABLE_DIR/latest"

mkdir -p "$LOG_DIR" "$BOOTSTRAP_DIR/inputs" "$STABLE_DIR"
: >"$REPORT"

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

check_file() {
  local path="$1" severity="$2" label="$3"
  if [[ -s "$path" ]]; then
    json_log "inputs" "$label" "info" true "ok" "$path" "$path"
  else
    json_log "inputs" "$label" "$severity" false "missing" "$path" "$path"
  fi
}

trim_file() {
  local file="$1"
  if [[ -s "$file" ]]; then
    head -c 1000 "$file" | tr '\n' ' '
  fi
}

copy_input() {
  local path="$1" name="$2"
  if [[ -s "$path" ]]; then
    cp "$path" "$BOOTSTRAP_DIR/inputs/$name"
  fi
}

finalize() {
  local passed total blockers warnings result policy_capable flannel live_np preflight_run preflight_result preflight_blockers
  passed="$(jq -s '[.[] | select(.passed == true)] | length' "$REPORT")"
  total="$(jq -s 'length' "$REPORT")"
  blockers="$(jq -s '[.[] | select(.passed != true and .severity == "blocker")] | length' "$REPORT")"
  warnings="$(jq -s '[.[] | select(.passed != true and ((.severity == "warn") or (.severity == "warning")))] | length' "$REPORT")"
  result="pass"
  if [[ "$blockers" -gt 0 ]]; then
    result="blocked"
  fi

  policy_capable="$(jq -r '.policy_capable_count // 0' "$CNI_SUMMARY_JSON" 2>/dev/null || echo 0)"
  flannel="$(jq -r '.flannel_marker_count // 0' "$CNI_SUMMARY_JSON" 2>/dev/null || echo 0)"
  live_np="$(jq -r '.live_network_policy_count // (.items | length) // 0' "$PREFLIGHT_JSON" "$LIVE_NETWORK_POLICY_JSON" 2>/dev/null | tail -n1 || echo 0)"
  preflight_run="$(jq -r '.run_id // "unknown"' "$PREFLIGHT_JSON" 2>/dev/null || echo unknown)"
  preflight_result="$(jq -r '.result // "unknown"' "$PREFLIGHT_JSON" 2>/dev/null || echo unknown)"
  preflight_blockers="$(jq -r '.blockers // 0' "$PREFLIGHT_JSON" 2>/dev/null || echo 0)"

  jq -n \
    --arg run_id "$RUN_ID" \
    --arg result "$result" \
    --arg generated_at "$(date -Iseconds)" \
    --arg bootstrap_dir "$BOOTSTRAP_DIR" \
    --arg stable_bootstrap_dir "$STABLE_BOOTSTRAP_DIR" \
    --arg preflight_run_id "$preflight_run" \
    --arg preflight_result "$preflight_result" \
    --argjson preflight_blockers "$preflight_blockers" \
    --argjson policy_capable_count "$policy_capable" \
    --argjson flannel_marker_count "$flannel" \
    --argjson live_network_policy_count "$live_np" \
    --argjson passed "$passed" \
    --argjson total "$total" \
    --argjson blockers "$blockers" \
    --argjson warnings "$warnings" \
    --slurpfile checks "$REPORT" \
    '{
      run_id:$run_id,
      result:$result,
      generated_at:$generated_at,
      bootstrap_dir:$bootstrap_dir,
      stable_bootstrap_dir:$stable_bootstrap_dir,
      review_required:true,
      formal_gate_note:"network-policy readiness bootstrap only; does not prove default-deny or allow-list enforcement",
      preflight_run_id:$preflight_run_id,
      preflight_result:$preflight_result,
      preflight_blockers:$preflight_blockers,
      policy_capable_count:$policy_capable_count,
      flannel_marker_count:$flannel_marker_count,
      live_network_policy_count:$live_network_policy_count,
      passed:$passed,
      total:$total,
      blockers:$blockers,
      warnings:$warnings,
      checks:$checks
    }' >"$SUMMARY"

  {
    echo "# NetworkPolicy Enforcement Readiness"
    echo
    echo "- Run ID: \`$RUN_ID\`"
    echo "- Result: \`$result\`"
    echo "- Latest formal preflight: \`$preflight_run\` / \`$preflight_result\`"
    echo "- Policy-capable CNI pods: \`$policy_capable\`"
    echo "- Flannel markers: \`$flannel\`"
    echo "- Live NetworkPolicy objects: \`$live_np\`"
    echo "- Bootstrap dir: \`$BOOTSTRAP_DIR\`"
    echo "- Stable bootstrap dir: \`$STABLE_BOOTSTRAP_DIR\`"
    echo
    echo "This package prepares the CNI migration and enforcement proof workflow. It does not install a CNI, does not run destructive network changes, and does not satisfy GATE-P0-07 or GATE-P0-10."
    echo
    echo "## Required Formal Closure"
    echo
    echo "After a policy-capable CNI is installed and Ready, rerun:"
    echo
    echo "\`\`\`bash"
    echo "ALLOW_BLOCKERS=false RUN_ENFORCEMENT_PROBE=auto tests/e2e/live_network_policy_enforcement_preflight.sh"
    echo "\`\`\`"
    echo
    echo "The formal gate only passes when baseline connectivity works, default-deny blocks the isolated probe, and allow-list restores the probe."
    echo
    echo "## Non-Passing Checks"
    echo
    jq -r '.checks[] | select(.passed|not) | "- [" + .severity + "] " + .name + ": " + .detail' "$SUMMARY"
  } >"$LOCAL_REPORT"

  rm -rf "$STABLE_BOOTSTRAP_DIR"
  mkdir -p "$STABLE_BOOTSTRAP_DIR"
  cp -R "$BOOTSTRAP_DIR/." "$STABLE_BOOTSTRAP_DIR/"
  cp "$LOCAL_REPORT" "$STABLE_DIR/network-policy-enforcement-readiness-latest.md"
  cp "$SUMMARY" "$STABLE_DIR/network-policy-enforcement-readiness-latest.json"

  echo "network-policy-enforcement-readiness result=$result summary=$SUMMARY"
  if [[ "$result" == "blocked" ]]; then
    exit 1
  fi
}

need_cmd git
need_cmd jq
need_cmd "$KUBECTL"

git rev-parse HEAD >"$LOG_DIR/commit-sha.txt"
git branch --show-current >"$LOG_DIR/git-branch.txt"
git status --short >"$LOG_DIR/git-status.txt"

check_file "$PREFLIGHT_JSON" "blocker" "latest NetworkPolicy enforcement preflight present"
check_file "$CNI_SUMMARY_JSON" "blocker" "latest CNI capability summary present"
check_file "$LIVE_NETWORK_POLICY_JSON" "warn" "latest live NetworkPolicy inventory present"
check_file "$NETWORK_POLICY_FILE" "blocker" "repo NetworkPolicy profile present"

set +e
kctl apply --dry-run=client -f "$NETWORK_POLICY_FILE" >"$LOG_DIR/network-policy-dry-run.txt" 2>"$LOG_DIR/network-policy-dry-run.err"
dry_run_rc=$?
set -e
if [[ "$dry_run_rc" -eq 0 ]]; then
  json_log "repo" "NetworkPolicy profile still client-dry-runs" "info" true "ok" "$NETWORK_POLICY_FILE" "network-policy-dry-run.txt"
else
  json_log "repo" "NetworkPolicy profile still client-dry-runs" "blocker" false "rc=$dry_run_rc" "$(trim_file "$LOG_DIR/network-policy-dry-run.err")" "network-policy-dry-run.err"
fi

copy_input "$PREFLIGHT_JSON" "network-policy-enforcement-preflight-latest.json"
copy_input "$CNI_SUMMARY_JSON" "live-cni-policy-capability-summary-latest.json"
copy_input "$LIVE_NETWORK_POLICY_JSON" "network-policy-live-latest.json"
copy_input "$NETWORK_POLICY_FILE" "00-network-policies.yaml"

policy_capable="$(jq -r '.policy_capable_count // 0' "$CNI_SUMMARY_JSON" 2>/dev/null || echo 0)"
flannel="$(jq -r '.flannel_marker_count // 0' "$CNI_SUMMARY_JSON" 2>/dev/null || echo 0)"
live_np="$(jq -r '.live_network_policy_count // (.items | length) // 0' "$PREFLIGHT_JSON" "$LIVE_NETWORK_POLICY_JSON" 2>/dev/null | tail -n1 || echo 0)"
preflight_result="$(jq -r '.result // "unknown"' "$PREFLIGHT_JSON" 2>/dev/null || echo unknown)"

if [[ "$policy_capable" -gt 0 ]]; then
  json_log "live" "policy-capable CNI is already present" "info" true "ok" "policy_capable_count=$policy_capable" "$CNI_SUMMARY_JSON"
else
  json_log "live" "policy-capable CNI is already present" "warn" false "missing" "policy_capable_count=0 flannel_markers=$flannel; formal negative probe remains invalid" "$CNI_SUMMARY_JSON"
fi

if [[ "$live_np" -gt 0 ]]; then
  json_log "live" "live NetworkPolicy objects are available for enforcement once CNI supports them" "info" true "ok" "live_network_policy_count=$live_np" "$LIVE_NETWORK_POLICY_JSON"
else
  json_log "live" "live NetworkPolicy objects are available for enforcement once CNI supports them" "blocker" false "missing" "live_network_policy_count=0" "$LIVE_NETWORK_POLICY_JSON"
fi

if [[ "$preflight_result" == "pass" ]]; then
  json_log "formal" "formal NetworkPolicy enforcement gate status" "info" true "pass" "$preflight_result" "$PREFLIGHT_JSON"
else
  json_log "formal" "formal NetworkPolicy enforcement gate status" "warn" false "$preflight_result" "expected until policy-capable CNI and probe proof exist" "$PREFLIGHT_JSON"
fi

cat >"$BOOTSTRAP_DIR/cni-migration-runbook.md" <<'MD'
# CNI Migration Runbook Template

## Boundary

This is a review-required runbook for closing NetworkPolicy enforcement. It does not install a CNI and does not prove enforcement by itself.

## Candidate Selection

- Candidate CNI:
- Selected version:
- Reason for selection:
- Operator:
- Maintenance window:
- Rollback owner:

## Pre-Change Capture

- `kubectl get nodes -o wide`
- `kubectl get pods -A -o wide`
- `kubectl get networkpolicy -A`
- `kubectl get daemonset -A | grep -Ei 'flannel|calico|cilium|antrea|kube-router|ovn|weave|canal'`
- Latest `doc/02_acceptance/05-security/network-policy-enforcement-preflight-latest.json`

## Change Steps

1. Freeze workload rollout changes.
2. Back up current CNI manifests and kube-system/kube-flannel DaemonSet state.
3. Install or migrate to the approved policy-capable CNI.
4. Wait for every CNI DaemonSet to be Ready on every schedulable node.
5. Confirm kube-dns, APISIX, control-plane services, Kafka, ClickHouse, PostgreSQL, Redis, MinIO and probe-agent paths remain healthy.
6. Rerun `ALLOW_BLOCKERS=false RUN_ENFORCEMENT_PROBE=auto tests/e2e/live_network_policy_enforcement_preflight.sh`.

## Required Exit Evidence

- Policy-capable CNI pod count is greater than 0.
- Live NetworkPolicy object count is greater than 0.
- Isolated probe proves baseline connectivity, default-deny blocking, and allow-list restoration.
- Production security preflight no longer reports the CNI enforcement blocker.
MD

cat >"$BOOTSTRAP_DIR/enforcement-probe.review-template.md" <<'MD'
# NetworkPolicy Enforcement Probe Review Template

- Run ID:
- CNI:
- CNI version:
- Nodes covered:
- Probe namespace:
- Baseline connectivity result:
- Default-deny result:
- Allow-list restoration result:
- Unexpected allowed flows:
- Unexpected blocked flows:
- Evidence files:
- Reviewer:
- Review result:
MD

cat >"$BOOTSTRAP_DIR/rollback-checklist.review-template.md" <<'MD'
# NetworkPolicy/CNI Rollback Checklist

- Rollback decision owner:
- Rollback trigger:
- Previous CNI manifests backed up:
- Current workloads protected from eviction:
- APISIX business entry validated after rollback:
- Kafka/Flink/ClickHouse/PostgreSQL/Redis/MinIO health validated after rollback:
- Probe-agent ingest path validated after rollback:
- Post-rollback production-security preflight run:
MD

cat >"$BOOTSTRAP_DIR/cni-selection.review-template.yaml" <<'YAML'
review_required: true
selected_cni: TBD
selected_version: TBD
candidate_options:
  - name: Calico
    policy_capable: true
    notes: TBD
  - name: Cilium
    policy_capable: true
    notes: TBD
  - name: Antrea
    policy_capable: true
    notes: TBD
approval:
  sre_owner: TBD
  security_owner: TBD
  maintenance_window: TBD
  rollback_owner: TBD
YAML

cat >"$BOOTSTRAP_DIR/preflight-command.review-template.sh" <<'SH'
#!/usr/bin/env bash
set -euo pipefail

ALLOW_BLOCKERS=false \
RUN_ENFORCEMENT_PROBE=auto \
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-network-policy-enforcement-post-cni}" \
tests/e2e/live_network_policy_enforcement_preflight.sh
SH

jq -n \
  --arg run_id "$RUN_ID" \
  --arg generated_at "$(date -Iseconds)" \
  --arg formal_gate_note "review-required CNI/NetworkPolicy readiness package; does not satisfy enforcement gate" \
  --arg preflight "$PREFLIGHT_JSON" \
  --arg cni_summary "$CNI_SUMMARY_JSON" \
  --arg network_policy_file "$NETWORK_POLICY_FILE" \
  --argjson policy_capable_count "$policy_capable" \
  --argjson flannel_marker_count "$flannel" \
  --argjson live_network_policy_count "$live_np" \
  '{
    package_id:"network_policy_enforcement_readiness_bootstrap",
    run_id:$run_id,
    generated_at:$generated_at,
    review_required:true,
    formal_gate_note:$formal_gate_note,
    current_state:{
      preflight:$preflight,
      cni_summary:$cni_summary,
      network_policy_file:$network_policy_file,
      policy_capable_count:$policy_capable_count,
      flannel_marker_count:$flannel_marker_count,
      live_network_policy_count:$live_network_policy_count
    },
    required_formal_evidence:[
      "network-policy-enforcement-preflight-latest.json result pass",
      "policy_capable_count > 0",
      "default-deny probe blocks isolated client",
      "allow-list probe restores isolated client",
      "production-security-preflight-latest.json has no NetworkPolicy/CNI blocker"
    ],
    files:{
      runbook:"cni-migration-runbook.md",
      probe_review_template:"enforcement-probe.review-template.md",
      rollback_checklist:"rollback-checklist.review-template.md",
      cni_selection:"cni-selection.review-template.yaml",
      post_cni_preflight_command:"preflight-command.review-template.sh",
      evidence_manifest:"evidence-manifest.bootstrap.json"
    }
  }' >"$BOOTSTRAP_DIR/cni-migration-readiness.bootstrap.json"

jq -n \
  --arg run_id "$RUN_ID" \
  --arg generated_at "$(date -Iseconds)" \
  --arg package_dir "$BOOTSTRAP_DIR" \
  '{
    run_id:$run_id,
    generated_at:$generated_at,
    package_dir:$package_dir,
    review_required:true,
    artifacts:[
      "cni-migration-readiness.bootstrap.json",
      "cni-migration-runbook.md",
      "enforcement-probe.review-template.md",
      "rollback-checklist.review-template.md",
      "cni-selection.review-template.yaml",
      "preflight-command.review-template.sh",
      "inputs/network-policy-enforcement-preflight-latest.json",
      "inputs/live-cni-policy-capability-summary-latest.json",
      "inputs/network-policy-live-latest.json",
      "inputs/00-network-policies.yaml"
    ]
  }' >"$BOOTSTRAP_DIR/evidence-manifest.bootstrap.json"

json_log "package" "NetworkPolicy enforcement readiness package written" "info" true "ok" "$BOOTSTRAP_DIR" "cni-migration-readiness.bootstrap.json"

finalize
