################################################################################
# Makefile for Flink Alert Generator Job
################################################################################

.PHONY: help build test docker-build docker-push start stop logs clean status test-data

# ==================== 变量定义 ====================
PROJECT_NAME := flink-alert-generator-job
VERSION := 1.0.0
DOCKER_REGISTRY ?= your-registry.example.com
DOCKER_IMAGE := $(DOCKER_REGISTRY)/$(PROJECT_NAME)
DOCKER_TAG := $(VERSION)

# Docker Compose 命令
COMPOSE := docker compose
ifeq ($(shell docker compose version 2>/dev/null),)
	COMPOSE := docker-compose
endif

# ==================== 帮助信息 ====================
help:
	@echo "============================================================"
	@echo "  Flink Alert Generator Job - Makefile"
	@echo "============================================================"
	@echo ""
	@echo "开发命令:"
	@echo "  make build          - 构建 Java JAR"
	@echo "  make test           - 运行单元测试"
	@echo "  make docker-build   - 构建 Docker 镜像"
	@echo ""
	@echo "本地环境:"
	@echo "  make start          - 启动本地开发环境"
	@echo "  make start-build    - 重新构建并启动"
	@echo "  make stop           - 停止本地环境"
	@echo "  make restart        - 重启本地环境"
	@echo "  make logs           - 查看 JobManager 日志"
	@echo "  make status         - 查看服务状态"
	@echo "  make clean          - 清理所有容器和数据"
	@echo ""
	@echo "测试:"
	@echo "  make test-data      - 发送测试数据"
	@echo ""
	@echo "部署:"
	@echo "  make docker-push    - 推送 Docker 镜像"
	@echo ""

# ==================== 构建 ====================
build:
	@echo "🔨 Building Java JAR..."
	mvn clean package -DskipTests -q
	@echo "✅ Build completed: target/$(PROJECT_NAME).jar"

test:
	@echo "🧪 Running tests..."
	mvn test

# ==================== Docker ====================
docker-build:
	@echo "🐳 Building Docker image: $(DOCKER_IMAGE):$(DOCKER_TAG)"
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .
	docker tag $(DOCKER_IMAGE):$(DOCKER_TAG) $(DOCKER_IMAGE):latest
	@echo "✅ Docker image built successfully"

docker-push: docker-build
	@echo "📤 Pushing Docker image: $(DOCKER_IMAGE):$(DOCKER_TAG)"
	docker push $(DOCKER_IMAGE):$(DOCKER_TAG)
	docker push $(DOCKER_IMAGE):latest
	@echo "✅ Docker image pushed successfully"

# ==================== 本地开发 ====================
start:
	@echo "🚀 Starting local environment..."
	$(COMPOSE) up -d
	@sleep 5
	@echo ""
	@echo "============================================================"
	@echo "  Services started successfully!"
	@echo "============================================================"
	@echo "  Flink Web UI:  http://localhost:8081"
	@echo "  Grafana:       http://localhost:3000 (admin/admin)"
	@echo "  Prometheus:    http://localhost:9090"
	@echo "  Kafka UI:      http://localhost:8080"
	@echo "============================================================"

start-build:
	@echo "🔨 Building and starting..."
	$(COMPOSE) up -d --build

stop:
	@echo "🛑 Stopping local environment..."
	$(COMPOSE) down
	@echo "✅ Services stopped"

restart:
	@echo "🔄 Restarting services..."
	$(COMPOSE) restart

logs:
	$(COMPOSE) logs -f flink-jobmanager

logs-tm:
	$(COMPOSE) logs -f flink-taskmanager

status:
	@echo "📊 Service status:"
	$(COMPOSE) ps

clean:
	@echo "🧹 Cleaning up..."
	$(COMPOSE) down -v --remove-orphans
	rm -rf target/
	@echo "✅ Cleanup completed"

# ==================== 测试 ====================
test-data:
	@echo "📤 Sending test data..."
	@chmod +x scripts/test-producer.sh
	./scripts/test-producer.sh 10 500

# ==================== 验收测试 ====================
acceptance-test: start
	@echo "⏳ Waiting for services to be ready..."
	@sleep 60
	@echo "🧪 Running acceptance tests..."
	@echo "  Checking Flink JobManager..."
	@curl -sf http://localhost:8081/overview > /dev/null && echo "  ✅ JobManager OK" || echo "  ❌ JobManager FAILED"
	@echo "  Checking Prometheus..."
	@curl -sf http://localhost:9090/-/healthy > /dev/null && echo "  ✅ Prometheus OK" || echo "  ❌ Prometheus FAILED"
	@echo "  Checking Grafana..."
	@curl -sf http://localhost:3000/api/health > /dev/null && echo "  ✅ Grafana OK" || echo "  ❌ Grafana FAILED"
	@echo "  Checking ClickHouse..."
	@curl -sf "http://localhost:8123/?query=SELECT%201" > /dev/null && echo "  ✅ ClickHouse OK" || echo "  ❌ ClickHouse FAILED"
	@echo "  Checking OpenSearch..."
	@curl -sf http://localhost:9200/_cluster/health > /dev/null && echo "  ✅ OpenSearch OK" || echo "  ❌ OpenSearch FAILED"
	@echo ""
	@echo "✅ Acceptance tests completed"
