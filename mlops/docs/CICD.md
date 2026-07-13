# MLOps CI/CD Pipeline

## Overview

```
┌──────────────────────────────────────────────────────────────────────┐
│                    MLOps CI/CD Pipeline                                │
├──────────────────────────────────────────────────────────────────────┤
│                                                                        │
│  CI (mlops-ci.yml)                    CD (mlops-cd.yml)               │
│  ┌─────────────────────┐              ┌─────────────────────────┐    │
│  │ Python Lint & Test  │              │ Docker Build & Push      │    │
│  │   ruff + mypy + bandit│            │   rule-manager + trainer │    │
│  │   pytest (17 tests)  │              └───────────┬─────────────┘    │
│  ├─────────────────────┤              ┌─────────────▼───────────┐    │
│  │ Go Build & Test     │              │ Deploy Argo Workflows    │    │
│  │   vet + build + test │              │   WorkflowTemplate       │    │
│  │   (8 orchestrator)   │              │   CronWorkflow           │    │
│  ├─────────────────────┤              │   ConfigMap update       │    │
│  │ Argo YAML Validate  │              └───────────┬─────────────┘    │
│  │   argo lint + YAML   │              ┌─────────────▼───────────┐    │
│  ├─────────────────────┤              │ Deploy Go Service        │    │
│  │ Java Compile Check  │              │   kubectl set image      │    │
│  │   mvn compile        │              │   rollout status         │    │
│  ├─────────────────────┤              │   health check           │    │
│  │ CI Gate              │              └───────────┬─────────────┘    │
│  │   all must pass      │              ┌─────────────▼───────────┐    │
│  └─────────────────────┘              │ Smoke Test (optional)    │    │
│                                        │   argo submit dry-run     │    │
│  Triggers:                             └─────────────────────────┘    │
│  - push to main (mlops/** paths)                                       │
│  - pull_request to main (mlops/** paths)    Triggers:                  │
│  - manual (workflow_dispatch)               - push to main             │
│                                              - manual (workflow_dispatch)│
└──────────────────────────────────────────────────────────────────────┘
```

## Files

| File | Purpose |
|------|---------|
| `.github/workflows/mlops-ci.yml` | CI pipeline: lint, type-check, test, validate |
| `.github/workflows/mlops-cd.yml` | CD pipeline: build images, deploy Argo, deploy Go |
| `mlops/Dockerfile` | Pre-built MLOps trainer image (fast Argo startup) |
| `Makefile` | Unified local build/test/deploy commands |
| `mlops/docs/CICD.md` | This document |

## CI Pipeline: 4 Checks + Gate

### 1. Python (pytest + ruff + mypy + bandit)

```bash
make python-test       # Run 17 unit tests
make python-lint       # ruff check (E,F,W,I,N)
make python-typecheck  # mypy (optional, deps not fully typed)
make python-security   # bandit security scan
```

### 2. Go (vet + build + test)

```bash
make go-vet            # go vet
make go-build          # go build ./...
make go-test-mlops     # orchestrator tests (8 pass)
```

### 3. Argo YAML Validation

```bash
make argo-lint         # argo lint for each workflow
```

### 4. Java Compile

```bash
make java-build        # mvn compile for behavior job
```

## CD Pipeline: 4 Stages

### Stage 1: Build Docker Images

| Image | Dockerfile | Registry Tag |
|-------|-----------|-------------|
| `traffic/rule-manager` | `go/.../Dockerfile.rule-manager` | `:sha-xxxxx` + `:latest` |
| `traffic/mlops-trainer` | `mlops/Dockerfile` | `:sha-xxxxx` + `:latest` |

### Stage 2: Deploy Argo Workflows

```bash
make argo-deploy       # kubectl apply WorkflowTemplate + CronWorkflow
make k8s-update-configmap  # Update mlops-scripts ConfigMap
```

### Stage 3: Deploy Go Service

```bash
make k8s-deploy-go     # kubectl apply go-services.yaml
make k8s-rollout-go    # kubectl rollout restart
```

### Stage 4: Smoke Test (optional)

Submits an Argo workflow in dry-run mode to verify template validity.

## Required GitHub Secrets

| Secret | Used By | Purpose |
|--------|---------|---------|
| `KUBE_CONFIG` | CD: deploy-argo, deploy-go | K8s cluster access |
| `REGISTRY_USERNAME` | CD: build | Container registry login |
| `REGISTRY_PASSWORD` | CD: build | Container registry login |

## Local Development Workflow

```bash
# Full CI pipeline (before PR)
make ci

# Full CD pipeline (after merge to main)
make cd

# Quick iteration on Go orchestrator
make go-test-mlops

# Quick iteration on Python scripts
make python-test

# Docker build + push
REGISTRY=my-registry/traffic TAG=v1.0 make docker-build-mlops docker-push-mlops
```

## Argo Workflow Template Update Workflow

```bash
# 1. Edit workflow template
vim mlops/workflows/mlops-workflow-template.yaml

# 2. Validate
make argo-lint

# 3. Deploy
make argo-deploy

# 4. Submit test run
make argo-submit

# 5. Monitor
argo logs -n traffic-analysis @latest --follow
```

## Rollback

```bash
# Rollback Go rule-manager deployment
kubectl rollout undo deployment/rule-manager -n traffic-analysis

# Rollback Argo WorkflowTemplate
kubectl apply -f mlops/workflows/mlops-workflow-template.yaml  # re-apply previous version

# Rollback ConfigMap
kubectl rollout undo deployment/rule-manager -n traffic-analysis
```

## Roadmap

- [ ] Add Argo Events (EventSource + Sensor) for Kafka-driven workflow triggers
- [ ] Add model performance monitoring in CD (compare new model vs baseline)
- [ ] Add A/B test deployment workflow (canary model deployment)
- [ ] Add Flink JAR deployment automation
- [ ] Add GitHub Environments for staging/production promotion
- [ ] Add Slack notification on CD success/failure
