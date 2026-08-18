import { defineConfig } from 'vite';
// 视觉开发插件(code-inspector-plugin):浏览器点击页面元素 → 打开本地 IDE 对应源码。
// 仅 dev 生效(hot:true 默认),生产构建不注入。
import { codeInspectorPlugin } from 'code-inspector-plugin';
import react from '@vitejs/plugin-react';
import { rmSync } from 'node:fs';
import path from 'node:path';

const packageNameFromId = (id: string) => {
  const nodeModulesPath = id.split('/node_modules/')[1];
  if (!nodeModulesPath) return undefined;
  const parts = nodeModulesPath.split('/');
  if (parts[0].startsWith('@')) return `${parts[0]}/${parts[1]}`;
  return parts[0];
};

const miscVendorPackages = new Set([
  '@emotion/hash',
  '@emotion/unitless',
  '@open-draft/deferred-promise',
  '@open-draft/logger',
  'classnames',
  'compute-scroll-into-view',
  'copy-to-clipboard',
  'fast-deep-equal',
  'graphql',
  'json2mq',
  'resize-observer-polyfill',
  'scroll-into-view-if-needed',
  'string-convert',
  'throttle-debounce',
  'toggle-selection',
  'tslib',
]);

const vendorChunkName = (packageName: string) => `vendor-${packageName.replace(/^@/, '').replace(/[/.]/g, '-')}`;

const productionMockWorkerGuard = () => ({
  name: 'production-mock-worker-guard',
  apply: 'build' as const,
  generateBundle(_options: unknown, bundle: Record<string, { type: string; moduleIds?: string[] }>) {
    const forbiddenModules = Object.values(bundle)
      .filter((item) => item.type === 'chunk')
      .flatMap((item) => item.moduleIds ?? [])
      .filter((id) =>
        id.includes('/src/mocks/')
        || id.endsWith('/src/services/mockData.ts')
        || id.includes('/node_modules/msw/')
        || id.includes('/node_modules/@mswjs/'),
      );
    if (forbiddenModules.length > 0) {
      throw new Error(`production bundle contains mock runtime modules: ${forbiddenModules.join(', ')}`);
    }
  },
  closeBundle() {
    // Vite copies public/ verbatim. The worker is required by the development
    // server but must not be present in a deployable production directory.
    rmSync(path.resolve(__dirname, 'dist/mockServiceWorker.js'), { force: true });
  },
});

export default defineConfig({
  root: __dirname,
  plugins: [
    codeInspectorPlugin({
      bundler: 'vite',
      // Windows/Linux 默认 Alt+Shift,Mac Option+Shift;点击元素跳转 IDE
      hotKeys: ['altKey', 'shiftKey'],
      // 展示点击元素所在文件与行列信息(悬浮/控制台输出)
      showSwitch: true,
    }),
    react(),
    productionMockWorkerGuard(),
  ],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    host: '127.0.0.1',
    port: 5173,
    proxy: {
      '/api': {
        target: process.env.VITE_DEV_APISIX_URL || 'http://10.0.5.8:30180',
        changeOrigin: true,
      },
      '/ws': {
        target: process.env.VITE_DEV_APISIX_URL || 'ws://10.0.5.8:30180',
        ws: true,
        changeOrigin: true,
      },
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: './src/test/setup.ts',
    exclude: ['node_modules/**', 'dist/**', 'e2e/**'],
    globals: true,
  },
  build: {
    manifest: true,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes('node_modules')) return undefined;
          if (id.includes('/node_modules/react/') || id.includes('/node_modules/react-dom/') || id.includes('/node_modules/scheduler/')) {
            return 'vendor-react';
          }
          if (id.includes('/node_modules/react-router')) return 'vendor-router';
          if (id.includes('/node_modules/@tanstack/') || id.includes('/node_modules/axios/')) return 'vendor-data';
          if (id.includes('/node_modules/zrender/')) return 'vendor-zrender';
          if (id.includes('/node_modules/echarts-for-react/')) return 'vendor-echarts-react';
          if (id.includes('/node_modules/echarts/')) return 'vendor-echarts';
          const packageName = packageNameFromId(id);
          if (!packageName) return 'vendor-misc';
          if (miscVendorPackages.has(packageName)) return 'vendor-misc';
          if (packageName === 'antd') return 'vendor-antd';
          if (packageName === '@ant-design/fast-color') return 'vendor-ant-design-fast-color';
          if (packageName.startsWith('@ant-design/')) return 'vendor-ant-design';
          if (packageName.startsWith('@rc-component/') || packageName.startsWith('rc-')) return 'vendor-rc';
          return vendorChunkName(packageName);
        },
      },
    },
  },
});
