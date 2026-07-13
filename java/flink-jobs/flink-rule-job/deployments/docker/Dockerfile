# ============================================================================
# Flink Rule Job Dockerfile
# 基于官方 Flink 1.17.2 镜像
# ============================================================================

# ==================== Stage 1: Builder ====================
FROM maven:3.9-eclipse-temurin-11 AS builder

LABEL stage=builder

WORKDIR /build

# 复制父 POM
COPY pom.xml ./pom.xml

# 复制模块 POM（利用 Docker 缓存）
COPY flink-common/pom.xml ./flink-common/pom.xml
COPY flink-rule-job/pom.xml ./flink-rule-job/pom.xml
COPY proto-java/pom.xml ./proto-java/pom.xml

# 下载依赖（利用缓存层）
RUN mvn dependency:go-offline -B -pl flink-rule-job -am || true

# 复制源代码
COPY flink-common/src ./flink-common/src
COPY flink-rule-job/src ./flink-rule-job/src
COPY proto-java/src ./proto-java/src

# 构建项目
RUN mvn clean package -DskipTests -pl flink-rule-job -am \
    && mv flink-rule-job/target/flink-rule-job-*.jar /build/flink-rule-job.jar

# ==================== Stage 2: Runtime ====================
FROM flink:1.17.2-scala_2.12-java11

# ==================== 元数据 ====================
LABEL maintainer="traffic-analysis-platform"
LABEL app="flink-rule-job"
LABEL version="1.0.0"
LABEL description="Dynamic Rule Engine using Flink Broadcast State Pattern"

# ==================== 环境变量 ====================
ENV FLINK_HOME=/opt/flink
ENV JOB_JAR=/opt/flink/usrlib/flink-rule-job.jar
ENV FLINK_CONF_DIR=/opt/flink/conf

# RocksDB 优化（针对 2TB 内存环境）
ENV ROCKSDB_SHAREDLIB_DIR=/opt/flink/lib/rocksdb

# JVM 优化参数
ENV FLINK_ENV_JAVA_OPTS="-XX:+UseG1GC \
    -XX:MaxGCPauseMillis=100 \
    -XX:+ParallelRefProcEnabled \
    -XX:+ExplicitGCInvokesConcurrent \
    -XX:+HeapDumpOnOutOfMemoryError \
    -XX:HeapDumpPath=/opt/flink/log/heapdump.hprof \
    -Dlog4j2.formatMsgNoLookups=true"

# ==================== 安装依赖 ====================
USER root

# 安装必要工具
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    jq \
    netcat-openbsd \
    dnsutils \
    procps \
    && rm -rf /var/lib/apt/lists/*

# ==================== 创建目录结构 ====================
RUN mkdir -p /opt/flink/usrlib \
    /opt/flink/log \
    /opt/flink/plugins/metrics-prometheus \
    /data/flink/checkpoints \
    /data/flink/savepoints \
    /data/flink/rocksdb \
    && chown -R flink:flink /opt/flink /data/flink

# ==================== 下载额外依赖 ====================
# Prometheus Metrics Reporter
RUN curl -fsSL https://repo1.maven.org/maven2/org/apache/flink/flink-metrics-prometheus/1.17.2/flink-metrics-prometheus-1.17.2.jar \
    -o /opt/flink/plugins/metrics-prometheus/flink-metrics-prometheus-1.17.2.jar

# ClickHouse JDBC Driver
RUN curl -fsSL https://repo1.maven.org/maven2/com/clickhouse/clickhouse-jdbc/0.4.6/clickhouse-jdbc-0.4.6-all.jar \
    -o /opt/flink/lib/clickhouse-jdbc-0.4.6-all.jar

# ==================== 复制应用 JAR ====================
COPY --from=builder /build/flink-rule-job.jar /opt/flink/usrlib/flink-rule-job.jar

# ==================== 复制配置文件 ====================
COPY docker/flink-conf.yaml /opt/flink/conf/flink-conf.yaml
COPY docker/log4j2.properties /opt/flink/conf/log4j2.properties
COPY src/main/resources/rule-job.properties /opt/flink/conf/rule-job.properties

# ==================== 复制启动脚本 ====================
COPY docker/docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

# ==================== 设置权限 ====================
RUN chown -R flink:flink /opt/flink

# ==================== 切换用户 ====================
USER flink

# ==================== 工作目录 ====================
WORKDIR /opt/flink

# ==================== 暴露端口 ====================
# 6123: RPC
# 6124: Blob Server
# 8081: Web UI
# 9250: Prometheus Metrics
EXPOSE 6123 6124 8081 9250

# ==================== 健康检查 ====================
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD curl -f http://localhost:8081/overview || exit 1

# ==================== 入口点 ====================
ENTRYPOINT ["/docker-entrypoint.sh"]
CMD ["help"]
