# MLOps Training Container Image
# Pre-installed Python dependencies for fast Argo Workflow startup
#
# Usage:
#   docker build -t traffic/mlops-trainer:latest -f mlops/Dockerfile mlops/
#   argo submit --from workflowtemplate/mlops-training-template -p trainer-image=traffic/mlops-trainer:latest
#
# 供应链完整性（代码审查 H40 收敛项）：
#   * 基础镜像固定到具体补丁版本 tag，并通过 BASE_IMAGE_DIGEST 钉死 sha256
#     digest（发布链路必须提供 digest，禁止 mutable tag 直跑）。
#   * 生产镜像不再安装 ruff/mypy/bandit（移入 mlops/requirements-dev.txt）。
#   * 哈希校验门禁：PIP_REQUIRE_HASHES=1 时使用已提交的
#     mlops/requirements.lock(mlops/scripts/generate_requirements_lock.sh
#     在目标 python:3.11 环境生成、含全部依赖 --hash)执行
#     `pip install --require-hashes`;锁变更必须随 requirements.txt 同步提交。

ARG BASE_IMAGE=python:3.11.11-slim
ARG BASE_IMAGE_DIGEST=
FROM ${BASE_IMAGE}${BASE_IMAGE_DIGEST:+@}${BASE_IMAGE_DIGEST}

LABEL org.opencontainers.image.title="MLOps Trainer"
LABEL org.opencontainers.image.description="Pre-built training container for traffic-analysis-platform MLOps pipeline"
LABEL org.opencontainers.image.source="https://github.com/1144160159/traffic-analysis-platform"

# Install system deps for lightgbm/xgboost native extensions
RUN apt-get update && apt-get install -y --no-install-recommends \
    libgomp1 \
    libstdc++6 \
    curl \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Install Python dependencies (layered for cache efficiency)
ARG PIP_INDEX_URL=https://pypi.org/simple
ARG PIP_REQUIRE_HASHES=0
COPY requirements.txt /tmp/requirements.txt
COPY requirements.lock /tmp/requirements.lock
RUN set -eu; \
    if [ "${PIP_REQUIRE_HASHES}" = "1" ]; then \
        pip install --no-cache-dir --index-url "${PIP_INDEX_URL}" \
            --require-hashes -r /tmp/requirements.lock; \
    else \
        pip install --no-cache-dir --index-url "${PIP_INDEX_URL}" -r /tmp/requirements.txt; \
    fi

# Copy training scripts
COPY scripts/ /app/scripts/

# Default working directory
WORKDIR /app

# Smoke test: verify imports work
RUN python -c "import xgboost; import lightgbm; import sklearn; import pandas; import numpy; print('All imports OK')"

# Argo-compatible entrypoint (executed via workflow container spec)
ENTRYPOINT ["python"]
