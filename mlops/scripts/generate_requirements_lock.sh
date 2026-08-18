#!/usr/bin/env bash
# Generate mlops/requirements.lock with --require-hashes compatible output.
#
# 代码审查 H40 收敛项：Dockerfile 的 PIP_REQUIRE_HASHES=1 门禁要求提交一份
# 包含全部传递依赖 --hash 的锁文件后才能启用 pip install --require-hashes。
# 本脚本必须在目标镜像同版本 Python（python:3.11）环境中执行：
#
#   docker run --rm -v "$PWD":/work -w /work python:3.11.11-slim \
#     bash /work/mlops/scripts/generate_requirements_lock.sh
#
# 产出 mlops/requirements.lock；提交后把 mlops/Dockerfile 中 PIP_REQUIRE_HASHES=1
# 分支从"拒绝构建"改为 `pip install --require-hashes -r /tmp/requirements.lock`。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
LOCK="$ROOT/mlops/requirements.lock"
REQ="$ROOT/mlops/requirements.txt"

if ! command -v pip-compile >/dev/null 2>&1; then
    pip install --quiet "pip-tools==7.4.1"
fi

pip-compile \
    --generate-hashes \
    --allow-unsafe \
    --output-file "$LOCK" \
    "$REQ"

echo "generated $LOCK ($(wc -l < "$LOCK") lines)"
echo "next: commit the lock, then enable --require-hashes in mlops/Dockerfile"
