#restart-ingest-gateway.sh
#!/bin/bash
set -e

echo "🔄 重启 Ingest Gateway..."

# 1. 强制停止
echo "📛 停止服务..."
pkill -9 ingest-gateway 2>/dev/null || echo "服务未运行"

# 等待端口释放
sleep 2

# 2. 检查端口
echo "🔍 检查端口..."
if lsof -i :50051 >/dev/null 2>&1; then
    echo "⚠️  端口 50051 仍被占用，强制释放..."
    lsof -ti:50051 | xargs kill -9 2>/dev/null || true
fi

# 3. 加载配置
echo "⚙️  加载配置..."
source config.env

# 4. 启动服务
echo "🚀 启动服务..."
mkdir -p logs
nohup ./bin/ingest-gateway > logs/ingest-gateway.log 2>&1 &

# 获取 PID
sleep 2
PID=$(pgrep -f ingest-gateway)

if [ -n "$PID" ]; then
    echo "✅ 服务已启动，PID: $PID"
    echo ""
    echo "服务端点："
    echo "  gRPC:    localhost:50051"
    echo "  Health:  localhost:8081"
    echo "  Metrics: localhost:9090"
    echo ""
    echo "查看日志: tail -f logs/ingest-gateway.log"
    echo "停止服务: kill $PID"
else
    echo "❌ 启动失败，查看日志："
    tail -20 logs/ingest-gateway.log
    exit 1
fi
