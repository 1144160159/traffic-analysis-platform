# Desktop Chrome Viewport Capability Probe

- Run ID: `20260702-desktop-chrome-viewport-capability-r3-cdp-tab-control-timeout`
- Result: `blocked`
- Backend: Codex Desktop Chrome extension
- Target: `http://10.0.5.8:30180/login`
- Title: `园区网络全流量采集与分析系统`

The Desktop Chrome bridge can expose the Chrome extension target, open `/login`, and capture a real screenshot before viewport-control experimentation. The current screenshot is JPEG/PNG-converted `2559x1271`, while the formal UI visual gate requires browser-generated `1920x1080` evidence with receiver metadata proving the uploaded/stored screenshot and browser viewport are both `1920x1080`.

Observed window metrics:

```json
{
  "inner_width": 2560,
  "inner_height": 1271,
  "device_pixel_ratio": 1.5,
  "visual_viewport_width": 2560,
  "visual_viewport_height": 1271.3333740234375,
  "resize_to_type": "undefined"
}
```

The exposed bridge methods include navigation, screenshot, DOM snapshot, evaluation, locators, waits, clicks, typing, and scrolling, but no direct viewport, bounds, or window resize method. The tab capability list does expose `cdp`, and `cdp.send(method, params, options)` is callable. In r3, `Emulation.setDeviceMetricsOverride` was attempted with `width=1920`, `height=1080`, `deviceScaleFactor=1`, `mobile=false`; the probe timed out, and subsequent `tab.playwright.evaluate`, `user.openTabs`, and `tabs.list` also timed out. Therefore the `ui_visual_interaction` gate must stay blocked until Desktop Chrome can reliably produce browser-generated `1920x1080` screenshots and receiver `capture-meta.json` confirms uploaded/stored size plus browser viewport without post-capture resizing.
