#!/usr/bin/env node
import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';

const root = process.cwd();
const pagesDir = path.join(root, 'web/ui/src/pages');
const outputPath = path.join(root, 'evidence/business-ui-constraints/inventory-latest.json');
const excludedPages = new Set(['LoginPage.tsx', 'NotFoundPage.tsx', 'OidcCallbackPage.tsx']);
const uiRequire = createRequire(path.join(root, 'web/ui/package.json'));
const ts = uiRequire('typescript');

function lineNumber(source, index) {
  return source.slice(0, index).split('\n').length;
}

function matches(source, pattern) {
  return [...source.matchAll(pattern)].map((match) => ({
    line: lineNumber(source, match.index ?? 0),
    text: match[0].trim().slice(0, 220),
  }));
}

function jsxAttributes(node) {
  return node.attributes.properties.reduce((attributes, property) => {
    if (ts.isJsxAttribute(property)) attributes.add(property.name.getText());
    if (ts.isJsxSpreadAttribute(property)) attributes.add('__spread__');
    return attributes;
  }, new Set());
}

function jsxElements(source) {
  const file = ts.createSourceFile('business-page.tsx', source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
  const elements = [];
  const visit = (node) => {
    if (ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node)) {
      elements.push({
        tag: node.tagName.getText(file),
        attributes: jsxAttributes(node),
        line: file.getLineAndCharacterOfPosition(node.getStart(file)).line + 1,
        text: node.getText(file).replace(/\s+/g, ' ').slice(0, 220),
        start: node.getStart(file),
      });
    }
    ts.forEachChild(node, visit);
  };
  visit(file);
  return elements;
}

function scanPage(fileName) {
  const source = fs.readFileSync(path.join(pagesDir, fileName), 'utf8');
  const elements = jsxElements(source);
  const interactiveControls = elements.filter(({ tag }) => tag === 'button' || tag === 'Button');
  const pageDelegatesBusinessActions = elements.some(({ attributes }) => (
    attributes.has('data-business-action-delegate') && attributes.has('onClick')
  ));
  const passiveControls = interactiveControls.filter(({ attributes }) => (
    !pageDelegatesBusinessActions &&
    !attributes.has('onClick') &&
    !attributes.has('href') &&
    !attributes.has('disabled') &&
    !attributes.has('htmlType') &&
    !attributes.has('__spread__')
  ));
  const svgCharts = elements.filter(({ start, tag }) => {
    if (tag !== 'svg') return false;
    return /(?:chart|trend|heat|distribution|sparkline|donut|ring|bar)/i.test(source.slice(Math.max(0, start - 260), start + 420));
  });
  const paginationMarkers = matches(source, /(?:pagination|\b(?:上一页|下一页|条\/页)\b)/g);
  const tableMarkers = matches(source, /(?:table|grid|rows|list)/gi);

  return {
    file: `web/ui/src/pages/${fileName}`,
    interactive_control_count: interactiveControls.length,
    passive_control_count: passiveControls.length,
    passive_control_examples: passiveControls.slice(0, 12).map(({ line, text }) => ({ line, text })),
    svg_chart_candidate_count: svgCharts.length,
    svg_chart_candidates: svgCharts.slice(0, 12).map(({ line, text }) => ({ line, text })),
    pagination_marker_count: paginationMarkers.length,
    table_marker_count: tableMarkers.length,
  };
}

const pages = fs.readdirSync(pagesDir)
  .filter((file) => file.endsWith('.tsx') && !excludedPages.has(file))
  .sort()
  .map(scanPage);

const summary = pages.reduce((total, page) => ({
  business_page_count: total.business_page_count + 1,
  interactive_control_count: total.interactive_control_count + page.interactive_control_count,
  passive_control_count: total.passive_control_count + page.passive_control_count,
  svg_chart_candidate_count: total.svg_chart_candidate_count + page.svg_chart_candidate_count,
  pages_without_pagination_markers: total.pages_without_pagination_markers + (page.pagination_marker_count === 0 ? 1 : 0),
}), {
  business_page_count: 0,
  interactive_control_count: 0,
  passive_control_count: 0,
  svg_chart_candidate_count: 0,
  pages_without_pagination_markers: 0,
});

const report = {
  generated_at: new Date().toISOString(),
  scope: 'web/ui/src/pages excluding login, oidc callback, and not-found',
  method: {
    passive_controls: 'TypeScript JSX AST control declarations without onClick, href, disabled, submit semantics, a spread attribute, or a page-level data-business-action-delegate handler. Delegated pages still require runtime click coverage.',
    svg_chart_candidates: 'TypeScript JSX AST SVG declarations with chart, trend, heat, distribution, sparkline, donut, ring, or bar context.',
    pagination: 'Text and class markers only. A marker is not proof that client-side pagination is functional.',
  },
  global_constraints: [
    'Dynamic business charts use ECharts backed by API data or typed fallback data.',
    'Every visible business button has a concrete navigation, drawer/modal, state change, or simulated API-backed action.',
    'Overflowing business content has a bounded scroll container.',
    'Business record tables have controlled pagination when their result set exceeds the visible capacity.',
  ],
  summary,
  pages,
};

fs.mkdirSync(path.dirname(outputPath), { recursive: true });
fs.writeFileSync(outputPath, `${JSON.stringify(report, null, 2)}\n`);
console.log(JSON.stringify(report, null, 2));
