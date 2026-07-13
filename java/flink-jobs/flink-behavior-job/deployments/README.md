# Flink Behavior Detection Job - Deployment Guide

## Overview

This directory contains deployment configurations for the Flink Behavior Detection Job.

## Directory Structure

```
deployment/
├── docker/
│   ├── Dockerfile                        # Production Docker image
│   ├── docker-compose.yml                # Local development environment
│   ├── docker-entrypoint.sh              # Container entrypoint script
│   └── flink-conf.yaml                   # Flink configuration
├── kubernetes/
│   ├── namespace.yaml                    # K8s namespace
│   ├── configmap.yaml                    # Configuration
│   ├── deployment-jobmanager.yaml        # JobManager deployment
│   ├── deployment-taskmanager.yaml       # TaskManager deployment
│   ├── service.yaml                      # Services
│   └── ingress.yaml                      # Ingress (optional)
├── monitoring/
│   ├── prometheus-rules.yaml             # Prometheus alert rules
│   ├── grafana-dashboard.json            # Grafana dashboard
│   └── alertmanager-config.yaml          # AlertManager configuration
└── scripts/
    ├── build.sh                          # Build script
    ├── deploy-local.sh                   # Local deployment
    └── deploy-k8s.sh                     # Kubernetes deployment
```

## Quick Start

### 1. Local Development (Docker Compose)

```bash
# Build the job JAR
cd flink-jobs/flink-behavior-job
mvn clean package -DskipTests

# Start the environment
cd deployment/docker
docker-compose up -d

# Check logs
docker-compose logs -f flink-jobmanager

# Submit the job
docker-compose exec flink-jobmanager flink run \
  -d \
  -c com.traffic.flink.behavior.BehaviorDetectionJob \
  /opt/flink/usrlib/flink-behavior-job-1.0.0.jar \
  --kafka.brokers kafka:9092 \
  --kafka.input.topic feature.stat.v1 \
  --kafka.output.topic detections.behavior.v1

# Access Flink UI
open http://localhost:8081

# Stop environment
docker-compose down -v
```

### 2. Production Deployment (Kubernetes)

```bash
# Build Docker image
cd deployment/docker
docker build -t traffic-analysis/flink-behavior-job:v1.0.0 .

# Push to registry
docker tag traffic-analysis/flink-behavior-job:v1.0.0 \
  your-registry.com/traffic-analysis/flink-behavior-job:v1.0.0
docker push your-registry.com/traffic-analysis/flink-behavior-job:v1.0.0

# Deploy to Kubernetes
cd ../kubernetes
kubectl apply -f namespace.yaml
kubectl apply -f configmap.yaml
kubectl apply -f deployment-jobmanager.yaml
kubectl apply -f deployment-taskmanager.yaml
kubectl apply -f service.yaml

# Check status
kubectl get pods -n flink-behavior-job
kubectl logs -f -n flink-behavior-job deployment/flink-jobmanager

# Access Flink UI (port-forward)
kubectl port-forward -n flink-behavior-job service/flink-jobmanager-rest 8081:8081
open http://localhost:8081
```

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `KAFKA_BROKERS` | Kafka broker list | `localhost:9092` |
| `KAFKA_INPUT_TOPIC` | Input topic | `feature.stat.v1` |
| `KAFKA_OUTPUT_TOPIC` | Output topic | `detections.behavior.v1` |
| `CLICKHOUSE_URL` | ClickHouse URL | `localhost:8123` |
| `CLICKHOUSE_DATABASE` | ClickHouse database | `traffic` |
| `CLICKHOUSE_TABLE` | ClickHouse table | `detections_behavior_local` |
| `MODEL_PATH` | Model files path | `/opt/flink/models` |
| `MODEL_VERSION` | Model version | `v1.0` |
| `CHECKPOINT_PATH` | Checkpoint storage | `file:///opt/flink/checkpoints` |
| `PARALLELISM` | Job parallelism | `4` |

### Model Configuration

Enable/disable specific models:

```bash
# Enable all models
--model.enabled scan,tunnel,dga,encrypted,anomaly,c2,data_exfil,botnet,malware,phishing

# Enable only specific models
--model.enabled scan,tunnel,c2
```

### Threshold Configuration

Adjust detection thresholds per model:

```bash
--threshold.scan 0.7 \
--threshold.tunnel 0.75 \
--threshold.dga 0.8 \
--threshold.c2 0.75
```

## Monitoring

### Metrics Endpoints

- **Flink Web UI**: `http://jobmanager:8081`
- **Prometheus Metrics**: `http://jobmanager:9249/metrics`
- **TaskManager Metrics**: `http://taskmanager:9249/metrics`

### Key Metrics

| Metric | Description | Alert Threshold |
|--------|-------------|-----------------|
| `flink_jobmanager_job_uptime` | Job uptime | < 60s |
| `flink_jobmanager_job_numRestarts` | Restart count | > 3 |
| `flink_taskmanager_job_task_numRecordsIn` | Input records/sec | < 10 (no traffic) |
| `flink_jobmanager_job_lastCheckpointDuration` | Checkpoint duration | > 120s |
| `flink_jobmanager_job_numberOfFailedCheckpoints` | Failed checkpoints | > 0 |

### Grafana Dashboard

Import `monitoring/grafana-dashboard.json` to visualize:

- Job health status
- Throughput and latency
- Checkpoint metrics
- Backpressure indicators
- Model inference statistics

## Troubleshooting

### Job Not Starting

```bash
# Check JobManager logs
kubectl logs -f deployment/flink-jobmanager -n flink-behavior-job

# Check TaskManager logs
kubectl logs -f deployment/flink-taskmanager -n flink-behavior-job

# Check job submission
kubectl exec -it deployment/flink-jobmanager -n flink-behavior-job -- \
  flink list -a
```

### High Checkpoint Duration

```bash
# Increase checkpoint timeout
--checkpoint.timeout.ms 300000

# Increase checkpoint interval
--checkpoint.interval.ms 120000

# Enable incremental checkpoints (RocksDB)
state.backend.incremental: true
```

### High Backpressure

```bash
# Increase parallelism
--parallelism 8

# Increase TaskManager resources
taskmanager.memory.process.size: 8192m
taskmanager.numberOfTaskSlots: 8

# Enable async inference
--inference.async.enabled true
--inference.async.capacity 200
```

### Kafka Consumer Lag

```bash
# Check consumer group lag
kafka-consumer-groups --bootstrap-server kafka:9092 \
  --group flink-behavior-job \
  --describe

# Increase parallelism to match partition count
--parallelism 6  # if 6 Kafka partitions
```

## Performance Tuning

### Memory Configuration

```yaml
# For 2TB RAM node (Node-8)
jobmanager.memory.process.size: 4096m
taskmanager.memory.process.size: 16384m
taskmanager.memory.managed.size: 4096m
taskmanager.numberOfTaskSlots: 8
```

### Network Buffers

```yaml
taskmanager.network.memory.min: 512m
taskmanager.network.memory.max: 1024m
taskmanager.network.memory.buffers-per-channel: 4
```

### RocksDB Optimization

```yaml
state.backend.rocksdb.predefined-options: SPINNING_DISK_OPTIMIZED_HIGH_MEM
state.backend.rocksdb.memory.write-buffer-ratio: 0.5
state.backend.rocksdb.checkpoint.transfer.thread.num: 8
```

## Scaling

### Horizontal Scaling

```bash
# Scale TaskManagers
kubectl scale deployment flink-taskmanager -n flink-behavior-job --replicas=3

# Update parallelism
kubectl set env deployment/flink-jobmanager -n flink-behavior-job \
  PARALLELISM=12
```

### Vertical Scaling

```yaml
# Update resource requests/limits
resources:
  requests:
    memory: "16Gi"
    cpu: "4"
  limits:
    memory: "32Gi"
    cpu: "8"
```

## Backup and Recovery

### Savepoint Management

```bash
# Create savepoint
kubectl exec -it deployment/flink-jobmanager -n flink-behavior-job -- \
  flink savepoint <job-id> s3://bucket/savepoints/behavior-job

# Stop job with savepoint
kubectl exec -it deployment/flink-jobmanager -n flink-behavior-job -- \
  flink stop <job-id> -p s3://bucket/savepoints/behavior-job

# Restore from savepoint
kubectl exec -it deployment/flink-jobmanager -n flink-behavior-job -- \
  flink run -s s3://bucket/savepoints/behavior-job/<savepoint-path> \
  /opt/flink/usrlib/flink-behavior-job-1.0.0.jar
```

## Security

### Secrets Management

```bash
# Create secrets for sensitive data
kubectl create secret generic flink-behavior-secrets \
  --from-literal=clickhouse-password='xxx' \
  --from-literal=kafka-ssl-keystore-password='xxx' \
  -n flink-behavior-job
```

### Network Policies

See `kubernetes/network-policy.yaml` for isolation rules.

## References

- [Flink Documentation](https://nightlies.apache.org/flink/flink-docs-release-1.17/)
- [Flink on Kubernetes](https://nightlies.apache.org/flink/flink-docs-release-1.17/docs/deployment/resource-providers/native_kubernetes/)
- [Project Architecture](../../docs/architecture.md)
