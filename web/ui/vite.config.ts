import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
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

export default defineConfig({
  plugins: [react()],
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
