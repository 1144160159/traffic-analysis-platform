#!/usr/bin/env bash
# rust/probe-agent/scripts/setup-dev-openeuler.sh
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_info "========================================="
log_info " Probe Agent - openEuler Dev Setup"
log_info "========================================="

# 1. 检查系统版本
if [ -f /etc/os-release ]; then
    . /etc/os-release
    if [[ "$NAME" != "openEuler" ]]; then
        log_warn "Not running on openEuler, some features may not work"
    fi
fi

# 2. 安装系统依赖
log_info "Installing system dependencies..."
sudo dnf update -y
sudo dnf groupinstall -y "Development Tools"
sudo dnf install -y \
    clang \
    llvm \
    lld \
    elfutils-libelf-devel \
    zlib-devel \
    openssl-devel \
    kernel-devel \
    kernel-headers \
    pkg-config \
    git \
    protobuf-compiler \
    protobuf-devel

# 3. 安装 Rust
if ! command -v rustc &> /dev/null; then
    log_info "Installing Rust..."
    curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y --default-toolchain 1.75.0
    source "$HOME/.cargo/env"
else
    log_info "Rust already installed: $(rustc --version)"
fi

# 4. 安装 eBPF 工具链
log_info "Installing bpf-linker..."
cargo install bpf-linker --version 0.9.11 || log_warn "bpf-linker installation failed"

# 5. 安装 xtask
log_info "Setting up xtask..."
cd "$(dirname "$0")/.."
cargo build --package xtask

# 6. 配置内核参数（可选）
log_info "Checking kernel parameters..."
if [ "$(sysctl -n net.core.bpf_jit_enable)" != "1" ]; then
    log_warn "BPF JIT not enabled, enabling..."
    sudo sysctl -w net.core.bpf_jit_enable=1
fi

# 7. 创建必要的目录
log_info "Creating directories..."
sudo mkdir -p /etc/probe-agent/certs
sudo mkdir -p /var/lib/probe-agent/cache
sudo mkdir -p /var/lib/probe-agent/send-cache
sudo mkdir -p /var/log/probe-agent
sudo chown -R $USER:$USER /var/lib/probe-agent /var/log/probe-agent

# 8. 检查网卡是否支持 XDP
log_info "Checking XDP support..."
if command -v ethtool &> /dev/null; then
    for iface in $(ip -o link show | awk -F': ' '{print $2}' | grep -v lo); do
        driver=$(ethtool -i "$iface" 2>/dev/null | grep ^driver | awk '{print $2}')
        if [ -n "$driver" ]; then
            log_info "Interface: $iface, Driver: $driver"
        fi
    done
else
    log_warn "ethtool not found, cannot check XDP support"
fi

log_info "========================================="
log_info "✓ Development environment setup complete"
log_info "========================================="
log_info ""
log_info "Next steps:"
log_info "  1. Build eBPF: cargo xtask build-ebpf"
log_info "  2. Build app: cargo build --release"
log_info "  3. Run tests: cargo test"
log_info ""