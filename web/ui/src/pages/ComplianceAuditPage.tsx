import {
  AuditOutlined,
  CheckCircleOutlined,
  FileDoneOutlined,
  FilePdfOutlined,
  FileSearchOutlined,
  FileWordOutlined,
  FolderOpenOutlined,
  InfoCircleOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  ToolOutlined,
  WarningOutlined,
} from "@ant-design/icons";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Alert,
  Button,
  Descriptions,
  Drawer,
  Empty,
  Modal,
  Select,
  Space,
  Table,
  Tooltip,
} from "antd";
import type { ColumnsType } from "antd/es/table";
import ReactECharts from "echarts-for-react";
import { useMemo, useState } from "react";
import { MetricTile } from "@/components/MetricTile";
import { StatusTag } from "@/components/StatusTag";
import { WorkPanel } from "@/components/WorkPanel";
import type { NavRoute } from "@/routes/routeManifest";
import { hasRequiredScope } from "@/routes/access";
import { localBypassUser, type CurrentUser } from "@/services/api";
import {
  createComplianceRemediations,
  downloadComplianceArtifact,
  exportComplianceEvidencePackage,
  exportComplianceReport,
  fetchComplianceAuditTrail,
  fetchComplianceReports,
  finalizeComplianceReport,
  generateComplianceReport,
  type ComplianceReport,
  type ComplianceSection,
} from "@/services/complianceGovernanceApi";
import type { PageSnapshot } from "@/services/mockData";

type ComplianceDialog =
  | "generate"
  | "evidence"
  | "report-export"
  | "remediation"
  | "finalize"
  | undefined;

const indicatorDefinitions = [
  [
    "采集覆盖 >= 95%",
    "采集探针覆盖率",
    "采集监测 / 探针管理",
    "探针元数据",
    "采集感知",
  ],
  [
    "数据质量 >= 90%",
    "数据完整性、去重率",
    "采集监测 / 数据质量",
    "Kafka / Flink / ClickHouse",
    "采集监测",
  ],
  [
    "告警链路 <= 5 分钟",
    "告警闭环、关键证据率",
    "威胁分析 / 告警中心",
    "告警 / 关联引擎",
    "威胁分析",
  ],
  [
    "PCAP 证据覆盖 >= 90%",
    "PCAP hash 命中率",
    "威胁分析 / PCAP 检索",
    "PCAP 存储",
    "威胁分析",
  ],
  [
    "模型效果 F1 >= 0.80",
    "模型效果评估",
    "检测运营 / 模型管理",
    "模型服务 / 反馈池",
    "检测运营",
  ],
  [
    "审计链路完整",
    "操作留痕完整性",
    "审计配置 / 审计日志",
    "审计日志 / 操作日志",
    "审计配置",
  ],
  [
    "部署基线一致",
    "部署 manifest 与运行镜像一致",
    "检测运营 / 部署管理",
    "Kubernetes / containerd",
    "检测运营",
  ],
];

export function ComplianceAuditPage({ route }: { route: NavRoute }) {
  const queryClient = useQueryClient();
  const [dialog, setDialog] = useState<ComplianceDialog>();
  const [reportType, setReportType] = useState<"weekly" | "monthly">("weekly");
  const [exportFormat, setExportFormat] = useState<"pdf" | "docx">("pdf");
  const [selectedSection, setSelectedSection] = useState<ComplianceSection>();
  const [actionResult, setActionResult] = useState("");
  const currentUser =
    queryClient.getQueryData<CurrentUser>(["current-user"]) ?? localBypassUser;
  const canReadAudit = hasRequiredScope(currentUser, ["audit:read"]);
  const canGenerate = hasRequiredScope(currentUser, ["compliance:write"]);
  const canExport = hasRequiredScope(currentUser, ["compliance:export"]);
  const canRemediate = hasRequiredScope(currentUser, ["compliance:remediate"]);
  const canFinalize = hasRequiredScope(currentUser, ["compliance:finalize"]);
  const reportsQuery = useQuery({
    queryKey: ["compliance-reports"],
    queryFn: fetchComplianceReports,
  });
  const auditQuery = useQuery({
    queryKey: ["compliance-audit-trail"],
    queryFn: fetchComplianceAuditTrail,
    enabled: canReadAudit,
  });
  const reports = reportsQuery.data?.reports ?? [];
  const latest = reports[0];

  const generateMutation = useMutation({
    mutationFn: () => generateComplianceReport({ reportType }),
    onSuccess: async (report) => {
      setActionResult(
        report.status === "completed"
          ? `报告 ${report.report_id} 已与审计记录原子提交。`
          : `报告 ${report.report_id} 已按服务端真实结论保存为“${reportStatusLabel(report.status)}”，未误判为通过。`,
      );
      setDialog(undefined);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["compliance-reports"] }),
        queryClient.invalidateQueries({ queryKey: ["compliance-audit-trail"] }),
      ]);
    },
  });

  const exportMutation = useMutation({
    mutationFn: () => {
      if (!latest) throw new Error("请先生成一份真实合规报告");
      return exportComplianceEvidencePackage(latest.report_id);
    },
    onSuccess: async (bundle) => {
      downloadComplianceArtifact(bundle);
      setActionResult(
        `证据包 ${bundle.export_id} 已由服务端生成、审计并下载；${bundle.sha256}`,
      );
      setDialog(undefined);
      await queryClient.invalidateQueries({
        queryKey: ["compliance-audit-trail"],
      });
    },
  });

  const reportExportMutation = useMutation({
    mutationFn: () => {
      if (!latest) throw new Error("请先生成一份真实合规报告");
      return exportComplianceReport(latest.report_id, exportFormat);
    },
    onSuccess: async (artifact) => {
      downloadComplianceArtifact(artifact);
      setActionResult(
        `${exportFormat.toUpperCase()} 运行报告 ${artifact.export_id} 已由服务端渲染、审计并下载；${artifact.sha256}`,
      );
      setDialog(undefined);
      await queryClient.invalidateQueries({
        queryKey: ["compliance-audit-trail"],
      });
    },
  });

  const remediationMutation = useMutation({
    mutationFn: () => {
      if (!latest) throw new Error("请先生成一份真实合规报告");
      return createComplianceRemediations(latest.report_id);
    },
    onSuccess: async (result) => {
      setActionResult(
        `报告 ${result.report_id} 的整改任务已持久化：新建 ${result.created} 条，复用 ${result.reused} 条，共 ${result.total} 条。`,
      );
      setDialog(undefined);
      await queryClient.invalidateQueries({
        queryKey: ["compliance-audit-trail"],
      });
    },
  });

  const finalizeMutation = useMutation({
    mutationFn: () => {
      if (!latest) throw new Error("请先生成一份真实合规报告");
      return finalizeComplianceReport(latest.report_id);
    },
    onSuccess: async (result) => {
      setActionResult(
        `验收记录 ${result.finalization_id} 已固化；${result.report_sha256}`,
      );
      setDialog(undefined);
      await queryClient.invalidateQueries({
        queryKey: ["compliance-audit-trail"],
      });
    },
  });

  const metrics = useMemo(
    () =>
      buildComplianceMetrics(
        route.page.kpis,
        latest,
        reports.length,
        auditQuery.data?.total ?? 0,
      ),
    [route.page.kpis, latest, reports.length, auditQuery.data?.total],
  );
  const gateRows = useMemo(() => buildGateRows(latest), [latest]);
  const isLoading =
    reportsQuery.isLoading || (canReadAudit && auditQuery.isLoading);
  const loadError =
    reportsQuery.error ?? (canReadAudit ? auditQuery.error : undefined);

  const columns: ColumnsType<ReturnType<typeof buildGateRows>[number]> = [
    {
      title: "维度",
      dataIndex: "dimension",
      width: 104,
      render: (value) => (
        <span className="taf-compliance-dimension">
          <CheckCircleOutlined />
          {value}
        </span>
      ),
    },
    { title: "任务书指标（覆盖率）", dataIndex: "indicator", width: 136 },
    { title: "测试项（通过/总数）", dataIndex: "tests", width: 112 },
    { title: "数据源（覆盖率）", dataIndex: "source", width: 124 },
    { title: "证据状态（完整度）", dataIndex: "evidence", width: 122 },
    { title: "最近复验", dataIndex: "reviewedAt", width: 92 },
    {
      title: "结果",
      dataIndex: "result",
      width: 76,
      render: (value) => <StatusTag value={value} />,
    },
  ];

  return (
    <div className="taf-page taf-compliance">
      <section className="taf-compliance-shell">
        <main className="taf-compliance-main">
          <header className="taf-compliance-titlebar">
            <div>
              <small>审计配置 / 合规审计</small>
              <h1>
                {route.page.title} <InfoCircleOutlined />
              </h1>
            </div>
            <Space size={8} wrap>
              <Button
                size="small"
                type="primary"
                icon={<FileDoneOutlined />}
                disabled={!canGenerate}
                onClick={() => setDialog("generate")}
              >
                生成验收报告
              </Button>
              <Button
                size="small"
                icon={<FolderOpenOutlined />}
                disabled={!latest || !canExport}
                onClick={() => setDialog("evidence")}
              >
                导出证据包
              </Button>
              <Tooltip
                title={
                  latest && canExport
                    ? "由服务端生成并审计 PDF"
                    : "需要报告与 compliance:export"
                }
              >
                <Button
                  size="small"
                  icon={<FilePdfOutlined />}
                  disabled={!latest || !canExport}
                  onClick={() => {
                    setExportFormat("pdf");
                    setDialog("report-export");
                  }}
                >
                  导出 PDF
                </Button>
              </Tooltip>
              <Tooltip
                title={
                  latest && canExport
                    ? "由服务端生成并审计 Word"
                    : "需要报告与 compliance:export"
                }
              >
                <Button
                  size="small"
                  icon={<FileWordOutlined />}
                  disabled={!latest || !canExport}
                  onClick={() => {
                    setExportFormat("docx");
                    setDialog("report-export");
                  }}
                >
                  导出 Word
                </Button>
              </Tooltip>
              <Tooltip
                title={
                  latest && canRemediate
                    ? "为未通过门禁持久化整改任务"
                    : "需要报告与 compliance:remediate"
                }
              >
                <Button
                  size="small"
                  icon={<ToolOutlined />}
                  disabled={!latest || !canRemediate}
                  onClick={() => setDialog("remediation")}
                >
                  创建整改任务
                </Button>
              </Tooltip>
              <Tooltip
                title={
                  latest && canFinalize
                    ? "创建不可变报告快照；重复固化将冲突关闭"
                    : "需要报告与 compliance:finalize"
                }
              >
                <Button
                  size="small"
                  icon={<SafetyCertificateOutlined />}
                  disabled={!latest || !canFinalize}
                  onClick={() => setDialog("finalize")}
                >
                  固化验收记录
                </Button>
              </Tooltip>
              <Button
                size="small"
                icon={<ReloadOutlined />}
                loading={isLoading}
                onClick={() =>
                  void Promise.all([
                    reportsQuery.refetch(),
                    auditQuery.refetch(),
                  ])
                }
                aria-label="刷新合规数据"
              />
            </Space>
          </header>

          {loadError && (
            <Alert
              type="error"
              showIcon
              message="真实合规 API 数据加载失败"
              description={
                loadError instanceof Error
                  ? loadError.message
                  : "请检查合规报告、审计轨迹、权限和 APISIX 路由。"
              }
            />
          )}
          {actionResult && (
            <Alert
              className="taf-compliance-action-result"
              type="success"
              showIcon
              closable
              onClose={() => setActionResult("")}
              message={actionResult}
            />
          )}
          {!canReadAudit && (
            <Alert
              className="taf-compliance-audit-scope"
              type="info"
              showIcon
              message="当前账号可读取合规报告，但无 audit:read；审计轨迹区域按权限关闭。"
            />
          )}

          <div className="taf-compliance-kpis">
            {metrics.map((metric) => (
              <MetricTile key={metric.label} metric={metric} />
            ))}
          </div>

          <div className="taf-compliance-workbench">
            <section className="taf-compliance-left">
              <WorkPanel title="A. 验收门禁矩阵" extra={<AuditOutlined />}>
                <Table
                  rowKey="key"
                  size="small"
                  loading={isLoading}
                  pagination={false}
                  columns={columns}
                  dataSource={gateRows}
                  scroll={{ x: 766 }}
                  onRow={(row) => ({
                    onClick: () => setSelectedSection(row.section),
                  })}
                />
              </WorkPanel>
              <WorkPanel title="B. 指标映射追踪表">
                <IndicatorTrace latest={latest} />
              </WorkPanel>
            </section>

            <section className="taf-compliance-righttop">
              <WorkPanel title="C. 证据包完整度" extra={<FileSearchOutlined />}>
                <EvidencePackage
                  latest={latest}
                  auditCount={auditQuery.data?.total ?? 0}
                />
              </WorkPanel>
              <WorkPanel
                title={`D. 运行报告预览${latest ? `（${formatDate(latest.time_range.start)} ~ ${formatDate(latest.time_range.end)}）` : ""}`}
              >
                <ReportPreview reports={reports} />
              </WorkPanel>
            </section>

            <WorkPanel
              title="E. 缺口治理看板"
              className="taf-compliance-gap-panel"
            >
              <GapBoard
                latest={latest}
                onRemediate={
                  canRemediate ? () => setDialog("remediation") : undefined
                }
              />
            </WorkPanel>
            <WorkPanel
              title="F. 第三方评测批次"
              className="taf-compliance-batch-panel"
            >
              <ThirdPartyBatches reports={reports} />
            </WorkPanel>
          </div>
        </main>
      </section>

      <Modal
        title="生成合规验收报告"
        open={dialog === "generate"}
        onCancel={() => setDialog(undefined)}
        onOk={() => generateMutation.mutate()}
        confirmLoading={generateMutation.isPending}
        okText="生成并写入审计"
      >
        <Alert
          type="info"
          showIcon
          message="数据源失败将直接失败关闭；无有效样本会保存为“证据不足”，不会生成假通过。"
        />
        <Descriptions
          bordered
          size="small"
          column={1}
          className="taf-compliance-dialog-fields"
        >
          <Descriptions.Item label="报告类型">
            <Select
              value={reportType}
              onChange={setReportType}
              options={[
                { value: "weekly", label: "周报（最近 7 天）" },
                { value: "monthly", label: "月报（最近 30 天）" },
              ]}
            />
          </Descriptions.Item>
          <Descriptions.Item label="权限">compliance:write</Descriptions.Item>
          <Descriptions.Item label="原子性">
            报告与 COMPLIANCE_REPORT_GENERATED 审计同事务提交
          </Descriptions.Item>
        </Descriptions>
        {generateMutation.isError && (
          <Alert
            type="error"
            showIcon
            message={
              generateMutation.error instanceof Error
                ? generateMutation.error.message
                : "生成失败"
            }
          />
        )}
      </Modal>

      <Modal
        className="taf-compliance-workflow-modal"
        width="min(920px, calc(100vw - 48px))"
        title="导出合规证据包"
        open={dialog === "evidence"}
        onCancel={() => setDialog(undefined)}
        onOk={() => exportMutation.mutate()}
        confirmLoading={exportMutation.isPending}
        okText="生成 ZIP 并下载"
      >
        <EvidenceExportWorkbench
          latest={latest}
          auditCount={auditQuery.data?.total ?? 0}
        />
        {exportMutation.isError && (
          <Alert
            type="error"
            showIcon
            message={
              exportMutation.error instanceof Error
                ? exportMutation.error.message
                : "导出失败"
            }
          />
        )}
      </Modal>

      <Modal
        className="taf-compliance-workflow-modal"
        width="min(880px, calc(100vw - 48px))"
        title="导出运行报告"
        open={dialog === "report-export"}
        onCancel={() => setDialog(undefined)}
        onOk={() => reportExportMutation.mutate()}
        confirmLoading={reportExportMutation.isPending}
        okText={`导出 ${exportFormat === "pdf" ? "PDF" : "Word"} 运行报告`}
      >
        <ReportExportWorkbench
          latest={latest}
          format={exportFormat}
          onFormatChange={setExportFormat}
        />
        {reportExportMutation.isError && (
          <Alert
            type="error"
            showIcon
            message={
              reportExportMutation.error instanceof Error
                ? reportExportMutation.error.message
                : "报告导出失败"
            }
          />
        )}
      </Modal>

      <Modal
        title="创建整改任务"
        open={dialog === "remediation"}
        onCancel={() => setDialog(undefined)}
        onOk={() => remediationMutation.mutate()}
        confirmLoading={remediationMutation.isPending}
        okText="持久化整改任务"
      >
        <Alert
          type="warning"
          showIcon
          message="仅为当前报告中未通过或证据不足的真实 section 建立任务；重复执行会复用同一门禁任务。"
        />
        <GapBoard latest={latest} />
        {remediationMutation.isError && (
          <Alert
            type="error"
            showIcon
            message={
              remediationMutation.error instanceof Error
                ? remediationMutation.error.message
                : "整改任务创建失败"
            }
          />
        )}
      </Modal>

      <Modal
        title="固化验收记录"
        open={dialog === "finalize"}
        onCancel={() => setDialog(undefined)}
        onOk={() => finalizeMutation.mutate()}
        confirmLoading={finalizeMutation.isPending}
        okText="创建不可变快照"
      >
        <Alert
          type="info"
          showIcon
          message="服务端将保存报告完整快照与 SHA-256；同一报告只允许固化一次，重复请求返回 409。"
        />
        <Descriptions
          bordered
          size="small"
          column={1}
          className="taf-compliance-dialog-fields"
        >
          <Descriptions.Item label="报告 ID">
            {latest?.report_id ?? "未选择"}
          </Descriptions.Item>
          <Descriptions.Item label="报告状态">
            {latest?.status ?? "未生成"}
          </Descriptions.Item>
          <Descriptions.Item label="权限">
            compliance:finalize
          </Descriptions.Item>
        </Descriptions>
        {finalizeMutation.isError && (
          <Alert
            type="error"
            showIcon
            message={
              finalizeMutation.error instanceof Error
                ? finalizeMutation.error.message
                : "固化失败"
            }
          />
        )}
      </Modal>

      <Drawer
        className="taf-compliance-gate-drawer"
        title="合规门禁详情"
        open={Boolean(selectedSection)}
        onClose={() => setSelectedSection(undefined)}
        width="min(720px, calc(100vw - 48px))"
        footer={
          <Space>
            <Button onClick={() => setSelectedSection(undefined)}>返回</Button>
            <Button
              icon={<ToolOutlined />}
              disabled={!canRemediate}
              onClick={() => setDialog("remediation")}
            >
              创建整改任务
            </Button>
            <Button
              type="primary"
              icon={<ReloadOutlined />}
              disabled={!canGenerate}
              onClick={() => setDialog("generate")}
            >
              生成复验报告
            </Button>
          </Space>
        }
      >
        {selectedSection && (
          <GateDetailWorkbench section={selectedSection} report={latest} />
        )}
      </Drawer>
    </div>
  );
}

function EvidenceExportWorkbench({
  latest,
  auditCount,
}: {
  latest?: ComplianceReport;
  auditCount: number;
}) {
  const [preflightResult, setPreflightResult] = useState("");
  const sections = latest?.sections ?? [];
  const passed = sections.filter((section) => section.status === "pass").length;
  const gaps = sections.length - passed;
  const completeness = sections.length
    ? Math.round((passed / sections.length) * 100)
    : 0;
  return (
    <div className="taf-compliance-workflow">
      <div className="taf-compliance-workflow-summary">
        {[
          ["门禁项", sections.length, "真实 report sections"],
          ["通过", passed, "服务端判定"],
          ["待补证据", gaps, gaps ? "导出后仍需整改" : "无缺口"],
          ["完整度", `${completeness}%`, `${passed}/${sections.length || 0}`],
          [
            "审批状态",
            latest?.status === "completed" ? "可导出" : "证据不足",
            "导出不改变报告结论",
          ],
        ].map(([label, value, note]) => (
          <span key={label}>
            <em>{label}</em>
            <b>{value}</b>
            <small>{note}</small>
          </span>
        ))}
      </div>
      <div className="taf-compliance-workflow-grid taf-compliance-evidence-export-grid">
        <section>
          <h3>
            <SafetyCertificateOutlined /> 证据范围
          </h3>
          <label>
            <input type="checkbox" checked readOnly /> 全部真实报告 section（
            {sections.length}/{sections.length}）
          </label>
          {sections.map((section) => (
            <label key={section.section_name}>
              <input type="checkbox" checked readOnly />
              <span>{section.title}</span>
              <small>{sectionLabel(section.status)}</small>
            </label>
          ))}
          {!sections.length && (
            <Empty
              image={Empty.PRESENTED_IMAGE_SIMPLE}
              description="尚无报告 section"
            />
          )}
          <footer>仅导出当前租户、当前报告中的服务端数据。</footer>
        </section>
        <section>
          <h3>
            <FileSearchOutlined /> 证据清单（
            {sections.length + (latest ? 2 : 0)} 项）
          </h3>
          <div className="taf-compliance-workflow-table">
            <div>
              <span>证据项</span>
              <span>证据类型</span>
              <span>来源/标识</span>
              <span>状态</span>
            </div>
            {latest && (
              <button type="button">
                <span>合规报告</span>
                <span>report.json</span>
                <span>{latest.report_id}</span>
                <span className="is-ok">已就绪</span>
              </button>
            )}
            {latest && (
              <button type="button">
                <span>证据清单</span>
                <span>manifest.json</span>
                <span>服务端 SHA-256</span>
                <span className="is-ok">已就绪</span>
              </button>
            )}
            {sections.map((section) => (
              <button type="button" key={section.section_name}>
                <span>{section.title}</span>
                <span>report section</span>
                <span>{section.section_name}</span>
                <span className={statusClass(sectionLabel(section.status))}>
                  {sectionLabel(section.status)}
                </span>
              </button>
            ))}
          </div>
        </section>
        <section>
          <h3>
            <SafetyCertificateOutlined /> 导出安全
          </h3>
          <Descriptions bordered size="small" column={1}>
            <Descriptions.Item label="导出格式">
              ZIP（证据包）
            </Descriptions.Item>
            <Descriptions.Item label="租户隔离">
              仅当前 JWT tenant
            </Descriptions.Item>
            <Descriptions.Item label="下载权限">
              compliance:export
            </Descriptions.Item>
            <Descriptions.Item label="审计记录">
              当前 {auditCount} 条；成功后追加 COMPLIANCE_EVIDENCE_EXPORTED
            </Descriptions.Item>
            <Descriptions.Item label="完整性">
              服务端返回 SHA-256
            </Descriptions.Item>
            <Descriptions.Item label="报告 ID">
              {latest?.report_id ?? "未选择"}
            </Descriptions.Item>
          </Descriptions>
          <Button
            size="small"
            icon={<CheckCircleOutlined />}
            disabled={!latest}
            onClick={() =>
              setPreflightResult(
                latest
                  ? `已预检：报告 ${latest.report_id}、${sections.length} 个门禁、${gaps} 个缺口；导出将保留真实结论。`
                  : "",
              )
            }
          >
            预检证据
          </Button>
          {preflightResult && (
            <Alert type="info" showIcon message={preflightResult} />
          )}
          <Alert
            type={gaps ? "warning" : "success"}
            showIcon
            message={
              gaps
                ? `当前有 ${gaps} 项未通过；证据包会保留真实状态。`
                : "当前 section 均通过，可生成证据包。"
            }
          />
        </section>
      </div>
    </div>
  );
}

function ReportExportWorkbench({
  latest,
  format,
  onFormatChange,
}: {
  latest?: ComplianceReport;
  format: "pdf" | "docx";
  onFormatChange: (value: "pdf" | "docx") => void;
}) {
  const [previewed, setPreviewed] = useState(false);
  const sections = latest?.sections ?? [];
  const gaps = sections.filter((section) => section.status !== "pass");
  return (
    <div className="taf-compliance-workflow">
      <div className="taf-compliance-workflow-summary">
        {[
          [
            "门禁通过率",
            sections.length
              ? `${Math.round(((sections.length - gaps.length) / sections.length) * 100)}%`
              : "0%",
            `${sections.length - gaps.length}/${sections.length}`,
          ],
          ["整改中", gaps.length, "未通过/证据不足"],
          ["风险说明", latest?.summary.sla_violations ?? 0, "SLA 违规"],
          ["审批链", "服务端审计", "导出事件可追溯"],
        ].map(([label, value, note]) => (
          <span key={label}>
            <em>{label}</em>
            <b>{value}</b>
            <small>{note}</small>
          </span>
        ))}
      </div>
      <div className="taf-compliance-workflow-grid taf-compliance-report-export-grid">
        <section>
          <h3>
            报告内容{" "}
            <small>
              {sections.length} 个门禁 + 2 个附录（{sections.length + 2}/
              {sections.length + 2}）
            </small>
          </h3>
          {[
            "运行摘要",
            ...sections.map((section) => section.title),
            "审计留痕",
          ].map((title) => (
            <label key={title}>
              <input type="checkbox" checked readOnly />
              {title}
            </label>
          ))}
          <div className="taf-compliance-report-sheet">
            <FileDoneOutlined />
            <b>合规运行报告</b>
            <small>{latest?.report_id ?? "尚无报告"}</small>
          </div>
        </section>
        <section>
          <h3>数据范围</h3>
          <Descriptions bordered size="small" column={1}>
            <Descriptions.Item label="时间范围">
              {latest
                ? `${formatTime(latest.time_range.start)} ~ ${formatTime(latest.time_range.end)}`
                : "未选择"}
            </Descriptions.Item>
            <Descriptions.Item label="站点范围">当前租户聚合</Descriptions.Item>
            <Descriptions.Item label="租户范围">
              {latest?.tenant_id ?? "未选择"}
            </Descriptions.Item>
            <Descriptions.Item label="输出格式">
              <Select
                value={format}
                onChange={onFormatChange}
                options={[
                  { value: "pdf", label: "PDF" },
                  { value: "docx", label: "Word (.docx)" },
                ]}
              />
            </Descriptions.Item>
            <Descriptions.Item label="报告语言">
              {format === "pdf"
                ? "英文标签 / 系统字段原文"
                : "简体中文 / 系统字段原文"}
            </Descriptions.Item>
            <Descriptions.Item label="原始统计">
              包含真实 summary 与 sections
            </Descriptions.Item>
          </Descriptions>
        </section>
        <section>
          <h3>导出预检</h3>
          <div className="taf-compliance-preflight">
            <span className={gaps.length ? "is-risk" : "is-ok"}>
              <WarningOutlined /> 缺失证据 <b>{gaps.length} 项</b>
              <small>报告不会隐藏未通过结论</small>
            </span>
            <span className="is-ok">
              <CheckCircleOutlined /> 文件生成 <b>服务端</b>
              <small>返回内容、MIME 与 SHA-256</small>
            </span>
            <span className="is-ok">
              <CheckCircleOutlined /> 报告水印 <b>报告 ID</b>
              <small>{latest?.report_id ?? "未选择"}</small>
            </span>
            <span className="is-ok">
              <CheckCircleOutlined /> 签名状态 <b>审计留痕</b>
              <small>COMPLIANCE_REPORT_EXPORTED</small>
            </span>
          </div>
          <Button
            size="small"
            icon={<FileSearchOutlined />}
            disabled={!latest}
            onClick={() => setPreviewed((value) => !value)}
          >
            {previewed ? "收起预览" : "预览报告"}
          </Button>
          {previewed && (
            <div className="taf-compliance-inline-preview">
              <b>{latest?.report_id}</b>
              <span>状态：{reportStatusLabel(latest?.status ?? "")}</span>
              <span>
                摘要：告警 {latest?.summary.total_alerts ?? 0}，已闭环{" "}
                {latest?.summary.resolved_alerts ?? 0}
              </span>
              <span>
                门禁：
                {sections
                  .map(
                    (section) =>
                      `${section.title}=${sectionLabel(section.status)}`,
                  )
                  .join("；")}
              </span>
            </div>
          )}
        </section>
      </div>
    </div>
  );
}

function GateDetailWorkbench({
  section,
  report,
}: {
  section: ComplianceSection;
  report?: ComplianceReport;
}) {
  const entries = Object.entries(section.content);
  return (
    <div className="taf-compliance-gate-detail">
      <div className="taf-compliance-workflow-summary">
        {[
          ["门禁状态", sectionLabel(section.status), section.status],
          ["影响范围", `${entries.length} 个指标`, section.section_name],
          [
            "证据缺口",
            section.status === "pass" ? 0 : entries.length,
            "真实 section 字段",
          ],
          ["复验次数", 0, "后端未提供复验历史"],
          ["审计编号", report?.report_id ?? "未生成", "报告 ID"],
        ].map(([label, value, note]) => (
          <span key={label}>
            <em>{label}</em>
            <b>{value}</b>
            <small>{note}</small>
          </span>
        ))}
      </div>
      <div className="taf-compliance-gate-grid">
        <section>
          <h3>门禁规则</h3>
          <Descriptions bordered size="small" column={1}>
            <Descriptions.Item label="规则名称">
              {section.title}
            </Descriptions.Item>
            <Descriptions.Item label="规则编号">
              {section.section_name}
            </Descriptions.Item>
            <Descriptions.Item label="规则层级">
              服务端合规 section
            </Descriptions.Item>
            <Descriptions.Item label="当前结果">
              <StatusTag value={sectionLabel(section.status)} />
            </Descriptions.Item>
            <Descriptions.Item label="报告状态">
              {report?.status ?? "未生成"}
            </Descriptions.Item>
            <Descriptions.Item label="责任团队">
              {section.section_name}
            </Descriptions.Item>
          </Descriptions>
        </section>
        <section>
          <h3>检查项矩阵（共 {entries.length} 项）</h3>
          <div className="taf-compliance-workflow-table">
            <div>
              <span>检查项</span>
              <span>实际值</span>
              <span>证据来源</span>
              <span>状态</span>
            </div>
            {entries.map(([key, value]) => (
              <button type="button" key={key}>
                <span>{key}</span>
                <span>{String(value)}</span>
                <span>{section.section_name}</span>
                <span className={statusClass(sectionLabel(section.status))}>
                  {sectionLabel(section.status)}
                </span>
              </button>
            ))}
          </div>
        </section>
        <section>
          <h3>整改动作</h3>
          <Alert
            type={section.status === "pass" ? "success" : "warning"}
            showIcon
            message={
              section.status === "pass"
                ? "该门禁当前通过，无需创建整改任务。"
                : "可从页面顶栏创建持久化整改任务并写入审计。"
            }
          />
          <Descriptions bordered size="small" column={1}>
            <Descriptions.Item label="报告 ID">
              {report?.report_id ?? "未生成"}
            </Descriptions.Item>
            <Descriptions.Item label="生成时间">
              {report ? formatTime(report.generated_at) : "-"}
            </Descriptions.Item>
            <Descriptions.Item label="复验历史">
              后端未提供，不补造
            </Descriptions.Item>
            <Descriptions.Item label="证据路径">
              包含于 report.json / sections
            </Descriptions.Item>
          </Descriptions>
        </section>
      </div>
    </div>
  );
}

function buildComplianceMetrics(
  labels: string[],
  latest: ComplianceReport | undefined,
  reportCount: number,
  auditCount: number,
): PageSnapshot["metrics"] {
  const sections = latest?.sections ?? [];
  const passed = sections.filter((item) => item.status === "pass").length;
  const gaps = sections.filter((item) => item.status !== "pass").length;
  const evidenceReady = sections.filter(
    (item) =>
      item.status === "pass" ||
      item.status === "warning" ||
      item.status === "warn" ||
      item.status === "fail",
  ).length;
  const total = sections.length;
  const gateRate = total ? (passed / total) * 100 : 0;
  const resolved = latest?.summary.total_alerts
    ? (latest.summary.resolved_alerts / latest.summary.total_alerts) * 100
    : 0;
  const values: Record<string, PageSnapshot["metrics"][number]> = {
    门禁通过率: {
      label: "门禁通过率",
      value: `${gateRate.toFixed(1)}%`,
      delta: total ? `${passed}/${total} 门禁` : "未提供证据",
      status: gateRate >= 80 ? "ok" : gateRate > 0 ? "warn" : "risk",
    },
    未达标项: {
      label: "未达标项",
      value: `${gaps} 项`,
      delta: latest ? reportStatusLabel(latest.status) : "尚无报告",
      status: gaps ? "risk" : "ok",
    },
    证据完整度: {
      label: "证据完整度",
      value: total ? `${((evidenceReady / total) * 100).toFixed(1)}%` : "0.0%",
      delta: `${evidenceReady}/${total} 有可判定证据`,
      status:
        evidenceReady === total && total > 0
          ? "ok"
          : evidenceReady
            ? "warn"
            : "risk",
    },
    复验通过率: {
      label: "复验通过率",
      value: `${resolved.toFixed(1)}%`,
      delta: latest?.summary.total_alerts ? "告警闭环" : "无有效样本",
      status: resolved >= 80 ? "ok" : "warn",
    },
    第三方批次: {
      label: "第三方批次",
      value: "0 批次",
      delta: "后端未提供第三方证据",
      status: "warn",
    },
    报告生成数: {
      label: "报告生成数",
      value: `${reportCount} 份`,
      delta: `${auditCount} 审计`,
      status: reportCount ? "info" : "warn",
    },
  };
  return labels.map(
    (label) =>
      values[label] ?? {
        label,
        value: "未提供",
        delta: "真实 API 无字段",
        status: "warn",
      },
  );
}

function buildGateRows(latest?: ComplianceReport) {
  const sections = latest?.sections ?? [];
  const definitions: Record<string, { indicator: string; source: string }> = {
    collection_coverage: {
      indicator: "采集覆盖 >= 95%",
      source: "探针元数据 / 心跳",
    },
    data_quality: {
      indicator: "数据质量 >= 90%",
      source: "Kafka / Flink / ClickHouse",
    },
    alert_response: { indicator: "响应闭环 >= 80%", source: "traffic.alerts" },
    pcap_evidence: {
      indicator: "PCAP 覆盖 >= 90%",
      source: "pcap_index / evidence",
    },
    model_quality: { indicator: "模型 F1 >= 0.80", source: "模型评估注册表" },
    audit_integrity: { indicator: "审计链路完整", source: "audit_logs" },
    deployment_baseline: {
      indicator: "部署基线一致",
      source: "Kubernetes manifest",
    },
    critical_alerts: { indicator: "SLA 违规 <= 3", source: "traffic.alerts" },
    feedback_quality: { indicator: "反馈证据已留痕", source: "traffic.alerts" },
  };
  return sections.map((section, index) => ({
    key: section.section_name || String(index),
    dimension: section.title,
    indicator: definitions[section.section_name]?.indicator ?? "服务端合规门禁",
    tests: evidenceCount(section),
    source:
      definitions[section.section_name]?.source ??
      String(section.content.source ?? "未提供"),
    evidence:
      String(section.content.evidence_status ?? "") === "insufficient"
        ? "证据不足"
        : "服务端聚合",
    reviewedAt: latest ? formatDate(latest.generated_at) : "未生成",
    result: sectionLabel(section.status),
    section,
  }));
}

function IndicatorTrace({ latest }: { latest?: ComplianceReport }) {
  const statusByName = new Map(
    latest?.sections.map((section) => [
      section.section_name,
      sectionLabel(section.status),
    ]) ?? [],
  );
  const sectionOrder = [
    "collection_coverage",
    "data_quality",
    "alert_response",
    "pcap_evidence",
    "model_quality",
    "audit_integrity",
    "deployment_baseline",
  ];
  return (
    <div className="taf-compliance-indicators">
      <div>
        <span>任务书指标</span>
        <span>测试项</span>
        <span>对应页面</span>
        <span>对应数据源</span>
        <span>责任模块</span>
        <span>状态</span>
      </div>
      {indicatorDefinitions.map((row, index) => (
        <button key={row[0]} type="button">
          {row.map((cell) => (
            <span key={cell}>{cell}</span>
          ))}
          <span
            className={statusClass(
              statusByName.get(sectionOrder[index]) ?? "证据不足",
            )}
          >
            {statusByName.get(sectionOrder[index]) ?? "证据不足"}
          </span>
        </button>
      ))}
    </div>
  );
}

function EvidencePackage({
  latest,
  auditCount,
}: {
  latest?: ComplianceReport;
  auditCount: number;
}) {
  const rows = latest
    ? [
        [
          "合规报告",
          latest.report_id,
          latest.status === "completed"
            ? "通过"
            : reportStatusLabel(latest.status),
          formatTime(latest.generated_at),
        ],
        [
          "审计日志",
          `${auditCount} 条`,
          auditCount ? "通过" : "证据不足",
          formatTime(latest.generated_at),
        ],
        ["PCAP hash", "后端未提供", "证据不足", "-"],
        ["模型版本", "后端未提供", "证据不足", "-"],
        ["规则版本", "后端未提供", "证据不足", "-"],
        ["部署 manifest", "后端未提供", "证据不足", "-"],
      ]
    : [];
  if (!rows.length)
    return (
      <Empty
        image={Empty.PRESENTED_IMAGE_SIMPLE}
        description="尚无真实合规报告"
      />
    );
  return (
    <div className="taf-compliance-evidence">
      <div>
        <span>证据项</span>
        <span>编号 / hash</span>
        <span>校验状态</span>
        <span>最后更新时间</span>
      </div>
      {rows.map((row) => (
        <button key={row[0]} type="button">
          <span>{row[0]}</span>
          <span title={row[1]}>{row[1]}</span>
          <span className={statusClass(row[2])}>{row[2]}</span>
          <span>{row[3]}</span>
        </button>
      ))}
    </div>
  );
}

function ReportPreview({ reports }: { reports: ComplianceReport[] }) {
  if (!reports.length)
    return (
      <Empty
        image={Empty.PRESENTED_IMAGE_SIMPLE}
        description="生成报告后展示真实趋势"
      />
    );
  const latest = reports[0];
  const sorted = [...reports].reverse().slice(-12);
  const labels = sorted.map((item) => formatDate(item.generated_at));
  const resolvedRates = sorted.map((item) =>
    item.summary.total_alerts
      ? Number(
          (
            (item.summary.resolved_alerts / item.summary.total_alerts) *
            100
          ).toFixed(1),
        )
      : 0,
  );
  const sla = sorted.map((item) => item.summary.sla_violations);
  const falsePositive = sorted.map((item) => item.summary.false_positives);
  const critical = sorted.map((item) => item.summary.critical_alerts);
  return (
    <div className="taf-compliance-report">
      <div className="taf-compliance-report-metrics">
        {[
          ["总告警数", latest.summary.total_alerts, "真实 ClickHouse"],
          [
            "处置完成",
            latest.summary.resolved_alerts,
            `处置率 ${resolvedRates[resolvedRates.length - 1] ?? 0}%`,
          ],
          ["SLA 违规", latest.summary.sla_violations, "严重/高风险"],
          ["误报反馈", latest.summary.false_positives, "反馈样本"],
          [
            "平均响应",
            `${latest.summary.avg_response_time_min.toFixed(1)}m`,
            "已闭环告警",
          ],
        ].map(([label, value, note]) => (
          <span key={label}>
            <em>{label}</em>
            <b>{value}</b>
            <small>{note}</small>
          </span>
        ))}
      </div>
      <ReactECharts
        className="taf-compliance-trend-chart"
        option={{
          animation: false,
          tooltip: { trigger: "axis" },
          title: ["处置率%", "SLA 违规", "误报反馈", "严重告警"].map(
            (text, index) => ({
              text,
              left: `${index * 25 + 1}%`,
              top: 0,
              textStyle: { color: "#91b8ce", fontSize: 8, fontWeight: 500 },
            }),
          ),
          grid: [0, 1, 2, 3].map((index) => ({
            left: `${index * 25 + 1}%`,
            width: "22%",
            top: 15,
            bottom: 5,
            containLabel: false,
          })),
          xAxis: [0, 1, 2, 3].map((index) => ({
            type: "category",
            gridIndex: index,
            data: labels,
            boundaryGap: false,
            axisLabel: { show: false },
            axisTick: { show: false },
            axisLine: { lineStyle: { color: "#24495d" } },
          })),
          yAxis: [0, 1, 2, 3].map((index) => ({
            type: "value",
            gridIndex: index,
            scale: true,
            axisLabel: { show: false },
            splitLine: { lineStyle: { color: "rgba(56,151,201,.08)" } },
          })),
          series: [
            {
              name: "处置率%",
              type: "line",
              xAxisIndex: 0,
              yAxisIndex: 0,
              data: resolvedRates,
              smooth: true,
              symbol: "none",
            },
            {
              name: "SLA违规",
              type: "line",
              xAxisIndex: 1,
              yAxisIndex: 1,
              data: sla,
              smooth: true,
              symbol: "none",
            },
            {
              name: "误报反馈",
              type: "line",
              xAxisIndex: 2,
              yAxisIndex: 2,
              data: falsePositive,
              smooth: true,
              symbol: "none",
            },
            {
              name: "严重告警",
              type: "line",
              xAxisIndex: 3,
              yAxisIndex: 3,
              data: critical,
              smooth: true,
              symbol: "none",
            },
          ],
        }}
      />
      <footer>
        摘要：仅展示合规报告中的真实 ClickHouse
        聚合；未提供的数据不会由浏览器补造。
      </footer>
    </div>
  );
}

function GapBoard({
  latest,
  onRemediate,
}: {
  latest?: ComplianceReport;
  onRemediate?: () => void;
}) {
  const gaps =
    latest?.sections.filter((section) => section.status !== "pass") ?? [];
  if (!gaps.length)
    return (
      <Empty
        image={Empty.PRESENTED_IMAGE_SIMPLE}
        description={latest ? "当前报告无未通过 section" : "尚无报告"}
      />
    );
  return (
    <div className="taf-compliance-gap">
      <div>
        <span>未达标项</span>
        <span>原因</span>
        <span>责任模块</span>
        <span>计划完成</span>
        <span>复验状态</span>
        <span>操作</span>
      </div>
      {gaps.map((section) => (
        <button key={section.section_name} type="button" onClick={onRemediate}>
          <span className="is-risk">{sectionLabel(section.status)}</span>
          <span>{section.title}</span>
          <span>{section.section_name}</span>
          <span>待任务确认</span>
          <span>{sectionLabel(section.status)}</span>
          <em
            title={
              onRemediate ? "创建或复用持久化整改任务" : "请使用页面整改动作"
            }
          >
            {onRemediate ? "创建任务" : "可整改"}
          </em>
        </button>
      ))}
      <footer>
        <span>共 {gaps.length} 项</span>
        <span>证据不足/阻断 {gaps.length}</span>
      </footer>
    </div>
  );
}

function ThirdPartyBatches({ reports }: { reports: ComplianceReport[] }) {
  const batches = reportsOfType(reports, "third");
  if (!batches.length)
    return (
      <div className="taf-compliance-batches taf-compliance-batches-empty">
        <div>
          <span>批次号</span>
          <span>样本来源</span>
          <span>测试范围</span>
          <span>通过率</span>
          <span>复测记录</span>
          <span>趋势</span>
        </div>
        <button type="button">
          <span>未提供</span>
          <span>后端未接入</span>
          <span>第三方评测</span>
          <span className="is-risk">不可判定</span>
          <span>无真实记录</span>
          <span>无数据</span>
        </button>
        <footer>
          <span>0 批次</span>
          <span>禁止以前端样例替代第三方证据</span>
        </footer>
      </div>
    );
  return (
    <div className="taf-compliance-batches">
      <div>
        <span>批次号</span>
        <span>样本来源</span>
        <span>测试范围</span>
        <span>通过率</span>
        <span>复测记录</span>
        <span>趋势</span>
      </div>
      {batches.map((report) => (
        <button key={report.report_id} type="button">
          <span>{report.report_id}</span>
          <span>第三方评测</span>
          <span>{report.report_type}</span>
          <span>{report.status}</span>
          <span>{formatDate(report.generated_at)}</span>
          <span>真实 API</span>
        </button>
      ))}
    </div>
  );
}

const reportsOfType = (reports: ComplianceReport[], type: string) =>
  reports.filter((report) => report.report_type.toLowerCase().includes(type));
const sectionLabel = (status: string) =>
  status === "pass"
    ? "通过"
    : status === "warn" || status === "warning"
      ? "待整改"
      : status === "insufficient_evidence" || status === "blocked"
        ? "证据不足"
        : "未达标";
const reportStatusLabel = (status: string) =>
  status === "completed"
    ? "已通过"
    : status === "non_compliant"
      ? "未达标"
      : status === "invalidated"
        ? "已吊销"
        : "证据不足";
const statusClass = (value: string) =>
  value.includes("通过")
    ? "is-ok"
    : value.includes("待")
      ? "is-warn"
      : "is-risk";
const evidenceCount = (section: ComplianceSection) =>
  section.content.total_alerts !== undefined
    ? `${section.content.resolved_alerts ?? 0} / ${section.content.total_alerts}`
    : section.content.false_positives !== undefined
      ? `${section.content.false_positives} 条`
      : "未提供";
const formatDate = (epoch: number) =>
  epoch
    ? new Date(epoch).toLocaleDateString("zh-CN", {
        month: "2-digit",
        day: "2-digit",
      })
    : "-";
const formatTime = (epoch: number) =>
  epoch ? new Date(epoch).toLocaleString("zh-CN", { hour12: false }) : "-";
