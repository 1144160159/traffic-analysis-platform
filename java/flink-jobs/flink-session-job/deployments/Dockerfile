# ==============================================================================
# Flink Session Job Dockerfile
# 基于官方 Flink 1.17.2 镜像
# ==============================================================================

FROM flink:1.17.2-scala_2.12-java11

# 维护者信息
LABEL maintainer="traffic-analysis-platform"
LABEL description="Flink Session Aggregation Job"
LABEL version="1.0.0"

# 设置工作目录
WORKDIR /opt/flink

# ==================== 环境变量 ====================
ENV FLINK_HOME=/opt/flink
ENV FLINK_JOB_PARALLELISM=12
ENV FLINK_TASK_MANAGER_MEMORY=8192m
ENV FLINK_JOB_MANAGER_MEMORY=2048m

# ==================== 安装依赖 ====================
# 切换到 root 用户安装系统依赖
USER root

# 安装必要的系统工具
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    curl \
    wget \
    vim \
    net-tools \
    dnsutils \
    procps \
    && rm -rf /var/lib/apt/lists/*

# ==================== 复制 JAR 包 ====================
# 复制应用 JAR（从构建上下文的 target 目录）
COPY target/flink-session-job-*.jar /opt/flink/usrlib/flink-session-job.jar

# ==================== 复制配置文件 ====================
# 复制作业配置文件
COPY src/main/resources/session-job.properties /opt/flink/conf/session-job.properties
COPY src/main/resources/log4j2.xml /opt/flink/conf/log4j2-session-job.xml

# ==================== Flink 配置优化 ====================
# 创建自定义 Flink 配置文件
RUN echo "# Session Job Flink Configuration" >> /opt/flink/conf/flink-conf.yaml && \
    echo "taskmanager.memory.process.size: 8192m" >> /opt/flink/conf/flink-conf.yaml && \
    echo "taskmanager.memory.managed.fraction: 0.4" >> /opt/flink/conf/flink-conf.yaml && \
    echo "taskmanager.memory.network.fraction: 0.1" >> /opt/flink/conf/flink-conf.yaml && \
    echo "state.backend: rocksdb" >> /opt/flink/conf/flink-conf.yaml && \
    echo "state.backend.incremental: true" >> /opt/flink/conf/flink-conf.yaml && \
    echo "state.backend.rocksdb.ttl.compaction.filter.enabled: true" >> /opt/flink/conf/flink-conf.yaml && \
    echo "execution.checkpointing.interval: 60000" >> /opt/flink/conf/flink-conf.yaml && \
    echo "execution.checkpointing.mode: EXACTLY_ONCE" >> /opt/flink/conf/flink-conf.yaml && \
    echo "execution.checkpointing.timeout: 600000" >> /opt/flink/conf/flink-conf.yaml && \
    echo "restart-strategy: failure-rate" >> /opt/flink/conf/flink-conf.yaml && \
    echo "restart-strategy.failure-rate.max-failures-per-interval: 3" >> /opt/flink/conf/flink-conf.yaml && \
    echo "restart-strategy.failure-rate.failure-rate-interval: 5 min" >> /opt/flink/conf/flink-conf.yaml && \
    echo "restart-strategy.failure-rate.delay: 30 s" >> /opt/flink/conf/flink-conf.yaml && \
    echo "metrics.reporters: prom" >> /opt/flink/conf/flink-conf.yaml && \
    echo "metrics.reporter.prom.factory.class: org.apache.flink.metrics.prometheus.PrometheusReporterFactory" >> /opt/flink/conf/flink-conf.yaml && \
    echo "metrics.reporter.prom.port: 9250-9260" >> /opt/flink/conf/flink-conf.yaml

# ==================== 下载额外依赖（Prometheus Reporter） ====================
# Prometheus Metrics Reporter
RUN wget -P /opt/flink/lib/ \
    https://repo1.maven.org/maven2/org/apache/flink/flink-metrics-prometheus/1.17.2/flink-metrics-prometheus-1.17.2.jar

# ClickHouse JDBC Driver
RUN wget -P /opt/flink/lib/ \
    https://repo1.maven.org/maven2/com/clickhouse/clickhouse-jdbc/0.4.6/clickhouse-jdbc-0.4.6-all.jar

# Kafka Connector
RUN wget -P /opt/flink/lib/ \
    https://repo1.maven.org/maven2/org/apache/flink/flink-connector-kafka/1.17.2/flink-connector-kafka-1.17.2.jar

# JDBC Connector
RUN wget -P /opt/flink/lib/ \
    https://repo1.maven.org/maven2/org/apache/flink/flink-connector-jdbc/3.1.0-1.17/flink-connector-jdbc-3.1.0-1.17.jar

# OpenSearch Connector (使用 Elasticsearch Connector)
RUN wget -P /opt/flink/lib/ \
    https://repo1.maven.org/maven2/org/apache/flink/flink-connector-elasticsearch7/3.0.1-1.17/flink-connector-elasticsearch7-3.0.1-1.17.jar

# ==================== 创建日志目录 ====================
RUN mkdir -p /opt/flink/log && \
    chown -R flink:flink /opt/flink/log && \
    chown -R flink:flink /opt/flink/usrlib && \
    chown -R flink:flink /opt/flink/conf

# ==================== 健康检查脚本 ====================
COPY docker/healthcheck.sh /opt/flink/bin/healthcheck.sh
RUN chmod +x /opt/flink/bin/healthcheck.sh && \
    chown flink:flink /opt/flink/bin/healthcheck.sh

# 切换回 flink 用户
USER flink

# ==================== 暴露端口 ====================
# JobManager WebUI: 8081
# JobManager RPC: 6123
# TaskManager RPC: 6122
# Prometheus Metrics: 9250-9260
EXPOSE 8081 6123 6122 9250-9260

# ==================== 健康检查 ====================
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
  CMD ["/opt/flink/bin/healthcheck.sh"]

# ==================== 入口点 ====================
# 使用官方镜像的 docker-entrypoint.sh
ENTRYPOINT ["/docker-entrypoint.sh"]

# 默认命令（可被 docker-compose 或 k8s 覆盖）
CMD ["help"]
