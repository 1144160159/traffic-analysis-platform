# =============================================================================
# Flink PCAP Index Job Dockerfile
# 基于官方 Flink 1.17 镜像，适配 openEuler 22.03 LTS 部署环境
# =============================================================================

# ==================== 构建阶段 ====================
FROM maven:3.9-eclipse-temurin-17 AS builder

WORKDIR /build

# 复制 Maven 配置（利用 Docker 缓存）
COPY pom.xml .
COPY flink-jobs/pom.xml flink-jobs/
COPY flink-jobs/flink-common/pom.xml flink-jobs/flink-common/
COPY flink-jobs/flink-pcap-index-job/pom.xml flink-jobs/flink-pcap-index-job/

# 下载依赖（缓存层）
RUN mvn dependency:go-offline -pl flink-jobs/flink-pcap-index-job -am -B

# 复制源码
COPY proto/ proto/
COPY flink-jobs/flink-common/src flink-jobs/flink-common/src
COPY flink-jobs/flink-pcap-index-job/src flink-jobs/flink-pcap-index-job/src

# 构建 JAR（跳过测试加速构建）
RUN mvn clean package -pl flink-jobs/flink-pcap-index-job -am -DskipTests -B

# ==================== 运行阶段 ====================
FROM flink:1.17.2-scala_2.12-java17

LABEL maintainer="traffic-analysis-platform"
LABEL version="1.0.0"
LABEL description="Flink PCAP Index Job - PCAP 索引元数据处理作业"

# ==================== 环境变量 ====================
ENV FLINK_HOME=/opt/flink
ENV JOB_JAR_NAME=flink-pcap-index-job-1.0.0.jar
ENV JOB_CLASS=com.traffic.flink.pcap.PcapIndexJob

# JVM 调优参数（适配 2TB 内存节点）
ENV FLINK_PROPERTIES="\
jobmanager.memory.process.size: 2g\n\
taskmanager.memory.process.size: 4g\n\
taskmanager.numberOfTaskSlots: 4\n\
parallelism.default: 2\n\
state.backend: rocksdb\n\
state.backend.rocksdb.memory.managed: true\n\
state.checkpoints.dir: file:///opt/flink/checkpoints\n\
execution.checkpointing.interval: 30s\n\
execution.checkpointing.mode: AT_LEAST_ONCE\n\
execution.checkpointing.timeout: 60s\n\
restart-strategy: fixed-delay\n\
restart-strategy.fixed-delay.attempts: 3\n\
restart-strategy.fixed-delay.delay: 10s\n\
metrics.reporter.prom.factory.class: org.apache.flink.metrics.prometheus.PrometheusReporterFactory\n\
metrics.reporter.prom.port: 9249\n\
"

# ==================== 安装依赖 ====================
USER root

# 安装必要工具
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    netcat-openbsd \
    jq \
    && rm -rf /var/lib/apt/lists/*

# ==================== 复制 JAR 文件 ====================
COPY --from=builder /build/flink-jobs/flink-pcap-index-job/target/${JOB_JAR_NAME} ${FLINK_HOME}/usrlib/

# ==================== 复制配置文件 ====================
COPY flink-jobs/flink-pcap-index-job/src/main/resources/pcap-index-job.properties ${FLINK_HOME}/conf/
COPY flink-jobs/flink-pcap-index-job/src/main/resources/log4j2.xml ${FLINK_HOME}/conf/

# ==================== 下载 ClickHouse JDBC 驱动 ====================
RUN curl -fsSL -o ${FLINK_HOME}/lib/clickhouse-jdbc-0.5.0-all.jar \
    https://repo1.maven.org/maven2/com/clickhouse/clickhouse-jdbc/0.5.0/clickhouse-jdbc-0.5.0-all.jar

# ==================== 下载 Kafka Connector ====================
RUN curl -fsSL -o ${FLINK_HOME}/lib/flink-sql-connector-kafka-1.17.2.jar \
    https://repo1.maven.org/maven2/org/apache/flink/flink-sql-connector-kafka/1.17.2/flink-sql-connector-kafka-1.17.2.jar

# ==================== 下载 Prometheus Metrics Reporter ====================
RUN curl -fsSL -o ${FLINK_HOME}/lib/flink-metrics-prometheus-1.17.2.jar \
    https://repo1.maven.org/maven2/org/apache/flink/flink-metrics-prometheus/1.17.2/flink-metrics-prometheus-1.17.2.jar

# ==================== 创建目录 ====================
RUN mkdir -p ${FLINK_HOME}/checkpoints \
    && mkdir -p /var/log/flink \
    && chown -R flink:flink ${FLINK_HOME}/checkpoints \
    && chown -R flink:flink /var/log/flink \
    && chown -R flink:flink ${FLINK_HOME}/usrlib

# ==================== 健康检查脚本 ====================
COPY flink-jobs/flink-pcap-index-job/docker/scripts/healthcheck.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/healthcheck.sh

# ==================== 切换用户 ====================
USER flink

WORKDIR ${FLINK_HOME}

# ==================== 健康检查 ====================
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD /usr/local/bin/healthcheck.sh

# ==================== 默认命令 ====================
# 由 docker-compose 或 K8s 指定具体启动模式
CMD ["help"]
