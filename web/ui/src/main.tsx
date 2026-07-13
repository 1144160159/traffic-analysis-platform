import React from 'react';
import ReactDOM from 'react-dom/client';
import App from '@/App';
import { appConfig } from '@/config/runtime';
import '@/styles/tokens.css';
import '@/styles/global.css';
import '@/styles/app-shell.css';
import '@/styles/pages.css';

const start = async () => {
  if (appConfig.useMock) {
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
