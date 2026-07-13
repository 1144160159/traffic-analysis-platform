# =============================================================================
# Flink Feature Job Dockerfile
# 基于官方 Flink 1.17 镜像，适配 openEuler 22.03 LTS 生产环境
# =============================================================================

ARG FLINK_VERSION=1.17.2
ARG SCALA_VERSION=2.12
ARG JAVA_VERSION=11

FROM flink:${FLINK_VERSION}-scala_${SCALA_VERSION}-java${JAVA_VERSION}

LABEL maintainer="traffic-analysis-platform"
LABEL version="3.0"
LABEL description="Flink Feature Extraction Job for Traffic Analysis Platform"

# ==================== 环境变量 ====================
ENV FLINK_HOME=/opt/flink
ENV FEATURE_JOB_HOME=/opt/feature-job
ENV TZ=UTC

# ==================== 系统依赖 ====================
USER root

RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    wget \
    netcat-openbsd \
    dnsutils \
    procps \
    jq \
    && rm -rf /var/lib/apt/lists/*

# ==================== 创建目录结构 ====================
RUN mkdir -p ${FEATURE_JOB_HOME}/lib \
    && mkdir -p ${FEATURE_JOB_HOME}/conf \
    && mkdir -p ${FEATURE_JOB_HOME}/plugins \
    && mkdir -p /var/log/flink \
    && mkdir -p /data/flink/checkpoints \
    && mkdir -p /data/flink/savepoints \
    && chown -R flink:flink ${FEATURE_JOB_HOME} \
    && chown -R flink:flink /var/log/flink \
    && chown -R flink:flink /data/flink

# ==================== 下载必要的连接器 ====================
# Kafka Connector
RUN wget -q -P ${FLINK_HOME}/lib/ \
    https://repo1.maven.org/maven2/org/apache/flink/flink-connector-kafka/1.17.2/flink-connector-kafka-1.17.2.jar

# JDBC Connector
RUN wget -q -P ${FLINK_HOME}/lib/ \
    https://repo1.maven.org/maven2/org/apache/flink/flink-connector-jdbc/3.1.0-1.17/flink-connector-jdbc-3.1.0-1.17.jar

# PostgreSQL Driver
RUN wget -q -P ${FLINK_HOME}/lib/ \
    https://repo1.maven.org/maven2/org/postgresql/postgresql/42.6.0/postgresql-42.6.0.jar

# ClickHouse JDBC Driver
RUN wget -q -P ${FLINK_HOME}/lib/ \
    https://repo1.maven.org/maven2/com/clickhouse/clickhouse-jdbc/0.4.6/clickhouse-jdbc-0.4.6-all.jar

# Protobuf Runtime
RUN wget -q -P ${FLINK_HOME}/lib/ \
    https://repo1.maven.org/maven2/com/google/protobuf/protobuf-java/3.24.0/protobuf-java-3.24.0.jar

# ==================== Flink 配置优化 ====================
COPY docker/flink-conf.yaml ${FLINK_HOME}/conf/flink-conf.yaml
COPY docker/log4j2.properties ${FLINK_HOME}/conf/log4j2.properties

# ==================== 复制应用程序 ====================
COPY target/flink-feature-job-*.jar ${FEATURE_JOB_HOME}/lib/
COPY src/main/resources/feature-job.properties ${FEATURE_JOB_HOME}/conf/

# ==================== 复制启动脚本 ====================
COPY docker/entrypoint.sh /docker-entrypoint.sh
COPY docker/submit-job.sh ${FEATURE_JOB_HOME}/submit-job.sh

RUN chmod +x /docker-entrypoint.sh \
    && chmod +x ${FEATURE_JOB_HOME}/submit-job.sh

# ==================== 健康检查 ====================
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD curl -f http://localhost:8081/overview || exit 1

# ==================== 切换用户 ====================
USER flink

WORKDIR ${FLINK_HOME}

ENTRYPOINT ["/docker-entrypoint.sh"]
CMD ["help"]
