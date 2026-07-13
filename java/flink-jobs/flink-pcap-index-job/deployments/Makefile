# =============================================================================
# Flink PCAP Index Job - Makefile
# =============================================================================
#
# 常用命令:
#   make build      - 构建 Docker 镜像
#   make up         - 启动所有服务
#   make down       - 停止所有服务
#   make submit     - 提交 Flink 作业
#   make logs       - 查看日志
#   make clean      - 清理数据
#
# =============================================================================

.PHONY: help build up down logs submit clean test status

# 默认目标
.DEFAULT_GOAL := help

# 项目配置
PROJECT_NAME := pcap-index
COMPOSE_FILE := docker-compose.yml
IMAGE_NAME := traffic-platform/flink-pcap-index-job
IMAGE_TAG := latest

# 颜色
GREEN := \033[0;32m
YELLOW := \033[1;33m
BLUE := \033[0;34m
NC := \033[0m

# ==================== 帮助 ====================
help: ## 显示帮助信息
	@echo ""
	@echo "$(BLUE)Flink PCAP Index Job - 开发环境命令$(NC)"
	@echo ""
	@echo "$(GREEN)可用命令:$(NC)"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(YELLOW)%-15s$(NC) %s\n", $$1, $$2}'
	@echo ""

# ==================== 构建 ====================
build: ## 构建 Docker 镜像
	@echo "$(BLUE)构建 Docker 镜像...$(NC)"
	docker-compose -f $(COMPOSE_FILE) build
	@echo "$(GREEN)构建完成!$(NC)"

build-no-cache: ## 无缓存构建 Docker 镜像
	@echo "$(BLUE)无缓存构建 Docker 镜像...$(NC)"
	docker-compose -f $(COMPOSE_FILE) build --no-cache
	@echo "$(GREEN)构建完成!$(NC)"

# ==================== 服务管理 ====================
up: ## 启动所有服务
	@echo "$(BLUE)启动所有服务...$(NC)"
	docker-compose -f $(COMPOSE_FILE) up -d
	@echo "$(GREEN)服务已启动!$(NC)"
	@echo ""
	@echo "访问地址:"
	@echo "  Flink Web UI:  http://localhost:8081"
	@echo "  Prometheus:    http://localhost:9090"
	@echo "  Grafana:       http://localhost:3000 (admin/admin)"
	@echo "  Kafka UI:      http://localhost:8080"

up-build: ## 构建并启动所有服务
	@echo "$(BLUE)构建并启动所有服务...$(NC)"
	docker-compose -f $(COMPOSE_FILE) up -d --build

down: ## 停止所有服务
	@echo "$(BLUE)停止所有服务...$(NC)"
	docker-compose -f $(COMPOSE_FILE) down
	@echo "$(GREEN)服务已停止!$(NC)"

restart: ## 重启所有服务
	@echo "$(BLUE)重启所有服务...$(NC)"
	docker-compose -f $(COMPOSE_FILE) restart

restart-flink: ## 仅重启 Flink 服务
	@echo "$(BLUE)重启 Flink 服务...$(NC)"
	docker-compose -f $(COMPOSE_FILE) restart flink-jobmanager flink-taskmanager

# ==================== 日志 ====================
logs: ## 查看所有日志
	docker-compose -f $(COMPOSE_FILE) logs -f

logs-flink: ## 查看 Flink 日志
	docker-compose -f $(COMPOSE_FILE) logs -f flink-jobmanager flink-taskmanager

logs-kafka: ## 查看 Kafka 日志
	docker-compose -f $(COMPOSE_FILE) logs -f kafka

logs-clickhouse: ## 查看 ClickHouse 日志
	docker-compose -f $(COMPOSE_FILE) logs -f clickhouse

# ==================== 作业管理 ====================
submit: ## 提交 Flink 作业
	@echo "$(BLUE)提交 Flink 作业...$(NC)"
	./scripts/submit-job.sh local

submit-detached: ## 后台提交 Flink 作业
	@echo "$(BLUE)后台提交 Flink 作业...$(NC)"
	docker-compose -f $(COMPOSE_FILE) exec -T flink-jobmanager \
		flink run --detached \
		--class com.traffic.flink.pcap.PcapIndexJob \
		/opt/flink/usrlib/flink-pcap-index-job-1.0.0.jar \
		--kafka.brokers kafka:29092 \
		--kafka.input.topic pcap.index.v1 \
		--clickhouse.url clickhouse:8123 \
		--clickhouse.database traffic \
		--clickhouse.table pcap_index_local \
		--debug.print true

cancel-jobs: ## 取消所有运行中的作业
	@echo "$(BLUE)取消所有运行中的作业...$(NC)"
	@curl -sf http://localhost:8081/jobs | jq -r '.jobs[] | select(.status == "RUNNING") | .id' | \
		xargs -I {} curl -X PATCH "http://localhost:8081/jobs/{}?mode=cancel"
	@echo "$(GREEN)已取消所有作业!$(NC)"

# ==================== 状态检查 ====================
status: ## 检查服务状态
	@echo "$(BLUE)服务状态:$(NC)"
	@docker-compose -f $(COMPOSE_FILE) ps
	@echo ""
	@echo "$(BLUE)Flink 作业状态:$(NC)"
	@curl -sf http://localhost:8081/jobs 2>/dev/null | jq '.jobs' || echo "Flink 未就绪"

health: ## 健康检查
	@echo "$(BLUE)健康检查:$(NC)"
	@echo -n "  Kafka:       "
	@docker-compose -f $(COMPOSE_FILE) exec -T kafka kafka-topics --bootstrap-server localhost:9092 --list > /dev/null 2>&1 && echo "$(GREEN)OK$(NC)" || echo "$(YELLOW)NOT READY$(NC)"
	@echo -n "  ClickHouse:  "
	@docker-compose -f $(COMPOSE_FILE) exec -T clickhouse clickhouse-client --query "SELECT 1" > /dev/null 2>&1 && echo "$(GREEN)OK$(NC)" || echo "$(YELLOW)NOT READY$(NC)"
	@echo -n "  Flink:       "
	@curl -sf http://localhost:8081/overview > /dev/null 2>&1 && echo "$(GREEN)OK$(NC)" || echo "$(YELLOW)NOT READY$(NC)"
	@echo -n "  Prometheus:  "
	@curl -sf http://localhost:9090/-/ready > /dev/null 2>&1 && echo "$(GREEN)OK$(NC)" || echo "$(YELLOW)NOT READY$(NC)"
	@echo -n "  Grafana:     "
	@curl -sf http://localhost:3000/api/health > /dev/null 2>&1 && echo "$(GREEN)OK$(NC)" || echo "$(YELLOW)NOT READY$(NC)"

# ==================== 测试 ====================
test: ## 运行集成测试
	@echo "$(BLUE)运行集成测试...$(NC)"
	docker-compose -f $(COMPOSE_FILE) --profile test up test-producer

test-produce: ## 发送测试消息到 Kafka
	@echo "$(BLUE)发送测试消息...$(NC)"
	@echo '{"tenant_id":"test-tenant","probe_id":"probe-001","file_key":"s3://test/pcap/test.pcap","ts_start":1705320000000,"ts_end":1705323600000,"byte_size":1073741824}' | \
		docker-compose -f $(COMPOSE_FILE) exec -T kafka \
		kafka-console-producer --bootstrap-server localhost:9092 --topic pcap.index.v1
	@echo "$(GREEN)测试消息已发送!$(NC)"

test-consume: ## 消费 DLQ 消息
	@echo "$(BLUE)消费 DLQ 消息...$(NC)"
	docker-compose -f $(COMPOSE_FILE) exec -T kafka \
		kafka-console-consumer --bootstrap-server localhost:9092 --topic dlq.pcap-index-job --from-beginning --max-messages 10

# ==================== 清理 ====================
clean: ## 停止服务并清理数据卷
	@echo "$(BLUE)停止服务并清理数据...$(NC)"
	docker-compose -f $(COMPOSE_FILE) down -v
	@echo "$(GREEN)清理完成!$(NC)"

clean-images: ## 清理构建的镜像
	@echo "$(BLUE)清理构建的镜像...$(NC)"
	docker rmi $(IMAGE_NAME):$(IMAGE_TAG) 2>/dev/null || true
	docker image prune -f

# ==================== 开发工具 ====================
shell-flink: ## 进入 Flink 容器
	docker-compose -f $(COMPOSE_FILE) exec flink-jobmanager bash

shell-kafka: ## 进入 Kafka 容器
	docker-compose -f $(COMPOSE_FILE) exec kafka bash

shell-clickhouse: ## 进入 ClickHouse 容器
	docker-compose -f $(COMPOSE_FILE) exec clickhouse clickhouse-client

# ==================== 监控 ====================
metrics: ## 查看 Prometheus 指标
	@echo "$(BLUE)Flink Metrics:$(NC)"
	@curl -sf http://localhost:9249/metrics | grep -E "^pcap_|^clickhouse_|^dlq_" | head -30

alerts: ## 查看当前告警
	@echo "$(BLUE)当前告警:$(NC)"
	@curl -sf http://localhost:9090/api/v1/alerts | jq '.data.alerts'
