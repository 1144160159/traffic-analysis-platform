import { defineConfig } from 'vite';
// 视觉开发插件(code-inspector-plugin):浏览器点击页面元素 → 打开本地 IDE 对应源码。
// 仅 dev 生效;注意:vite.config.js 优先于 vite.config.ts,两文件需同步维护。
import { codeInspectorPlugin } from 'code-inspector-plugin';
import react from '@vitejs/plugin-react';
import { rmSync } from 'node:fs';
import path from 'node:path';
var packageNameFromId = function (id) {
    var nodeModulesPath = id.split('/node_modules/')[1];
    if (!nodeModulesPath)
        return undefined;
    var parts = nodeModulesPath.split('/');
    if (parts[0].startsWith('@'))
        return "".concat(parts[0], "/").concat(parts[1]);
    return parts[0];
};
var miscVendorPackages = new Set([
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
var vendorChunkName = function (packageName) { return "vendor-".concat(packageName.replace(/^@/, '').replace(/[/.]/g, '-')); };
var productionMockWorkerGuard = function () { return ({
    name: 'production-mock-worker-guard',
    apply: 'build',
    generateBundle: function (_options, bundle) {
        var forbiddenModules = Object.values(bundle)
            .filter(function (item) { return item.type === 'chunk'; })
            .flatMap(function (item) { var _a; return (_a = item.moduleIds) !== null && _a !== void 0 ? _a : []; })
            .filter(function (id) {
            return id.includes('/src/mocks/')
                || id.endsWith('/src/services/mockData.ts')
                || id.includes('/node_modules/msw/')
                || id.includes('/node_modules/@mswjs/');
        });
        if (forbiddenModules.length > 0) {
            throw new Error("production bundle contains mock runtime modules: ".concat(forbiddenModules.join(', ')));
        }
    },
    closeBundle: function () {
        // Vite copies public/ verbatim. The worker is required by the development
        // server but must not be present in a deployable production directory.
        rmSync(path.resolve(__dirname, 'dist/mockServiceWorker.js'), { force: true });
    },
}); };
export default defineConfig({
    root: __dirname,
    plugins: [
    codeInspectorPlugin({
      bundler: 'vite',
      hotKeys: ['altKey', 'shiftKey'],
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
                manualChunks: function (id) {
                    if (!id.includes('node_modules'))
                        return undefined;
                    if (id.includes('/node_modules/react/') || id.includes('/node_modules/react-dom/') || id.includes('/node_modules/scheduler/')) {
                        return 'vendor-react';
                    }
                    if (id.includes('/node_modules/react-router'))
                        return 'vendor-router';
                    if (id.includes('/node_modules/@tanstack/') || id.includes('/node_modules/axios/'))
                        return 'vendor-data';
                    if (id.includes('/node_modules/zrender/'))
                        return 'vendor-zrender';
                    if (id.includes('/node_modules/echarts-for-react/'))
                        return 'vendor-echarts-react';
                    if (id.includes('/node_modules/echarts/'))
                        return 'vendor-echarts';
                    var packageName = packageNameFromId(id);
                    if (!packageName)
                        return 'vendor-misc';
                    if (miscVendorPackages.has(packageName))
                        return 'vendor-misc';
                    if (packageName === 'antd')
                        return 'vendor-antd';
                    if (packageName === '@ant-design/fast-color')
                        return 'vendor-ant-design-fast-color';
                    if (packageName.startsWith('@ant-design/'))
                        return 'vendor-ant-design';
                    if (packageName.startsWith('@rc-component/') || packageName.startsWith('rc-'))
                        return 'vendor-rc';
                    return vendorChunkName(packageName);
                },
            },
        },
    },
});
