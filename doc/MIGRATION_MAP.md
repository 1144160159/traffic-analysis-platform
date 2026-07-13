# doc 目录迁移映射

更新时间：2026-06-19  
目的：记录本次文档整理后的旧路径到新路径映射。旧内容按用途归类、合并或归档；已融合进主线且不再承担证据作用的快照标注为删除。

## 1. 主线新增

| 新路径 | 说明 |
|---|---|
| `doc/README.md` | 文档入口、阅读路径和合并说明 |
| `doc/01_design/课题一产品与技术总体设计.md` | 合并后的产品与技术总体设计 Markdown 主线 |
| `doc/MIGRATION_MAP.md` | 本迁移映射 |

## 2. 原始依据

| 旧路径 | 新路径 |
|---|---|
| `doc/任务书.docx` | `doc/00_sources/任务书.docx` |
| `doc/实施方案.docx` | `doc/00_sources/实施方案.docx` |

## 3. 正式设计与汇报输出

| 旧路径 | 新路径 |
|---|---|
| `doc/面向园区网络的全流量采集分析系统-产品设计.docx` | `doc/01_design/docx/面向园区网络的全流量采集分析系统-产品设计.docx` |
| `doc/面向园区网络的全流量采集分析系统-技术设计.docx` | `doc/01_design/docx/面向园区网络的全流量采集分析系统-技术设计.docx` |
| `doc/面向园区网络的全流量采集分析系统-技术选型.docx` | `doc/01_design/docx/面向园区网络的全流量采集分析系统-技术选型.docx` |
| `doc/面向园区网络的全流量采集分析系统-研发计划.docx` | `doc/01_design/docx/面向园区网络的全流量采集分析系统-研发计划.docx` |
| `doc/面向园区网络的全流量采集分析系统-汇报PPT.pptx` | `doc/01_design/docx/面向园区网络的全流量采集分析系统-汇报PPT.pptx` |

## 4. 验收与评审

| 旧路径 | 新路径 |
|---|---|
| `doc/acceptance/README.md` | `doc/02_acceptance/README.md` |
| `doc/review/generate_review_ledger.mjs` | `doc/03_review/generate_review_ledger.mjs` |
| `doc/review/multi_role_review_ledger.csv` | `doc/03_review/multi_role_review_ledger.csv` |
| `doc/review/multi_role_review_summary.json` | `doc/03_review/multi_role_review_summary.json` |
| `doc/review/专家深评整改清单.md` | `doc/03_review/专家深评整改清单.md` |

## 5. 图形资产

| 旧路径 | 新路径 |
|---|---|
| `doc/assets/generated/*` | `doc/04_assets/generated/*` |
| `doc/diagrams/*` | `doc/04_assets/diagrams/*` |

## 6. 交付快照

| 旧路径 | 新路径 |
|---|---|
| `doc/deliverables_20260619/*` | 已融合至 `doc/01_design/docx/面向园区网络的全流量采集分析系统-产品设计.docx` 和 `doc/01_design/docx/面向园区网络的全流量采集分析系统-技术设计.docx`，旧快照删除 |

## 7. 状态、计划与运维材料

| 旧路径 | 新路径 |
|---|---|
| `doc/system-function-status-2026-06-17.md` | 已合并至 `doc/05_status/代码实证状态核对-2026-06-19.md` 和 `agent.md`，旧快照删除 |
| `doc/系统梳理与开发计划.md` | 已合并至 `doc/05_status/未开发项梳理-2026-06-19.md`、`doc/05_status/代码实证状态核对-2026-06-19.md` 和 `doc/01_design/课题一产品与技术总体设计.md`，旧计划删除 |
| `doc/基础设施实施进度.txt` | `doc/06_ops/基础设施实施进度.txt` |
| `doc/服务器信息` | `doc/06_ops/服务器信息` |

## 8. 历史归档

| 旧路径 | 新路径 |
|---|---|
| `doc/初稿/*` | `doc/archive/initial_drafts/*` |
| `doc/课题一工程系统总体方案-补强版.md` | `doc/archive/merged_20260619/课题一工程系统总体方案-补强版.md` |
| `doc/面向园区网络的全流量采集分析系统-深化设计与评审闭环.md` | `doc/archive/merged_20260619/面向园区网络的全流量采集分析系统-深化设计与评审闭环.md` |
| `doc/数据采集分工设计.md` | `doc/archive/merged_20260619/数据采集分工设计.md` |
| `doc/设计文档规范化标题大纲.md` | `doc/archive/merged_20260619/设计文档规范化标题大纲.md` |

## 9. 维护约定

1. 新增正文内容优先写入 `doc/01_design/课题一产品与技术总体设计.md`。
2. 新增验收证据放入 `doc/02_acceptance/`。
3. 新增评审整改放入 `doc/03_review/专家深评整改清单.md`。
4. 新增图形资产放入 `doc/04_assets/generated/` 或 `doc/04_assets/diagrams/`。
5. 对外交付包从 `doc/01_design/docx/`、`doc/02_acceptance/`、`doc/04_assets/` 组装；正式快照在发布包或外部归档中留痕，不再在 `doc/` 下单独保留日期目录。
6. 历史材料原则上归档不删除；已融合进主线且会造成双主线的交付快照，可在迁移表标注后删除。
