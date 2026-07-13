#!/bin/bash
################################################################################
# 启动脚本 - Flink Alert Generator Job 本地开发环境
################################################################################

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_DIR"

echo "============================================================"
echo "  Flink Alert Generator Job - 本地开发环境"
echo "============================================================"

# 检查 Docker 和 Docker Compose
if ! command -v docker &> /dev/null; then
    echo "❌ Docker 未安装，请先安装 Docker"
    exit 1
fi

if ! docker compose version &> /dev/null && ! docker-compose version &> /dev/null; then
    echo "❌ Docker Compose 未安装，请先安装 Docker Compose"
    exit 1
fi

# 使用正确的 compose 命令
COMPOSE_CMD="docker compose"
if ! docker compose version &> /dev/null; then
    COMPOSE_CMD="docker-compose"
fi

# 解析参数
ACTION="${1:-up}"
BUILD_FLAG=""

case "$ACTION" in
    up)
        echo "🚀 启动服务..."
        if [[ "$2" == "--build" ]]; then
            BUILD_FLAG="--build"
        fi
        $COMPOSE_CMD up -d $BUILD_FLAG
        
        echo ""
        echo "⏳ 等待服务启动..."
        sleep 10
        
        echo ""
        echo "============================================================"
        echo "  服务启动完成！"
        echo "============================================================"
        echo ""
        echo "  📊 Flink Web UI:    http://localhost:8081"
        echo "  📈 Grafana:         http://localhost:3000 (admin/admin)"
        echo "  🔍 Prometheus:      http://localhost:9090"
        echo "  📨 Kafka UI:        http://localhost:8080"
        echo "  🗄️  ClickHouse:      http://localhost:8123"
        echo "  🔎 OpenSearch:      http://localhost:9200"
        echo ""
        echo "  查看日志: $COMPOSE_CMD logs -f flink-jobmanager"
        echo "============================================================"
        ;;
    
    down)
        echo "🛑 停止服务..."
        $COMPOSE_CMD down
        echo "✅ 服务已停止"
        ;;
    
    restart)
        echo "🔄 重启服务..."
        $COMPOSE_CMD restart
        echo "✅ 服务已重启"
        ;;
    
    logs)
        SERVICE="${2:-flink-jobmanager}"
        echo "📋 查看日志: $SERVICE"
        $COMPOSE_CMD logs -f "$SERVICE"
        ;;
    
    clean)
        echo "🧹 清理所有数据..."
        $COMPOSE_CMD down -v
        echo "✅ 数据已清理"
        ;;
    
    status)
        echo "📊 服务状态:"
        $COMPOSE_CMD ps
        ;;
    
    build)
        echo "🔨 构建镜像..."
        $COMPOSE_CMD build
        echo "✅ 构建完成"
        ;;
    
    *)
        echo "用法: $0 {up|down|restart|logs|clean|status|build}"
        echo ""
        echo "  up [--build]  - 启动服务 (可选: 重新构建)"
        echo "  down          - 停止服务"
        echo "  restart       - 重启服务"
        echo "  logs [服务名] - 查看日志 (默认: flink-jobmanager)"
        echo "  clean         - 清理所有数据和容器"
        echo "  status        - 查看服务状态"
        echo "  build         - 构建镜像"
        exit 1
        ;;
esac
