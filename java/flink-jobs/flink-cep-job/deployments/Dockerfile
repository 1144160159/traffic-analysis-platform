# =============================================================================
# Flink CEP Job Dockerfile
# 基于官方 Flink 镜像，添加 CEP Job JAR 和依赖
# =============================================================================

ARG FLINK_VERSION=1.17.2
ARG SCALA_VERSION=2.12

FROM flink:${FLINK_VERSION}-scala_${SCALA_VERSION}-java11 AS builder

# 构建阶段：编译 CEP Job
WORKDIR /build

# 安装 Maven
RUN apt-get update && \
    apt-get install -y maven && \
    rm -rf /var/lib/apt/lists/*

# 复制项目文件
COPY pom.xml .
COPY flink-jobs/pom.xml flink-jobs/
COPY flink-jobs/flink-common/pom.xml flink-jobs/flink-common/
COPY flink-jobs/flink-cep-job/pom.xml flink-jobs/flink-cep-job/

# 下载依赖（利用 Docker 缓存）
RUN mvn dependency:go-offline -pl flink-jobs/flink-cep-job -am -B

# 复制源代码
COPY proto/ proto/
COPY flink-jobs/flink-common/src/ flink-jobs/flink-common/src/
COPY flink-jobs/flink-cep-job/src/ flink-jobs/flink-cep-job/src/

# 编译打包
RUN mvn clean package -pl flink-jobs/flink-cep-job -am -DskipTests -B

# =============================================================================
# 运行时镜像
# =============================================================================
FROM flink:${FLINK_VERSION}-scala_${SCALA_VERSION}-java11

LABEL maintainer="Traffic Analysis Platform Team"
LABEL description="Flink CEP Correlation Job for Alert Campaign Detection"
LABEL version="1.0.0"

# 环境变量
ENV FLINK_HOME=/opt/flink
ENV JOB_JAR_PATH=/opt/flink/usrlib/flink-cep-job.jar

# 安装必要工具
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        curl \
        jq \
        netcat-openbsd \
        dumb-init && \
    rm -rf /var/lib/apt/lists/*

# 创建目录
RUN mkdir -p /opt/flink/usrlib /opt/flink/plugins/metrics-prometheus /var/log/flink

# 复制 JAR 文件
COPY --from=builder /build/flink-jobs/flink-cep-job/target/flink-cep-job-*-jar-with-dependencies.jar ${JOB_JAR_PATH}

# 复制配置文件
COPY flink-jobs/flink-cep-job/src/main/resources/cep-job.properties /opt/flink/conf/cep-job.properties
COPY flink-jobs/flink-cep-job/src/main/resources/log4j2.xml /opt/flink/conf/log4j2.xml

# 添加 Prometheus 指标插件
RUN curl -fsSL https://repo1.maven.org/maven2/org/apache/flink/flink-metrics-prometheus/${FLINK_VERSION}/flink-metrics-prometheus-${FLINK_VERSION}.jar \
    -o /opt/flink/plugins/metrics-prometheus/flink-metrics-prometheus.jar

# 添加 ClickHouse JDBC 驱动
RUN curl -fsSL https://repo1.maven.org/maven2/com/clickhouse/clickhouse-jdbc/0.4.6/clickhouse-jdbc-0.4.6-all.jar \
    -o /opt/flink/lib/clickhouse-jdbc.jar

# 复制自定义 Flink 配置
COPY flink-jobs/flink-cep-job/docker/flink-conf.yaml /opt/flink/conf/flink-conf.yaml

# 复制启动脚本
COPY flink-jobs/flink-cep-job/docker/docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD curl -f http://localhost:8081/overview || exit 1

# 暴露端口
# 8081: Web UI
# 6123: RPC
# 6124: Blob Server
# 6125: Query Server
# 9249: Prometheus Metrics
EXPOSE 8081 6123 6124 6125 9249

# 使用 dumb-init 作为 PID 1
ENTRYPOINT ["/usr/bin/dumb-init", "--"]
CMD ["/docker-entrypoint.sh"]
