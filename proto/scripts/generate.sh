#!/usr/bin/env bash
#
# proto/scripts/generate.sh
# Protobuf 代码生成脚本 - 为 Rust、Go、Java 生成代码
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROTO_DIR="$(dirname "$SCRIPT_DIR")"
PROJECT_ROOT="$(dirname "$PROTO_DIR")"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# ==================== 设置 Go 环境 ====================
setup_go_env() {
    export GOPATH="${GOPATH:-$HOME/go}"
    export PATH="$PATH:$GOPATH/bin:/usr/local/go/bin"
}

# ==================== 安装 Go 插件 ====================
install_go_plugins() {
    log_info "安装 Go protoc 插件..."
    
    if ! command -v go &> /dev/null; then
        log_error "Go 未安装，请先安装 Go 1.21+"
        echo "  安装命令: yum install -y golang  或  apt install -y golang-go"
        exit 1
    fi
    
    log_info "  Go 版本: $(go version)"
    
    # 安装 protoc-gen-go
    if ! command -v protoc-gen-go &> /dev/null; then
        log_info "  安装 protoc-gen-go..."
        go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
    fi
    
    # 安装 protoc-gen-go-grpc
    if ! command -v protoc-gen-go-grpc &> /dev/null; then
        log_info "  安装 protoc-gen-go-grpc..."
        go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
    fi
    
    # 再次验证
    if command -v protoc-gen-go &> /dev/null && command -v protoc-gen-go-grpc &> /dev/null; then
        log_success "Go 插件安装完成"
        log_info "  protoc-gen-go: $(which protoc-gen-go)"
        log_info "  protoc-gen-go-grpc: $(which protoc-gen-go-grpc)"
    else
        log_error "Go 插件安装失败"
        log_error "请手动执行:"
        echo "  export PATH=\"\$PATH:\$HOME/go/bin\""
        echo "  go install google.golang.org/protobuf/cmd/protoc-gen-go@latest"
        echo "  go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest"
        exit 1
    fi
}

# ==================== 安装 buf ====================
install_buf() {
    if command -v buf &> /dev/null; then
        return 0
    fi
    
    log_info "安装 buf..."
    
    local buf_version="1.28.1"
    local os_type="Linux"
    local arch_type="x86_64"
    
    # 检测架构
    case "$(uname -m)" in
        aarch64|arm64) arch_type="aarch64" ;;
        x86_64) arch_type="x86_64" ;;
    esac
    
    curl -sSL "https://github.com/bufbuild/buf/releases/download/v${buf_version}/buf-${os_type}-${arch_type}" \
        -o /tmp/buf
    
    chmod +x /tmp/buf
    
    if [ -w /usr/local/bin ]; then
        mv /tmp/buf /usr/local/bin/buf
    else
        sudo mv /tmp/buf /usr/local/bin/buf
    fi
    
    log_success "buf 安装完成: $(buf --version)"
}

# ==================== 依赖检查 ====================
check_dependencies() {
    log_info "检查依赖工具..."
    
    # 设置 Go 环境
    setup_go_env
    
    # 安装 buf (如果需要)
    install_buf
    
    # 检查 protoc
    if ! command -v protoc &> /dev/null; then
        log_error "protoc 未安装"
        echo "  openEuler/CentOS: yum install -y protobuf-compiler"
        echo "  Ubuntu/Debian:    apt install -y protobuf-compiler"
        exit 1
    fi
    log_info "  protoc: $(protoc --version)"
    
    # 安装 Go 插件 (如果需要)
    install_go_plugins
    
    log_success "所有依赖检查通过"
}

# ==================== 清理旧文件 ====================
clean_generated() {
    log_info "清理旧的生成文件..."
    
    # Go
    local go_proto_dir="${PROJECT_ROOT}/go/control-plane/pkg/proto/traffic/v1"
    if [ -d "$go_proto_dir" ]; then
        rm -f "$go_proto_dir"/*.pb.go 2>/dev/null || true
        log_info "  已清理 Go 生成文件"
    fi
    
    # Java
    local java_proto_dir="${PROJECT_ROOT}/java/flink-jobs/flink-common/src/main/java/com/traffic/proto/v1"
    if [ -d "$java_proto_dir" ]; then
        rm -f "$java_proto_dir"/*.java 2>/dev/null || true
        log_info "  已清理 Java 生成文件"
    fi
    
    # Rust (proto-gen)
    local rust_proto_dir="${PROJECT_ROOT}/rust/probe-agent/proto-gen/src"
    if [ -d "$rust_proto_dir" ]; then
        rm -f "$rust_proto_dir"/traffic.v1.rs 2>/dev/null || true
        rm -f "$rust_proto_dir"/traffic.v1.tonic.rs 2>/dev/null || true
        log_info "  已清理 Rust 生成文件"
    fi
    
    log_success "清理完成"
}

# ==================== 创建输出目录 ====================
create_output_dirs() {
    log_info "创建输出目录..."
    
    mkdir -p "${PROJECT_ROOT}/go/control-plane/pkg/proto/traffic/v1"
    mkdir -p "${PROJECT_ROOT}/java/flink-jobs/flink-common/src/main/java/com/traffic/proto/v1"
    mkdir -p "${PROJECT_ROOT}/rust/probe-agent/proto-gen/src"
    
    log_success "输出目录已创建"
}

# ==================== 生成 Go 和 Java 代码 ====================
generate_go_java() {
    log_info "生成 Go 和 Java 代码 (使用 buf)..."
    
    cd "$PROTO_DIR"
    
    # 使用 buf 生成
    buf generate
    
    log_success "Go 和 Java 代码生成完成"
}

# ==================== 生成 Rust 代码 ====================
generate_rust() {
    log_info "生成 Rust 代码 (使用 prost-build)..."
    
    local rust_proto_gen_dir="${PROJECT_ROOT}/rust/probe-agent/proto-gen"
    
    if [ ! -f "${rust_proto_gen_dir}/Cargo.toml" ]; then
        log_warn "Rust proto-gen 项目不存在，跳过 Rust 代码生成"
        return 0
    fi
    
    cd "$rust_proto_gen_dir"
    
    # 通过 cargo build 触发 build.rs 生成代码
    if command -v cargo &> /dev/null; then
        cargo build --release 2>&1 | grep -v "Compiling" || true
        log_success "Rust 代码生成完成"
    else
        log_warn "cargo 不可用，跳过 Rust 代码生成"
    fi
}

# ==================== 验证生成结果 ====================
verify_output() {
    log_info "验证生成结果..."
    
    local errors=0
    
    # 验证 Go
    local go_dir="${PROJECT_ROOT}/go/control-plane/pkg/proto/traffic/v1"
    local go_files=(
        "common.pb.go"
        "flow.pb.go"
        "session.pb.go"
        "feature.pb.go"
        "detection.pb.go"
        "alert.pb.go"
        "pcap.pb.go"
        "ingest.pb.go"
        "campaign.pb.go"
        "ingest_grpc.pb.go"
    )
    
    echo ""
    log_info "Go 文件检查:"
    for f in "${go_files[@]}"; do
        if [ -f "${go_dir}/${f}" ]; then
            echo -e "  ${GREEN}✓${NC} $f"
        else
            echo -e "  ${RED}✗${NC} $f 未生成"
            ((errors++))
        fi
    done
    
    # 验证 Java
    local java_dir="${PROJECT_ROOT}/java/flink-jobs/flink-common/src/main/java/com/traffic/proto/v1"
    local java_files=(
        "EventHeader.java"
        "FiveTuple.java"
        "FlowEvent.java"
        "SessionEvent.java"
        "FeatureStatV1.java"
        "DetectionEvent.java"
        "Alert.java"
        "PcapIndexMeta.java"
        "Campaign.java"
        "IngestServiceGrpc.java"
    )
    
    echo ""
    log_info "Java 文件检查:"
    for f in "${java_files[@]}"; do
        if [ -f "${java_dir}/${f}" ]; then
            echo -e "  ${GREEN}✓${NC} $f"
        else
            echo -e "  ${RED}✗${NC} $f 未生成"
            ((errors++))
        fi
    done
    
    # 验证 Rust (如果存在)
    echo ""
    log_info "Rust 文件检查:"
    local rust_lib="${PROJECT_ROOT}/rust/probe-agent/proto-gen/src/lib.rs"
    if [ -f "$rust_lib" ]; then
        echo -e "  ${GREEN}✓${NC} lib.rs"
    else
        echo -e "  ${YELLOW}!${NC} Rust proto-gen 需要通过 cargo build 生成"
    fi
    
    echo ""
    if [ $errors -gt 0 ]; then
        log_error "验证失败: $errors 个文件未生成"
        exit 1
    fi
    
    log_success "所有文件验证通过"
}

# ==================== 统计生成结果 ====================
print_summary() {
    echo ""
    echo "=========================================="
    echo "           代码生成统计"
    echo "=========================================="
    
    # Go
    local go_count=$(find "${PROJECT_ROOT}/go/control-plane/pkg/proto" -name "*.pb.go" 2>/dev/null | wc -l)
    echo "  Go:    ${go_count} 个文件"
    
    # Java
    local java_count=$(find "${PROJECT_ROOT}/java/flink-jobs/flink-common/src/main/java/com/traffic/proto" -name "*.java" 2>/dev/null | wc -l)
    echo "  Java:  ${java_count} 个文件"
    
    # Rust
    local rust_count=$(find "${PROJECT_ROOT}/rust/probe-agent/proto-gen/src" -name "*.rs" 2>/dev/null | wc -l)
    echo "  Rust:  ${rust_count} 个文件"
    
    echo "=========================================="
    echo ""
}

# ==================== 主流程 ====================
main() {
    echo ""
    echo "╔══════════════════════════════════════════════╗"
    echo "║   Traffic Analysis Platform - Proto Gen      ║"
    echo "╚══════════════════════════════════════════════╝"
    echo ""
    
    check_dependencies
    clean_generated
    create_output_dirs
    generate_go_java
    generate_rust
    verify_output
    print_summary
    
    log_success "所有代码生成完成!"
    echo ""
}

# 执行
main "$@"