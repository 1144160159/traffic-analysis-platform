#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MODE="${1:-quick}"

usage() {
  cat <<'EOF'
Usage: tests/run_tests.sh [quick|full|live|go|web|java|rust|proto]

quick  - Go unit tests, Web build, K8s pod health
full   - Go, Web, Java/Flink, Rust, Proto checks
live   - K8s/APISIX/DB-backed E2E smoke
EOF
}

kctl() {
  env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy kubectl "$@"
}

run_go() {
  cd "${ROOT_DIR}/go/control-plane"
  go test ./... -count=1
}

run_web() {
  cd "${ROOT_DIR}/web/ui"
  npm run lint:check
  npm run build
  npm run test -- --run
}

run_java() {
  cd "${ROOT_DIR}/java/flink-jobs"
  mvn test
}

run_rust() {
  cd "${ROOT_DIR}/rust/probe-agent"
  cargo test --workspace
}

run_proto() {
  cd "${ROOT_DIR}/proto"
  buf lint
  ./scripts/generate.sh
}

run_k8s_health() {
  kctl get pods -A --field-selector=status.phase!=Running,status.phase!=Succeeded
}

run_live() {
  cd "${ROOT_DIR}"
  ROUNDS="${ROUNDS:-100}" LOG_DIR="${LOG_DIR:-.artifacts/e2e}" tests/e2e/live_100_round_smoke.sh
}

case "${MODE}" in
  quick)
    run_go
    cd "${ROOT_DIR}/web/ui"
    npm run lint:check
    npm run build
    run_k8s_health
    ;;
  full)
    run_go
    run_web
    run_java
    run_rust
    run_proto
    ;;
  live)
    run_live
    ;;
  go)
    run_go
    ;;
  web)
    run_web
    ;;
  java)
    run_java
    ;;
  rust)
    run_rust
    ;;
  proto)
    run_proto
    ;;
  -h|--help|help)
    usage
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac
