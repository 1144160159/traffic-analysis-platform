#!/usr/bin/env bash
set -euo pipefail

ALLOW_BLOCKERS=false \
RUN_ENFORCEMENT_PROBE=auto \
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-network-policy-enforcement-post-cni}" \
tests/e2e/live_network_policy_enforcement_preflight.sh
