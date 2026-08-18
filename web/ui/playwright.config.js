var __assign = (this && this.__assign) || function () {
    __assign = Object.assign || function(t) {
        for (var s, i = 1, n = arguments.length; i < n; i++) {
            s = arguments[i];
            for (var p in s) if (Object.prototype.hasOwnProperty.call(s, p))
                t[p] = s[p];
        }
        return t;
    };
    return __assign.apply(this, arguments);
};
import { defineConfig, devices } from '@playwright/test';
export default defineConfig({
    testDir: './e2e',
    timeout: 30000,
    expect: { timeout: 5000 },
    use: {
        baseURL: process.env.PLAYWRIGHT_BASE_URL || 'http://127.0.0.1:4177',
        trace: 'on-first-retry',
    },
    projects: [
        {
            name: 'chromium',
            use: __assign(__assign({}, devices['Desktop Chrome']), { viewport: { width: 1440, height: 900 } }),
        },
    ],
});
