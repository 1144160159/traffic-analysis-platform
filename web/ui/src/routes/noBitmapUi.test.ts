import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

const sourceRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const uiRoot = path.resolve(sourceRoot, '..');
const textExtensions = new Set(['.css', '.ts', '.tsx']);
const disallowedPublicUiTargets = [
  'public/ui-assets/backgrounds/login.png',
];
const disallowedPatterns = [
  { label: 'backgroundAsset helper', pattern: /backgroundAsset\s*\(/ },
  { label: 'inline backgroundImage style', pattern: /backgroundImage\s*:/ },
  { label: 'CSS url() image hook', pattern: /url\s*\(/ },
  { label: 'bitmap asset file extension', pattern: /\.(?:png|jpe?g|webp|gif|avif|svg)(?:['")?#\s]|$)/i },
  { label: 'legacy bitmap background CSS variable', pattern: /--(?:page-bg|topic-bg|taf-probes-bg|taf-notfound-bg)/ },
];
const bitmapIconExtensions = new Set(['.png', '.jpg', '.jpeg', '.webp', '.gif', '.avif']);
const requiredBitmapIconVariants = [
  { scale: 1, width: 24, height: 24, label: '24px/@1x' },
  { scale: 2, width: 48, height: 48, label: '48px/@2x' },
  { scale: 3, width: 72, height: 72, label: '72px/@3x' },
];

type IconDensityVariant = {
  width?: number;
  height?: number;
  scale?: number | string;
  density?: number | string;
};

function collectSourceFiles(dir: string): string[] {
  const entries = fs.readdirSync(dir, { withFileTypes: true });
  return entries.flatMap((entry) => {
    const fullPath = path.join(dir, entry.name);
    if (entry.isDirectory()) return collectSourceFiles(fullPath);
    if (!textExtensions.has(path.extname(entry.name))) return [];
    if (/\.test\.[tj]sx?$/.test(entry.name) || /\.spec\.[tj]sx?$/.test(entry.name)) return [];
    return [fullPath];
  });
}

function lineNumber(content: string, index: number) {
  return content.slice(0, index).split('\n').length;
}

function normalizeScale(value: number | string | undefined) {
  if (typeof value === 'number') return value;
  if (typeof value !== 'string') return undefined;
  const match = /(\d+(?:\.\d+)?)\s*x?/i.exec(value.replace('@', ''));
  return match ? Number(match[1]) : undefined;
}

function iconVariants(manifest: {
  variants?: IconDensityVariant[];
  outputs?: IconDensityVariant[];
  asset?: { variants?: IconDensityVariant[] };
  output?: { variants?: IconDensityVariant[] };
  bitmap?: { variants?: IconDensityVariant[] };
}) {
  return [
    ...(manifest.variants ?? []),
    ...(manifest.outputs ?? []),
    ...(manifest.asset?.variants ?? []),
    ...(manifest.output?.variants ?? []),
    ...(manifest.bitmap?.variants ?? []),
  ];
}

function screenshotIconManifestPath(relativeIconPath: string) {
  const directPath = path.join(uiRoot, relativeIconPath.replace(/\.(?:png|jpe?g|webp|gif|avif|svg)$/i, '.source.json'));
  if (fs.existsSync(directPath)) return directPath;

  const extension = path.extname(relativeIconPath);
  const withoutExtension = relativeIconPath.slice(0, -extension.length);
  const baseVariantPath = `${withoutExtension.replace(/(?:-24|@(?:2|3)x)$/i, '')}.source.json`;
  return path.join(uiRoot, baseVariantPath);
}

function validateScreenshotIconManifest(relativeIconPath: string) {
  const manifestPath = screenshotIconManifestPath(relativeIconPath);
  if (!fs.existsSync(manifestPath)) {
    return `${relativeIconPath}:1 screenshot icon missing source manifest`;
  }

  const manifest = JSON.parse(fs.readFileSync(manifestPath, 'utf8')) as {
    usage?: string;
    format?: string;
    asset?: { type?: string; format?: string; variants?: IconDensityVariant[] };
    output?: { type?: string; format?: string; variants?: IconDensityVariant[] };
    bitmap?: { variants?: IconDensityVariant[] };
    variants?: IconDensityVariant[];
    outputs?: IconDensityVariant[];
  };
  if (!manifest.usage) {
    return `${relativeIconPath}:1 screenshot icon manifest missing usage`;
  }

  const extension = path.extname(relativeIconPath).toLowerCase();
  const outputFormat = [
    manifest.format,
    manifest.asset?.type,
    manifest.asset?.format,
    manifest.output?.type,
    manifest.output?.format,
  ].filter(Boolean).join(' ').toLowerCase();
  if (extension === '.svg' || outputFormat.includes('svg')) {
    return null;
  }
  if (!bitmapIconExtensions.has(extension)) {
    return `${relativeIconPath}:1 screenshot icon must be SVG or bitmap icon`;
  }

  const variants = iconVariants(manifest);
  const missing = requiredBitmapIconVariants.filter((required) => !variants.some((variant) => (
    variant.width === required.width &&
    variant.height === required.height &&
    normalizeScale(variant.scale ?? variant.density) === required.scale
  )));
  if (missing.length > 0) {
    return `${relativeIconPath}:1 screenshot bitmap icon missing variants ${missing.map((item) => item.label).join(', ')}`;
  }
  return null;
}

function validateScreenshotPanelManifest(relativePanelPath: string) {
  const manifestPath = path.join(uiRoot, relativePanelPath.replace(/\.(?:png|jpe?g|webp|gif|avif|svg)$/i, '.source.json'));
  if (!fs.existsSync(manifestPath)) {
    return `${relativePanelPath}:1 screenshot panel missing source manifest`;
  }

  const manifest = JSON.parse(fs.readFileSync(manifestPath, 'utf8')) as {
    usage?: string;
    scope?: string;
    contains_business_dynamic_diagram?: boolean;
    source?: { image?: string; bbox?: { x?: number; y?: number; w?: number; h?: number } };
  };
  if (!manifest.usage) {
    return `${relativePanelPath}:1 screenshot panel manifest missing usage`;
  }
  if (!manifest.source?.image || !manifest.source?.bbox) {
    return `${relativePanelPath}:1 screenshot panel manifest missing source image or bbox`;
  }
  if (!/bottom|footer/i.test(`${manifest.scope ?? ''} ${manifest.usage}`)) {
    return `${relativePanelPath}:1 screenshot panel must be scoped to an explicitly approved bottom/footer panel`;
  }
  if (manifest.contains_business_dynamic_diagram !== false) {
    return `${relativePanelPath}:1 screenshot panel must explicitly exclude API-driven business diagrams`;
  }
  return null;
}

function validateScreenshotBackgroundManifest(relativeBackgroundPath: string) {
  const manifestPath = path.join(uiRoot, relativeBackgroundPath.replace(/\.(?:png|jpe?g|webp|gif|avif|svg)$/i, '.source.json'));
  if (!fs.existsSync(manifestPath)) {
    return `${relativeBackgroundPath}:1 screenshot background missing source manifest`;
  }

  const manifest = JSON.parse(fs.readFileSync(manifestPath, 'utf8')) as {
    usage?: string;
    scope?: string;
    resource_type?: string;
    contains_business_page_ui?: boolean;
    contains_business_dynamic_diagram?: boolean;
    source?: { image?: string; masked_regions?: unknown[] };
  };
  if (!manifest.usage) {
    return `${relativeBackgroundPath}:1 screenshot background manifest missing usage`;
  }
  if (!/background/i.test(`${manifest.scope ?? ''} ${manifest.usage} ${manifest.resource_type ?? ''}`)) {
    return `${relativeBackgroundPath}:1 screenshot background must be scoped to an explicitly approved page background`;
  }
  if (manifest.contains_business_page_ui !== false) {
    return `${relativeBackgroundPath}:1 screenshot background must explicitly exclude business page UI`;
  }
  if (manifest.contains_business_dynamic_diagram !== false) {
    return `${relativeBackgroundPath}:1 screenshot background must explicitly exclude API-driven business diagrams`;
  }
  if (!manifest.source?.image) {
    return `${relativeBackgroundPath}:1 screenshot background manifest missing source image`;
  }
  return null;
}

function allowedScreenshotReferences(content: string, index: number) {
  const context = content.slice(Math.max(0, index - 140), Math.min(content.length, index + 140));
  const publicBackground = /(?:\/|['"])(ui-assets\/backgrounds\/[^'")?#\s]+\.(?:png|jpe?g|webp|gif|avif|svg))/i.exec(context);
  if (publicBackground) {
    const violation = validateScreenshotBackgroundManifest(path.join('public', publicBackground[1]));
    return { allowed: true, violations: violation ? [violation] : [] };
  }
  const assetPath = /assets\/((?:generated-icons|screenshot-icons|screenshot-panels|screenshot-backgrounds)\/[^'")?#\s]+\.(?:png|jpe?g|webp|gif|avif|svg))/i.exec(context);
  if (!assetPath) return { allowed: false, violations: [] as string[] };
  if (assetPath[1].startsWith('screenshot-icons/')) {
    const violation = validateScreenshotIconManifest(path.join('src/assets', assetPath[1]));
    return { allowed: true, violations: violation ? [violation] : [] };
  }
  if (assetPath[1].startsWith('screenshot-panels/')) {
    const violation = validateScreenshotPanelManifest(path.join('src/assets', assetPath[1]));
    return { allowed: true, violations: violation ? [violation] : [] };
  }
  if (assetPath[1].startsWith('screenshot-backgrounds/')) {
    const violation = validateScreenshotBackgroundManifest(path.join('src/assets', assetPath[1]));
    return { allowed: true, violations: violation ? [violation] : [] };
  }
  return { allowed: true, violations: [] };
}

describe('no-bitmap UI implementation guard', () => {
  it('keeps frontend UI backgrounds and visual layers as DOM/CSS instead of bitmap assets', () => {
    const violations: string[] = [];

    for (const publicTarget of disallowedPublicUiTargets) {
      const absoluteTarget = path.join(uiRoot, publicTarget);
      if (fs.existsSync(absoluteTarget)) {
        violations.push(`${publicTarget}:1 public page bitmap target`);
      }
    }

    for (const file of collectSourceFiles(sourceRoot)) {
      const relative = path.relative(sourceRoot, file);
      const content = fs.readFileSync(file, 'utf8');

      for (const rule of disallowedPatterns) {
        const match = rule.pattern.exec(content);
        if (match) {
          if (rule.label === 'CSS url() image hook' && /^url\s*\(\s*['"]?#/i.test(content.slice(match.index))) {
            continue;
          }
          if (
            (rule.label === 'bitmap asset file extension' || rule.label === 'CSS url() image hook') &&
            allowedScreenshotReferences(content, match.index).allowed
          ) {
            violations.push(...allowedScreenshotReferences(content, match.index).violations);
            continue;
          }
          violations.push(`${relative}:${lineNumber(content, match.index)} ${rule.label}`);
        }
      }

      const imageTag = /<img\b/g.exec(content);
      if (
        imageTag &&
        !(relative === 'pages/LoginPage.tsx' && content.includes('alt="登录验证码"')) &&
        !content.includes('data-screenshot-background') &&
        !content.includes('data-generated-icon')
      ) {
        violations.push(`${relative}:${lineNumber(content, imageTag.index)} raw img tag`);
      }
    }

    expect(violations).toEqual([]);
  });
});
