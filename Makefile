# traffic-analysis-platform Makefile
# Unified build, test, and deploy orchestration

SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c
MAKEFLAGS += --warn-undefined-variables

GO_DIR        := go/control-plane
JAVA_DIR      := java/flink-jobs
MLOPS_DIR     := mlops
PROTO_DIR     := proto
DEPLOY_DIR    := deployments/kubernetes

REGISTRY      ?= traffic
TAG           ?= latest

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

# ============================ ALL ============================

.PHONY: all
all: proto go-build java-build python-test ## Build everything

.PHONY: proto
proto: ## Generate protobuf code
	cd $(PROTO_DIR) && buf lint && ./scripts/generate.sh

# ============================ Go ============================

.PHONY: go-build
go-build: ## Build all Go services
	cd $(GO_DIR) && go build ./...

.PHONY: go-test
go-test: ## Run all Go tests
	cd $(GO_DIR) && go test ./... -count=1

.PHONY: go-test-mlops
go-test-mlops: ## Run Go MLOps tests only
	cd $(GO_DIR) && go test ./internal/rules/... -v -count=1

.PHONY: go-vet
go-vet: ## Run go vet
	cd $(GO_DIR) && go vet ./...

.PHONY: go-lint
go-lint: ## Lint Go code
	cd $(GO_DIR) && golangci-lint run ./...

.PHONY: go-clean
go-clean: ## Clean Go build cache
	cd $(GO_DIR) && go clean -cache -testcache

# ============================ Java ============================

.PHONY: java-build
java-build: ## Compile all Flink jobs
	cd $(JAVA_DIR) && mvn compile -q

.PHONY: java-test
java-test: ## Run all Flink tests
	cd $(JAVA_DIR) && mvn test

.PHONY: java-package
java-package: ## Package Flink JARs
	cd $(JAVA_DIR) && mvn package -DskipTests -q

.PHONY: java-clean
java-clean: ## Clean Java build
	cd $(JAVA_DIR) && mvn clean -q

# ============================ Python / MLOps ============================

.PHONY: python-test
python-test: ## Run MLOps Python tests
	cd $(MLOPS_DIR) && python -m pytest scripts/test_mlops.py -v --tb=short

.PHONY: python-lint
python-lint: ## Lint MLOps Python scripts
	cd $(MLOPS_DIR) && ruff check scripts/

.PHONY: python-typecheck
python-typecheck: ## Type-check Python scripts
	cd $(MLOPS_DIR) && mypy scripts/ --ignore-missing-imports

.PHONY: python-security
python-security: ## Security scan Python scripts
	cd $(MLOPS_DIR) && bandit -r scripts/ -ll

# ============================ Docker ============================

.PHONY: docker-build-mlops
docker-build-mlops: ## Build MLOps trainer image
	docker build -t $(REGISTRY)/mlops-trainer:$(TAG) -f $(MLOPS_DIR)/Dockerfile $(MLOPS_DIR)

.PHONY: docker-build-go
docker-build-go: ## Build all Go service images
	mkdir -p $(GO_DIR)/bin
	for svc in rule-manager alert-service auth-service graph-service asset-service ingest-gateway threat-intel-service; do \
		(cd $(GO_DIR) && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/$$svc ./cmd/$$svc) ; \
		docker build -t $(REGISTRY)/$$svc:$(TAG) \
			-f $(GO_DIR)/deployments/docker/Dockerfile.runtime \
			--build-arg SERVICE_NAME=$$svc \
			$(GO_DIR) ; \
	done
	docker build -t $(REGISTRY)/forensics-service:$(TAG) \
		-f $(GO_DIR)/deployments/docker/Dockerfile.forensics-service \
		$(GO_DIR)

.PHONY: docker-build-probe
docker-build-probe: ## Build Rust probe-agent image
	docker build -t $(REGISTRY)/probe-agent:$(TAG) -f rust/probe-agent/docker/Dockerfile rust/probe-agent

.PHONY: docker-build-web
docker-build-web: ## Build Web UI image
	docker build -t $(REGISTRY)/web-ui:$(TAG) -f web/ui/deployments/Dockerfile web/ui

.PHONY: docker-build-flink-log
docker-build-flink-log: ## Build Flink log job image
	cd $(JAVA_DIR) && mvn -pl flink-log-job -am package -DskipTests -q
	docker build -t $(REGISTRY)/flink-log-job:$(TAG) \
		-f $(JAVA_DIR)/flink-log-job/deployments/Dockerfile \
		$(JAVA_DIR)/flink-log-job

.PHONY: docker-push-mlops
docker-push-mlops: ## Push MLOps trainer image
	docker push $(REGISTRY)/mlops-trainer:$(TAG)

# ============================ Argo Workflows ============================

.PHONY: argo-lint
argo-lint: ## Validate Argo Workflow YAML
	argo lint $(MLOPS_DIR)/workflows/training-workflow.yaml
	argo lint $(MLOPS_DIR)/workflows/mlops-workflow-template.yaml
	argo lint $(MLOPS_DIR)/workflows/cron-training-workflow.yaml

.PHONY: argo-deploy
argo-deploy: ## Deploy Argo WorkflowTemplate and CronWorkflow
	kubectl apply -f $(MLOPS_DIR)/workflows/mlops-workflow-template.yaml
	kubectl apply -f $(MLOPS_DIR)/workflows/cron-training-workflow.yaml

.PHONY: argo-submit
argo-submit: ## Submit training workflow from template
	argo submit -n traffic-analysis \
		--from workflowtemplate/mlops-training-template \
		--generate-name mlops-manual- \
		-p model-type=xgboost \
		-p lookback-days=7

.PHONY: argo-list
argo-list: ## List recent MLOps workflows
	argo list -n traffic-analysis | head -20

.PHONY: argo-clean
argo-clean: ## Delete old workflows
	argo delete -n traffic-analysis --older 7d

# ============================ K8s ============================

.PHONY: k8s-deploy-go
k8s-deploy-go: ## Deploy Go services
	kubectl apply -f $(DEPLOY_DIR)/applications/go-services.yaml

.PHONY: k8s-update-configmap
k8s-update-configmap: ## Update MLOps scripts ConfigMap
	kubectl create configmap mlops-scripts \
		--namespace=traffic-analysis \
		--from-file=$(MLOPS_DIR)/scripts/extract_data.py \
		--from-file=$(MLOPS_DIR)/scripts/train_model.py \
		--from-file=$(MLOPS_DIR)/scripts/evaluate_model.py \
		--from-file=$(MLOPS_DIR)/scripts/register_model.py \
		--from-file=$(MLOPS_DIR)/requirements.txt \
		--dry-run=client -o yaml | kubectl apply -f -

.PHONY: k8s-rollout-go
k8s-rollout-go: ## Rollout restart Go services
	kubectl rollout restart deployment/rule-manager -n traffic-analysis

.PHONY: k8s-status
k8s-status: ## Show MLOps K8s resources
	-kubectl get pods -n traffic-analysis -l 'app in (rule-manager)' -o wide
	-kubectl get workflowtemplate -n traffic-analysis | grep mlops
	-kubectl get cronworkflow -n traffic-analysis

# ============================ Full Pipeline ============================

.PHONY: ci
ci: go-vet go-build go-test-mlops python-lint python-test argo-lint ## Full CI pipeline (local)

.PHONY: cd
cd: docker-build-mlops docker-build-go argo-deploy k8s-update-configmap ## Full CD pipeline (local)

# ============================ Unified Tests ============================

.PHONY: test-quick
test-quick: ## Run quick Go/Web/K8s test gate
	tests/run_tests.sh quick

.PHONY: test-full
test-full: ## Run full Go/Web/Java/Rust/Proto test gate
	tests/run_tests.sh full

.PHONY: test-live
test-live: ## Run K8s/APISIX/DB-backed live E2E smoke
	tests/run_tests.sh live
