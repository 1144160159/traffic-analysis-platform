import React from 'react';
import ReactDOM from 'react-dom/client';
import App from '@/App';
import { appConfig } from '@/config/runtime';
import '@/styles/tokens.css';
import '@/styles/global.css';
import '@/styles/app-shell.css';
import '@/styles/page-state.css';
import '@/styles/pages.css';
import '@/styles/alert-detail.css';
import '@/styles/baseline-workbench.css';
import '@/styles/campaign-workbench-drawer.css';
import '@/styles/topics-exact.css';

const start = async () => {
  // Mock interception is a development-only capability. Keep the DEV guard in
  // the same branch as the dynamic import so Rollup can erase the entire MSW
  // dependency graph from every production build.
  if (import.meta.env.DEV && appConfig.useMock) {
    const { worker } = await import('@/mocks/browser');
    await worker.start({ onUnhandledRequest: 'bypass', quiet: true });
  }

  ReactDOM.createRoot(document.getElementById('root')!).render(
    <React.StrictMode>
      <App />
    </React.StrictMode>,
  );
};

void start();
