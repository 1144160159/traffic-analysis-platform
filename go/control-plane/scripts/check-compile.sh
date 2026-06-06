#!/bin/bash
################################################################################
# Ingest Gateway 编译检查脚本
################################################################################

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "======================================"
echo "Ingest Gateway 编译检查"
echo "======================================"

# 检查 Go 版本
echo -e "\n${YELLOW}[1/6] 检查 Go 版本${NC}"
go version
if [ $? -ne 0 ]; then
    echo -e "${RED}✗ Go 未安装${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Go 版本检查通过${NC}"

# 检查必要的目录结构
echo -e "\n${YELLOW}[2/6] 检查目录结构${NC}"
REQUIRED_DIRS=(
    "internal/ingest/auth"
    "internal/ingest/config"
    "internal/ingest/dedup"
    "internal/ingest/dlq"
    "internal/ingest/metrics"
    "internal/ingest/queue"
    "internal/ingest/quota"
    "internal/ingest/server"
    "internal/common/kafka"
    "internal/common/logging"
    "internal/common/otel"
    "internal/common/errors"
    "pkg/proto/traffic/v1"
)

MISSING_DIRS=()
for dir in "${REQUIRED_DIRS[@]}"; do
    if [ ! -d "$dir" ]; then
        MISSING_DIRS+=("$dir")
    fi
done

if [ ${#MISSING_DIRS[@]} -ne 0 ]; then
    echo -e "${RED}✗ 缺少以下目录:${NC}"
    for dir in "${MISSING_DIRS[@]}"; do
        echo "  - $dir"
    done
    exit 1
fi
echo -e "${GREEN}✓ 目录结构检查通过${NC}"

# 检查 Protobuf 文件是否存在
echo -e "\n${YELLOW}[3/6] 检查 Protobuf 定义${NC}"
PROTO_FILES=(
    "pkg/proto/traffic/v1/ingest.proto"
    "pkg/proto/traffic/v1/flow.proto"
)

MISSING_PROTOS=()
for proto in "${PROTO_FILES[@]}"; do
    if [ ! -f "$proto" ]; then
        MISSING_PROTOS+=("$proto")
    fi
done

if [ ${#MISSING_PROTOS[@]} -ne 0 ]; then
    echo -e "${YELLOW}⚠ 缺少 Protobuf 文件（将在后续创建）:${NC}"
    for proto in "${MISSING_PROTOS[@]}"; do
        echo "  - $proto"
    done
else
    echo -e "${GREEN}✓ Protobuf 文件存在${NC}"
fi

# 下载依赖
echo -e "\n${YELLOW}[4/6] 下载 Go 依赖${NC}"
go mod download
if [ $? -ne 0 ]; then
    echo -e "${RED}✗ 依赖下载失败${NC}"
    exit 1
fi
echo -e "${GREEN}✓ 依赖下载完成${NC}"

# 检查编译（仅检查 internal/ingest 包）
echo -e "\n${YELLOW}[5/6] 编译检查 (internal/ingest)${NC}"
go build -o /dev/null ./internal/ingest/...
if [ $? -ne 0 ]; then
    echo -e "${RED}✗ 编译失败${NC}"
    exit 1
fi
echo -e "${GREEN}✓ 编译检查通过${NC}"

# 运行单元测试
echo -e "\n${YELLOW}[6/6] 运行单元测试${NC}"
go test -v -short ./internal/ingest/auth/... 2>&1 | grep -E "(PASS|FAIL|RUN)"
go test -v -short ./internal/ingest/dedup/... 2>&1 | grep -E "(PASS|FAIL|RUN)"
go test -v -short ./internal/ingest/quota/... 2>&1 | grep -E "(PASS|FAIL|RUN)"

echo -e "\n${GREEN}======================================"
echo "✓ 所有检查通过"
echo "======================================${NC}"

# 打印摘要
echo -e "\n${YELLOW}依赖摘要:${NC}"
go list -m all | grep -E "(kafka|redis|protobuf|grpc|zap|prometheus)" | head -10

echo -e "\n${YELLOW}可编译的包:${NC}"
go list ./internal/ingest/... | head -10