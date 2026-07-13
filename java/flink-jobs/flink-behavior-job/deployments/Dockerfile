################################################################################
# FILE: flink-jobs/flink-behavior-job/Dockerfile
# 
# Flink Behavior Detection Job Docker Image
# 基于官方 Flink 1.17 镜像，添加作业 JAR 和依赖
#
# 构建命令：
#   docker build -t traffic-analysis/flink-behavior-job:v1.0.0 .
#
# 多阶段构建：
#   Stage 1: Maven 构建 JAR
#   Stage 2: 基于官方 Flink 镜像部署
################################################################################

# ==============================================================================
# Stage 1: 构建阶段
# ==============================================================================
FROM maven:3.9.6-eclipse-temurin-17 AS builder

LABEL maintainer="traffic-analysis-team"
LABEL stage="builder"

WORKDIR /build

# 复制 Maven 配置（利用 Docker 缓存层）
COPY pom.xml .
COPY flink-jobs/pom.xml flink-jobs/
COPY flink-jobs/flink-common/pom.xml flink-jobs/flink-common/
COPY flink-jobs/flink-behavior-job/pom.xml flink-jobs/flink-behavior-job/

# 下载依赖（缓存层）
RUN mvn dependency:go-offline -B -pl flink-jobs/flink-behavior-job -am

# 复制源代码
COPY proto/ proto/
COPY flink-jobs/flink-common/src/ flink-jobs/flink-common/src/
COPY flink-jobs/flink-behavior-job/src/ flink-jobs/flink-behavior-job/src/

# 构建 JAR（跳过测试以加速构建）
RUN mvn clean package -B -pl flink-jobs/flink-behavior-job -am \
    -DskipTests \
    -Dmaven.javadoc.skip=true \
    -Dcheckstyle.skip=true

# ==============================================================================
# Stage 2: 运行阶段
# ==============================================================================
FROM flink:1.17.2-scala_2.12-java17

LABEL maintainer="traffic-analysis-team"
LABEL component="flink-behavior-job"
LABEL version="1.0.0"

# 环境变量
ENV FLINK_HOME=/opt/flink
ENV JOB_JAR_NAME=flink-behavior-job-1.0.0.jar
ENV MODEL_PATH=/opt/flink/models
ENV CHECKPOINT_PATH=/opt/flink/checkpoints
ENV TZ=Asia/Shanghai

# 设置时区
RUN ln -snf /usr/share/zoneinfo/$TZ /etc/localtime && echo $TZ > /etc/timezone

# 创建必要目录
RUN mkdir -p ${FLINK_HOME}/usrlib \
    && mkdir -p ${MODEL_PATH} \
    && mkdir -p ${CHECKPOINT_PATH} \
    && mkdir -p ${FLINK_HOME}/log \
    && chown -R flink:flink ${FLINK_HOME}/usrlib \
    && chown -R flink:flink ${MODEL_PATH} \
    && chown -R flink:flink ${CHECKPOINT_PATH} \
    && chown -R flink:flink ${FLINK_HOME}/log

# 复制作业 JAR
COPY --from=builder --chown=flink:flink \
    /build/flink-jobs/flink-behavior-job/target/${JOB_JAR_NAME} \
    ${FLINK_HOME}/usrlib/

# 复制配置文件
COPY --chown=flink:flink \
    flink-jobs/flink-behavior-job/src/main/resources/behavior-job.properties \
    ${FLINK_HOME}/conf/

# 复制额外的 Flink 配置
COPY --chown=flink:flink docker/flink-conf.yaml ${FLINK_HOME}/conf/flink-conf.yaml

# 复制启动脚本
COPY --chown=flink:flink docker/docker-entrypoint-job.sh /docker-entrypoint-job.sh
RUN chmod +x /docker-entrypoint-job.sh

# 添加 ClickHouse JDBC 驱动
RUN wget -q -O ${FLINK_HOME}/lib/clickhouse-jdbc-0.5.0-all.jar \
    https://repo1.maven.org/maven2/com/clickhouse/clickhouse-jdbc/0.5.0/clickhouse-jdbc-0.5.0-all.jar

# 添加 Kafka Connector（如果不在 fat jar 中）
# RUN wget -q -O ${FLINK_HOME}/lib/flink-sql-connector-kafka-1.17.2.jar \
#     https://repo1.maven.org/maven2/org/apache/flink/flink-sql-connector-kafka/1.17.2/flink-sql-connector-kafka-1.17.2.jar

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD curl -f http://localhost:8081/overview || exit 1

# 切换到 flink 用户
USER flink

WORKDIR ${FLINK_HOME}

# 默认入口点（可被 docker-compose 或 K8s 覆盖）
ENTRYPOINT ["/docker-entrypoint-job.sh"]

# 默认命令：启动 JobManager
CMD ["jobmanager"]
