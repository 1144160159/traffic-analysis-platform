# Traffic Analysis Web UI

This is the rebuilt React front end for the campus full-traffic collection and analysis system.

The implementation is grounded in:

- `doc/01_design/面向园区网络的全流量采集分析系统-UI前端规范.md`
- `doc/01_design/面向园区网络的全流量采集分析系统-左侧菜单信息架构.md`
- `doc/01_design/面向园区网络的全流量采集分析系统-二级菜单功能点与表现形式矩阵.md`
- `doc/04_assets/ui_suite_gpt_v1/manifest.json`

## Local

```bash
npm install
npm run dev
```

## K8s

The production image serves static assets through Nginx and proxies `/api` and `/ws` to APISIX inside the `traffic-analysis` namespace.
