# Windows Node REPL Env Matrix Smoke

- Result: `blocked_chrome_bridge`
- Generated: `2026-07-02T20:27:30.814Z`
- Windows host: `10.3.6.59`
- MCP config: `pass`
- Matrix cases: `12`
- JS pass cases: `7`
- Chrome extension-ready cases: `0`

This matrix tries selected node_repl environment subsets over SSH stdio. It records environment key names only, never values. It does not upload screenshots or close visual acceptance.

## Matrix

- `none`: js=`pass` chrome=`native_pipe_or_trust_unavailable` env_keys=`none`
- `trusted-browser-client`: js=`pass` chrome=`js_error` env_keys=`NODE_REPL_TRUSTED_BROWSER_CLIENT_SHA256S`
- `trusted-code`: js=`pass` chrome=`js_error` env_keys=`NODE_REPL_TRUSTED_CODE_PATHS`
- `native-pipe`: js=`pass` chrome=`native_pipe_or_trust_unavailable` env_keys=`SKY_CUA_NATIVE_PIPE,SKY_CUA_NATIVE_PIPE_DIRECTORY`
- `browser-backend-metadata`: js=`pass` chrome=`native_pipe_or_trust_unavailable` env_keys=`BROWSER_USE_AVAILABLE_BACKENDS,BROWSER_USE_CODEX_APP_VERSION,BROWSER_USE_CODEX_APP_BUILD_FLAVOR`
- `codex-context`: js=`sandbox_firewall_denied` chrome=`skipped` env_keys=`CODEX_HOME,CODEX_CLI_PATH`
- `node-paths`: js=`pass` chrome=`native_pipe_or_trust_unavailable` env_keys=`NODE_REPL_NODE_PATH,NODE_REPL_NODE_MODULE_DIRS`
- `trust-and-pipe`: js=`pass` chrome=`js_error` env_keys=`NODE_REPL_TRUSTED_BROWSER_CLIENT_SHA256S,NODE_REPL_TRUSTED_CODE_PATHS,SKY_CUA_NATIVE_PIPE,SKY_CUA_NATIVE_PIPE_DIRECTORY`
- `trust-pipe-backend-codex`: js=`sandbox_firewall_denied` chrome=`skipped` env_keys=`NODE_REPL_TRUSTED_BROWSER_CLIENT_SHA256S,NODE_REPL_TRUSTED_CODE_PATHS,SKY_CUA_NATIVE_PIPE,SKY_CUA_NATIVE_PIPE_DIRECTORY,BROWSER_USE_AVAILABLE_BACKENDS,BROWSER_USE_CODEX_APP_VERSION,BROWSER_USE_CODEX_APP_BUILD_FLAVOR,CODEX_HOME,CODEX_CLI_PATH`
- `all-except-node-paths`: js=`sandbox_firewall_denied` chrome=`skipped` env_keys=`NODE_REPL_INSTRUCTIONS_USE_CASE_BROWSER,BROWSER_USE_CODEX_APP_VERSION,NODE_REPL_NATIVE_PIPE_CONNECT_TIMEOUT_MS,BROWSER_USE_CODEX_APP_BUILD_FLAVOR,CODEX_HOME,SKY_CUA_NATIVE_PIPE_DIRECTORY,NODE_REPL_INSTRUCTIONS_USE_CASE_CHROME,SKY_CUA_NATIVE_PIPE,NODE_REPL_TRUSTED_BROWSER_CLIENT_SHA256S,NODE_REPL_TRUSTED_CODE_PATHS,BROWSER_USE_AVAILABLE_BACKENDS,CODEX_CLI_PATH`
- `all-except-native-pipe`: js=`sandbox_firewall_denied` chrome=`skipped` env_keys=`NODE_REPL_INSTRUCTIONS_USE_CASE_BROWSER,BROWSER_USE_CODEX_APP_VERSION,NODE_REPL_NATIVE_PIPE_CONNECT_TIMEOUT_MS,BROWSER_USE_CODEX_APP_BUILD_FLAVOR,CODEX_HOME,NODE_REPL_NODE_MODULE_DIRS,NODE_REPL_NODE_PATH,NODE_REPL_INSTRUCTIONS_USE_CASE_CHROME,NODE_REPL_TRUSTED_BROWSER_CLIENT_SHA256S,NODE_REPL_TRUSTED_CODE_PATHS,BROWSER_USE_AVAILABLE_BACKENDS,CODEX_CLI_PATH`
- `full`: js=`sandbox_firewall_denied` chrome=`skipped` env_keys=`NODE_REPL_INSTRUCTIONS_USE_CASE_BROWSER,BROWSER_USE_CODEX_APP_VERSION,NODE_REPL_NATIVE_PIPE_CONNECT_TIMEOUT_MS,BROWSER_USE_CODEX_APP_BUILD_FLAVOR,CODEX_HOME,SKY_CUA_NATIVE_PIPE_DIRECTORY,NODE_REPL_NODE_MODULE_DIRS,NODE_REPL_NODE_PATH,NODE_REPL_INSTRUCTIONS_USE_CASE_CHROME,SKY_CUA_NATIVE_PIPE,NODE_REPL_TRUSTED_BROWSER_CLIENT_SHA256S,NODE_REPL_TRUSTED_CODE_PATHS,BROWSER_USE_AVAILABLE_BACKENDS,CODEX_CLI_PATH`

## Blockers

- No tested node_repl environment subset reached the Chrome extension backend from SSH stdio

