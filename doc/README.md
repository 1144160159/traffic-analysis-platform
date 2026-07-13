# traffic-analysis-platform 文档索引与合并说明

更新时间：2026-06-26
适用范围：课题一“园区网络流量智能检测与分析”的产品设计、技术设计、工程验收和试点交付材料。

## 1. 推荐阅读路径

| 读者 | 先读 | 再读 | 目的 |
|---|---|---|---|
| 项目负责人/评审专家 | `01_design/课题一产品与技术总体设计.md` | `02_acceptance/README.md`、`03_review/专家深评整改清单.md`、`05_status/未开发项梳理-2026-06-19.md`、`05_status/代码实证状态核对-2026-06-19.md` | 快速掌握系统定位、指标闭环和验收缺口 |
| 产品/售前 | `01_design/课题一产品与技术总体设计.md` | `01_design/docx/面向园区网络的全流量采集分析系统-产品设计.docx`、`04_assets/generated/`、`04_assets/ui_suite_gpt_v1/` | 准备演示、试点、用户沟通和材料包装 |
| 技术/研发 | `01_design/课题一产品与技术总体设计.md` | `01_design/docx/面向园区网络的全流量采集分析系统-技术设计.docx`、`agent.md`、`common/`、`proto/` | 对齐架构、数据链路、接口契约和实现边界 |
| Codex/自动化工程 | `01_design/Codex-Loop-Engineering-设计.md`、`01_design/自动开发Loop引擎设计.md` | `agent.md`、`scripts/codex_loop/README.md`、`02_acceptance/README.md`、`03_review/专家深评整改清单.md`、`05_status/代码实证状态核对-2026-06-19.md` | 建立自主开发、真实数据验证、K8s live 验证和证据沉淀闭环 |
| 测试/验收 | `02_acceptance/README.md` | `05_status/代码实证状态核对-2026-06-19.md`、`05_status/未开发项梳理-2026-06-19.md`、`03_review/专家深评整改清单.md` | 区分 smoke、regression、acceptance、third-party 证据 |
| 实施/SRE | `01_design/课题一产品与技术总体设计.md` | `05_status/未开发项梳理-2026-06-19.md`、`06_ops/基础设施实施进度.txt`、`deployments/` | 对齐现场部署、生产安全、HA 和可观测性 |

## 2. 主文档

| 文件 | 状态 | 说明 |
|---|---|---|
| `01_design/课题一产品与技术总体设计.md` | 当前主线 | 合并 6 月 18 深化设计、补强版总体方案、6 月 19 交付版、采集分工、验收门禁和评审整改结论 |
| `01_design/Codex-Loop-Engineering-设计.md` | 当前主线 | Codex 自主工程闭环设计，覆盖任务状态机、K8s live 验证、真实数据策略、自动化矩阵和证据账本 |
| `01_design/自动开发Loop引擎设计.md` | 当前主线 | Codex Loop Engineering 的工程化执行层设计，覆盖项目架构地图、运行前 preflight、per-lane resource quota、dynamic resource monitor、workspace isolation planner/activation/cleanup、workspace backend、executor pool stress、remote pool stress、remote pool K8s stress/bootstrap/readiness、objective stop、生产级成熟度审计、bounded soak、隔离执行计划与显式执行闸门、隔离执行池桥接、bounded executor pool、HTTP queue service/backend、上帝视角、产品/功能/视觉/架构设计包、任务级上下文包、工作流编排、租约锁、repo-json 审计队列、SQLite/WAL 事务队列、远程 worker 仲裁、lease owner 回写校验、轻量 worker、bounded daemon、可选 supervisor、systemd/K8s 部署计划、loop-control 镜像构建与边界、发布冻结、回滚计划、运行指标、健康检查、恢复审计、实现守门、Codex patch work order、模型画像选择、安全 Codex runner、外部 Codex adapter、diff-aware/semantic/LLM Reviewer、证据判定、失败修复计划、自动修复下一步计划、任务队列/锁/重试计划、任务状态闭环、前端备份重做专线、任务模型、策略门禁、证据落盘和 MVP 落地路径；MVP 脚手架已落到 `scripts/codex_loop/` |
| `01_design/面向园区网络的全流量采集分析系统-UI设计套装.md` | 当前主线 | 基于 Product Design 流程形成的 UI 设计套装，已确认 1+2 混合视觉，公开产品标题统一为“园区网络全流量采集与分析系统” |
| `02_acceptance/README.md` | 当前主线 | 验收证据包索引，明确功能回归与任务书验收不能混用 |
| `03_review/专家深评整改清单.md` | 当前主线 | 多角色深评后的 P0/P1 整改入口 |
| `05_status/未开发项梳理-2026-06-19.md` | 当前主线 | 区分未开发、未生产化、未验收闭环和旧文档误报 |
| `05_status/代码实证状态核对-2026-06-19.md` | 当前主线 | 从当前源码、部署清单、测试脚本直接核对真实实现状态，校正未开发项清单 |
| `05_status/UI视觉复刻实现状态-2026-06-26.md` | 当前主线 | 记录前端按 UI 设计图 1:1 复刻的实现、验证证据、Desktop 浏览器验证边界和仍未关闭的项目级门禁 |
| `MIGRATION_MAP.md` | 当前主线 | 记录旧路径到新路径的完整迁移映射 |

## 3. 支撑文档

| 文件 | 处理方式 | 说明 |
|---|---|---|
| `00_sources/任务书.docx` | 保留原文 | 课题一指标和验收要求的原始依据 |
| `00_sources/实施方案.docx` | 保留原文 | 多源异构融合、工程实施和阶段目标依据 |
| `01_design/docx/面向园区网络的全流量采集分析系统-产品设计.docx` | 后续同步 | 正式 PRD 输出，后续从主设计稿同步更新 |
| `01_design/docx/面向园区网络的全流量采集分析系统-技术设计.docx` | 后续同步 | 正式 SDD 输出，后续从主设计稿同步更新 |
| `01_design/docx/面向园区网络的全流量采集分析系统-技术选型.docx` | 保留 | 技术栈和选型说明 |
| `01_design/docx/面向园区网络的全流量采集分析系统-研发计划.docx` | 后续同步 | 研发排期和任务计划输出 |
| `04_assets/generated/` | 保留 | 产品蓝图、态势感知大屏概念 UI、GPT 生成图存档台账、架构、流程、验收门禁等视觉资产 |
| `04_assets/ui_suite_gpt_v1/` | 当前主线 | GPT 生图版高保真 UI 套装，含最终页面图、浮层图、foundation 规范板、4K 背景、prompt、manifest 和接力状态文档 |
| `04_assets/diagrams/` | 保留 | Mermaid 图源和 Figma/FigJam 图源 |
| `../scripts/codex_loop/` | 当前主线 | 自动开发 Loop MVP 脚手架，含任务发现、运行前 preflight、per-lane resource quota、dynamic resource monitor、workspace isolation planner/activation/cleanup、`git-worktree`/`local-clone` workspace backend、executor pool stress、remote pool stress、remote pool K8s stress/bootstrap/readiness、objective stop、生产级成熟度审计、bounded soak、隔离执行计划与显式执行闸门、隔离执行池桥接、bounded executor pool、HTTP queue service/backend、上帝视角、纠偏引导、设计包生成、任务级上下文包、工作流编排、租约锁、repo-json 审计队列、SQLite/WAL 事务队列、远程 worker 仲裁、lease owner 回写校验、轻量 worker、bounded daemon、可选 supervisor、systemd/K8s 部署计划、loop-control Dockerfile、image build 证据与 image layout 门禁、发布冻结、回滚计划、运行指标、健康检查、恢复审计、实现守门、Codex patch work order、模型画像选择、安全 Codex runner、外部 Codex adapter、diff-aware/semantic/LLM Reviewer、证据判定、失败修复计划、自动修复下一步计划、任务队列/锁/重试计划、任务状态闭环、计划生成、证据收集、状态更新、策略和首批任务 YAML |

## 4. 已合并内容来源

| 来源 | 合并到主设计稿的位置 |
|---|---|
| `archive/merged_20260619/面向园区网络的全流量采集分析系统-深化设计与评审闭环.md` | 总体定位、功能点闭环、技术设计、演示与试点、评审口径 |
| `archive/merged_20260619/课题一工程系统总体方案-补强版.md` | 建设目标、系统边界、三大闭环、数据质量、检测质量、性能容量、安全合规 |
| 原 2026-06-19 对外交付版 Markdown/DOCX（已融合至产品设计和技术设计 DOCX 后删除） | 一句话定位、REQ-T1 追溯、FigJam/交付件说明 |
| `archive/merged_20260619/数据采集分工设计.md` | 多源数据采集分工、流量/资产/日志/用户行为数据链路 |
| `02_acceptance/README.md` | 证据分层、P0 门禁、状态标记规则 |
| `03_review/专家深评整改清单.md` | P0/P1 风险、技术总监红线、算法/实施/UI/测试等角色整改结论 |
| `05_status/未开发项梳理-2026-06-19.md`、`05_status/代码实证状态核对-2026-06-19.md` 和 `agent.md` | 当前系统状态、真实链路证据、剩余专项缺口 |

## 5. 后续建议

1. 将 `01_design/课题一产品与技术总体设计.md` 作为唯一 Markdown 主线。
2. 稳定后再生成新的 PRD/SDD DOCX，替换或归档旧版本。
3. 旧状态快照已合并删除；后续状态统一维护在 `05_status/未开发项梳理-2026-06-19.md` 和 `05_status/代码实证状态核对-2026-06-19.md`。
4. `06_ops/基础设施实施进度.txt`、`06_ops/服务器信息` 已归入运维区；后续如需对外交付，应脱敏后从 `01_design/docx/`、`02_acceptance/`、`04_assets/` 组装交付包。
5. 对外交付快照不再在 `doc/` 下单独保留；需要留痕时以发布包或外部归档记录为准。
