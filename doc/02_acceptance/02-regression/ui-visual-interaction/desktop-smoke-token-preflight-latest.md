# Desktop Smoke Token Preflight

- Result: `pass`
- Generated: `2026-07-02T16:50:31.581Z`
- Base URL: `http://127.0.0.1:5173`
- APISIX URL: `http://10.0.5.8:30180`
- Route: `/dashboard`
- Final path: `/dashboard`
- Final URL: `http://127.0.0.1:5173/dashboard`

This is a local readiness check only. It proves that the current Vite target can consume a valid short-lived smoke JWT and keep a protected route authenticated. It does not replace Windows Codex Desktop Chrome extension visual or interaction evidence.

## Checks

- `pass` Vite auth enabled: status=200 VITE_AUTH_ENABLED=true
- `pass` Vite mock disabled: status=200 VITE_USE_MOCK=false
- `pass` Vite desktop smoke token enabled: status=200 VITE_DESKTOP_SMOKE_TOKEN_ENABLED=true
- `pass` JWT generated from K8s secret: traffic-analysis/traffic-credentials#JWT_SECRET
- `pass` short-lived JWT accepted by auth/me: status=200 username=codex-ui-desktop-admin
- `pass` smoke hash consumed by app: final_url=http://127.0.0.1:5173/dashboard
- `pass` protected route stayed on expected path: final_path=/dashboard
- `pass` protected route did not fall back to login: title=园区网络全流量采集与分析系统
- `pass` no browser runtime errors: {"console_errors":0,"page_errors":0,"request_failures":0,"server_errors":0}
