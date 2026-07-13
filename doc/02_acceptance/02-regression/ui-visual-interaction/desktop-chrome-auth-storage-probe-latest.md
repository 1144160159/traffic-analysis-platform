# Desktop Chrome Authenticated Route Probe

Result: blocked

The short-lived JWT was accepted by `/api/v1/auth/me` as `codex-ui-desktop-admin`, so the backend token itself is valid. The Desktop Chrome authenticated UI path is still blocked because current project rules require wrapper tools and forbid login form submission, production runtime has `DESKTOP_SMOKE_TOKEN_ENABLED=false`, and the Chrome extension evaluate context cannot access page `window.localStorage` for token injection.

Acceptance effect: UI visual/interaction remains blocked at `visual=0/30`, `interactions=1/28`; `/alerts` was not marked pass.

Remediation: provide an approved Desktop Chrome wrapper for same-origin session setup, enable the smoke hash path for a documented acceptance window, or add a one-time non-production smoke callback.
