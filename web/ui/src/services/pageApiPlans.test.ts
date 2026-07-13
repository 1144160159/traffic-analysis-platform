import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';
import { allRoutes } from '@/routes/routeManifest';
import {
  getPageActionPlan,
  getPageApiPlan,
  getPageLoadSecondaryEndpoints,
  getPlanEndpoints,
  pageApiPlans,
} from '@/services/pageApiPlans';

const repoRoot = resolve(__dirname, '../../../..');
const apisixRoutesYaml = readFileSync(
  resolve(repoRoot, 'deployments/kubernetes/configmaps/apisix-routes.yaml'),
  'utf8',
);

const apisixRoutePrefixes = Array.from(apisixRoutesYaml.matchAll(/uri:\s+(\/api\/v1\/[^\s*"]+)\*/g)).map(
  (match) => stripApiPrefix(match[1]),
);

describe('pageApiPlans', () => {
  it('defines an explicit real API plan for every UI route', () => {
    const missing = allRoutes.map((route) => route.id).filter((id) => !(id in pageApiPlans));
    expect(missing).toEqual([]);
  });

  it('keeps every planned endpoint under an APISIX routed prefix', () => {
    const uncovered = allRoutes.flatMap((route) =>
      getPlanEndpoints(getPageApiPlan(route.id))
        .filter((endpoint) => !isCoveredByApisix(endpoint))
        .map((endpoint) => `${route.id}:${endpoint}`),
    );

    expect(uncovered).toEqual([]);
  });

  it('keeps mock-only UI page endpoints out of production plans', () => {
    const mockOnly = Object.entries(pageApiPlans).flatMap(([pageId, plan]) =>
      getPlanEndpoints(plan)
        .filter((endpoint) => endpoint.includes('/ui/pages'))
        .map((endpoint) => `${pageId}:${endpoint}`),
    );

    expect(mockOnly).toEqual([]);
  });

  it('keeps page-load secondary calls limited to endpoints that can be read without route context', () => {
    const unresolvedPageLoadEndpoints = allRoutes.flatMap((route) =>
      getPageLoadSecondaryEndpoints(route.id)
        .filter((endpoint) => endpoint.includes('{'))
        .map((endpoint) => `${route.id}:${endpoint}`),
    );

    expect(unresolvedPageLoadEndpoints).toEqual([]);
    expect(getPageLoadSecondaryEndpoints('baselines')).toEqual([]);
    expect(getPageLoadSecondaryEndpoints('whitelist')).toEqual([]);
  });

  it('binds data-quality DLQ replay to the protected ingest replay API', () => {
    const action = getPageActionPlan('data-quality', 'dlq-fallback-replay');

    expect(action).toBeTruthy();
    expect(action?.method).toBe('POST');
    expect(action?.endpoint).toBe('/v1/dlq/replay/fallback');
    expect(action?.requiredScopes).toContain('dlq:replay');
    expect(action?.acceptedScopes).toContain('admin:write');
    expect(action?.defaultBody).toMatchObject({ dry_run: true });
    expect(action?.guardrails).toContain('approved_by must differ from requested_by');
    expect(isCoveredByApisix(action!.endpoint)).toBe(true);
  });

  it('binds encrypted egress operations to the audited action endpoint', () => {
    const endpoints = getPlanEndpoints(getPageApiPlan('encrypted-traffic'));
    const createAlert = getPageActionPlan('encrypted-traffic', 'egress-create-alert');
    const evidenceLookup = getPageActionPlan('encrypted-traffic', 'egress-evidence-lookup');
    const graphDrilldown = getPageActionPlan('encrypted-traffic', 'egress-entity-graph');
    const writeAudit = getPageActionPlan('encrypted-traffic', 'egress-audit-write');

    expect(endpoints).toContain('/v1/encrypted-traffic/egress-actions');
    expect(createAlert?.method).toBe('POST');
    expect(createAlert?.requiredScopes).toContain('alert:write');
    expect(createAlert?.auditEvent).toBe('ENCRYPTED_EGRESS_ALERT_REQUESTED');
    expect(createAlert?.guardrails).toContain('the request must persist an audit_logs row before reporting success');
    expect(evidenceLookup?.auditEvent).toBe('ENCRYPTED_EGRESS_EVIDENCE_LOOKUP');
    expect(graphDrilldown?.auditEvent).toBe('ENCRYPTED_EGRESS_GRAPH_DRILLDOWN');
    expect(writeAudit?.auditEvent).toBe('ENCRYPTED_EGRESS_AUDIT_WRITTEN');
    expect(isCoveredByApisix('/v1/encrypted-traffic/egress-actions')).toBe(true);
  });

  it('binds encrypted evidence center reads and actions to audited APIs', () => {
    const endpoints = getPlanEndpoints(getPageApiPlan('encrypted-traffic'));
    const createTask = getPageActionPlan('encrypted-traffic', 'evidence-create-task');
    const downloadPcap = getPageActionPlan('encrypted-traffic', 'evidence-download-pcap');
    const verifyHash = getPageActionPlan('encrypted-traffic', 'evidence-verify-hash');
    const exportPackage = getPageActionPlan('encrypted-traffic', 'evidence-export-package');
    const preservation = getPageActionPlan('encrypted-traffic', 'evidence-preserve');
    const expertReview = getPageActionPlan('encrypted-traffic', 'evidence-expert-review');
    const exportReport = getPageActionPlan('encrypted-traffic', 'evidence-export-report');
    const writeAudit = getPageActionPlan('encrypted-traffic', 'evidence-write-audit');

    expect(endpoints).toContain('/v1/encrypted-traffic/evidence');
    expect(endpoints).toContain('/v1/encrypted-traffic/evidence-actions');
    expect(createTask?.requiredScopes).toContain('alert:write');
    expect(createTask?.auditEvent).toBe('ENCRYPTED_EVIDENCE_TASK_REQUESTED');
    expect(downloadPcap?.auditEvent).toBe('ENCRYPTED_EVIDENCE_PCAP_DOWNLOAD_REQUESTED');
    expect(verifyHash?.auditEvent).toBe('ENCRYPTED_EVIDENCE_HASH_VERIFICATION_REQUESTED');
    expect(exportPackage?.auditEvent).toBe('ENCRYPTED_EVIDENCE_EXPORT_REQUESTED');
    expect(preservation?.auditEvent).toBe('ENCRYPTED_EVIDENCE_PRESERVATION_REQUESTED');
    expect(expertReview?.auditEvent).toBe('ENCRYPTED_EVIDENCE_EXPERT_REVIEW_REQUESTED');
    expect(exportReport?.auditEvent).toBe('ENCRYPTED_EVIDENCE_REPORT_EXPORT_REQUESTED');
    expect(writeAudit?.auditEvent).toBe('ENCRYPTED_EVIDENCE_AUDIT_WRITTEN');
    expect(isCoveredByApisix('/v1/encrypted-traffic/evidence-actions')).toBe(true);
  });

  it('binds topic governance overlays to audited control-plane APIs', () => {
    const endpoints = getPlanEndpoints(getPageApiPlan('topics'));
    const saveAction = getPageActionPlan('topics', 'topic-view-save');
    const scopeAction = getPageActionPlan('topics', 'topic-scope-update');
    const createSubscriptionAction = getPageActionPlan('topics', 'topic-subscription-create');
    const toggleSubscriptionAction = getPageActionPlan('topics', 'topic-subscription-toggle');
    const reportExportAction = getPageActionPlan('topics', 'topic-report-export');
    const evidenceExportAction = getPageActionPlan('topics', 'topic-evidence-package-export');

    expect(endpoints).toContain('/v1/topics/tunnel');
    expect(endpoints).toContain('/v1/topics/exfil');
    expect(endpoints).toContain('/v1/topics/apt');
    expect(endpoints).toContain('/v1/topics/views');
    expect(endpoints).toContain('/v1/topics/subscriptions');
    expect(endpoints).toContain('/v1/topics/scopes/{topic}');
    expect(endpoints).toContain('/v1/topics/subscriptions/{id}');
    expect(endpoints).toContain('/v1/topics/reports/export');
    expect(endpoints).toContain('/v1/topics/evidence-packages/export');
    expect(saveAction?.auditEvent).toBe('TOPIC_VIEW_SAVED');
    expect(saveAction?.guardrails).toContain('saved views must persist topic_saved_views rows');
    expect(scopeAction?.method).toBe('PUT');
    expect(scopeAction?.auditEvent).toBe('TOPIC_SCOPE_UPDATED');
    expect(createSubscriptionAction?.requiredScopes).toContain('topic:write');
    expect(createSubscriptionAction?.guardrails).toContain('subscriptions must persist topic_subscriptions rows');
    expect(toggleSubscriptionAction?.method).toBe('PATCH');
    expect(toggleSubscriptionAction?.guardrails).toContain('cross-tenant subscription updates must return 404');
    expect(reportExportAction?.requiredScopes).toContain('topic:export');
    expect(reportExportAction?.auditEvent).toBe('TOPIC_REPORT_EXPORTED');
    expect(evidenceExportAction?.auditEvent).toBe('TOPIC_EVIDENCE_PACKAGE_EXPORTED');
    expect(isCoveredByApisix('/v1/topics/views')).toBe(true);
    expect(isCoveredByApisix('/v1/topics/scopes/tunnel')).toBe(true);
    expect(isCoveredByApisix('/v1/topics/subscriptions/sub-001')).toBe(true);
    expect(isCoveredByApisix('/v1/topics/reports/export')).toBe(true);
    expect(isCoveredByApisix('/v1/topics/evidence-packages/export')).toBe(true);
  });

  it('binds fusion to the threat intel service through APISIX', () => {
    const endpoints = getPlanEndpoints(getPageApiPlan('fusion'));
    const conflictAction = getPageActionPlan('fusion', 'fusion-conflict-resolve');
    const ruleAction = getPageActionPlan('fusion', 'fusion-rule-update');

    expect(endpoints).toContain('/v1/fusion/stats');
    expect(endpoints).toContain('/v1/fusion/entities');
    expect(endpoints).toContain('/v1/threat-intel/entries');
    expect(endpoints).toContain('/v1/fusion/conflicts/{id}/resolve');
    expect(endpoints).toContain('/v1/fusion/rules/{id}');
    expect(conflictAction?.requiredScopes).toContain('rule:write');
    expect(conflictAction?.auditEvent).toBe('FUSION_CONFLICT_RESOLVED');
    expect(ruleAction?.method).toBe('PATCH');
    expect(ruleAction?.auditEvent).toBe('FUSION_RULE_UPDATED');
    expect(isCoveredByApisix('/v1/threat-intel/entries')).toBe(true);
    expect(isCoveredByApisix('/v1/fusion/conflicts/cf-001/resolve')).toBe(true);
    expect(isCoveredByApisix('/v1/fusion/rules/ip-mac-bind')).toBe(true);
  });

  it('binds behavior baseline reset to an audited alert-write action', () => {
    const endpoints = getPlanEndpoints(getPageApiPlan('baselines'));
    const resetAction = getPageActionPlan('baselines', 'baseline-reset');

    expect(endpoints).toContain('/v1/baselines');
    expect(endpoints).toContain('/v1/baselines/{id}');
    expect(endpoints).toContain('/v1/baselines/{id}/reset');
    expect(resetAction?.method).toBe('POST');
    expect(resetAction?.requiredScopes).toContain('alert:write');
    expect(resetAction?.acceptedScopes).toContain('admin:*');
    expect(resetAction?.auditEvent).toBe('BEHAVIOR_BASELINE_RESET');
    expect(resetAction?.guardrails).toContain('baseline reset requires alert:write or admin scope');
    expect(resetAction?.guardrails).toContain('tenant_id must come from the authenticated token for write operations');
    expect(resetAction?.guardrails).toContain('successful resets must persist behavior_baseline_resets and audit_logs rows');
    expect(isCoveredByApisix('/v1/baselines/ip:10.12.4.12')).toBe(true);
    expect(isCoveredByApisix('/v1/baselines/ip:10.12.4.12/reset')).toBe(true);
  });

  it('binds playbook actions to SOAR state-machine APIs', () => {
    const endpoints = getPlanEndpoints(getPageApiPlan('playbooks'));
    const executeAction = getPageActionPlan('playbooks', 'playbook-execute');
    const updateAction = getPageActionPlan('playbooks', 'playbook-state-update');

    expect(endpoints).toContain('/v1/playbooks/catalog');
    expect(endpoints).toContain('/v1/playbooks/executions');
    expect(endpoints).toContain('/v1/playbooks/{name}/execute');
    expect(endpoints).toContain('/v1/playbooks/{name}');
    expect(executeAction?.method).toBe('POST');
    expect(executeAction?.guardrails).toContain('manual execution must respect enabled and max_runs state');
    expect(updateAction?.method).toBe('PATCH');
    expect(updateAction?.guardrails).toContain('state updates must be persisted in alert_playbook_overrides');
    expect(isCoveredByApisix('/v1/playbooks/block-scanner/execute')).toBe(true);
    expect(isCoveredByApisix('/v1/playbooks/block-scanner')).toBe(true);
  });

  it('binds forensics cancel to the PCAP task state-machine API', () => {
    const endpoints = getPlanEndpoints(getPageApiPlan('forensics'));
    const cancelAction = getPageActionPlan('forensics', 'forensics-cancel-job');

    expect(endpoints).toContain('/v1/pcap/jobs');
    expect(endpoints).toContain('/v1/pcap/jobs/{id}/cancel');
    expect(cancelAction?.method).toBe('POST');
    expect(cancelAction?.requiredScopes).toContain('pcap:write');
    expect(cancelAction?.auditEvent).toBe('PCAP_CANCEL');
    expect(cancelAction?.guardrails).toContain('only queued or processing jobs can be cancelled');
    expect(cancelAction?.guardrails).toContain('successful cancels must be persisted in tasks and audit_logs');
    expect(isCoveredByApisix('/v1/pcap/jobs/pcap-job-001/cancel')).toBe(true);
  });

  it('binds asset discovery to SNMP/LLDP credential and run APIs', () => {
    const endpoints = getPlanEndpoints(getPageApiPlan('assets'));
    const credentialAction = getPageActionPlan('assets', 'asset-discovery-credential-register');
    const runAction = getPageActionPlan('assets', 'asset-active-discovery-run');

    expect(endpoints).toContain('/v1/assets');
    expect(endpoints).toContain('/v1/assets/discovery/runs');
    expect(endpoints).toContain('/v1/assets/discovery/neighbors');
    expect(endpoints).toContain('/v1/assets/discovery/credentials');
    expect(credentialAction?.method).toBe('POST');
    expect(credentialAction?.requiredScopes).toContain('asset:discover');
    expect(credentialAction?.auditEvent).toBe('ASSET_DISCOVERY_CREDENTIAL_REGISTER');
    expect(credentialAction?.guardrails).toContain('plaintext SNMP community or credentials must not be sent to the API');
    expect(runAction?.method).toBe('POST');
    expect(runAction?.requiredScopes).toContain('asset:discover');
    expect(runAction?.auditEvent).toBe('ASSET_ACTIVE_DISCOVERY_RUN');
    expect(runAction?.guardrails).toContain('active discovery must write asset_discovery_runs and asset_topology_links');
    expect(isCoveredByApisix('/v1/assets/discovery/credentials')).toBe(true);
    expect(isCoveredByApisix('/v1/assets/discovery/runs')).toBe(true);
    expect(isCoveredByApisix('/v1/assets/discovery/neighbors')).toBe(true);
  });

  it('binds probe operations to audited control-plane APIs', () => {
    const endpoints = getPlanEndpoints(getPageApiPlan('probes'));
    const batchUpgradeAction = getPageActionPlan('probes', 'probe-batch-upgrade');
    const configAction = getPageActionPlan('probes', 'probe-config-push');
    const connectivityAction = getPageActionPlan('probes', 'probe-connectivity-test');
    const certAction = getPageActionPlan('probes', 'probe-cert-rotate');

    expect(endpoints).toContain('/v1/probes');
    expect(endpoints).toContain('/v1/probes/batch-upgrade');
    expect(endpoints).toContain('/v1/probes/{id}/config');
    expect(endpoints).toContain('/v1/probes/{id}/connectivity-test');
    expect(endpoints).toContain('/v1/probes/{id}/certificates/rotate');
    expect(batchUpgradeAction?.method).toBe('POST');
    expect(batchUpgradeAction?.requiredScopes).toContain('probe:write');
    expect(batchUpgradeAction?.auditEvent).toBe('PROBE_BATCH_UPGRADE');
    expect(batchUpgradeAction?.guardrails).toContain('batch upgrades must persist probe_operations rows and update probes.software_version');
    expect(configAction?.auditEvent).toBe('PROBE_CONFIG_PUSH');
    expect(configAction?.defaultBody).toMatchObject({ capture_mode: 'af_packet' });
    expect(configAction?.guardrails).toContain('tenant_id must come from the authenticated token for write operations');
    expect(connectivityAction?.auditEvent).toBe('PROBE_CONNECTIVITY_TEST');
    expect(connectivityAction?.guardrails).toContain('cross-tenant probe operations must return 404');
    expect(certAction?.auditEvent).toBe('PROBE_CERT_ROTATE');
    expect(certAction?.defaultBody).toMatchObject({ secret_ref: 'k8s://traffic-analysis/traffic-credentials#PROBE_MTLS_CERT' });
    expect(certAction?.guardrails).toContain('plaintext certificate, private_key, token or password fields must be rejected');
    expect(isCoveredByApisix('/v1/probes')).toBe(true);
    expect(isCoveredByApisix('/v1/probes/batch-upgrade')).toBe(true);
    expect(isCoveredByApisix('/v1/probes/PROBE-DC-01/config')).toBe(true);
    expect(isCoveredByApisix('/v1/probes/PROBE-DC-01/connectivity-test')).toBe(true);
    expect(isCoveredByApisix('/v1/probes/PROBE-DC-01/certificates/rotate')).toBe(true);
  });

  it('binds rules enable and disable to the rule state-machine APIs', () => {
    const endpoints = getPlanEndpoints(getPageApiPlan('rules'));
    const enableAction = getPageActionPlan('rules', 'rule-enable');
    const disableAction = getPageActionPlan('rules', 'rule-disable');

    expect(endpoints).toContain('/v1/rules');
    expect(endpoints).toContain('/v1/rules/{id}/enable');
    expect(endpoints).toContain('/v1/rules/{id}/disable');
    expect(enableAction?.method).toBe('POST');
    expect(disableAction?.method).toBe('POST');
    expect(enableAction?.requiredScopes).toContain('rule:enable');
    expect(disableAction?.requiredScopes).toContain('rule:enable');
    expect(enableAction?.auditEvent).toBe('RULE_ENABLE');
    expect(disableAction?.auditEvent).toBe('RULE_DISABLE');
    expect(enableAction?.guardrails).toContain('successful enables must write rule_outbox and audit_logs');
    expect(disableAction?.guardrails).toContain('successful disables must write rule_outbox and audit_logs');
    expect(isCoveredByApisix('/v1/rules/rule-001/enable')).toBe(true);
    expect(isCoveredByApisix('/v1/rules/rule-001/disable')).toBe(true);
  });

  it('binds deployment actions to the deployment state-machine APIs', () => {
    const endpoints = getPlanEndpoints(getPageApiPlan('deployments'));
    const grayAction = getPageActionPlan('deployments', 'deployment-start-gray');
    const activateAction = getPageActionPlan('deployments', 'deployment-activate');
    const pauseAction = getPageActionPlan('deployments', 'deployment-pause');
    const resumeAction = getPageActionPlan('deployments', 'deployment-resume');
    const rollbackAction = getPageActionPlan('deployments', 'deployment-rollback');

    expect(endpoints).toContain('/v1/deployments');
    expect(endpoints).toContain('/v1/deployments/{id}/gray');
    expect(endpoints).toContain('/v1/deployments/{id}/activate');
    expect(endpoints).toContain('/v1/deployments/{id}/pause');
    expect(endpoints).toContain('/v1/deployments/{id}/resume');
    expect(endpoints).toContain('/v1/deployments/{id}/rollback');
    expect(grayAction?.requiredScopes).toContain('deploy:gray');
    expect(activateAction?.requiredScopes).toContain('deploy:activate');
    expect(pauseAction?.auditEvent).toBe('DEPLOY_PAUSE');
    expect(resumeAction?.auditEvent).toBe('DEPLOY_RESUME');
    expect(rollbackAction?.requiredScopes).toContain('deploy:rollback');
    expect(rollbackAction?.guardrails).toContain('successful rollbacks must persist deployment_history and audit_logs');
    expect(isCoveredByApisix('/v1/deployments/deploy-001/gray')).toBe(true);
    expect(isCoveredByApisix('/v1/deployments/deploy-001/activate')).toBe(true);
    expect(isCoveredByApisix('/v1/deployments/deploy-001/pause')).toBe(true);
    expect(isCoveredByApisix('/v1/deployments/deploy-001/resume')).toBe(true);
    expect(isCoveredByApisix('/v1/deployments/deploy-001/rollback')).toBe(true);
  });

  it('binds model version actions to the model registry state-machine APIs', () => {
    const endpoints = getPlanEndpoints(getPageApiPlan('models'));
    const registerAction = getPageActionPlan('models', 'model-version-register');
    const activateAction = getPageActionPlan('models', 'model-version-activate');
    const deprecateAction = getPageActionPlan('models', 'model-version-deprecate');
    const feedbackAction = getPageActionPlan('models', 'model-feedback-append');
    const retrainAction = getPageActionPlan('models', 'model-retrain-request');
    const evaluationAction = getPageActionPlan('models', 'model-evaluation-request');
    const rollbackAction = getPageActionPlan('models', 'model-version-rollback');
    const contextAction = getPageActionPlan('models', 'model-context-action');

    expect(endpoints).toContain('/v1/models');
    expect(endpoints).toContain('/v1/models/{id}/versions');
    expect(endpoints).toContain('/v1/models/{id}/versions/{version}/activate');
    expect(endpoints).toContain('/v1/models/{id}/versions/{version}/deprecate');
    expect(endpoints).toContain('/v1/models/{id}/feedback-samples');
    expect(endpoints).toContain('/v1/models/{id}/retrain');
    expect(endpoints).toContain('/v1/models/{id}/versions/{version}/evaluate');
    expect(endpoints).toContain('/v1/models/{id}/versions/{version}/rollback');
    expect(endpoints).toContain('/v1/models/{id}/actions');
    expect(registerAction?.method).toBe('POST');
    expect(registerAction?.requiredScopes).toContain('model:create');
    expect(registerAction?.auditEvent).toBe('MODEL_VERSION_CREATE');
    expect(activateAction?.requiredScopes).toContain('model:activate');
    expect(activateAction?.auditEvent).toBe('MODEL_VERSION_ACTIVATE');
    expect(activateAction?.guardrails).toContain('activation must deprecate any previous active version for the same model');
    expect(deprecateAction?.auditEvent).toBe('MODEL_VERSION_DEPRECATE');
    expect(deprecateAction?.guardrails).toContain('registered versions must return 409 until they enter an allowed state');
    expect(feedbackAction?.auditEvent).toBe('MODEL_FEEDBACK_SAMPLES_APPENDED');
    expect(retrainAction?.requiredScopes).toContain('model:write');
    expect(evaluationAction?.auditEvent).toBe('MODEL_EVALUATION_REQUESTED');
    expect(rollbackAction?.auditEvent).toBe('MODEL_VERSION_ROLLED_BACK');
    expect(contextAction?.auditEvent).toBe('MODEL_CONTEXT_ACTION_REQUESTED');
    expect(isCoveredByApisix('/v1/models/model-001/versions')).toBe(true);
    expect(isCoveredByApisix('/v1/models/model-001/versions/v2/activate')).toBe(true);
    expect(isCoveredByApisix('/v1/models/model-001/versions/v2/deprecate')).toBe(true);
    expect(isCoveredByApisix('/v1/models/model-001/feedback-samples')).toBe(true);
    expect(isCoveredByApisix('/v1/models/model-001/retrain')).toBe(true);
    expect(isCoveredByApisix('/v1/models/model-001/versions/v2/evaluate')).toBe(true);
    expect(isCoveredByApisix('/v1/models/model-001/versions/v2/rollback')).toBe(true);
    expect(isCoveredByApisix('/v1/models/model-001/actions')).toBe(true);
  });

  it('binds MLOps orchestration controls to audited action contracts', () => {
    const endpoints = getPlanEndpoints(getPageApiPlan('mlops'));
    const retrain = getPageActionPlan('mlops', 'mlops-training-submit');
    const stop = getPageActionPlan('mlops', 'mlops-task-stop');
    const register = getPageActionPlan('mlops', 'mlops-model-register');

    expect(endpoints).toContain('/v1/mlops/retrain');
    expect(endpoints).toContain('/v1/mlops/tasks/{id}/stop');
    expect(endpoints).toContain('/v1/models/{id}/versions');
    expect(retrain?.auditEvent).toBe('MLOPS_RETRAIN_REQUESTED');
    expect(stop?.guardrails).toContain('stop requires explicit confirmation');
    expect(stop?.requiredScopes).toContain('model:write');
    expect(register?.requiredScopes).toContain('model:create');
    expect(isCoveredByApisix('/v1/mlops/retrain')).toBe(true);
    expect(isCoveredByApisix('/v1/mlops/tasks/TR-001/stop')).toBe(true);
  });

  it('binds compliance report generation to an audited admin action', () => {
    const endpoints = getPlanEndpoints(getPageApiPlan('compliance'));
    const generateAction = getPageActionPlan('compliance', 'compliance-report-generate');

    expect(endpoints).toContain('/v1/compliance/reports');
    expect(endpoints).toContain('/v1/compliance/audit-trail');
    expect(endpoints).toContain('/v1/compliance/reports/generate');
    expect(generateAction?.method).toBe('POST');
    expect(generateAction?.requiredScopes).toContain('admin:*');
    expect(generateAction?.auditEvent).toBe('COMPLIANCE_REPORT_GENERATED');
    expect(generateAction?.defaultBody).toMatchObject({ report_type: 'weekly' });
    expect(generateAction?.guardrails).toContain('report generation requires admin:*');
    expect(generateAction?.guardrails).toContain('successful generation must write COMPLIANCE_REPORT_GENERATED audit_logs rows');
    expect(isCoveredByApisix('/v1/compliance/reports/generate')).toBe(true);
  });

  it('binds whitelist governance to audited create, approval, extension and disable APIs', () => {
    const endpoints = getPlanEndpoints(getPageApiPlan('whitelist'));
    const createAction = getPageActionPlan('whitelist', 'whitelist-create');
    const approvalAction = getPageActionPlan('whitelist', 'whitelist-submit-approval');
    const extendAction = getPageActionPlan('whitelist', 'whitelist-extend');
    const disableAction = getPageActionPlan('whitelist', 'whitelist-disable');

    expect(endpoints).toContain('/v1/whitelist');
    expect(endpoints).toContain('/v1/whitelist/check');
    expect(endpoints).toContain('/v1/whitelist/{id}');
    expect(createAction?.method).toBe('POST');
    expect(createAction?.requiredScopes).toContain('alert:write');
    expect(createAction?.auditEvent).toBe('WHITELIST_CREATED');
    expect(createAction?.guardrails).toContain('tenant_id must come from the authenticated token');
    expect(approvalAction?.method).toBe('PATCH');
    expect(approvalAction?.auditEvent).toBe('WHITELIST_APPROVAL_SUBMITTED');
    expect(approvalAction?.guardrails).toContain('cross-tenant updates must return 404');
    expect(extendAction?.auditEvent).toBe('WHITELIST_EXTENDED');
    expect(extendAction?.defaultBody).toMatchObject({ expires_at: '2030-01-01T00:00:00Z' });
    expect(disableAction?.method).toBe('PATCH');
    expect(disableAction?.auditEvent).toBe('WHITELIST_DISABLED');
    expect(disableAction?.guardrails).toContain('disabled entries must stop matching /v1/whitelist/check');
    expect(isCoveredByApisix('/v1/whitelist')).toBe(true);
    expect(isCoveredByApisix('/v1/whitelist/check')).toBe(true);
    expect(isCoveredByApisix('/v1/whitelist/entry-001')).toBe(true);
  });

  it('binds notification governance to settings, test-send and silence rule APIs', () => {
    const endpoints = getPlanEndpoints(getPageApiPlan('notifications'));
    const settingsAction = getPageActionPlan('notifications', 'notification-settings-update');
    const testAction = getPageActionPlan('notifications', 'notification-test-send');
    const createSilenceAction = getPageActionPlan('notifications', 'notification-silence-rule-create');
    const toggleSilenceAction = getPageActionPlan('notifications', 'notification-silence-rule-toggle');

    expect(endpoints).toContain('/v1/notifications/settings');
    expect(endpoints).toContain('/v1/notifications/test');
    expect(endpoints).toContain('/v1/notifications/silence-rules');
    expect(endpoints).toContain('/v1/notifications/silence-rules/{id}');
    expect(settingsAction?.method).toBe('PUT');
    expect(settingsAction?.requiredScopes).toContain('admin:*');
    expect(settingsAction?.auditEvent).toBe('NOTIFICATION_SETTINGS_UPDATED');
    expect(settingsAction?.guardrails).toContain('plaintext webhook tokens, passwords and API keys must be rejected');
    expect(testAction?.method).toBe('POST');
    expect(testAction?.auditEvent).toBe('NOTIFICATION_TEST_SENT');
    expect(createSilenceAction?.method).toBe('POST');
    expect(createSilenceAction?.auditEvent).toBe('NOTIFICATION_SILENCE_RULE_CREATED');
    expect(createSilenceAction?.guardrails).toContain('ends_at must be after starts_at');
    expect(toggleSilenceAction?.method).toBe('PATCH');
    expect(toggleSilenceAction?.defaultBody).toMatchObject({ enabled: false });
    expect(toggleSilenceAction?.guardrails).toContain('cross-tenant silence rule updates must return 404');
    expect(isCoveredByApisix('/v1/notifications/settings')).toBe(true);
    expect(isCoveredByApisix('/v1/notifications/test')).toBe(true);
    expect(isCoveredByApisix('/v1/notifications/silence-rules')).toBe(true);
    expect(isCoveredByApisix('/v1/notifications/silence-rules/rule-001')).toBe(true);
  });

  it('binds settings governance to user settings and token lifecycle APIs', () => {
    const endpoints = getPlanEndpoints(getPageApiPlan('settings'));
    const saveAction = getPageActionPlan('settings', 'settings-preferences-save');
    const createAction = getPageActionPlan('settings', 'settings-token-create');
    const regenerateAction = getPageActionPlan('settings', 'settings-token-regenerate');
    const revokeAction = getPageActionPlan('settings', 'settings-token-revoke');
    const scopeAction = getPageActionPlan('settings', 'settings-token-scope-update');
    const validateAction = getPageActionPlan('settings', 'settings-token-validate');

    expect(endpoints).toContain('/v1/tokens/scopes');
    expect(endpoints).toContain('/v1/tokens');
    expect(endpoints).toContain('/v1/tokens/scopes/probe');
    expect(endpoints).toContain('/v1/auth/settings/display');
    expect(endpoints).toContain('/v1/auth/settings/{category}');
    expect(endpoints).toContain('/v1/tokens/{id}/regenerate');
    expect(endpoints).toContain('/v1/tokens/{id}/revoke');
    expect(endpoints).toContain('/v1/tokens/{id}/scopes');
    expect(endpoints).toContain('/v1/tokens/validate');
    expect(saveAction?.method).toBe('PUT');
    expect(saveAction?.requiredScopes).toContain('token:read');
    expect(saveAction?.auditEvent).toBe('USER_UPDATE');
    expect(saveAction?.guardrails).toContain('settings category must be notifications or display');
    expect(createAction?.method).toBe('POST');
    expect(createAction?.requiredScopes).toContain('token:write');
    expect(createAction?.auditEvent).toBe('create_token');
    expect(createAction?.guardrails).toContain('plain token is returned only once and token_hash must never be returned');
    expect(regenerateAction?.method).toBe('POST');
    expect(regenerateAction?.guardrails).toContain('old raw token must be rejected after regenerate');
    expect(revokeAction?.method).toBe('POST');
    expect(revokeAction?.guardrails).toContain('revoked raw tokens must fail /v1/tokens/validate');
    expect(scopeAction?.method).toBe('PUT');
    expect(scopeAction?.auditEvent).toBe('update_token_scopes');
    expect(scopeAction?.guardrails).toContain('invalid scopes must be rejected by auth-service');
    expect(validateAction?.method).toBe('POST');
    expect(validateAction?.guardrails).toContain('validation responses must not expose token_hash');
    expect(isCoveredByApisix('/v1/auth/settings/display')).toBe(true);
    expect(isCoveredByApisix('/v1/tokens')).toBe(true);
    expect(isCoveredByApisix('/v1/tokens/token-001/regenerate')).toBe(true);
    expect(isCoveredByApisix('/v1/tokens/token-001/revoke')).toBe(true);
    expect(isCoveredByApisix('/v1/tokens/token-001/scopes')).toBe(true);
    expect(isCoveredByApisix('/v1/tokens/validate')).toBe(true);
  });
});

function isCoveredByApisix(endpoint: string) {
  return apisixRoutePrefixes.some((prefix) => endpoint === prefix || endpoint.startsWith(`${prefix}/`));
}

function stripApiPrefix(endpoint: string) {
  return endpoint.replace(/^\/api/, '');
}
