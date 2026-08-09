#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP="$(mktemp -d)"
cleanup() { rm -rf "$TMP"; }
trap cleanup EXIT
export FAKE_KUBECTL_STATE="$TMP/state"
mkdir -p "$FAKE_KUBECTL_STATE"

cat >"$TMP/kubectl" <<'FAKE'
#!/usr/bin/env bash
set -euo pipefail
state=${FAKE_KUBECTL_STATE:?}
case "${1:-}" in
  get)
    [[ "${2:-}" == "secret" ]]
    name=${3:?}
    namespace=default
    output=""
    shift 3
    while [[ $# -gt 0 ]]; do
      case "$1" in
        -n) namespace=$2; shift 2;;
        -o) output=$2; shift 2;;
        *) shift;;
      esac
    done
    key=${output#jsonpath=\{.data.}
    key=${key%\}}
    path="$state/$namespace/$name/$key"
    [[ -f "$path" ]] || exit 1
    base64 -w0 "$path"
    ;;
  create)
    [[ "${2:-}" == "secret" && "${3:-}" == "generic" ]]
    name=${4:?}
    namespace=default
    shift 4
    args=("$@")
    for ((i=0; i<${#args[@]}; i++)); do
      if [[ "${args[$i]}" == "-n" ]]; then namespace=${args[$((i+1))]}; fi
    done
    for arg in "${args[@]}"; do
      case "$arg" in
        --from-literal=*)
          pair=${arg#--from-literal=}
          key=${pair%%=*}
          value=${pair#*=}
          encoded=$(printf '%s' "$value" | base64 -w0)
          printf '%s\t%s\t%s\t%s\n' "$namespace" "$name" "$key" "$encoded"
          ;;
      esac
    done
    ;;
  apply)
    while IFS=$'\t' read -r namespace name key encoded; do
      [[ -n "$namespace" ]] || continue
      mkdir -p "$state/$namespace/$name"
      printf '%s' "$encoded" | base64 -d >"$state/$namespace/$name/$key"
    done
    ;;
  *)
    echo "unsupported fake kubectl command: $*" >&2
    exit 2
    ;;
esac
FAKE
chmod +x "$TMP/kubectl"

export TRAFFIC_DEPLOY_LIB_ONLY=true
export KUBECTL="$TMP/kubectl"
# shellcheck source=/dev/null
source "$ROOT/deployments/kubernetes/deploy.sh"

sync_kafka_service_identity_secrets >/dev/null
first_digest="$(find "$FAKE_KUBECTL_STATE" -type f -print0 | sort -z | xargs -0 sha256sum | sha256sum | awk '{print $1}')"
sync_kafka_service_identity_secrets >/dev/null
second_digest="$(find "$FAKE_KUBECTL_STATE" -type f -print0 | sort -z | xargs -0 sha256sum | sha256sum | awk '{print $1}')"
[[ "$first_digest" == "$second_digest" ]]

python3 - "$FAKE_KUBECTL_STATE" "$ROOT/contracts/events/kafka-acl-catalog.v1.json" <<'PY'
import json
from pathlib import Path
import sys

state = Path(sys.argv[1])
catalog = json.loads(Path(sys.argv[2]).read_text())
expected = [
    item for item in catalog["principals"]
    if item.get("rollout_state") == "expand" and isinstance(item.get("credential"), dict)
]
assert len(expected) == 18, len(expected)
passwords = {}
aggregate_expected = {}
for item in expected:
    credential = item["credential"]
    secret = state / credential["namespace"] / credential["secret_name"]
    assert secret.is_dir(), secret
    username = (secret / "username").read_text()
    password = (secret / "password").read_text()
    assert username == item["principal"].removeprefix("User:")
    assert len(password) >= 32
    assert username not in passwords
    passwords[username] = password
    aggregate_expected[credential["password_env"]] = password
assert len(set(passwords.values())) == len(expected)
assert "traffic-audit-materializer" in passwords
assert "traffic-flink-session" in passwords

aggregate = state / "middleware/kafka-principal-credentials"
aggregate_passwords = {path.name: path.read_text() for path in aggregate.iterdir()}
assert aggregate_passwords == aggregate_expected
print(f"kafka_workload_identity_sync=pass workloads={len(expected)} idempotent=true")
PY
