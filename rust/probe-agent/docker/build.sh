#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")/.."
REGISTRY="${REGISTRY:-traffic}"
TAG="${TAG:-latest}"
IMAGE="${REGISTRY}/probe-agent:${TAG}"
echo "Building ${IMAGE}..."
docker build -f docker/Dockerfile -t "${IMAGE}" .
echo "Done. Push with: docker push ${IMAGE}"
