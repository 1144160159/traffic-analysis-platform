# GPT 生图执行状态

更新时间：2026-06-28

说明：本文保留 2026-06-20 API 自动生图失败记录作为历史归档；现行 UI 套装范围和完成状态以 `manifest.json`、`CHAT_IMAGEGEN_INVENTORY.md`、`GENERATION_STATE.md`、`UI_IMAGEGEN_METHOD.md` 和 `CONTEXT_HANDOFF.md` 为准。

## 已完成

- 已确认最终视觉参考图：`doc/04_assets/generated/campus_full_traffic_system_visual_reference_20260620_business_corrected.png`
- 已确认高保真输出尺寸：所有 UI 图一律生成 `1920x1080 px`
- 已梳理全量 UI 图范围：8 张 foundations + 27 张页面图 + 70 张浮层图 + 48 张组件板 + 16 张状态图 + 12 张响应式图，共 181 张
- 已生成本地清单：`doc/04_assets/ui_suite_gpt_v1/manifest.json`
- 已生成逐图 prompt：`doc/04_assets/ui_suite_gpt_v1/prompts/*.prompt.txt`
- 已生成可恢复执行脚本：`doc/04_assets/ui_suite_gpt_v1/run_generation.sh`
- 已生成 foundations 拼接参考板：`doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-generation-reference.png`
- 已增加全局生图约束：后续所有生成、编辑或重生成的 UI 图片，无论是页面、浮层、组件、状态图还是响应式适配图，都必须严格遵循 foundations 的 UI 规范。
- 已将生成硬门禁升级为 foundations 强约束：AppShell、栅格、色彩 token、状态语义、字号密度、圆角、表格行高、ECharts 深色样式和响应式策略都必须严格遵守。
- 已完成 manifest 交付基线 181/181：foundations 8/8、pages 27/27、overlays 70/70、components 48/48、states 16/16、responsive 12/12。
- 已校验 181 张目标图全部存在、尺寸均为 `1920x1080`，且每张均保留 `*.raw-imagegen.png` 或 `*.raw-deterministic.png` 追溯文件。
- 已按 2026-06-27 新口径完成后续 overlay：弹窗、抽屉、下拉和确认框只展示业务区域本体，不强制携带公共 AppShell。

## 历史 API 阻塞归档

下列内容仅保留为 2026-06-20 API 自动生图失败记录，不再代表当前状态；当前常规 manifest 队列已通过聊天生成、提取和确定性绘制方式完成。历史试跑包括：

- `2026-06-20 04:04`：条目 `alerts`，目标 `doc/04_assets/ui_suite_gpt_v1/screens/pages/alerts.png`，OpenAI 返回 `billing_hard_limit_reached`，未生成 PNG。
- `2026-06-20 16:32`：条目 `dashboard` / `screen`，`SIZE=1920x1080` 时 `gpt-image-2` 拒绝尺寸，因为宽高必须是 16 的倍数。
- `2026-06-20 16:32`：改用 `SIZE=1920x1088` 后，请求到达 OpenAI，但本地 key 返回 `401 Incorrect API key provided`，未覆盖项目 PNG。
- `2026-06-20 16:44`：升级脚本后再次生成 `dashboard`，已使用 foundations 拼接参考板并自动请求 `1920x1088`，请求到达 OpenAI，但本地 key 仍返回 `401 Incorrect API key provided`，未覆盖项目 PNG。

脚本处理：

- `run_generation.sh` 现在默认使用 foundations 拼接参考板。
- 当 `MODEL=gpt-image-2` 且目标为 `1920x1080` 时，脚本会请求 `1920x1088` 并自动裁剪回 `1920x1080`。

2026-06-28 收口：上述 API 阻塞不再影响当前交付状态。`dashboard.png`、`screen.png` 和其余 manifest 图均已在后续批次中完成或返工，并以 `GENERATION_STATE.md`、`CONTEXT_HANDOFF.md` 和当前落盘文件为准。

## 聊天窗口生成替代方案

由于 API 自动调用曾不可用，后续批次改为通过 GPT 聊天窗口逐张生成、提取，或对严格 AppShell/组件板使用确定性绘制并保存到本地文件系统。该替代流程已完成当前 manifest 队列。

现行工业级完整清单为 181 张：

- 8 张视觉基线与规范板
- 27 张页面主图
- 70 张业务浮层图
- 48 张元件与组件板
- 16 张通用状态图
- 12 张响应式与大屏适配图

详细清单见：`doc/04_assets/ui_suite_gpt_v1/CHAT_IMAGEGEN_INVENTORY.md`。

### 2026-06-20 聊天生图执行记录归档

- 已通过内置 GPT ImageGen 在聊天中生成首张核心页面：`综合态势 / 态势大屏`。
- 本轮聊天生成提示词已并入 canonical `doc/04_assets/ui_suite_gpt_v1/prompts/screen.prompt.txt`；历史 `screen-chat-imagegen-v1.prompt.txt` 未保留在当前 prompt 目录。
- 预期归档目标：`doc/04_assets/ui_suite_gpt_v1/screens/pages/screen.png`。
- 当前导出状态：历史记录为 `pending-export`；现行 manifest 图已全部落盘，详见 `GENERATION_STATE.md`。
- 本轮已尝试使用 Computer Use 自动下载聊天图片；Windows 侧 Computer Use 客户端可定位，但当前会话返回 `Computer Use native pipe path is unavailable`，无法通过桌面自动化点击图片下载按钮。
- 已执行 Windows 收件箱同步脚本，`pulled_count=0`，说明 `C:\Users\11441\Downloads\traffic-ui-imagegen-inbox` 当前没有新图片。

## 恢复生成

当前无下一张常规生成项；以下命令仅用于未来质量返工、单张重生或用户新增范围后的恢复执行。

```bash
cd /home/wangwt/phase_2/code/traffic-analysis-platform
QUALITY=medium SIZE=1920x1080 bash doc/04_assets/ui_suite_gpt_v1/run_generation.sh
```

单张验证：

```bash
ONLY_ID=alerts QUALITY=medium SIZE=1920x1080 bash doc/04_assets/ui_suite_gpt_v1/run_generation.sh
```

Dry-run 检查：

```bash
DRY_RUN=1 LIMIT=3 bash doc/04_assets/ui_suite_gpt_v1/run_generation.sh
```
