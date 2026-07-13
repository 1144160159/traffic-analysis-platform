#!/usr/bin/env bash
set -euo pipefail

APISIX="${APISIX:-http://10.0.5.8:30180}"
LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/20260629-ui-contract-preflight}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-ui-contract-preflight}"
REGRESSION_DIR="${REGRESSION_DIR:-doc/02_acceptance/02-regression}"
ALLOW_BLOCKERS="${ALLOW_BLOCKERS:-false}"
DESKTOP_CHROME_STATUS="${DESKTOP_CHROME_STATUS:-missing}"
DESKTOP_CHROME_URL="${DESKTOP_CHROME_URL:-}"
DESKTOP_CHROME_TITLE="${DESKTOP_CHROME_TITLE:-}"
DESKTOP_CHROME_DETAIL="${DESKTOP_CHROME_DETAIL:-not provided by MCP wrapper}"
DESKTOP_CHROME_ARTIFACT="${DESKTOP_CHROME_ARTIFACT:-}"
DESKTOP_CHROME_BUSINESS_STATUS="${DESKTOP_CHROME_BUSINESS_STATUS:-missing}"
DESKTOP_CHROME_BUSINESS_URL="${DESKTOP_CHROME_BUSINESS_URL:-}"
DESKTOP_CHROME_BUSINESS_DETAIL="${DESKTOP_CHROME_BUSINESS_DETAIL:-not provided by Desktop Chrome business smoke}"
DESKTOP_CHROME_BUSINESS_ARTIFACT="${DESKTOP_CHROME_BUSINESS_ARTIFACT:-}"
EXPECTED_LOGIN_TITLE="${EXPECTED_LOGIN_TITLE:-园区网络全流量采集与分析系统}"

REPORT="$LOG_DIR/live-ui-contract-preflight-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-ui-contract-preflight-$RUN_ID-summary.json"
LOCAL_REPORT="$LOG_DIR/local-report.md"
CONTRACT_MATRIX="$LOG_DIR/ui-contract-matrix.json"

mkdir -p "$LOG_DIR" "$REGRESSION_DIR"
: >"$REPORT"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 2
  fi
}

json_log() {
  local phase="$1" name="$2" severity="$3" passed="$4" status="$5" detail="${6:-}" artifact="${7:-}"
  jq -nc \
    --arg ts "$(date -Iseconds)" \
    --arg phase "$phase" \
    --arg name "$name" \
    --arg severity "$severity" \
    --argjson passed "$passed" \
    --arg status "$status" \
    --arg detail "$detail" \
    --arg artifact "$artifact" \
    '{ts:$ts, phase:$phase, name:$name, severity:$severity, passed:$passed, status:$status, detail:$detail, artifact:$artifact}' >>"$REPORT"
}

trim_file() {
  local file="$1"
  if [[ -s "$file" ]]; then
    head -c 1200 "$file" | tr '\n' ' '
  fi
}

run_command() {
  local phase="$1" name="$2" severity="$3" artifact="$4"
  shift 4
  local out_file="$LOG_DIR/$artifact.out"
  local err_file="$LOG_DIR/$artifact.err"
  set +e
  "$@" >"$out_file" 2>"$err_file"
  local rc=$?
  set -e
  if [[ "$rc" -eq 0 ]]; then
    json_log "$phase" "$name" "info" true "ok" "$(trim_file "$out_file")" "$artifact.out"
  else
    json_log "$phase" "$name" "$severity" false "rc=$rc" "$(trim_file "$err_file")" "$artifact.err"
  fi
  return 0
}

need_cmd git
need_cmd jq
need_cmd node
need_cmd npm
need_cmd curl

git rev-parse HEAD >"$LOG_DIR/commit-sha.txt"
git branch --show-current >"$LOG_DIR/git-branch.txt"
git status --short >"$LOG_DIR/git-status.txt"
git diff --stat >"$LOG_DIR/git-diff-stat.txt" || true

run_command "repo" "UI suite frontend contracts validate" "blocker" "validate-frontend-contracts" \
  node doc/04_assets/ui_suite_gpt_v1/validate_frontend_contracts.mjs

VALIDATE_SUMMARY="$(grep -E 'errors: [0-9]+, warnings: [0-9]+' "$LOG_DIR/validate-frontend-contracts.out" | tail -n 1 || true)"
if [[ -n "$VALIDATE_SUMMARY" ]]; then
  VALIDATE_ERRORS="$(sed -E 's/.*errors: ([0-9]+), warnings: ([0-9]+).*/\1/' <<<"$VALIDATE_SUMMARY")"
  VALIDATE_WARNINGS="$(sed -E 's/.*errors: ([0-9]+), warnings: ([0-9]+).*/\2/' <<<"$VALIDATE_SUMMARY")"
  if [[ "$VALIDATE_ERRORS" -eq 0 && "$VALIDATE_WARNINGS" -eq 0 ]]; then
    json_log "contract" "UI suite validator has zero errors and zero warnings" "info" true "ok" "$VALIDATE_SUMMARY" "validate-frontend-contracts.out"
  elif [[ "$VALIDATE_ERRORS" -eq 0 ]]; then
    json_log "contract" "UI suite validator has warnings" "warn" false "warnings=$VALIDATE_WARNINGS" "$VALIDATE_SUMMARY" "validate-frontend-contracts.out"
  else
    json_log "contract" "UI suite validator has errors" "blocker" false "errors=$VALIDATE_ERRORS" "$VALIDATE_SUMMARY" "validate-frontend-contracts.out"
  fi
else
  json_log "contract" "UI suite validator summary parse" "warn" false "missing" "validator did not print errors/warnings summary" "validate-frontend-contracts.out"
fi

run_command "test" "routeManifest and route access Vitest" "blocker" "web-route-vitest" \
  npm --prefix web/ui run test -- --run src/routes/routeManifest.test.ts src/routes/access.test.ts

run_command "live" "APISIX login HTML reachable by HTTP GET" "blocker" "apisix-login-get" \
  curl --noproxy '*' -sS -L -m 15 "$APISIX/login"

if grep -q '<div id="root"' "$LOG_DIR/apisix-login-get.out"; then
  json_log "live" "APISIX login returns Web UI shell" "info" true "ok" "$APISIX/login" "apisix-login-get.out"
else
  json_log "live" "APISIX login returns Web UI shell" "blocker" false "missing-root" "login response did not contain root app shell" "apisix-login-get.out"
fi

DESKTOP_STATUS_NORMALIZED="$(tr '[:upper:]' '[:lower:]' <<<"$DESKTOP_CHROME_STATUS")"
case "$DESKTOP_STATUS_NORMALIZED" in
  pass|passed|ok)
    if [[ "$DESKTOP_CHROME_URL" == "$APISIX/login"* && "$DESKTOP_CHROME_TITLE" == "$EXPECTED_LOGIN_TITLE" ]]; then
      json_log "browser" "Desktop Chrome wrapper opened login page" "info" true "ok" "url=$DESKTOP_CHROME_URL title=$DESKTOP_CHROME_TITLE" "$DESKTOP_CHROME_ARTIFACT"
    else
      json_log "browser" "Desktop Chrome wrapper opened login page" "blocker" false "mismatch" "url=$DESKTOP_CHROME_URL title=$DESKTOP_CHROME_TITLE expected_url=$APISIX/login expected_title=$EXPECTED_LOGIN_TITLE" "$DESKTOP_CHROME_ARTIFACT"
    fi
    ;;
  *)
    json_log "browser" "Desktop Chrome wrapper opened login page" "blocker" false "$DESKTOP_STATUS_NORMALIZED" "$DESKTOP_CHROME_DETAIL" ""
    ;;
esac

BUSINESS_STATUS_NORMALIZED="$(tr '[:upper:]' '[:lower:]' <<<"$DESKTOP_CHROME_BUSINESS_STATUS")"
case "$BUSINESS_STATUS_NORMALIZED" in
  pass|passed|ok)
    if [[ "$DESKTOP_CHROME_BUSINESS_URL" == "$APISIX/alerts"* || "$DESKTOP_CHROME_BUSINESS_URL" == "$APISIX/dashboard"* ]]; then
      json_log "browser" "Desktop Chrome wrapper opened protected business page" "info" true "ok" "$DESKTOP_CHROME_BUSINESS_DETAIL url=$DESKTOP_CHROME_BUSINESS_URL" "$DESKTOP_CHROME_BUSINESS_ARTIFACT"
    else
      json_log "browser" "Desktop Chrome wrapper opened protected business page" "blocker" false "mismatch" "$DESKTOP_CHROME_BUSINESS_DETAIL url=$DESKTOP_CHROME_BUSINESS_URL expected=$APISIX/alerts or $APISIX/dashboard" "$DESKTOP_CHROME_BUSINESS_ARTIFACT"
    fi
    ;;
  *)
    json_log "browser" "Desktop Chrome wrapper opened protected business page" "blocker" false "$BUSINESS_STATUS_NORMALIZED" "$DESKTOP_CHROME_BUSINESS_DETAIL" "$DESKTOP_CHROME_BUSINESS_ARTIFACT"
    ;;
esac

node >"$CONTRACT_MATRIX" <<'JS'
const fs = require('node:fs');
const path = require('node:path');

const ROOT = process.cwd();
const read = (file) => fs.readFileSync(path.join(ROOT, file), 'utf8');
const readJson = (file) => JSON.parse(read(file));
const exists = (file) => fs.existsSync(path.join(ROOT, file));

const checks = [];
const add = (name, severity, passed, detail = {}, artifact = '') => {
  checks.push({ name, severity, passed, detail, artifact });
};

const designDoc = read('doc/01_design/面向园区网络的全流量采集分析系统-左侧菜单信息架构.md');
const routeText = read('web/ui/src/routes/routeManifest.tsx');
const routeMap = readJson('doc/04_assets/ui_suite_gpt_v1/specs/route-page-map.json');
const taskMatrix = readJson('doc/04_assets/ui_suite_gpt_v1/specs/frontend-task-matrix.json');
const flows = readJson('doc/04_assets/ui_suite_gpt_v1/specs/business-flow-acceptance.json');
const codeGap = exists('doc/04_assets/ui_suite_gpt_v1/specs/frontend-code-gap.json')
  ? readJson('doc/04_assets/ui_suite_gpt_v1/specs/frontend-code-gap.json')
  : null;

const menuSection = designDoc
  .split('## 2. 一级菜单与二级菜单')[1]
  .split('## 3. 与旧菜单的差异')[0];
const expectedMenu = menuSection
  .split('\n')
  .map((line) => line.match(/^\|\s*([^|]+?)\s*\|\s*([^|]+?)\s*\|\s*`([^`]+)`\s*\|/))
  .filter(Boolean)
  .map((match) => ({
    groupTitle: match[1].trim(),
    title: match[2].trim(),
    path: match[3].trim(),
  }))
  .filter((row) => row.groupTitle !== '一级菜单' && row.path.startsWith('/'));

const expectedGroups = [
  ['overview', '综合态势'],
  ['collection-monitoring', '采集监测'],
  ['threat-analysis', '威胁分析'],
  ['asset-graph', '资产图谱'],
  ['detection-ops', '检测运营'],
  ['audit-config', '审计配置'],
];

const navStart = routeText.indexOf('export const navGroups');
const navEnd = routeText.indexOf('export const navRoutes');
const navSection = routeText.slice(navStart, navEnd);
const groups = [];
const groupRe = /\{\s*id:\s*'([^']+)',\s*title:\s*'([^']+)',[\s\S]*?children:\s*\[([\s\S]*?)\n\s*\],\n\s*\}/g;
for (const match of navSection.matchAll(groupRe)) {
  const children = [...match[3].matchAll(/makeRoute\('([^']+)',\s*'([^']+)',\s*'([^']+)',\s*'([^']+)'/g)].map((route) => ({
    domain: route[1],
    id: route[2],
    title: route[3],
    path: route[4],
  }));
  groups.push({ id: match[1], title: match[2], children });
}
const navRoutes = groups.flatMap((group) =>
  group.children.map((route) => ({
    groupId: group.id,
    groupTitle: group.title,
    title: route.title,
    path: route.path,
    id: route.id,
    domain: route.domain,
  })),
);

const legacySection = routeText.slice(routeText.indexOf('export const legacyTopicRoutes'), routeText.indexOf('export const detailRoutes'));
const legacyTopicPaths = [...legacySection.matchAll(/makeRoute\('overview',\s*'([^']+)',\s*'([^']+)',\s*'([^']+)'/g)].map((match) => match[3]);

const detailSection = routeText.slice(routeText.indexOf('export const detailRoutes'), routeText.indexOf('export const allRoutes'));
const detailPaths = [...detailSection.matchAll(/makeRoute\('threat-analysis',\s*'([^']+)',\s*'([^']+)',\s*'([^']+)'/g)].map((match) => match[3]);

const sameJson = (left, right) => JSON.stringify(left) === JSON.stringify(right);
const expectedGroupActual = groups.map((group) => [group.id, group.title]);
add('routeManifest top-level groups match design order', 'blocker', sameJson(expectedGroupActual, expectedGroups), {
  expected: expectedGroups,
  actual: expectedGroupActual,
}, 'web/ui/src/routes/routeManifest.tsx');

add('design menu table extracts 24 active second-level routes', 'blocker', expectedMenu.length === 24, {
  count: expectedMenu.length,
  routes: expectedMenu.map((row) => row.path),
}, 'doc/01_design/面向园区网络的全流量采集分析系统-左侧菜单信息架构.md');

const expectedRouteRows = expectedMenu.map((row) => [row.groupTitle, row.title, row.path]);
const actualRouteRows = navRoutes.map((row) => [row.groupTitle, row.title, row.path]);
add('routeManifest keeps all 24 menu routes in designed group and order', 'blocker', sameJson(actualRouteRows, expectedRouteRows), {
  expected: expectedRouteRows,
  actual: actualRouteRows,
}, 'web/ui/src/routes/routeManifest.tsx');

const navPathSet = new Set(navRoutes.map((route) => route.path));
const legacyExpected = ['/topics/tunnel', '/topics/exfil', '/topics/apt'];
add('legacy topic deep links are not left-nav items', 'blocker', legacyExpected.every((route) => !navPathSet.has(route)), {
  legacyExpected,
  navPaths: [...navPathSet],
}, 'web/ui/src/routes/routeManifest.tsx');
add('legacy topic deep links remain registered for redirect compatibility', 'blocker', sameJson(legacyTopicPaths, legacyExpected), {
  expected: legacyExpected,
  actual: legacyTopicPaths,
}, 'web/ui/src/routes/routeManifest.tsx');

const detailExpected = ['/alerts/:alertId', '/campaigns/:campaignId'];
add('detail routes are registered outside the left navigation', 'blocker', sameJson(detailPaths, detailExpected) && detailExpected.every((route) => !navPathSet.has(route)), {
  expected: detailExpected,
  actual: detailPaths,
}, 'web/ui/src/routes/routeManifest.tsx');

const routeMapRoutes = new Set(routeMap.map((item) => item.route));
const missingRouteMap = [...navPathSet].filter((route) => !routeMapRoutes.has(route));
add('UI suite route-page-map covers every left-nav route', 'blocker', missingRouteMap.length === 0, {
  missing: missingRouteMap,
  routeMapCount: routeMap.length,
}, 'doc/04_assets/ui_suite_gpt_v1/specs/route-page-map.json');

const missingPageContracts = routeMap
  .map((item) => item.contract)
  .filter(Boolean)
  .filter((contract) => !exists(contract));
add('UI suite page contracts exist for route-page-map entries', 'blocker', missingPageContracts.length === 0, {
  missing: missingPageContracts,
}, 'doc/04_assets/ui_suite_gpt_v1/specs/page-contracts');

const flowRoutes = new Set(flows.flatMap((flow) => flow.routes ?? []));
const missingFlowRoutes = ['/login', ...navRoutes.map((route) => route.path), ...detailExpected].filter((route) => !flowRoutes.has(route));
add('business-flow acceptance covers login, all menu routes, and detail routes', 'blocker', missingFlowRoutes.length === 0, {
  missing: missingFlowRoutes,
  flowCount: flows.length,
}, 'doc/04_assets/ui_suite_gpt_v1/specs/business-flow-acceptance.json');

add('frontend task matrix keeps expected page and overlay contract counts', 'blocker',
  taskMatrix.summary?.pages === 28 && taskMatrix.summary?.overlays === 70 && taskMatrix.summary?.manifestItems === 181,
  taskMatrix.summary,
  'doc/04_assets/ui_suite_gpt_v1/specs/frontend-task-matrix.json',
);

if (codeGap) {
  add('frontend code gap has no route gaps or direct fetch violations', 'blocker',
    codeGap.summary?.routeGaps === 0 && codeGap.summary?.directFetchViolations === 0,
    {
      routeGaps: codeGap.summary?.routeGaps,
      directFetchViolations: codeGap.summary?.directFetchViolations,
    },
    'doc/04_assets/ui_suite_gpt_v1/specs/frontend-code-gap.json',
  );
  add('frontend code gap covers all planned API endpoints', 'blocker',
    codeGap.summary?.apiEndpointsCovered === codeGap.summary?.apiEndpoints,
    {
      covered: codeGap.summary?.apiEndpointsCovered,
      total: codeGap.summary?.apiEndpoints,
    },
    'doc/04_assets/ui_suite_gpt_v1/specs/frontend-code-gap.json',
  );
  add('frontend code gap keeps AppShell token deltas at zero', 'warn',
    (codeGap.summary?.appShellTokenGaps ?? 0) === 0,
    { appShellTokenGaps: codeGap.summary?.appShellTokenGaps },
    'doc/04_assets/ui_suite_gpt_v1/specs/frontend-code-gap.json',
  );
  add('frontend code gap has all overlay contracts statically traceable', 'warn',
    (codeGap.summary?.overlaysMissing ?? 0) === 0,
    {
      overlaysConfirmed: codeGap.summary?.overlaysConfirmed,
      overlaysPartial: codeGap.summary?.overlaysPartial,
      overlaysMissing: codeGap.summary?.overlaysMissing,
    },
    'doc/04_assets/ui_suite_gpt_v1/specs/frontend-code-gap.json',
  );
} else {
  add('frontend code gap report exists', 'blocker', false, {
    missing: 'doc/04_assets/ui_suite_gpt_v1/specs/frontend-code-gap.json',
  });
}

console.log(JSON.stringify({
  generatedAt: new Date().toISOString(),
  summary: {
    designMenuRoutes: expectedMenu.length,
    navGroups: groups.length,
    navRoutes: navRoutes.length,
    legacyTopicRoutes: legacyTopicPaths.length,
    detailRoutes: detailPaths.length,
    routePageMapEntries: routeMap.length,
    businessFlows: flows.length,
  },
  groups,
  checks,
}, null, 2));
JS

jq -c '.checks[]' "$CONTRACT_MATRIX" | while IFS= read -r check; do
  NAME="$(jq -r '.name' <<<"$check")"
  SEVERITY="$(jq -r '.severity' <<<"$check")"
  PASSED="$(jq -r '.passed' <<<"$check")"
  ARTIFACT="$(jq -r '.artifact // ""' <<<"$check")"
  DETAIL="$(jq -c '.detail' <<<"$check")"
  if [[ "$PASSED" == "true" ]]; then
    json_log "contract" "$NAME" "info" true "ok" "$DETAIL" "$ARTIFACT"
  else
    json_log "contract" "$NAME" "$SEVERITY" false "failed" "$DETAIL" "$ARTIFACT"
  fi
done

jq -s \
  --arg run_id "$RUN_ID" \
  --arg apisix "$APISIX" \
  --arg report "$REPORT" \
  --arg contract_matrix "$CONTRACT_MATRIX" \
  --arg local_report "$LOCAL_REPORT" \
  '{
    run_id:$run_id,
    generated_at: now | todateiso8601,
    apisix:$apisix,
    result: (if ([.[] | select(.severity == "blocker" and .passed == false)] | length) > 0 then "blocked" elif ([.[] | select(.severity == "warn" and .passed == false)] | length) > 0 then "warn" else "pass" end),
    total: length,
    passed: ([.[] | select(.passed == true)] | length),
    blockers: ([.[] | select(.severity == "blocker" and .passed == false)] | length),
    warnings: ([.[] | select(.severity == "warn" and .passed == false)] | length),
    report:$report,
    contract_matrix:$contract_matrix,
    local_report:$local_report,
    checks: .
  }' "$REPORT" >"$SUMMARY"

node - "$SUMMARY" "$LOCAL_REPORT" <<'JS'
const fs = require('node:fs');
const [summaryFile, reportFile] = process.argv.slice(2);
const summary = JSON.parse(fs.readFileSync(summaryFile, 'utf8'));
const failed = summary.checks.filter((check) => !check.passed);
const blockers = failed.filter((check) => check.severity === 'blocker');
const warnings = failed.filter((check) => check.severity === 'warn');
const row = (cells) => `| ${cells.join(' | ')} |`;
const table = (items) => {
  if (!items.length) return '- 无';
  return [
    row(['阶段', '检查', '等级', '状态', '证据']),
    row(['---', '---', '---', '---', '---']),
    ...items.map((item) => row([item.phase, item.name, item.severity, item.status, item.artifact || '-'])),
  ].join('\n');
};
const md = `# UI 契约回归预检报告

- Run ID：\`${summary.run_id}\`
- 结果：\`${summary.result}\`
- APISIX：\`${summary.apisix}\`
- 检查数：${summary.passed}/${summary.total} passed，blockers=${summary.blockers}，warnings=${summary.warnings}

## Blockers

${table(blockers)}

## Warnings

${table(warnings)}

## 证据

- NDJSON：\`${summary.report}\`
- Summary：\`${summaryFile}\`
- UI contract matrix：\`${summary.contract_matrix}\`

## 口径

本报告证明设计菜单、UI 图契约、routeManifest、前端权限单测、APISIX 登录入口和 Desktop Chrome 合法登录态业务页点击是否对齐。Desktop Chrome 是 MCP 桥接工具，shell 脚本不直接操作浏览器；本轮结果通过 \`DESKTOP_CHROME_STATUS\` / \`DESKTOP_CHROME_BUSINESS_STATUS\` 等变量记录 wrapper 实测结论。
`;
fs.writeFileSync(reportFile, md);
JS

cp "$SUMMARY" "$REGRESSION_DIR/ui-contract-preflight-latest.json"
cp "$LOCAL_REPORT" "$REGRESSION_DIR/ui-contract-preflight-latest.md"
cp "$CONTRACT_MATRIX" "$REGRESSION_DIR/ui-contract-matrix-latest.json"

RESULT="$(jq -r '.result' "$SUMMARY")"
echo "ui contract preflight result: $RESULT"
echo "summary: $SUMMARY"
echo "local report: $LOCAL_REPORT"

if [[ "$RESULT" == "blocked" && "$ALLOW_BLOCKERS" != "true" ]]; then
  exit 1
fi
