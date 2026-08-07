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
SOURCE_REVISION ?= unknown
FLINK_JOB_MODULE ?=
FLINK_APPLICATION_TAG ?=
FLINK_APPLICATION_IMAGE ?=
FLINK_SAVEPOINT_MANIFEST ?=
FLINK_ENDPOINTS_MANIFEST ?=

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
	for svc in rule-manager alert-service auth-service graph-service asset-service ingest-gateway threat-intel-service audit-materializer; do \
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

.PHONY: docker-build-probe-control-smoke
docker-build-probe-control-smoke: ## Build isolated F-PROBE G2 smoke image
	mkdir -p $(GO_DIR)/bin
	cd $(GO_DIR) && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
		go build -trimpath -ldflags="-s -w -buildid=" \
		-o bin/probe-control-smoke ./cmd/probe-control-smoke
	docker build --label org.opencontainers.image.revision=$(SOURCE_REVISION) \
		-t $(REGISTRY)/probe-control-smoke:$(TAG) \
		-f $(GO_DIR)/deployments/docker/Dockerfile.runtime \
		--build-arg SERVICE_NAME=probe-control-smoke \
		$(GO_DIR)

.PHONY: docker-build-web
docker-build-web: ## Build Web UI image
	docker build -t $(REGISTRY)/web-ui:$(TAG) -f web/ui/deployments/Dockerfile web/ui

.PHONY: docker-build-flink-log
docker-build-flink-log: ## Build Flink log job image
	cd $(JAVA_DIR) && mvn -pl flink-log-job -am package -DskipTests -q
	docker build -t $(REGISTRY)/flink-log-job:$(TAG) \
		-f $(JAVA_DIR)/flink-log-job/deployments/Dockerfile \
		$(JAVA_DIR)/flink-log-job

.PHONY: docker-build-flink-application
docker-build-flink-application: ## Build one Flink Application image; set FLINK_JOB_MODULE and immutable candidate tag
	test -n "$(FLINK_JOB_MODULE)"
	test -n "$(FLINK_APPLICATION_TAG)"
	case "$(FLINK_JOB_MODULE)" in \
		flink-session-job|flink-feature-job|flink-rule-job|flink-behavior-job|flink-alert-generator-job|flink-cep-job|flink-pcap-index-job|flink-log-job|flink-user-behavior-job) ;; \
		*) echo "invalid FLINK_JOB_MODULE" >&2; exit 1 ;; \
	esac
	cd $(JAVA_DIR) && mvn -pl "$(FLINK_JOB_MODULE)" -am package -DskipTests -q
	docker build \
		-t "$(REGISTRY)/$(FLINK_JOB_MODULE):$(FLINK_APPLICATION_TAG)" \
		--build-arg "JOB_MODULE=$(FLINK_JOB_MODULE)" \
		-f $(JAVA_DIR)/deployments/Dockerfile.application \
		$(JAVA_DIR)

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

# ============================ Alignment Remediation ============================

.PHONY: alignment-inventory
alignment-inventory: ## Print deterministic routes/actions/API/scope compatibility inventory
	python scripts/alignment/inventory.py

.PHONY: alignment-generate-client
alignment-generate-client: ## Generate contract-derived TypeScript client and remediation ledger
	python scripts/alignment/generate_ts_client.py
	python scripts/alignment/build_ledger.py

.PHONY: alignment-validate
alignment-validate: ## Validate W0 registry, contracts, OpenAPI, migration and scope guards
	python scripts/alignment/validate.py
	python scripts/alignment/verify_ui_page_design_contracts.py
	python scripts/alignment/verify_product_design_audit_policy.py
	python scripts/alignment/build_feature_contract_registry.py --check
	python scripts/alignment/verify_feature_contract_registry.py
	python scripts/alignment/build_adapter_risk_registry.py --check
	python scripts/alignment/verify_common_response_adapter.py
	python scripts/alignment/check_openapi.py
	python scripts/alignment/check_migrations.py
	python scripts/alignment/check_event_catalog.py
	python scripts/alignment/generate_kafka_acl_plan.py --check-generated
	python scripts/alignment/verify_kafka_dlq_commit_barrier.py
	python scripts/alignment/verify_pcap_metadata_ack.py
	python scripts/alignment/verify_asset_expand_guardrails.py
	python scripts/alignment/verify_flink_state_recovery.py
	python scripts/alignment/verify_flink_checkpoint_ha.py
	python scripts/alignment/verify_flink_sink_reconciliation.py
	python scripts/alignment/verify_flink_job_registry.py
	python scripts/alignment/verify_pg_transaction_outbox.py
	python scripts/alignment/verify_pg_ha_pitr.py
	python scripts/alignment/verify_redis_reliability_domains.py
	python scripts/alignment/verify_minio_object_governance.py
	python scripts/alignment/verify_minio_service_identities.py
	python scripts/alignment/verify_minio_tls_material.py
	python scripts/alignment/verify_minio_tls_cutover.py
	python scripts/alignment/verify_kafka_event_envelope.py
	python scripts/alignment/verify_clickhouse_schema_authority.py
	python scripts/alignment/verify_clickhouse_deterministic_sharding.py
	python scripts/alignment/verify_clickhouse_query_paths.py
	python scripts/alignment/verify_clickhouse_append_only_semantics.py
	python scripts/alignment/verify_clickhouse_retention_lifecycle.py
	python scripts/alignment/verify_clickhouse_ha_security_backup.py
	python scripts/alignment/verify_opensearch_index_governance.py
	python scripts/alignment/verify_opensearch_search_pagination.py
	python scripts/alignment/verify_opensearch_projection_reconciliation.py
	python scripts/alignment/verify_opensearch_ha_security_restore.py
	python scripts/alignment/verify_trace_watermark_reconcile.py
	python scripts/alignment/verify_data_quality_control_plane.py
	python scripts/alignment/verify_configuration_catalog.py
	python scripts/alignment/backfill_openapi_scopes.py --check
	python scripts/alignment/verify_gateway_route_catalog.py
	python scripts/alignment/build_service_identity_catalog.py --check
	python scripts/alignment/verify_service_identity_catalog.py
	python scripts/alignment/build_pki_catalog.py --check
	python scripts/alignment/verify_pki_catalog.py
	python scripts/alignment/build_dr_recovery_catalog.py --check
	python scripts/alignment/verify_dr_recovery_catalog.py
	python scripts/alignment/inventory_pg_mutations.py --check
	bash tests/alignment/test_kafka_service_identity_sync.sh

.PHONY: alignment-verify-flink-state-recovery
alignment-verify-flink-state-recovery: ## Verify T-FLINK-002 deterministic IDs, operator state, UIDs, late data and async budgets
	python scripts/alignment/verify_flink_state_recovery.py

.PHONY: alignment-verify-kafka-dlq-commit-barrier
alignment-verify-kafka-dlq-commit-barrier: ## Verify T-KAFKA-003 durable DLQ and source-offset commit barriers
	python scripts/alignment/verify_kafka_dlq_commit_barrier.py
	cd $(GO_DIR) && go test -race ./internal/common/kafka

.PHONY: alignment-verify-pcap-metadata-ack
alignment-verify-pcap-metadata-ack: ## Verify F-PROBE-001/T-KAFKA-003 non-final durable PCAP metadata receipts
	python scripts/alignment/verify_pcap_metadata_ack.py
	cd $(GO_DIR) && go test ./internal/ingest/queue ./internal/ingest/server

.PHONY: alignment-verify-asset-expand-guardrails
alignment-verify-asset-expand-guardrails: ## Verify F-ASSET-001..006 default-off and approval-bound PostgreSQL expand controls
	python scripts/alignment/verify_asset_expand_guardrails.py
	python -m unittest tests.alignment.test_asset_expand_renderer tests.alignment.test_asset_expand_guardrails -v
	cd $(GO_DIR) && go test ./internal/asset/... ./cmd/asset-service/...

.PHONY: alignment-verify-asset-expand-g1
alignment-verify-asset-expand-g1: ## Replay the F-ASSET-001..006 PostgreSQL expand twice in an owned sentinel container (RUN_ID required)
	test -n "$(RUN_ID)"
	python scripts/alignment/verify_asset_expand_ephemeral.py --run-id "$(RUN_ID)" $(if $(OUTPUT),--output "$(OUTPUT)",)

.PHONY: alignment-verify-asset-projection-opensearch-g1
alignment-verify-asset-projection-opensearch-g1: ## Verify F-ASSET-002 projection semantics in an owned OpenSearch container (RUN_ID required)
	test -n "$(RUN_ID)"
	python scripts/alignment/verify_asset_projection_opensearch_ephemeral.py --run-id "$(RUN_ID)" $(if $(OUTPUT),--output "$(OUTPUT)",)

.PHONY: alignment-verify-minio-object-governance
alignment-verify-minio-object-governance: ## Verify T-MINIO-002/003/004 bucket, lifecycle and fail-closed credential governance
	python scripts/alignment/verify_minio_object_governance.py

.PHONY: alignment-verify-minio-service-identities
alignment-verify-minio-service-identities: ## Verify T-MINIO-003/004 scoped identities, policies, ExternalSecrets and consumers
	python scripts/alignment/verify_minio_service_identities.py

.PHONY: alignment-verify-minio-tls-material
alignment-verify-minio-tls-material: ## Verify T-MINIO-003/T-PKI-001 candidate-only TLS material and fail-closed guards
	python scripts/alignment/verify_minio_tls_material.py

.PHONY: alignment-verify-minio-tls-cutover
alignment-verify-minio-tls-cutover: ## Verify default-off atomic MinIO TLS server and all-client cutover bundle
	python scripts/alignment/verify_minio_tls_cutover.py

.PHONY: alignment-capture-minio-tls-candidate-images
alignment-capture-minio-tls-candidate-images: ## Capture read-only local image metadata for the default-off MinIO TLS bundle (RUN_ID and G0_MANIFEST required)
	test -n "$(RUN_ID)"
	test -n "$(G0_MANIFEST)"
	python scripts/alignment/capture_minio_tls_candidate_images.py --run-id "$(RUN_ID)" --g0-manifest "$(G0_MANIFEST)"

.PHONY: alignment-verify-feature-contract-registry
alignment-verify-feature-contract-registry: ## Verify F-COMMON-001 canonical feature ownership, formal contract hashes and explicit coverage gaps
	python scripts/alignment/build_feature_contract_registry.py --check
	python scripts/alignment/verify_feature_contract_registry.py

.PHONY: alignment-verify-common-response-adapter
alignment-verify-common-response-adapter: ## Verify F-COMMON-002/F-COMMON-004 protocol and F-ADAPTER-002 risk ratchet
	python scripts/alignment/build_adapter_risk_registry.py --check
	python scripts/alignment/verify_common_response_adapter.py

.PHONY: alignment-capture-common-response-adapter
alignment-capture-common-response-adapter: ## Capture immutable WP-01 common-response and adapter-ratchet evidence (RUN_ID and G0_MANIFEST required)
	test -n "$(RUN_ID)"
	test -n "$(G0_MANIFEST)"
	python scripts/alignment/capture_common_response_adapter.py --run-id "$(RUN_ID)" --g0-manifest "$(G0_MANIFEST)"

.PHONY: alignment-capture-feature-contract-registry
alignment-capture-feature-contract-registry: ## Capture repository and read-only runtime-adoption evidence (RUN_ID and G0_MANIFEST required)
	test -n "$(RUN_ID)"
	test -n "$(G0_MANIFEST)"
	python scripts/alignment/capture_feature_contract_registry.py --run-id "$(RUN_ID)" --g0-manifest "$(G0_MANIFEST)"

.PHONY: alignment-verify-flink-checkpoint-ha
alignment-verify-flink-checkpoint-ha: ## Verify T-FLINK-003 checkpoint, HA, startup and upgrade guards
	python scripts/alignment/verify_flink_checkpoint_ha.py

.PHONY: alignment-verify-flink-sink-reconciliation
alignment-verify-flink-sink-reconciliation: ## Verify T-FLINK-004 sink ACK, replay and reconciliation guards
	python scripts/alignment/verify_flink_sink_reconciliation.py

.PHONY: alignment-verify-flink-job-registry
alignment-verify-flink-job-registry: ## Verify T-FLINK-005 registry, release binding, runtime diff and rescale guards
	python scripts/alignment/verify_flink_job_registry.py

.PHONY: alignment-verify-pg-transaction-outbox
alignment-verify-pg-transaction-outbox: ## Verify T-PG-002 business, history, audit, outbox and idempotency boundaries
	python scripts/alignment/verify_pg_transaction_outbox.py

.PHONY: alignment-verify-pg-mutation-inventory
alignment-verify-pg-mutation-inventory: ## Verify T-PG-002 PostgreSQL mutation ownership and review queue snapshot
	python scripts/alignment/inventory_pg_mutations.py --check

.PHONY: alignment-verify-pg-ha-pitr
alignment-verify-pg-ha-pitr: ## Verify T-PG-006 safe-hold, fencing and PITR repository guards
	python scripts/alignment/verify_pg_ha_pitr.py

.PHONY: alignment-verify-redis-reliability-domains
alignment-verify-redis-reliability-domains: ## Verify T-REDIS-001 noeviction, cache isolation and workload binding guards
	python scripts/alignment/verify_redis_reliability_domains.py

.PHONY: alignment-verify-kafka-event-envelope
alignment-verify-kafka-event-envelope: ## Verify T-KAFKA-002 additive envelope, deterministic identity and source context
	python scripts/alignment/verify_kafka_event_envelope.py

.PHONY: alignment-verify-trace-watermark-reconcile
alignment-verify-trace-watermark-reconcile: ## Verify T-OBS-001 W3C trace, alert projections and bounded cross-store reconcile guards
	python scripts/alignment/verify_trace_watermark_reconcile.py

.PHONY: alignment-capture-trace-watermark-reconcile
alignment-capture-trace-watermark-reconcile: ## Capture repository and read-only live T-OBS-001 evidence (RUN_ID and G0_MANIFEST required)
	test -n "$(RUN_ID)"
	test -n "$(G0_MANIFEST)"
	python scripts/alignment/capture_trace_watermark_reconcile.py --run-id "$(RUN_ID)" --g0-manifest "$(G0_MANIFEST)"

.PHONY: alignment-verify-data-quality-control-plane
alignment-verify-data-quality-control-plane: ## Verify T-DQ-001 persistent baselines, real handoff signals and unknown-state guards
	python scripts/alignment/verify_data_quality_control_plane.py

.PHONY: alignment-verify-configuration-catalog
alignment-verify-configuration-catalog: ## Verify T-CONFIG-001 redacted catalog, precedence, authority hashes and source drift
	python scripts/alignment/build_configuration_catalog.py --check
	python scripts/alignment/verify_configuration_catalog.py

.PHONY: alignment-capture-configuration-catalog
alignment-capture-configuration-catalog: ## Capture repository and redacted read-only live T-CONFIG-001 evidence (RUN_ID and G0_MANIFEST required)
	test -n "$(RUN_ID)"
	test -n "$(G0_MANIFEST)"
	python scripts/alignment/capture_configuration_catalog.py --run-id "$(RUN_ID)" --g0-manifest "$(G0_MANIFEST)"

.PHONY: alignment-verify-gateway-route-catalog
alignment-verify-gateway-route-catalog: ## Verify T-GW-001 route, OpenAPI scope, upstream Service and admin exposure guards
	python scripts/alignment/backfill_openapi_scopes.py --check
	python scripts/alignment/build_gateway_route_catalog.py --check
	python scripts/alignment/verify_gateway_route_catalog.py

.PHONY: alignment-capture-gateway-route-catalog
alignment-capture-gateway-route-catalog: ## Capture repository and read-only live T-GW-001 evidence (RUN_ID and G0_MANIFEST required)
	test -n "$(RUN_ID)"
	test -n "$(G0_MANIFEST)"
	python scripts/alignment/capture_gateway_route_catalog.py --run-id "$(RUN_ID)" --g0-manifest "$(G0_MANIFEST)"

.PHONY: alignment-verify-service-identity-catalog
alignment-verify-service-identity-catalog: ## Verify T-SEC-001 workload identity, Secret reference, Kafka principal and tenant-authority guards
	python scripts/alignment/build_service_identity_catalog.py --check
	python scripts/alignment/verify_service_identity_catalog.py

.PHONY: alignment-capture-service-identity-catalog
alignment-capture-service-identity-catalog: ## Capture repository and redacted read-only live T-SEC-001 evidence (RUN_ID and G0_MANIFEST required)
	test -n "$(RUN_ID)"
	test -n "$(G0_MANIFEST)"
	python scripts/alignment/capture_service_identity_catalog.py --run-id "$(RUN_ID)" --g0-manifest "$(G0_MANIFEST)"

.PHONY: alignment-verify-pki-catalog
alignment-verify-pki-catalog: ## Verify T-PKI-001 certificate domains, transport downgrade guards, validity and rotation readiness
	python scripts/alignment/build_pki_catalog.py --check
	python scripts/alignment/verify_pki_catalog.py

.PHONY: alignment-capture-pki-catalog
alignment-capture-pki-catalog: ## Capture repository and public-certificate-only live T-PKI-001 evidence (RUN_ID and G0_MANIFEST required)
	test -n "$(RUN_ID)"
	test -n "$(G0_MANIFEST)"
	python scripts/alignment/capture_pki_catalog.py --run-id "$(RUN_ID)" --g0-manifest "$(G0_MANIFEST)"

.PHONY: alignment-verify-dr-recovery-catalog
alignment-verify-dr-recovery-catalog: ## Verify T-DR-001 eight-domain recovery authority, order, restore evidence and execution guards
	python scripts/alignment/build_dr_recovery_catalog.py --check
	python scripts/alignment/verify_dr_recovery_catalog.py

.PHONY: alignment-capture-dr-recovery-catalog
alignment-capture-dr-recovery-catalog: ## Capture repository and read-only live T-DR-001 topology evidence (RUN_ID and G0_MANIFEST required)
	test -n "$(RUN_ID)"
	test -n "$(G0_MANIFEST)"
	python scripts/alignment/capture_dr_recovery_catalog.py --run-id "$(RUN_ID)" --g0-manifest "$(G0_MANIFEST)"

.PHONY: alignment-capture-data-quality-control-plane
alignment-capture-data-quality-control-plane: ## Capture repository and read-only live T-DQ-001 evidence (RUN_ID and G0_MANIFEST required)
	test -n "$(RUN_ID)"
	test -n "$(G0_MANIFEST)"
	python scripts/alignment/capture_data_quality_control_plane.py --run-id "$(RUN_ID)" --g0-manifest "$(G0_MANIFEST)"

.PHONY: alignment-render-data-quality-expand
alignment-render-data-quality-expand: ## Render suspended approval-bound T-DQ-001 PG expand (all DQ_* variables required)
	test -n "$(RUN_ID)"
	test -n "$(G0_MANIFEST)"
	test -n "$(DQ_APPROVAL_ID)"
	test -n "$(DQ_APPROVED_BY)"
	test -n "$(DQ_POSTGRES_SYSTEM_IDENTIFIER)"
	test -n "$(DQ_EXPECTED_MIGRATION_STATE)"
	test -n "$(DQ_NOT_BEFORE)"
	test -n "$(DQ_EXPIRES_AT)"
	test -n "$(DQ_OUTPUT)"
	python scripts/alignment/render_data_quality_postgres_expand.py \
		--run-id "$(RUN_ID)" \
		--approval-id "$(DQ_APPROVAL_ID)" \
		--approved-by "$(DQ_APPROVED_BY)" \
		--postgres-system-identifier "$(DQ_POSTGRES_SYSTEM_IDENTIFIER)" \
		--expected-migration-state "$(DQ_EXPECTED_MIGRATION_STATE)" \
		--not-before "$(DQ_NOT_BEFORE)" \
		--expires-at "$(DQ_EXPIRES_AT)" \
		--g0-manifest "$(G0_MANIFEST)" \
		--output "$(DQ_OUTPUT)"

.PHONY: alignment-verify-clickhouse-schema-authority
alignment-verify-clickhouse-schema-authority: ## Verify T-CH-001 migration authority and legacy DDL inventory
	python scripts/alignment/verify_clickhouse_schema_authority.py

.PHONY: alignment-verify-clickhouse-deterministic-sharding
alignment-verify-clickhouse-deterministic-sharding: ## Verify T-CH-002 deterministic sharding inventory and guarded V2 candidate
	python scripts/alignment/verify_clickhouse_deterministic_sharding.py

.PHONY: alignment-capture-clickhouse-deterministic-sharding
alignment-capture-clickhouse-deterministic-sharding: ## Capture repository-side T-CH-002 evidence (RUN_ID and G0_MANIFEST required)
	test -n "$(RUN_ID)"
	test -n "$(G0_MANIFEST)"
	python scripts/alignment/capture_clickhouse_deterministic_sharding.py --run-id "$(RUN_ID)" --g0-manifest "$(G0_MANIFEST)"

.PHONY: alignment-verify-clickhouse-query-paths
alignment-verify-clickhouse-query-paths: ## Verify T-CH-003 guarded alert query optimization slice
	python scripts/alignment/verify_clickhouse_query_paths.py

.PHONY: alignment-capture-clickhouse-query-paths
alignment-capture-clickhouse-query-paths: ## Capture repository-side T-CH-003 evidence (RUN_ID and G0_MANIFEST required)
	test -n "$(RUN_ID)"
	test -n "$(G0_MANIFEST)"
	python scripts/alignment/capture_clickhouse_query_paths.py --run-id "$(RUN_ID)" --g0-manifest "$(G0_MANIFEST)"

.PHONY: alignment-verify-clickhouse-append-only
alignment-verify-clickhouse-append-only: ## Verify T-CH-004 Go alert/evidence Distributed writer and no synchronous mutation slice
	python scripts/alignment/verify_clickhouse_append_only_semantics.py

.PHONY: alignment-capture-clickhouse-append-only
alignment-capture-clickhouse-append-only: ## Capture repository-side T-CH-004 evidence (RUN_ID and G0_MANIFEST required)
	test -n "$(RUN_ID)"
	test -n "$(G0_MANIFEST)"
	python scripts/alignment/capture_clickhouse_append_only_semantics.py --run-id "$(RUN_ID)" --g0-manifest "$(G0_MANIFEST)"

.PHONY: alignment-verify-clickhouse-retention
alignment-verify-clickhouse-retention: ## Verify T-CH-005 retention matrix, PCAP object grace and versioned session rollup
	python scripts/alignment/verify_clickhouse_retention_lifecycle.py

.PHONY: alignment-capture-clickhouse-retention
alignment-capture-clickhouse-retention: ## Capture repository and read-only live T-CH-005 evidence (RUN_ID and G0_MANIFEST required)
	test -n "$(RUN_ID)"
	test -n "$(G0_MANIFEST)"
	python scripts/alignment/capture_clickhouse_retention_lifecycle.py --run-id "$(RUN_ID)" --g0-manifest "$(G0_MANIFEST)"

.PHONY: alignment-verify-clickhouse-ha-security-backup
alignment-verify-clickhouse-ha-security-backup: ## Verify T-CH-006 metrics, fail-closed profile, dataset semantics and restore guards
	python scripts/alignment/verify_clickhouse_ha_security_backup.py

.PHONY: alignment-verify-opensearch-index-governance
alignment-verify-opensearch-index-governance: ## Verify T-OS-002 versioned templates, aliases, ISM and guarded runtime targets
	python scripts/alignment/verify_opensearch_index_governance.py

.PHONY: alignment-capture-opensearch-index-governance
alignment-capture-opensearch-index-governance: ## Capture repository and read-only live T-OS-002 evidence (RUN_ID and G0_MANIFEST required)
	test -n "$(RUN_ID)"
	test -n "$(G0_MANIFEST)"
	python scripts/alignment/capture_opensearch_index_governance.py --run-id "$(RUN_ID)" --g0-manifest "$(G0_MANIFEST)"

.PHONY: alignment-verify-opensearch-search-pagination
alignment-verify-opensearch-search-pagination: ## Verify T-OS-003 bounded shallow search, signed search_after cursors and PIT lifecycle
	python scripts/alignment/verify_opensearch_search_pagination.py

.PHONY: alignment-capture-opensearch-search-pagination
alignment-capture-opensearch-search-pagination: ## Capture repository and read-only live T-OS-003 evidence (RUN_ID and G0_MANIFEST required)
	@test -n "$(RUN_ID)" || (echo "RUN_ID is required" && exit 1)
	@test -n "$(G0_MANIFEST)" || (echo "G0_MANIFEST is required" && exit 1)
	python scripts/alignment/capture_opensearch_search_pagination.py --run-id "$(RUN_ID)" --g0-manifest "$(G0_MANIFEST)"

.PHONY: alignment-verify-opensearch-projection-reconciliation
alignment-verify-opensearch-projection-reconciliation: ## Verify T-OS-004 durable debt, external versions and bounded rebuild/reconcile guards
	python scripts/alignment/verify_opensearch_projection_reconciliation.py

.PHONY: alignment-capture-opensearch-projection-reconciliation
alignment-capture-opensearch-projection-reconciliation: ## Capture repository and read-only pre-canary T-OS-004 evidence (RUN_ID and G0_MANIFEST required)
	@test -n "$(RUN_ID)" || (echo "RUN_ID is required" && exit 1)
	@test -n "$(G0_MANIFEST)" || (echo "G0_MANIFEST is required" && exit 1)
	python scripts/alignment/capture_opensearch_projection_reconciliation.py --run-id "$(RUN_ID)" --g0-manifest "$(G0_MANIFEST)"

.PHONY: alignment-verify-opensearch-ha-security-restore
alignment-verify-opensearch-ha-security-restore: ## Verify T-OS-005 three-zone TLS identities alerts and isolated restore guards
	python scripts/alignment/verify_opensearch_ha_security_restore.py

.PHONY: alignment-capture-opensearch-ha-security-restore
alignment-capture-opensearch-ha-security-restore: ## Capture repository and read-only live T-OS-005 evidence (RUN_ID and G0_MANIFEST required)
	@test -n "$(RUN_ID)" || (echo "RUN_ID is required" && exit 1)
	@test -n "$(G0_MANIFEST)" || (echo "G0_MANIFEST is required" && exit 1)
	python scripts/alignment/capture_opensearch_ha_security_restore.py --run-id "$(RUN_ID)" --g0-manifest "$(G0_MANIFEST)"

.PHONY: alignment-capture-clickhouse-ha-security-backup
alignment-capture-clickhouse-ha-security-backup: ## Capture repository and read-only live T-CH-006 evidence (RUN_ID and G0_MANIFEST required)
	test -n "$(RUN_ID)"
	test -n "$(G0_MANIFEST)"
	python scripts/alignment/capture_clickhouse_ha_security_backup.py --run-id "$(RUN_ID)" --g0-manifest "$(G0_MANIFEST)"

.PHONY: alignment-capture-clickhouse-live-schema
alignment-capture-clickhouse-live-schema: ## Capture read-only T-CH-001 system table/column/replica evidence (RUN_ID required)
	test -n "$(RUN_ID)"
	python scripts/alignment/capture_clickhouse_live_schema.py --run-id "$(RUN_ID)"

.PHONY: alignment-test
alignment-test: alignment-generate-client alignment-validate ## Run alignment unit and IAM catalog tests
	python -m unittest discover -s tests/alignment -v
	cd $(GO_DIR) && go test ./internal/auth/model -count=1

.PHONY: alignment-verify-flink-live
alignment-verify-flink-live: ## Verify the canonical nine active Flink jobs; set FLINK_REST_ENDPOINT when outside cluster DNS
	python scripts/alignment/verify_flink_nine_jobs.py --endpoint "$${FLINK_REST_ENDPOINT:-http://flink-jobmanager.flink.svc:8081}"

.PHONY: alignment-render-flink-application
alignment-render-flink-application: ## Render one guarded Application Cluster; set module, digest image and savepoint manifest
	test -n "$(FLINK_JOB_MODULE)"
	test -n "$(FLINK_APPLICATION_IMAGE)"
	test -n "$(FLINK_SAVEPOINT_MANIFEST)"
	python scripts/alignment/render_flink_application_cluster.py \
		--job-id "$(FLINK_JOB_MODULE)" \
		--image "$(FLINK_APPLICATION_IMAGE)" \
		--savepoint-manifest "$(FLINK_SAVEPOINT_MANIFEST)"

.PHONY: alignment-verify-flink-applications-live
alignment-verify-flink-applications-live: ## Verify nine isolated Application Clusters; set FLINK_ENDPOINTS_MANIFEST
	test -n "$(FLINK_ENDPOINTS_MANIFEST)"
	python scripts/alignment/verify_flink_application_clusters.py \
		--endpoints-manifest "$(FLINK_ENDPOINTS_MANIFEST)"

.PHONY: alignment-baseline
alignment-baseline: ## Capture immutable W0 evidence; set SOURCE_WORKTREE and optional RUN_ID
	test -n "$(SOURCE_WORKTREE)"
	python scripts/alignment/capture_baseline.py --source-worktree "$(SOURCE_WORKTREE)" $(if $(RUN_ID),--run-id "$(RUN_ID)",)

.PHONY: alignment-g0-evidence
alignment-g0-evidence: ## Run and hash G0 alignment/full/Python logs; set RUN_ID and optional INVENTORY_MANIFEST
	test -n "$(RUN_ID)"
	python scripts/alignment/capture_g0.py --run-id "$(RUN_ID)" $(if $(G0_PROFILE),--profile "$(G0_PROFILE)",) $(if $(INVENTORY_MANIFEST),--inventory-manifest "$(INVENTORY_MANIFEST)",)

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
