# Blocker Owner Matrix

| Gate | Owner | Closure State | Next Action |
|---|---|---|---|
| desktop_browser_smoke | QA / Desktop runtime | windows_tunnel_payload_ready_but_bridge_tool_missing | expose the current-session Codex Desktop Chrome bridge tool for the Windows VSCode machine at 10.3.6.59, run the generated payload through Chrome extension backend, upload the bridge run summary, then rerun UI visual and project completion gates |
| ui_visual_interaction | Frontend / QA | full_gap_capture_package_ready_but_desktop_chrome_execution_missing | execute the full 30 visual / 28 interaction Windows tunnel payload in Desktop Chrome extension backend, upload 58 screenshots plus one bridge run summary, finalize evidence, then rerun the UI visual interaction gate |
| production_security | Security / SRE | external_cni_and_waiver_required | install or migrate to a policy-capable CNI, review runtime waivers for privileged/hostNetwork workloads, then rerun production security preflight |
| network_policy_enforcement | Security / Network | external_cni_required | use network-policy readiness package to migrate CNI, then run isolated default-deny and allow-list probe |
| ha_rto_rpo | SRE / QA | maintenance_window_required | execute destructive Kafka/Flink/ClickHouse/PostgreSQL/MinIO drills using HA bootstrap templates and publish formal RTO/RPO reports |
| capture_performance | Performance / Probe | hardware_window_required | fill hardware and traffic profiles, run 10 x 100Gbps and 512Mpps tests, then rerun capture performance preflight |
| detection_quality | Algorithm / Third-party QA | third_party_adjudication_required | freeze dataset, fill labels and predictions, lock thresholds, obtain third-party attestation, then rerun detection quality preflight |
| asset_discovery_coverage | Implementation / Site owner | site_inventory_required | review observed asset inventory bootstrap with site owner, produce authoritative SITE_ASSET_INVENTORY_JSON, then rerun coverage gate |
| trial_third_party_signoff | Project manager / User / Third-party | signature_and_external_report_required | fill signoff placeholders, resolve upstream exceptions, attach pilot/third-party/economic-benefit confirmations, then rerun project completion audit |
