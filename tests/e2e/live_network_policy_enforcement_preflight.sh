#!/usr/bin/env bash
set -euo pipefail

LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/$(date +%Y%m%d%H%M%S)-network-policy-enforcement-preflight}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-network-policy-enforcement-preflight}"
KUBECTL="${KUBECTL:-kubectl}"
ALLOW_BLOCKERS="${ALLOW_BLOCKERS:-false}"
SECURITY_DIR="${SECURITY_DIR:-doc/02_acceptance/05-security}"
NETWORK_POLICY_FILE="${NETWORK_POLICY_FILE:-deployments/kubernetes/security/00-network-policies.yaml}"
RUN_ENFORCEMENT_PROBE="${RUN_ENFORCEMENT_PROBE:-auto}"
CLEANUP="${CLEANUP:-true}"
SAFE_RUN_ID="$(printf '%s' "$RUN_ID" | tr -c 'a-zA-Z0-9-' '-' | cut -c1-40 | sed 's/-*$//')"
PROBE_NAMESPACE="${PROBE_NAMESPACE:-np-enforce-${SAFE_RUN_ID:-probe}}"
CLIENT_IMAGE="${CLIENT_IMAGE:-busybox:1.36}"
SERVER_IMAGE="${SERVER_IMAGE:-docker.io/library/nginx@sha256:8b1e78743a03dbb2c95171cc58639fef29abc8816598e27fb910ed2e621e589a}"

REPORT="$LOG_DIR/live-network-policy-enforcement-preflight-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-network-policy-enforcement-preflight-$RUN_ID-summary.json"
LOCAL_REPORT="$LOG_DIR/local-report.md"

mkdir -p "$LOG_DIR" "$SECURITY_DIR"
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

trim_file() {
  local file="$1"
  if [[ -s "$file" ]]; then
    head -c 800 "$file" | tr '\n' ' '
  fi
}

need_cmd git
need_cmd jq
need_cmd "$KUBECTL"

git rev-parse HEAD >"$LOG_DIR/commit-sha.txt"
git status --short >"$LOG_DIR/git-status.txt"

set +e
kctl apply --dry-run=client -f "$NETWORK_POLICY_FILE" >"$LOG_DIR/network-policy-dry-run.txt" 2>"$LOG_DIR/network-policy-dry-run.err"
NETWORK_POLICY_DRY_RUN_RC=$?
set -e
if [[ "$NETWORK_POLICY_DRY_RUN_RC" -eq 0 ]]; then
  json_log "repo" "NetworkPolicy profile client dry-run" "info" true "ok" "$NETWORK_POLICY_FILE" "network-policy-dry-run.txt"
else
  json_log "repo" "NetworkPolicy profile client dry-run" "blocker" false "rc=$NETWORK_POLICY_DRY_RUN_RC" "$(trim_file "$LOG_DIR/network-policy-dry-run.err")" "network-policy-dry-run.err"
fi

kctl get pods -A -o json >"$LOG_DIR/live-cni-pods.json"
jq '[
  .items[]
  | . as $pod
  | ([($pod.spec.containers // [])[] | .image] | join(" ")) as $images
  | (($pod.metadata.namespace // "") + " " + ($pod.metadata.name // "") + " " + $images) as $haystack
  | select($haystack | test("flannel|calico|cilium|antrea|kube-router|ovn|ovnkube|weave|canal"; "i"))
  | {
      namespace: $pod.metadata.namespace,
      pod: $pod.metadata.name,
      node: ($pod.spec.nodeName // ""),
      phase: ($pod.status.phase // ""),
      images: [($pod.spec.containers // [])[] | .image],
      policy_capable: ($haystack | test("calico|cilium|antrea|kube-router|ovn|ovnkube|weave|canal"; "i")),
      flannel_only_marker: ($haystack | test("flannel"; "i"))
    }
]' "$LOG_DIR/live-cni-pods.json" >"$LOG_DIR/live-cni-policy-capability.json"

CNI_POLICY_CAPABLE_COUNT="$(jq '[.[] | select(.policy_capable == true)] | length' "$LOG_DIR/live-cni-policy-capability.json")"
FLANNEL_MARKER_COUNT="$(jq '[.[] | select(.flannel_only_marker == true)] | length' "$LOG_DIR/live-cni-policy-capability.json")"
jq -n \
  --argjson policy_capable_count "$CNI_POLICY_CAPABLE_COUNT" \
  --argjson flannel_marker_count "$FLANNEL_MARKER_COUNT" \
  --argfile inventory "$LOG_DIR/live-cni-policy-capability.json" \
  '{
    policy_capable_count:$policy_capable_count,
    flannel_marker_count:$flannel_marker_count,
    networkpolicy_enforcement_ready:($policy_capable_count > 0),
    inventory:$inventory
  }' >"$LOG_DIR/live-cni-policy-capability-summary.json"

if [[ "$CNI_POLICY_CAPABLE_COUNT" -gt 0 ]]; then
  json_log "live" "NetworkPolicy enforcement-capable CNI present" "info" true "ok" "$CNI_POLICY_CAPABLE_COUNT policy-capable CNI pods" "live-cni-policy-capability-summary.json"
else
  json_log "live" "NetworkPolicy enforcement-capable CNI present" "blocker" false "missing" "policy-capable CNI pods=0 flannel_markers=$FLANNEL_MARKER_COUNT" "live-cni-policy-capability-summary.json"
fi

kctl get networkpolicy -A -o json >"$LOG_DIR/live-networkpolicies.json"
LIVE_NP_COUNT="$(jq '.items | length' "$LOG_DIR/live-networkpolicies.json")"
if [[ "$LIVE_NP_COUNT" -gt 0 ]]; then
  json_log "live" "live NetworkPolicy objects present" "info" true "ok" "$LIVE_NP_COUNT policies" "live-networkpolicies.json"
else
  json_log "live" "live NetworkPolicy objects present" "blocker" false "missing" "0 policies" "live-networkpolicies.json"
fi

run_probe=false
if [[ "$RUN_ENFORCEMENT_PROBE" == "true" ]]; then
  run_probe=true
elif [[ "$RUN_ENFORCEMENT_PROBE" == "auto" && "$CNI_POLICY_CAPABLE_COUNT" -gt 0 ]]; then
  run_probe=true
fi

cleanup_probe() {
  if [[ "$CLEANUP" == "true" && -n "${PROBE_NAMESPACE:-}" ]]; then
    kctl delete namespace "$PROBE_NAMESPACE" --ignore-not-found=true --wait=false >/dev/null 2>&1 || true
  fi
}

if [[ "$run_probe" == "true" ]]; then
  trap cleanup_probe EXIT
  cat >"$LOG_DIR/probe-namespace.yaml" <<YAML
apiVersion: v1
kind: Namespace
metadata:
  name: $PROBE_NAMESPACE
  labels:
    traffic.platform/network-policy-probe: "true"
YAML
  cat >"$LOG_DIR/probe-workloads.yaml" <<YAML
apiVersion: apps/v1
kind: Deployment
metadata:
  name: np-server
  namespace: $PROBE_NAMESPACE
spec:
  replicas: 1
  selector:
    matchLabels:
      app: np-server
  template:
    metadata:
      labels:
        app: np-server
    spec:
      containers:
      - name: nginx
        image: $SERVER_IMAGE
        imagePullPolicy: IfNotPresent
        ports:
        - containerPort: 80
---
apiVersion: v1
kind: Service
metadata:
  name: np-server
  namespace: $PROBE_NAMESPACE
spec:
  selector:
    app: np-server
  ports:
  - name: http
    port: 80
    targetPort: 80
---
apiVersion: v1
kind: Pod
metadata:
  name: np-client
  namespace: $PROBE_NAMESPACE
  labels:
    app: np-client
spec:
  restartPolicy: Never
  containers:
  - name: busybox
    image: $CLIENT_IMAGE
    imagePullPolicy: IfNotPresent
    command: ["sh", "-c", "sleep 3600"]
YAML
  cat >"$LOG_DIR/probe-default-deny.yaml" <<YAML
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: np-server-default-deny-ingress
  namespace: $PROBE_NAMESPACE
spec:
  podSelector:
    matchLabels:
      app: np-server
  policyTypes: [Ingress]
YAML
  cat >"$LOG_DIR/probe-allow-client.yaml" <<YAML
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: np-server-allow-client-ingress
  namespace: $PROBE_NAMESPACE
spec:
  podSelector:
    matchLabels:
      app: np-server
  policyTypes: [Ingress]
  ingress:
  - from:
    - podSelector:
        matchLabels:
          app: np-client
    ports:
    - protocol: TCP
      port: 80
YAML

  set +e
  kctl apply -f "$LOG_DIR/probe-namespace.yaml" >"$LOG_DIR/probe-namespace-apply.txt" 2>"$LOG_DIR/probe-namespace-apply.err"
  ns_rc=$?
  kctl apply -f "$LOG_DIR/probe-workloads.yaml" >"$LOG_DIR/probe-workloads-apply.txt" 2>"$LOG_DIR/probe-workloads-apply.err"
  workloads_rc=$?
  kctl -n "$PROBE_NAMESPACE" wait --for=condition=available deployment/np-server --timeout=120s >"$LOG_DIR/probe-server-wait.txt" 2>"$LOG_DIR/probe-server-wait.err"
  server_wait_rc=$?
  kctl -n "$PROBE_NAMESPACE" wait --for=condition=Ready pod/np-client --timeout=120s >"$LOG_DIR/probe-client-wait.txt" 2>"$LOG_DIR/probe-client-wait.err"
  client_wait_rc=$?
  set -e

  if [[ "$ns_rc" -ne 0 || "$workloads_rc" -ne 0 || "$server_wait_rc" -ne 0 || "$client_wait_rc" -ne 0 ]]; then
    json_log "probe" "isolated probe namespace and pods ready" "blocker" false "setup_failed" "ns=$ns_rc workloads=$workloads_rc server=$server_wait_rc client=$client_wait_rc" "probe-workloads-apply.err"
  else
    json_log "probe" "isolated probe namespace and pods ready" "info" true "ok" "$PROBE_NAMESPACE" "probe-workloads.yaml"

    set +e
    kctl -n "$PROBE_NAMESPACE" exec np-client -- wget -T 3 -qO- http://np-server >"$LOG_DIR/probe-baseline-wget.txt" 2>"$LOG_DIR/probe-baseline-wget.err"
    baseline_rc=$?
    kctl apply -f "$LOG_DIR/probe-default-deny.yaml" >"$LOG_DIR/probe-default-deny-apply.txt" 2>"$LOG_DIR/probe-default-deny-apply.err"
    deny_apply_rc=$?
    sleep 5
    kctl -n "$PROBE_NAMESPACE" exec np-client -- wget -T 3 -qO- http://np-server >"$LOG_DIR/probe-deny-wget.txt" 2>"$LOG_DIR/probe-deny-wget.err"
    deny_rc=$?
    kctl apply -f "$LOG_DIR/probe-allow-client.yaml" >"$LOG_DIR/probe-allow-client-apply.txt" 2>"$LOG_DIR/probe-allow-client-apply.err"
    allow_apply_rc=$?
    sleep 5
    kctl -n "$PROBE_NAMESPACE" exec np-client -- wget -T 3 -qO- http://np-server >"$LOG_DIR/probe-allow-wget.txt" 2>"$LOG_DIR/probe-allow-wget.err"
    allow_rc=$?
    set -e

    jq -n \
      --arg namespace "$PROBE_NAMESPACE" \
      --argjson baseline_rc "$baseline_rc" \
      --argjson deny_apply_rc "$deny_apply_rc" \
      --argjson deny_rc "$deny_rc" \
      --argjson allow_apply_rc "$allow_apply_rc" \
      --argjson allow_rc "$allow_rc" \
      '{
        namespace:$namespace,
        baseline_connectivity_passed:($baseline_rc == 0),
        default_deny_applied:($deny_apply_rc == 0),
        default_deny_blocked_client:($deny_rc != 0),
        allow_policy_applied:($allow_apply_rc == 0),
        allow_policy_restored_client:($allow_rc == 0),
        rc:{baseline:$baseline_rc, deny_apply:$deny_apply_rc, deny:$deny_rc, allow_apply:$allow_apply_rc, allow:$allow_rc}
      }' >"$LOG_DIR/enforcement-probe-summary.json"

    if [[ "$baseline_rc" -eq 0 && "$deny_apply_rc" -eq 0 && "$deny_rc" -ne 0 && "$allow_apply_rc" -eq 0 && "$allow_rc" -eq 0 ]]; then
      json_log "probe" "NetworkPolicy default deny and allow-list enforcement" "info" true "ok" "baseline pass, deny blocked, allow restored" "enforcement-probe-summary.json"
    else
      json_log "probe" "NetworkPolicy default deny and allow-list enforcement" "blocker" false "failed" "baseline=$baseline_rc deny_apply=$deny_apply_rc deny=$deny_rc allow_apply=$allow_apply_rc allow=$allow_rc" "enforcement-probe-summary.json"
    fi
  fi
else
  jq -n \
    --arg mode "$RUN_ENFORCEMENT_PROBE" \
    --argjson policy_capable_count "$CNI_POLICY_CAPABLE_COUNT" \
    '{
      skipped:true,
      mode:$mode,
      reason:(if $policy_capable_count == 0 then "policy-capable CNI missing" else "RUN_ENFORCEMENT_PROBE is false" end),
      policy_capable_count:$policy_capable_count
    }' >"$LOG_DIR/enforcement-probe-summary.json"
  if [[ "$CNI_POLICY_CAPABLE_COUNT" -eq 0 ]]; then
    json_log "probe" "NetworkPolicy default deny and allow-list enforcement" "blocker" false "skipped_cni_missing" "policy-capable CNI pods=0; negative probe would be a false pass on Flannel" "enforcement-probe-summary.json"
  else
    json_log "probe" "NetworkPolicy default deny and allow-list enforcement" "warn" false "skipped" "RUN_ENFORCEMENT_PROBE=$RUN_ENFORCEMENT_PROBE" "enforcement-probe-summary.json"
  fi
fi

TOTAL="$(jq -s 'length' "$REPORT")"
PASSED="$(jq -s '[.[] | select(.passed == true)] | length' "$REPORT")"
BLOCKERS="$(jq -s '[.[] | select(.passed == false and .severity == "blocker")] | length' "$REPORT")"
WARNINGS="$(jq -s '[.[] | select(.severity == "warn")] | length' "$REPORT")"
RESULT="pass"
if [[ "$BLOCKERS" -gt 0 ]]; then
  RESULT="blocked"
elif [[ "$WARNINGS" -gt 0 ]]; then
  RESULT="warn"
fi

jq -n \
  --arg run_id "$RUN_ID" \
  --arg result "$RESULT" \
  --arg report "$REPORT" \
  --arg local_report "$LOCAL_REPORT" \
  --arg network_policy_file "$NETWORK_POLICY_FILE" \
  --arg probe_namespace "$PROBE_NAMESPACE" \
  --argjson total "$TOTAL" \
  --argjson passed "$PASSED" \
  --argjson blockers "$BLOCKERS" \
  --argjson warnings "$WARNINGS" \
  --argjson cni_policy_capable_count "$CNI_POLICY_CAPABLE_COUNT" \
  --argjson flannel_marker_count "$FLANNEL_MARKER_COUNT" \
  --argjson live_network_policy_count "$LIVE_NP_COUNT" \
  --slurpfile checks "$REPORT" \
  '{
    run_id:$run_id,
    result:$result,
    report:$report,
    local_report:$local_report,
    network_policy_file:$network_policy_file,
    probe_namespace:$probe_namespace,
    total:$total,
    passed:$passed,
    blockers:$blockers,
    warnings:$warnings,
    cni_policy_capable_count:$cni_policy_capable_count,
    flannel_marker_count:$flannel_marker_count,
    live_network_policy_count:$live_network_policy_count,
    checks:$checks
  }' >"$SUMMARY"

cat >"$LOCAL_REPORT" <<MD
# NetworkPolicy Enforcement Preflight

- Run: \`$RUN_ID\`
- Result: \`$RESULT\`
- Checks: \`$PASSED/$TOTAL\` passed, \`$BLOCKERS\` blockers, \`$WARNINGS\` warnings
- Policy-capable CNI pods: \`$CNI_POLICY_CAPABLE_COUNT\`
- Flannel markers: \`$FLANNEL_MARKER_COUNT\`
- Live NetworkPolicy objects: \`$LIVE_NP_COUNT\`
- Enforcement probe namespace: \`$PROBE_NAMESPACE\`

This gate proves whether NetworkPolicy enforcement is available before GATE-P0-07
or GATE-P0-10 can be claimed. When a policy-capable CNI is present, the probe
uses an isolated namespace to prove three facts: baseline service connectivity,
default-deny ingress blocking, and allow-list ingress restoration.
MD

cp "$SUMMARY" "$SECURITY_DIR/network-policy-enforcement-preflight-latest.json"
cp "$LOCAL_REPORT" "$SECURITY_DIR/network-policy-enforcement-preflight-latest.md"
cp "$LOG_DIR/live-cni-policy-capability-summary.json" "$SECURITY_DIR/live-cni-policy-capability-summary-latest.json"
cp "$LOG_DIR/live-cni-policy-capability.json" "$SECURITY_DIR/live-cni-policy-capability-latest.json"
cp "$LOG_DIR/live-networkpolicies.json" "$SECURITY_DIR/network-policy-live-latest.json"
cp "$LOG_DIR/enforcement-probe-summary.json" "$SECURITY_DIR/network-policy-enforcement-probe-latest.json"

echo "network policy enforcement preflight result: $RESULT"
echo "summary: $SUMMARY"
echo "local report: $LOCAL_REPORT"

if [[ "$RESULT" == "blocked" && "$ALLOW_BLOCKERS" != "true" ]]; then
  exit 1
fi
