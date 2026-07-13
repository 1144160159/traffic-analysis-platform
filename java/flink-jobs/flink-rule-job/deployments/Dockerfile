# ============================================================================
# Multi-stage Dockerfile for Flink Rule Engine Job
# ============================================================================

# Stage 1: Build
FROM maven:3.9-eclipse-temurin-11 AS builder

WORKDIR /build

# Copy parent POM
COPY pom.xml .

# Copy modules
COPY flink-common/ ./flink-common/
COPY flink-rule-job/ ./flink-rule-job/

# Copy proto definitions (assuming they're in a separate module)
COPY proto-java/ ./proto-java/ 2>/dev/null || true

# Build the project
RUN mvn clean package -DskipTests -pl flink-rule-job -am

# Stage 2: Runtime
FROM flink:1.17.2-scala_2.12-java11

# Metadata
LABEL maintainer="traffic-analysis-platform"
LABEL app="flink-rule-job"
LABEL version="1.0.0"

# Environment variables
ENV FLINK_HOME=/opt/flink
ENV JOB_JAR_NAME=flink-rule-job-1.0.0.jar
ENV JOB_CLASS=com.traffic.flink.rule.RuleJob

# Create directories
RUN mkdir -p /opt/flink/jobs /opt/flink/conf /opt/flink/logs

# Copy JAR from builder
COPY --from=builder /build/flink-rule-job/target/flink-rule-job-*.jar /opt/flink/jobs/${JOB_JAR_NAME}

# Copy configuration
COPY flink-rule-job/src/main/resources/rule-job.properties /opt/flink/conf/
COPY flink-rule-job/src/main/resources/log4j2.xml /opt/flink/conf/

# Copy sample rules (optional, for testing)
COPY flink-rule-job/src/main/resources/sample-rules/ /opt/flink/sample-rules/

# Set working directory
WORKDIR /opt/flink

# Expose Prometheus metrics port
EXPOSE 9250

# Health check (check if Flink job is running)
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
  CMD curl -f http://localhost:8081/jobs || exit 1

# Default command (will be overridden by K8s deployment)
CMD ["standalone-job", \
     "--job-classname", "com.traffic.flink.rule.RuleJob", \
     "--fromSavepoint", "/opt/flink/savepoints/latest"]
