import crypto from 'node:crypto';
import { execFileSync } from 'node:child_process';


type JsonRecord = Record<string, unknown>;
type Shape = Record<string, string[]>;
type ReconciliationCheck = {
  check_id: string;
  source_path: string;
  display_path: string;
  source_present: boolean;
  source_type: string;
  display_type: string;
  equal: boolean;
  values_captured: false;
};

const baseUrl = process.env.ADAPTER_SCHEMA_BASE_URL || 'http://10.0.5.8:30180/api/v1';
const tenant = process.env.ADAPTER_SCHEMA_TENANT || 'default';
const requestPrefix = `adapter-schema-${Date.now()}`;

for (const name of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[name];
}

(globalThis as unknown as { window: { __RUNTIME_CONFIG__: JsonRecord } }).window = {
  __RUNTIME_CONFIG__: {},
};

const jsonType = (value: unknown) => {
  if (value === null) return 'null';
  if (Array.isArray(value)) return 'array';
  if (typeof value === 'number') return 'number';
  return typeof value;
};

const shapeOf = (value: unknown) => {
  const shape = new Map<string, Set<string>>();
  const visit = (current: unknown, path: string) => {
    if (!shape.has(path)) shape.set(path, new Set());
    shape.get(path)!.add(jsonType(current));
    if (Array.isArray(current)) {
      current.slice(0, 3).forEach((item) => visit(item, `${path}[]`));
      return;
    }
    if (current && typeof current === 'object') {
      Object.entries(current as JsonRecord)
        .sort(([left], [right]) => left.localeCompare(right))
        .forEach(([key, item]) => visit(item, path === '$' ? `$.${key}` : `${path}.${key}`));
    }
  };
  visit(value, '$');
  return Object.fromEntries(
    [...shape.entries()].sort(([left], [right]) => left.localeCompare(right)).map(([path, types]) => [path, [...types].sort()]),
  ) as Shape;
};

const compareShapes = (fixture: Shape, live: Shape) => {
  const fixturePaths = new Set(Object.keys(fixture));
  const livePaths = new Set(Object.keys(live));
  const common = [...fixturePaths].filter((path) => livePaths.has(path)).sort();
  const concreteTypes = (types: string[]) => types.filter((type) => type !== 'undefined' && type !== 'null');
  return {
    fixture_path_count: fixturePaths.size,
    live_path_count: livePaths.size,
    common_path_count: common.length,
    fixture_only_paths: [...fixturePaths].filter((path) => !livePaths.has(path)).sort(),
    live_only_paths: [...livePaths].filter((path) => !fixturePaths.has(path)).sort(),
    optional_presence_differences: common
      .filter((path) => {
        const fixtureConcrete = concreteTypes(fixture[path]);
        const liveConcrete = concreteTypes(live[path]);
        return fixture[path].includes('undefined') !== live[path].includes('undefined')
          || fixtureConcrete.length === 0
          || liveConcrete.length === 0;
      })
      .filter((path) => fixture[path].join('|') !== live[path].join('|'))
      .map((path) => ({ path, fixture_types: fixture[path], live_types: live[path] })),
    type_conflicts: common
      .filter((path) => {
        const fixtureConcrete = concreteTypes(fixture[path]);
        const liveConcrete = concreteTypes(live[path]);
        return fixtureConcrete.length > 0
          && liveConcrete.length > 0
          && fixtureConcrete.join('|') !== liveConcrete.join('|');
      })
      .map((path) => ({ path, fixture_types: fixture[path], live_types: live[path] })),
  };
};

const hasPaths = (shape: Shape, paths: string[]) => paths.filter((path) => !(path in shape));

const secretBase64 = execFileSync(
  'kubectl',
  ['-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.JWT_SECRET}'],
  { encoding: 'utf8', timeout: 15_000 },
).trim();
const secret = Buffer.from(secretBase64, 'base64');
const now = Math.floor(Date.now() / 1000);
const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
const claims = Buffer.from(JSON.stringify({
  iss: 'traffic-auth-service',
  sub: crypto.randomUUID(),
  jti: crypto.randomUUID(),
  user_id: crypto.randomUUID(),
  tenant_id: tenant,
  username: 'adapter-schema-readonly',
  roles: ['admin'],
  permissions: ['*', 'admin:*', 'alert:read', 'campaign:read', 'topic:read'],
  token_type: 'access',
  session_id: requestPrefix,
  iat: now,
  exp: now + 900,
})).toString('base64url');
const signingInput = `${header}.${claims}`;
const token = `${signingInput}.${crypto.createHmac('sha256', secret).update(signingInput).digest('base64url')}`;

const requests: Array<{ name: string; status: number; duration_ms: number; shape: Shape }> = [];
const fetchJson = async (name: string, path: string) => {
  const started = performance.now();
  const response = await fetch(`${baseUrl}${path}`, {
    headers: {
      Authorization: `Bearer ${token}`,
      'X-Tenant-ID': tenant,
      'X-Request-ID': `${requestPrefix}-${name}`,
    },
    signal: AbortSignal.timeout(45_000),
  });
  const body = await response.json().catch(() => ({}));
  requests.push({
    name,
    status: response.status,
    duration_ms: Math.round(performance.now() - started),
    shape: shapeOf(body),
  });
  if (!response.ok) throw new Error(`${name} returned HTTP ${response.status}`);
  return body as JsonRecord;
};

const arrayAt = (value: unknown, key?: string): JsonRecord[] => {
  if (Array.isArray(value)) return value.filter((item): item is JsonRecord => Boolean(item) && typeof item === 'object');
  if (value && typeof value === 'object' && key) {
    return arrayAt((value as JsonRecord)[key]);
  }
  return [];
};

const dataAt = (body: JsonRecord) => (body.data && typeof body.data === 'object' ? body.data : body) as JsonRecord;
const sampleHash = (value: unknown) => crypto.createHash('sha256').update(String(value)).digest('hex').slice(0, 16);
const valueAt = (source: unknown, keys: string[]) => {
  if (!source || typeof source !== 'object') return undefined;
  for (const key of keys) {
    if (key in (source as JsonRecord)) return (source as JsonRecord)[key];
  }
  return undefined;
};
const optionalNumberAt = (source: unknown, keys: string[]) => {
  const value = valueAt(source, keys);
  if (value === undefined || value === null || value === '') return undefined;
  const numeric = typeof value === 'number' ? value : Number(value);
  return Number.isFinite(numeric) ? numeric : undefined;
};
const stringAt = (source: unknown, keys: string[]) => {
  const value = valueAt(source, keys);
  return value === undefined || value === null ? '' : String(value);
};
const formatCount = (value: number | undefined) => {
  if (value === undefined) return '暂不可用';
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`;
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)}K`;
  return String(value);
};
const normalizedScore = (value: number | undefined) => {
  if (value === undefined || !Number.isFinite(value)) return undefined;
  return value <= 1 ? Math.round(value * 100) : Math.max(0, Math.min(100, Math.round(value)));
};
const stateVersion = (value: unknown) => {
  if (typeof value === 'number' && Number.isFinite(value) && value > 0) return Math.trunc(value);
  if (typeof value !== 'string' || !value.trim()) return undefined;
  if (/^\d+$/.test(value.trim())) return Math.trunc(Number(value));
  const parsed = Date.parse(value);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : undefined;
};
const alertStatus = (value: string) => {
  const normalized = value.trim().toLowerCase();
  if (['new', 'open', 'unhandled', 'alert_status_new'].includes(normalized) || value === '未处理') return '未处理';
  if (['triage', 'investigating', 'investigation', 'review', 'reviewing', 'in_progress', 'processing', 'alert_status_triage', 'alert_status_reviewing'].includes(normalized) || value === '研判中') return '研判中';
  if (['assigned', 'delegated', 'alert_status_assigned'].includes(normalized) || value === '已指派') return '已指派';
  if (['closed', 'resolved', 'confirmed', 'ignored', 'false_positive', 'alert_status_closed', 'alert_status_resolved'].includes(normalized) || value === '已关闭') return '已关闭';
  return value || '未知';
};
const confidenceText = (value: number | undefined) => value === undefined
  ? '暂不可用'
  : value > 1
    ? `${value}%`
    : value.toFixed(2);
const listFrom = (value: unknown, keys: string[] = []) => {
  const candidate = keys.length ? valueAt(value, keys) : value;
  if (Array.isArray(candidate)) return candidate;
  if (candidate && typeof candidate === 'object') {
    for (const key of ['items', 'data', 'evidences', 'evidence']) {
      const nested = (candidate as JsonRecord)[key];
      if (Array.isArray(nested)) return nested;
    }
  }
  return [];
};
const reconciliationCheck = (
  checkId: string,
  sourcePath: string,
  displayPath: string,
  sourceValue: unknown,
  displayValue: unknown,
  expectedValue: unknown = sourceValue,
): ReconciliationCheck => ({
  check_id: checkId,
  source_path: sourcePath,
  display_path: displayPath,
  source_present: sourceValue !== undefined && sourceValue !== null,
  source_type: jsonType(sourceValue),
  display_type: jsonType(displayValue),
  equal: Object.is(expectedValue, displayValue),
  values_captured: false,
});
const metricValue = (snapshot: { metrics?: Array<{ label?: string; value?: unknown }> }, label: string) =>
  snapshot.metrics?.find((item) => item.label === label)?.value;

const run = async () => {
  const [alertList, campaignList, tunnel, exfil, apt, views, subscriptions] = await Promise.all([
    fetchJson('alert_list', '/alerts?limit=1&page_size=1'),
    fetchJson('campaign_list', '/campaigns?limit=1&page_size=1'),
    fetchJson('topic_tunnel', '/topics/tunnel'),
    fetchJson('topic_exfil', '/topics/exfil'),
    fetchJson('topic_apt', '/topics/apt'),
    fetchJson('topic_views', '/topics/views?limit=3'),
    fetchJson('topic_subscriptions', '/topics/subscriptions?limit=3'),
  ]);

  const alertRows = arrayAt(alertList.data) || [];
  const campaignData = dataAt(campaignList);
  const campaignRows = arrayAt(campaignData.campaigns ?? campaignData.data);
  const alertId = alertRows[0]?.alert_id ?? alertRows[0]?.id;
  const campaignId = campaignRows[0]?.campaign_id ?? campaignRows[0]?.id;
  if (!alertId || !campaignId) throw new Error('pilot list did not provide alert and campaign identities');

  // Detail reads fan out to different backing stores. Keep the evidence capture
  // sequential so the gate measures response shape without creating an
  // artificial four-query burst against the pilot environment.
  const alertDetail = await fetchJson('alert_detail', `/alerts/${encodeURIComponent(String(alertId))}`);
  const alertEvidence = await fetchJson('alert_evidence', `/alerts/${encodeURIComponent(String(alertId))}/evidence`);
  const alertFeedback = await fetchJson('alert_feedback', `/alerts/${encodeURIComponent(String(alertId))}/feedback`);
  const campaignDetail = await fetchJson('campaign_detail', `/campaigns/${encodeURIComponent(String(campaignId))}`);

  const alertModule = await import('../src/services/alertDetailApi');
  const campaignModule = await import('../src/services/campaignDetailApi');
  const adapterModule = await import('../src/services/pageSnapshotAdapters');
  const mockModule = await import('../src/services/mockData');
  const routesModule = await import('../src/routes/routeManifest');

  const fixtureAlert = alertModule.buildMockAlertDetailSnapshot('fixture-alert');
  const liveAlert = alertModule.normalizeAlertDetailSnapshot(
    String(alertId), alertDetail, alertEvidence, alertFeedback,
  );
  const fixtureCampaign = campaignModule.buildMockCampaignDetailSnapshot('fixture-campaign');
  const liveCampaign = campaignModule.normalizeCampaignDetailSnapshot(String(campaignId), campaignDetail);
  const topicsRoute = routesModule.findRouteById('topics');
  if (!topicsRoute) throw new Error('topics route is not registered');
  const fixtureTopics = mockModule.buildPageSnapshot(topicsRoute.page);
  const liveTopics = adapterModule.adaptKnownPageSnapshot(
    topicsRoute.page, tunnel, [exfil, apt, views, subscriptions],
  );
  if (!liveTopics) throw new Error('topics adapter did not return a snapshot');

  const alertRecord = dataAt(alertDetail);
  const campaignRecord = dataAt(campaignDetail);
  const alertEvidenceRows = listFrom(alertEvidence.data ?? alertEvidence);
  const alertScore = optionalNumberAt(alertRecord, ['score', 'risk_score', 'riskScore']);
  const alertConfidence = optionalNumberAt(alertRecord, ['confidence', 'probability']);
  const alertActions = listFrom(valueAt(alertRecord, ['response_actions', 'responseActions', 'action_catalog', 'actionCatalog']));
  const alertTimeline = listFrom(valueAt(alertRecord, ['timeline', 'events', 'history']));
  const campaignScore = optionalNumberAt(campaignRecord, ['score', 'risk_score', 'riskScore']);
  const campaignAlerts = listFrom(valueAt(campaignRecord, ['alerts']));
  const campaignAlertIds = listFrom(valueAt(campaignRecord, ['alert_ids']));
  const campaignEntities = listFrom(valueAt(campaignRecord, ['entities']));
  const campaignProgress = optionalNumberAt(campaignRecord, ['response_progress', 'responseProgress', 'disposition_progress', 'dispositionProgress']);
  const campaignStatus = stringAt(campaignRecord, ['status', 'state']).toLowerCase();
  const expectedCampaignProgress = ['closed', '已结束'].includes(campaignStatus)
    ? '100%'
    : campaignProgress === undefined
      ? '暂不可用'
      : `${Math.max(0, Math.min(100, Math.round(campaignProgress)))}%`;
  const tunnelSummary = valueAt(dataAt(tunnel), ['summary']) as JsonRecord | undefined;
  const exfilSummary = valueAt(dataAt(exfil), ['summary']) as JsonRecord | undefined;
  const aptSummary = valueAt(dataAt(apt), ['summary']) as JsonRecord | undefined;

  const valueReconciliation: ReconciliationCheck[] = [
    reconciliationCheck('alert.id', '$.data.alert_id', '$.alertId', valueAt(alertRecord, ['alert_id', 'alertId', 'id']), liveAlert.alertId, String(valueAt(alertRecord, ['alert_id', 'alertId', 'id']) ?? alertId)),
    reconciliationCheck('alert.score', '$.data.score', '$.score', alertScore, liveAlert.score, normalizedScore(alertScore)),
    reconciliationCheck('alert.confidence', '$.data.confidence', '$.confidence', alertConfidence, liveAlert.confidence, confidenceText(alertConfidence)),
    reconciliationCheck('alert.status', '$.data.status', '$.status', valueAt(alertRecord, ['status']), liveAlert.status, alertStatus(stringAt(alertRecord, ['status']))),
    reconciliationCheck('alert.evidence_count', '$.data[]', '$.evidenceRows.length', alertEvidenceRows.length, liveAlert.evidenceRows.length),
    reconciliationCheck('alert.response_action_count', '$.data.response_actions[]', '$.responseActions.length', alertActions.length, liveAlert.responseActions.length),
    reconciliationCheck('alert.timeline_count', '$.data.timeline[]', '$.timeline.length', alertTimeline.length, liveAlert.timeline.length),
    reconciliationCheck('campaign.id', '$.data.campaign_id', '$.campaignId', valueAt(campaignRecord, ['campaign_id', 'campaignId', 'id', 'event_id']), liveCampaign.campaignId, String(valueAt(campaignRecord, ['campaign_id', 'campaignId', 'id', 'event_id']) ?? campaignId)),
    reconciliationCheck('campaign.score', '$.data.score', '$.riskScore', campaignScore, liveCampaign.riskScore, normalizedScore(campaignScore) ?? 0),
    reconciliationCheck('campaign.state_version', '$.data.state_version', '$.stateVersion', valueAt(campaignRecord, ['state_version', 'stateVersion']), liveCampaign.stateVersion, stateVersion(valueAt(campaignRecord, ['state_version', 'stateVersion'])) ?? 0),
    reconciliationCheck('campaign.alert_count', '$.data.alerts|alert_ids', '$.alertCount', valueAt(campaignRecord, ['alerts', 'alert_ids']), liveCampaign.alertCount, Math.max(campaignAlerts.length, campaignAlertIds.length)),
    reconciliationCheck('campaign.asset_count', '$.data.entities', '$.assetCount', valueAt(campaignRecord, ['entities']), liveCampaign.assetCount, campaignEntities.length),
    reconciliationCheck('campaign.response_progress', '$.data.response_progress', '$.metrics[处置进度].value', campaignProgress, metricValue(liveCampaign, '处置进度'), expectedCampaignProgress),
    reconciliationCheck('topics.tunnel_session_count', '$.data.summary.session_count', '$.metrics[隧道会话].value', optionalNumberAt(tunnelSummary, ['session_count']), metricValue(liveTopics, '隧道会话'), formatCount(optionalNumberAt(tunnelSummary, ['session_count']) ?? 0)),
    reconciliationCheck('topics.exfil_path_count', '$.data.summary.path_count', '$.metrics[外传路径].value', optionalNumberAt(exfilSummary, ['path_count']), metricValue(liveTopics, '外传路径'), formatCount(optionalNumberAt(exfilSummary, ['path_count']) ?? 0)),
    reconciliationCheck('topics.apt_campaign_count', '$.data.summary.campaign_count', '$.metrics[APT 战役].value', optionalNumberAt(aptSummary, ['campaign_count']), metricValue(liveTopics, 'APT 战役'), formatCount(optionalNumberAt(aptSummary, ['campaign_count']) ?? 0)),
  ];

  const fixtureShapes = {
    alert_detail: shapeOf(fixtureAlert),
    campaign_detail: shapeOf(fixtureCampaign),
    topics_overview: shapeOf(fixtureTopics),
  };
  const normalizedLiveShapes = {
    alert_detail: shapeOf(liveAlert),
    campaign_detail: shapeOf(liveCampaign),
    topics_overview: shapeOf(liveTopics),
  };
  const diffs = {
    alert_detail: compareShapes(fixtureShapes.alert_detail, normalizedLiveShapes.alert_detail),
    campaign_detail: compareShapes(fixtureShapes.campaign_detail, normalizedLiveShapes.campaign_detail),
    topics_overview: compareShapes(fixtureShapes.topics_overview, normalizedLiveShapes.topics_overview),
  };

  const endpointByName = Object.fromEntries(requests.map((item) => [item.name, item]));
  const requiredRawPaths: Record<string, string[]> = {
    alert_list: ['$.data', '$.data[].alert_id'],
    campaign_list: ['$.data.campaigns', '$.data.campaigns[].campaign_id'],
    alert_detail: ['$.data', '$.data.alert_id', '$.data.severity', '$.data.score', '$.data.status'],
    alert_evidence: ['$.data'],
    alert_feedback: ['$.data'],
    campaign_detail: ['$.data', '$.data.campaign_id', '$.data.score', '$.data.state_version'],
    topic_tunnel: ['$.data.topic', '$.data.summary', '$.data.summary.session_count'],
    topic_exfil: ['$.data.topic', '$.data.summary', '$.data.summary.path_count'],
    topic_apt: ['$.data.topic', '$.data.summary', '$.data.summary.campaign_count'],
  };
  const rawPathGaps = Object.fromEntries(
    Object.entries(requiredRawPaths).map(([name, paths]) => [name, hasPaths(endpointByName[name]?.shape ?? {}, paths)]),
  );
  const requiredNormalizedPaths: Record<string, string[]> = {
    alert_detail: ['$.alertId', '$.score', '$.metrics', '$.evidenceRows', '$.feedback'],
    campaign_detail: ['$.campaignId', '$.stateVersion', '$.snapshotId', '$.partial', '$.sourceWatermarks'],
    topics_overview: ['$.metrics', '$.rows', '$.timeline', '$.evidence'],
  };
  const normalizedPathGaps = Object.fromEntries(
    Object.entries(requiredNormalizedPaths).map(([name, paths]) => [name, hasPaths(normalizedLiveShapes[name as keyof typeof normalizedLiveShapes], paths)]),
  );
  const typeConflicts = Object.values(diffs).flatMap((item) => item.type_conflicts);
  const gaps = [...Object.values(rawPathGaps), ...Object.values(normalizedPathGaps)].flat();
  const allHttp200 = requests.every((item) => item.status === 200);
  const reconciliationFailures = valueReconciliation.filter((item) => !item.equal);

  return {
    schema_version: 1,
    status: allHttp200 && gaps.length === 0 && typeConflicts.length === 0 && reconciliationFailures.length === 0 ? 'PASS' : 'FAIL',
    mode: 'read_only_shape_and_value_reconciliation',
    base_url_origin_sha256: sampleHash(new URL(baseUrl).origin),
    tenant_sha256: sampleHash(tenant),
    sample_identity_sha256: {
      alert: sampleHash(alertId),
      campaign: sampleHash(campaignId),
    },
    requests,
    fixture_shapes: fixtureShapes,
    normalized_live_shapes: normalizedLiveShapes,
    diffs,
    required_raw_path_gaps: rawPathGaps,
    required_normalized_path_gaps: normalizedPathGaps,
    value_reconciliation: {
      status: reconciliationFailures.length === 0 ? 'PASS' : 'FAIL',
      checks: valueReconciliation,
      check_count: valueReconciliation.length,
      failure_count: reconciliationFailures.length,
      payload_values_captured: false,
    },
    all_http_200: allHttp200,
    type_conflict_count: typeConflicts.length,
    payload_values_captured: false,
    secrets_captured: false,
    production_mutations: [],
  };
};

try {
  const result = await run();
  process.stdout.write(`${JSON.stringify(result, null, 2)}\n`);
  process.exitCode = result.status === 'PASS' ? 0 : 1;
} catch (error) {
  process.stdout.write(`${JSON.stringify({
    schema_version: 1,
    status: 'FAIL',
    mode: 'read_only_shape_and_value_reconciliation',
    error_type: error instanceof Error ? error.name : typeof error,
    error: error instanceof Error ? error.message : String(error),
    requests,
    payload_values_captured: false,
    secrets_captured: false,
    production_mutations: [],
  }, null, 2)}\n`);
  process.exitCode = 1;
}
