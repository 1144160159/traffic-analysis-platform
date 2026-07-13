#!/bin/bash
# =============================================================================
# 构建所有 Flink Jobs
# =============================================================================

set -e

cd "$(dirname "$0")/.."

echo "=============================================="
echo "Building Flink Jobs"
echo "=============================================="

# 验证 Proto 代码存在
echo "Checking proto generated code..."
if [ ! -d "flink-common/src/main/java/com/traffic/proto" ]; then
    echo "ERROR: Proto generated code not found!"
    echo "Please run proto generation first."
    exit 1
fi

# 构建所有模块
echo "Building all modules..."
mvn clean package -DskipTests

# 验证 JAR 文件
echo ""
echo "Build artifacts:"
find . -name "*.jar" -path "*/target/*" | grep -v "original" | while read jar; do
    echo "  $(ls -lh "$jar" | awk '{print $5, $9}')"
done

echo ""
echo "=============================================="
echo "Build completed successfully!"
echo "=============================================="