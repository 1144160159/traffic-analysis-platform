# UI、软件设计模式与数据库设计资料库

更新时间：2026-08-11

本目录收录公开规范、官方设计系统网页快照、开放模式目录、出版社书目信息和出版社授权样章。它不是盗版书库，也不把书籍观点等同于强制标准。

## 下载结果

- 资料条目：39 项
- 已下载快照、文件或项目整理规范：25 项，合计 13,106,117 字节
- 仅保留官方链接：14 项；其中 11 项自动下载失败，3 项商业图书需要购买或授权在线访问
- 合法 PDF 样章：4 份，均已验证 PDF 文件头、可解析性、页数和 SHA-256
- 完整机器可读清单：[download_manifest.csv](00_Index/download_manifest.csv) / [download_manifest.json](00_Index/download_manifest.json)
- 所有条目的官方入口：[Official_Links](00_Index/Official_Links/)

状态含义：

- `FULL`：开放发布的完整规范页面。
- `OPEN_CATALOG`：作者或维护者公开发布的完整模式目录，不等于对应书籍全文。
- `SNAPSHOT`：官方网页离线快照；动态站点的样式、图片或交互可能仍需联网。
- `SAMPLE`：出版社明确公开的合法样章。
- `INDEX_ONLY`：标准或书籍的书目信息、摘要和官方入口；全文通常需要购买、机构订阅或登录。

## 一、UI 与可访问性

### 建议作为项目硬性基线

1. [WCAG 2.2](01_UI_and_Accessibility/01_Open_Standards/W3C_WCAG_2.2.html)：Web 可访问性验收采用 AA 级；AAA 仅按具体场景采用。
2. [WAI-ARIA 1.2](01_UI_and_Accessibility/01_Open_Standards/W3C_WAI-ARIA_1.2.html)：定义角色、状态与属性；优先使用原生 HTML 语义，只有原生语义不足时才使用 ARIA。
3. [ARIA Authoring Practices Guide](01_UI_and_Accessibility/01_Open_Standards/W3C_ARIA_APG.html)：用于复杂组件的键盘行为、焦点管理和可访问名称实现。
4. ISO 9241 系列：
   - 9241-210:2019：以人为中心的设计过程；
   - 9241-110:2020：任务适配、自描述性、符合预期、易学、可控、容错和参与度；
   - 9241-112:2025：信息呈现原则；
   - 9241-115:2024：概念设计、交互设计、界面与导航设计。

### 官方设计系统参考

- Material Design 3：跨平台 token、组件、布局和交互参考。
- Apple Human Interface Guidelines：Apple 原生平台规范。
- Fluent 2：桌面和生产力软件的层次、密度、布局与组件参考。
- Carbon：复杂 B 端、数据与运维产品的组件和模式参考。
- GOV.UK Design System / USWDS：表单、错误、内容设计、服务流程和可访问性参考。
- Ant Design：中文企业级 Web 产品的组件与交互参考。

使用原则：先确定一个主设计系统作为组件和 token 的来源，再从其他系统吸收经过论证的模式；不要把多个系统的视觉值和组件状态直接拼接成一套界面。

### UI 书籍

- *Designing Interfaces, 3rd Edition*：交互界面模式目录。当前只保存官方入口；全文受版权保护。
- [三本中文 UI 图书的正版获取与授权状态](01_UI_and_Accessibility/04_Book_Indexes/UI_图书正版获取与授权状态.md)：记录《破译 Web UI》《瞬间之美》《About Face 4》的出版社、ISBN、正版电子版或购买入口，以及不可抓取/导出的边界。
- [Web UI 栅格、尺寸与组件状态规范速查](01_UI_and_Accessibility/04_Book_Indexes/Web_UI_栅格_尺寸与组件状态规范速查.md)：把 WCAG 硬性门槛、常见栅格数值、组件状态矩阵和交付验收项整理成项目中性基线。

## 二、软件架构与设计模式

### 标准与质量门

- ISO/IEC/IEEE 42010:2022：架构描述应明确利益相关者、关注点、视角、视图、模型与对应关系。
- ISO/IEC 25010:2023：把功能适合性、性能效率、兼容性、交互能力、可靠性、安全性、可维护性、灵活性和安全风险等质量属性转为可验收要求。
- OWASP Secure by Design：用于设计阶段的信任边界、最小权限、安全默认值、隔离、数据保护、韧性与威胁建模检查。当前材料属于社区框架，不能冒充 ISO 标准。

### 核心模式资料

- *Design Patterns: Elements of Reusable Object-Oriented Software*（GoF）：23 个对象设计模式；使用时必须记录适用问题、替代方案和代价，禁止为“用了模式”而套模式。
- [Patterns of Enterprise Application Architecture](02_Software_Architecture_and_Patterns/02_Open_Pattern_Catalogs/PoEAA_Catalog.html)：分层、领域逻辑、对象关系映射、并发、会话与分布式边界模式。
- *Domain-Driven Design*：统一语言、实体、值对象、聚合、仓储、限界上下文和防腐层。
- [Enterprise Integration Patterns](02_Software_Architecture_and_Patterns/02_Open_Pattern_Catalogs/Enterprise_Integration_Patterns_Catalog.html)：消息构造、路由、转换、端点、可靠性和运维模式。

### 已下载合法样章

- [Domain-Driven Design Chapter 1](02_Software_Architecture_and_Patterns/03_Books_and_Legal_Samples/Domain_Driven_Design_Chapter_1.pdf)：16 页；SHA-256 `a6968f466088ed7a89bdfd3fc287bdbd714c4ee776903c759f49c92e6978742f`。
- [Domain-Driven Design Sample Pages](02_Software_Architecture_and_Patterns/03_Books_and_Legal_Samples/Domain_Driven_Design_Sample_Pages.pdf)：60 页；SHA-256 `f16b153c1e5e0847a7c7edf97971211e7f6d4922f2d2071628493660e5df0c06`。

## 三、数据库设计

### 建议作为项目硬性基线

1. 从业务事实和不变量出发建立概念模型，再建立逻辑模型，最后才映射到具体数据库物理模型。
2. 每张关系表必须有稳定主键；候选键、外键、唯一性、非空、取值域和检查约束应由数据库声明，不只依赖应用代码。
3. 默认规范化到 3NF/BCNF；反规范化只能由已测量的性能瓶颈驱动，并记录冗余来源、同步策略、失败恢复和一致性检查。
4. 命名采用稳定、一致、可搜索的词汇表；同一业务概念不得出现多个缩写或同名异义。参考 ISO/IEC 11179-5:2015。
5. 数据质量要求覆盖准确性、完整性、一致性、可信性、时效性、可访问性、可追溯性等，并形成可执行检查。参考 ISO/IEC 25012:2008。
6. 模式变更必须版本化、可回滚或可前向修复；迁移脚本、数据回填、兼容窗口和回滚证据一并评审。
7. 索引由查询负载、选择性、排序/连接路径和写放大共同决定；禁止只凭经验批量建索引。
8. OLTP、时序/事件、全文检索、图关系与 OLAP 的工作负载不同，模型和存储可以分层，但事实口径和主数据责任必须唯一。

### 核心书籍

- *Database Design for Mere Mortals, 4th Edition*：最适合作为关系数据库设计的实践主线，覆盖目标、访谈、表、字段、键、关系、业务规则、视图和完整性。
- *Database Design and Relational Theory, 2nd Edition*：补足函数依赖、BCNF、4NF、5NF、6NF、依赖保持与一致性理论。
- *Designing Data-Intensive Applications, 2nd Edition*：面向复制、分区、事务、批流处理和分布式一致性的系统设计，不替代关系模式设计。
- *The Data Warehouse Toolkit, 3rd Edition*：维度建模、事实表、维度表、粒度和分析型数据仓库设计。

### 已下载合法样章

- [Database Design for Mere Mortals, 4th Edition Sample](03_Database_Design/02_Books_and_Legal_Samples/Database_Design_for_Mere_Mortals_4e_Sample.pdf)：78 页；SHA-256 `6448a5b7f6cf59630b9f6210ae7e355b426c9071d1f557d0d2ee9ab3e9552cf4`。
- [The Data Warehouse Toolkit, 3rd Edition Chapter 1 Excerpt](03_Database_Design/02_Books_and_Legal_Samples/Data_Warehouse_Toolkit_3e_Chapter_1_Excerpt.pdf)：36 页；SHA-256 `901c49befd3fe08d6a0b6feceb5c14d12828f53253324a6f6d7521d2100c1763`。

## 版权与使用边界

- W3C、OWASP、开放模式目录和官方设计系统按各自许可使用。
- ISO 页面快照或链接不等于购买了标准正文；实施需要完整条款时，应通过 ISO、国家标准机构或单位订阅取得授权版本。
- 出版社样章仅用于评估与学习，不得扩展抓取、拼接或传播受版权保护的整书。
- `INDEX_ONLY` 条目没有本地全文，不能在评审中表述为“已完整阅读”。
