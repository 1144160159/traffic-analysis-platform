#!/usr/bin/env node

import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const ROOT = path.resolve(__dirname, '../../..');
const INDEX_PATH = path.join(ROOT, 'doc/04_assets/ui_suite_gpt_v1/specs/pixel-perfect-breakdown-index.json');

function repoPath(file) {
  return path.isAbsolute(file) ? file : path.join(ROOT, file);
}

function readJson(file, fallback = null) {
  const abs = repoPath(file);
  if (!fs.existsSync(abs)) return fallback;
  return JSON.parse(fs.readFileSync(abs, 'utf8'));
}

function writeJson(file, value) {
  const abs = repoPath(file);
  fs.mkdirSync(path.dirname(abs), { recursive: true });
  fs.writeFileSync(abs, `${JSON.stringify(value, null, 2)}\n`);
}

function appendReviewNote(reviewPath, note) {
  const abs = repoPath(reviewPath);
  if (!fs.existsSync(abs)) return false;
  const current = fs.readFileSync(abs, 'utf8');
  if (current.includes('## Visible Chrome Rerun Gate')) return false;
  fs.writeFileSync(abs, `${current.trimEnd()}\n\n${note.trimEnd()}\n`);
  return true;
}

function main() {
  const index = readJson(INDEX_PATH, { items: [] });
  const reset = [];
  const missing = [];
  const reason =
    'Pages are being restarted after fixing Windows Chrome CDP from SessionId=0 background Chrome to SessionId=1 interactive Chrome.';
  for (const item of index.items || []) {
    if (item.category !== 'pages') continue;
    const record = readJson(item.json, null);
    if (!record) {
      missing.push({ id: item.id, json: item.json });
      continue;
    }

    record.status = 'evidence-ready';
    record.accepted = false;
    record.visible_chrome_rerun = {
      status: 'required',
      reason,
      required_cdp_url: 'http://127.0.0.1:9224',
      required_windows_session_id: 1,
      started_at: new Date().toISOString(),
    };
    record.review_gate = {
      status: 'requires-new-independent-subagent-review',
      reason: 'Old page acceptance predates the visible Windows Chrome fix and cannot close the restarted pages gate.',
    };
    writeJson(item.json, record);

    const verificationPath = record.evidence?.verification || `evidence/ui-image-breakdowns/pages/${item.id}/verification.json`;
    const verification = readJson(verificationPath, null);
    if (verification) {
      verification.status = 'evidence-ready';
      verification.accepted = false;
      verification.main_thread_judgment = 'awaiting-visible-chrome-rerun-and-real-auxiliary-review';
      verification.main_thread_decision_basis =
        'Historical page evidence is retained, but final page acceptance is reopened because CDP previously ran in Windows SessionId=0 and was not visually observable by the user.';
      verification.visible_chrome_rerun = {
        status: 'required',
        reason,
        required_cdp_url: 'http://127.0.0.1:9224',
        required_windows_session_id: 1,
      };
      verification.auxiliary_agent_review = {
        agent: 'independent-subagent-required',
        status: 'requested',
        notes: [
          'This page requires a new independent review batch after the visible Windows Chrome SessionId=1 rerun.',
          'Previous page review batches are historical evidence only for this restarted gate.',
        ],
      };
      verification.independent_subagent_review = {
        status: 'required',
        review_batch: '',
      };
      writeJson(verificationPath, verification);
    }

    appendReviewNote(
      item.review,
      `## Visible Chrome Rerun Gate

- Main-thread restart: ${reason}
- Previous \`pixel-accepted\` and subagent review are retained as historical evidence only.
- This page must be recaptured through the current interactive Windows Chrome CDP path, then reviewed by a fresh independent subagent batch before main-thread acceptance.`,
    );

    reset.push({ id: item.id, record: item.json, verification: verificationPath, review: item.review });
  }

  console.log(JSON.stringify({ reset_count: reset.length, missing_count: missing.length, reset, missing }, null, 2));
}

main();
