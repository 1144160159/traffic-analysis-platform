# =============================================================================
# Flink Feature Job - Makefile
# =============================================================================

.PHONY: help build test docker-build docker-push up down logs submit \
        clean clean-all savepoint cancel status prometheus grafana

# ==================== 配置 ====================
IMAGE_NAME := flink-feature-job
IMAGE_TAG := latest
REGISTRY := docker.io/traffic-analysis

# ==================== 帮助 ====================
help: ## 显示帮助信息
	@echo "Flink Feature Job - Available Commands:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ==================== 构建 ====================
build: ## 构建 Java 项目
	@echo "Building Flink Feature Job..."
	mvn clean package -DskipTests

build-test: ## 构建并运行测试
	@echo "Building with tests..."
	mvn clean package

test: ## 运行测试
	@echo "Running tests..."
	mvn test

# ==================== Docker ====================
docker-build: build ## 构建 Docker 镜像
	@echo "Building Docker image..."
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) -f docker/Dockerfile .

docker-push: docker-build ## 推送 Docker 镜像
	@echo "Pushing Docker image..."
	docker tag $(IMAGE_NAME):$(IMAGE_TAG) $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)
	docker push $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

# ==================== Docker Compose ====================
up: ## 启动本地开发环境
	@echo "Starting local development environment..."
	docker-compose up -d

up-build: docker-build ## 重新构建并启动
	@echo "Rebuilding and starting..."
	docker-compose up -d --build

down: ## 停止本地开发环境
	@echo "Stopping local development environment..."
	docker-compose down

down-clean: ## 停止并清理数据
	@echo "Stopping and cleaning data..."
	docker-compose down -v --remove-orphans

restart: down up ## 重启环境

logs: ## 查看所有日志
	docker-compose logs -f

logs-flink: ## 查看 Flink 日志
	docker-compose logs -f jobmanager taskmanager

logs-job: ## 查看 Job Submitter 日志
	docker-compose logs -f job-submitter

# ==================== 作业管理 ====================
submit: ## 提交作业到集群
	@echo "Submitting job to Flink cluster..."
	docker-compose exec jobmanager /opt/feature-job/submit-job.sh submit

status: ## 查看作业状态
	@echo "Checking job status..."
	docker-compose exec jobmanager /opt/feature-job/submit-job.sh check
	@echo ""
	docker-compose exec jobmanager /opt/feature-job/submit-job.sh list

cancel: ## 取消作业 (使用: make cancel JOB_ID=xxx)
	@echo "Cancelling job $(JOB_ID)..."
	docker-compose exec jobmanager /opt/feature-job/submit-job.sh cancel $(JOB_ID)

savepoint: ## 创建 Savepoint (使用: make savepoint JOB_ID=xxx)
	@echo "Creating savepoint for job $(JOB_ID)..."
	docker-compose exec jobmanager /opt/feature-job/submit-job.sh savepoint $(JOB_ID)

stop: ## 停止作业并创建 Savepoint (使用: make stop JOB_ID=xxx)
	@echo "Stopping job $(JOB_ID) with savepoint..."
	docker-compose exec jobmanager /opt/feature-job/submit-job.sh stop $(JOB_ID)

# ==================== 监控 ====================
prometheus: ## 打开 Prometheus UI
	@echo "Opening Prometheus UI..."
	open http://localhost:9090 || xdg-open http://localhost:9090

grafana: ## 打开 Grafana UI
	@echo "Opening Grafana UI..."
	open http://localhost:3000 || xdg-open http://localhost:3000

flink-ui: ## 打开 Flink Web UI
	@echo "Opening Flink Web UI..."
	open http://localhost:8081 || xdg-open http://localhost:8081

kafka-ui: ## 打开 Kafka UI
	@echo "Opening Kafka UI..."
	open http://localhost:8080 || xdg-open http://localhost:8080

# ==================== 测试数据 ====================
generate-test-data: ## 生成测试数据
	@echo "Generating test data..."
	docker-compose exec kafka kafka-console-producer \
		--bootstrap-server localhost:9092 \
		--topic session.events.v1 < test/sample-sessions.json

consume-output: ## 消费输出数据
	@echo "Consuming output data..."
	docker-compose exec kafka kafka-console-consumer \
		--bootstrap-server localhost:9092 \
		--topic feature.stat.v1 \
		--from-beginning

consume-dlq: ## 消费 DLQ 数据
	@echo "Consuming DLQ data..."
	docker-compose exec kafka kafka-console-consumer \
		--bootstrap-server localhost:9092 \
		--topic dlq.feature-job \
		--from-beginning

# ==================== 数据库 ====================
psql: ## 连接 PostgreSQL
	docker-compose exec postgres psql -U postgres -d traffic

clickhouse: ## 连接 ClickHouse
	docker-compose exec clickhouse clickhouse-client

redis-cli: ## 连接 Redis
	docker-compose exec redis redis-cli

# ==================== 清理 ====================
clean: ## 清理构建产物
	@echo "Cleaning build artifacts..."
	mvn clean

clean-docker: ## 清理 Docker 资源
	@echo "Cleaning Docker resources..."
	docker-compose down -v --remove-orphans
	docker image prune -f

clean-all: clean clean-docker ## 清理所有
	@echo "All cleaned."

# ==================== 开发辅助 ====================
proto-gen: ## 生成 Protobuf 代码
	@echo "Generating Protobuf code..."
	cd ../../proto && buf generate

deps: ## 下载依赖
	@echo "Downloading dependencies..."
	mvn dependency:resolve

deps-tree: ## 显示依赖树
	mvn dependency:tree

# ==================== CI/CD ====================
ci-build: ## CI 构建
	@echo "CI Build..."
	mvn clean package -DskipTests -B

ci-test: ## CI 测试
	@echo "CI Test..."
	mvn test -B

ci-docker: ci-build docker-build ## CI Docker 构建

# ==================== 版本信息 ====================
version: ## 显示版本信息
	@echo "Flink Feature Job"
	@mvn help:evaluate -Dexpression=project.version -q -DforceStdout
	@echo ""
	@docker --version
	@docker-compose --version
