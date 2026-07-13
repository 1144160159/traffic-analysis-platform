################################################################################
# Dockerfile for Flink Alert Generator Job
# 
# 基于官方 Flink 1.17 镜像
# 构建策略：多阶段构建，减小镜像体积
# 
# 硬件目标：
# - Node-8 (2TB RAM): JobManager + TaskManager
# - Node-9 (512GB RAM): TaskManager
################################################################################

# ==================== Stage 1: Build ====================
FROM maven:3.9.6-eclipse-temurin-11 AS builder

WORKDIR /build

# 复制 Maven 配置（利用 Docker 缓存）
COPY pom.xml .
COPY settings.xml* ./

# 下载依赖（独立层，提高缓存命中率）
RUN --mount=type=cache,target=/root/.m2/repository \
    mvn dependency:go-offline -B -q

# 复制源代码
COPY src ./src

# 构建 JAR（跳过测试加速构建）
RUN --mount=type=cache,target=/root/.m2/repository \
    mvn clean package -DskipTests -B -q \
    && mv target/flink-alert-generator-job-*.jar target/flink-alert-generator-job.jar

# 验证 JAR 文件
RUN ls -lh target/flink-alert-generator-job.jar && \
    jar -tf target/flink-alert-generator-job.jar | head -20

# ==================== Stage 2: Runtime ====================
FROM flink:1.17.2-scala_2.12-java11

# 维护者信息
LABEL maintainer="traffic-analysis-platform@example.com"
LABEL description="Flink Alert Generator Job - Convert detections to alerts with deduplication"
LABEL version="1.0.0"
LABEL component="flink-alert-generator-job"

# ==================== 环境变量 ====================
ENV FLINK_JOB_NAME="alert-generator-job" \
    # 作业配置
    FLINK_PARALLELISM="4" \
    SINK_PARALLELISM="2" \
    # Checkpoint 配置
    FLINK_CHECKPOINT_INTERVAL="60000" \
    FLINK_CHECKPOINT_TIMEOUT="120000" \
    FLINK_CHECKPOINT_DIR="file:///opt/flink/checkpoints" \
    # Kafka 配置
    KAFKA_BROKERS="kafka:9092" \
    KAFKA_INPUT_TOPIC_BEHAVIOR="detections.behavior.v1" \
    KAFKA_INPUT_TOPIC_BUSINESS="detections.business.v1" \
    KAFKA_OUTPUT_TOPIC="alerts.v1" \
    KAFKA_GROUP_ID="flink-alert-generator-job" \
    # ClickHouse 配置
    CLICKHOUSE_URL="clickhouse:8123" \
    CLICKHOUSE_DB="traffic" \
    CLICKHOUSE_ALERT_TABLE="alerts_local" \
    CLICKHOUSE_EVIDENCE_TABLE="evidence_local" \
    CLICKHOUSE_USER="default" \
    CLICKHOUSE_PASSWORD="" \
    # OpenSearch 配置
    OPENSEARCH_URL="http://opensearch:9200" \
    OPENSEARCH_INDEX="alerts" \
    OPENSEARCH_USER="admin" \
    OPENSEARCH_PASSWORD="admin" \
    # Arkime 配置
    ARKIME_URL="http://arkime:8005" \
    ARKIME_TIME_BUFFER="120" \
    # 去重配置
    DEDUP_WINDOW_MINUTES="10" \
    # Severity 阈值
    SEVERITY_CRITICAL="0.9" \
    SEVERITY_HIGH="0.7" \
    SEVERITY_MEDIUM="0.5" \
    SEVERITY_LOW="0.3" \
    # 功能开关
    ENABLE_BUSINESS_DETECTION="true" \
    DEBUG_PRINT="false" \
    SKIP_HEALTH_CHECK="false" \
    # JVM 配置（针对大内存节点优化）
    JAVA_TOOL_OPTIONS="-XX:+UseG1GC \
        -XX:MaxGCPauseMillis=200 \
        -XX:+ParallelRefProcEnabled \
        -XX:+ExplicitGCInvokesConcurrent \
        -XX:+HeapDumpOnOutOfMemoryError \
        -XX:HeapDumpPath=/opt/flink/log/heapdump.hprof \
        -Djava.security.egd=file:/dev/./urandom"

# ==================== 安装依赖 ====================
USER root

RUN apt-get update && apt-get install -y --no-install-recommends \
    netcat-openbsd \
    curl \
    jq \
    procps \
    && rm -rf /var/lib/apt/lists/* \
    && apt-get clean

# ==================== 创建目录结构 ====================
RUN mkdir -p /opt/flink/usrlib \
             /opt/flink/config \
             /opt/flink/checkpoints \
             /opt/flink/savepoints \
             /opt/flink/log \
    && chown -R flink:flink /opt/flink/usrlib \
                            /opt/flink/config \
                            /opt/flink/checkpoints \
                            /opt/flink/savepoints \
                            /opt/flink/log

# ==================== 复制 JAR ====================
COPY --from=builder --chown=flink:flink \
    /build/target/flink-alert-generator-job.jar \
    /opt/flink/usrlib/

# ==================== 复制配置文件 ====================
COPY --chown=flink:flink \
    src/main/resources/alert-generator-job.properties \
    /opt/flink/config/

# ==================== 复制启动脚本 ====================
COPY --chown=flink:flink docker/entrypoint.sh /opt/flink/bin/custom-entrypoint.sh
RUN chmod +x /opt/flink/bin/custom-entrypoint.sh

# ==================== 健康检查 ====================
HEALTHCHECK --interval=30s --timeout=10s --start-period=90s --retries=3 \
    CMD curl -sf http://localhost:8081/overview || exit 1

# ==================== 切换用户 ====================
USER flink

# ==================== 工作目录 ====================
WORKDIR /opt/flink

# ==================== 暴露端口 ====================
# 6123: JobManager RPC
# 6121-6122: TaskManager RPC
# 8081: Web UI
# 9249: Prometheus Metrics
EXPOSE 6123 6121 6122 8081 9249

# ==================== 入口点 ====================
ENTRYPOINT ["/opt/flink/bin/custom-entrypoint.sh"]

# 默认命令：standalone-job 模式
CMD ["standalone-job", \
     "--job-classname", "com.traffic.flink.alert.AlertGeneratorJob", \
     "-Djobmanager.rpc.address=localhost"]
