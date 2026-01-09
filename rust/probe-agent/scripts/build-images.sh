#!/usr/bin/env bash
# rust/probe-agent/scripts/build-images.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

IMAGE_NAME="traffic-probe-agent"
IMAGE_TAG="${IMAGE_TAG:-latest}"
REGISTRY="${REGISTRY:-localhost:5000}"

log_info() {
    echo -e "\033[0;32m[INFO]\033[0m $1"
}

log_error() {
    echo -e "\033[0;31m[ERROR]\033[0m $1"
}

# 检测架构
ARCH=$(uname -m)
case "$ARCH" in
    x86_64)
        DOCKERFILE="Dockerfile.x86_64"
        PLATFORM="linux/amd64"
        ;;
    aarch64)
        DOCKERFILE="Dockerfile.aarch64"
        PLATFORM="linux/arm64"
        ;;
    *)
        log_error "Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

log_info "========================================="
log_info " Building Probe Agent Docker Image"
log_info "========================================="
log_info "Architecture: $ARCH"
log_info "Dockerfile: $DOCKERFILE"
log_info "Platform: $PLATFORM"
log_info "Image: $REGISTRY/$IMAGE_NAME:$IMAGE_TAG"
log_info "========================================="

cd "$PROJECT_ROOT/probe-agent"

# 构建镜像
log_info "Building Docker image..."
docker build \
    --platform "$PLATFORM" \
    -f "$DOCKERFILE" \
    -t "$REGISTRY/$IMAGE_NAME:$IMAGE_TAG" \
    -t "$REGISTRY/$IMAGE_NAME:$ARCH-$IMAGE_TAG" \
    ..

log_info "✓ Image built successfully"

# 推送镜像（可选）
if [ "${PUSH:-false}" = "true" ]; then
    log_info "Pushing image to registry..."
    docker push "$REGISTRY/$IMAGE_NAME:$IMAGE_TAG"
    docker push "$REGISTRY/$IMAGE_NAME:$ARCH-$IMAGE_TAG"
    log_info "✓ Image pushed successfully"
fi

log_info "========================================="
log_info " Build Complete"
log_info "========================================="
log_info ""
log_info "Run the image with:"
log_info "  docker run -v /path/to/config.yaml:/etc/probe-agent/config.yaml \\"
log_info "    --network host --privileged \\"
log_info "    $REGISTRY/$IMAGE_NAME:$IMAGE_TAG"
log_info ""