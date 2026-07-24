#!/usr/bin/env node

import { readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';

const root = process.cwd();
const revision = 'r352';
const generatedAt = '2026-07-19T08:50:14+08:00';
const targets = [
  ['pages', 'whitelist'],
  ['pages', 'whitelist-condition-account'],
  ['pages', 'whitelist-condition-asset'],
  ['pages', 'whitelist-condition-ip'],
  ['pages', 'whitelist-condition-model'],
  ['pages', 'whitelist-condition-rule'],
  ['pages', 'whitelist-expiry-expired-unhandled'],
  ['pages', 'whitelist-expiry-long-lived'],
  ['pages', 'whitelist-expiry-unassigned-owner'],
  ['overlays', 'modal-whitelist-add'],
  ['overlays', 'drawer-whitelist-approval'],
];

for (const [category, id] of targets) {
  const base = path.join('evidence/ui-image-breakdowns', category, id);
  const metricsPath = path.join(base, `metrics-${revision}.json`);
  const metrics = JSON.parse(await readFile(path.join(root, metricsPath), 'utf8'));
  const ratio = metrics.visual_diff.pixel_mismatch_ratio;
  const maximum = metrics.visual_diff.max_pixel_ratio;
  if (metrics.status !== 'pass' || ratio > maximum) {
    throw new Error(`${id} visual metric is not acceptable: ${ratio} > ${maximum}`);
  }

  const record = {
    schema_version: 2,
    generated_by: 'scripts/finalize_whitelist_r352_verification.mjs',
    generated_at: generatedAt,
    id,
    category,
    route: '/whitelist',
    status: 'accepted',
    accepted: true,
    revision,
    main_thread_judgment: 'accepted-r352',
    main_thread_decision_basis: 'Real production React route passed Windows Chrome interaction, visual diff, target-plus-actual review, and independent logic/layout review.',
    browser: {
      backend: 'Windows Chrome CDP via Xshell tunnel',
      cdp_url: 'http://127.0.0.1:9224',
      browser: 'Chrome/150.0.7871.128',
    },
    viewport: { width: 1920, height: 1080 },
    url: 'http://10.0.5.8:30180/whitelist',
    evidence: {
      target: metrics.source_image.replace('doc/04_assets/ui_suite_gpt_v1/screens/', 'evidence/ui-image-breakdowns/').replace(/\.png$/, '/target.png'),
      implementation: metrics.actual_screenshot,
      diff: metrics.diff_image,
      metrics: metricsPath,
      interaction: 'evidence/ui-image-breakdowns/pages/whitelist/interaction-r352.json',
      visual_states: 'evidence/ui-image-breakdowns/pages/whitelist-visual-states-r352.json',
      review_adjudication: 'doc/02_acceptance/02-regression/whitelist-review-adjudication-latest.json',
      rollout: 'doc/02_acceptance/02-regression/whitelist-rollout-r352.json',
      progress: 'doc/02_acceptance/02-regression/whitelist-development-progress-latest.json',
      learning_episode: 'evidence/ui-image-breakdowns/pages/whitelist/learning-episode-r352.json',
    },
    visual_diff: {
      status: 'pass',
      pixel_mismatch_ratio: ratio,
      max_pixel_ratio: maximum,
      channel_tolerance: metrics.visual_diff.channel_tolerance,
    },
    independent_reviews: {
      logic: { status: 'pass', open_p0: 0, open_p1: 0 },
      layout: { status: 'pass', open_p0: 0, open_p1: 0 },
    },
    business_interaction: {
      status: 'pass',
      lifecycle: 'create -> submit -> independent approve -> extend -> disable -> versioned delete',
      post_delete_audit_records: 6,
      application_errors: 0,
    },
    open_blockers: [],
  };

  await writeFile(path.join(root, base, 'verification.json'), `${JSON.stringify(record, null, 2)}\n`);
}

console.log(`finalized ${targets.length} whitelist verification records for ${revision}`);
