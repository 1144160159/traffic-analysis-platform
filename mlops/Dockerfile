# MLOps Training Container Image
# Pre-installed Python dependencies for fast Argo Workflow startup
#
# Usage:
#   docker build -t traffic/mlops-trainer:latest -f mlops/Dockerfile mlops/
#   argo submit --from workflowtemplate/mlops-training-template -p trainer-image=traffic/mlops-trainer:latest

FROM python:3.11-slim

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
COPY requirements.txt /tmp/requirements.txt
RUN pip install --no-cache-dir -r /tmp/requirements.txt && \
    pip install --no-cache-dir ruff mypy bandit

# Copy training scripts
COPY scripts/ /app/scripts/

# Default working directory
WORKDIR /app

# Smoke test: verify imports work
RUN python -c "import xgboost; import lightgbm; import sklearn; import pandas; import numpy; print('All imports OK')"

# Argo-compatible entrypoint (executed via workflow container spec)
ENTRYPOINT ["python"]
