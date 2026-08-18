// analysis-service 统一分析任务调度中心(核心主链权威)。
// 装配:PG→repo→services→HTTP;Scheduler.Tick 后台循环。
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/adapters"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/api"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/service"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/apitoken"
	authrepository "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/authz"
	commoncontracts "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/contracts"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	kafkaCommon "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/segmentio/kafka-go"
)

func main() {
	logger, _ := zap.NewProduction()
	defer func() { _ = logger.Sync() }()

	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("configuration failed", zap.Error(err))
	}

	db, err := sql.Open("postgres", cfg.PostgresDSN)
	if err != nil {
		logger.Fatal("open postgres", zap.Error(err))
	}
	db.SetMaxOpenConns(10)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		logger.Fatal("ping postgres", zap.Error(err))
	}
	logger.Info("connected to postgres (analysis authority)")

	repo := repository.NewRepo(db)
	compiler := service.NewPlanCompiler()
	defaultResolver := service.NewDefaultPlanResolver(compiler)
	customResolver := service.NewCustomPlanResolver(compiler)
	triggerSvc := service.NewTriggerService(repo, defaultResolver, customResolver, compiler)
	// 装配侧注入:自动默认计划的模板/目录从 PG 激活计划装配
	// (真实环境可将此加载器替换为目录缓存服务,不触碰服务层契约)。
	templateLoader := adapters.NewPGPlanTemplateProvider(repo).LoadTemplate
	triggerSvc.SetTemplateLoader(templateLoader)
	// 人工选择列车(P2):草稿保存(maker/checker 审批由同一服务承载)+触发绑定定制修订。
	planAuthorSvc := service.NewPlanAuthorService(repo, compiler, customResolver, templateLoader)
	finalizerSvc := service.NewFinalizerService(repo)
	cancelSvc := service.NewCancelService(repo)
	reportSvc := service.NewHumanReportService(repo)
	scheduler := service.NewScheduler(repoAdapter{repo: repo}, repoAdapter{repo: repo})
	scheduleSvc := service.NewScheduleService(repo)
	retrySvc := service.NewRetryService(repo)
	runStateWalker := service.NewRunStateWalker(repo, logger)

	handler := api.NewHandler(triggerSvc, finalizerSvc, cancelSvc, scheduler, logger)
	handler.SetReportService(reportSvc)
	handler.SetRunReader(repo)
	handler.SetPlanAuthorService(planAuthorSvc)
	handler.SetScheduleService(scheduleSvc)
	handler.SetRetryService(retrySvc)
	// §20 页面到 API 唯一映射补齐:任务定义权威、预检、整 Run 重试、调度资源视图。
	handler.SetTaskDefinitionService(service.NewTaskDefinitionService(repo))
	handler.SetPreflightService(service.NewPreflightService(triggerSvc))
	handler.SetRetryTaskService(service.NewRetryTaskService(repo))
	handler.SetResourceService(service.NewResourceService(repo))
	handler.SetAllowedActionsService(service.NewAllowedActionsService(repo))
	// 运行详情"结果"页签:CH 只读(env 门控;未配置时 fail-closed 503)。
	if chURL := strings.TrimSpace(os.Getenv("CH_READ_URL")); chURL != "" {
		handler.SetRunResultsClient(api.NewRunResultsClient(chURL,
			os.Getenv("CH_READ_USER"), os.Getenv("CH_READ_PASSWORD"), 5*time.Second))
		logger.Info("run results reader enabled", zap.String("ch_read_url", chURL))
	}
	mux := http.NewServeMux()
	handler.Register(mux)

	// 调度后台循环(每 5s;多副本安全:触发唯一约束幂等)
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if _, err := scheduler.Tick(context.Background(), "default"); err != nil {
				logger.Warn("scheduler tick", zap.Error(err))
			}
		}
	}()

	// 调度物化 worker(env 门控):领取 PENDING_MATERIALIZATION 触发 → 物化 Task/Run;
	// 每轮同时做 AdmissionReservation 过期扫描(RESERVED 超期 → EXPIRED,须重新准入)。
	if os.Getenv("MATERIALIZE_ENABLED") == "true" {
		worker := service.NewMaterializeWorker(repo, logger)
		go func() {
			ticker := time.NewTicker(3 * time.Second)
			defer ticker.Stop()
			ticks := 0
			for range ticker.C {
				if _, err := worker.ProcessOnce(context.Background(), 5); err != nil {
					logger.Warn("materialize tick", zap.Error(err))
				}
				if n, err := repo.ExpireReservationsAtomic(context.Background(), "default", time.Now()); err != nil {
					logger.Warn("reservation expire sweep", zap.Error(err))
				} else if n > 0 {
					logger.Info("reservations expired", zap.Int64("count", n))
				}
				// 陈旧 run 关闭(§76.45.4 兜底):窗口结束 10 分钟后仍未启动 → CANCELLED
				ticks++
				if ticks%20 == 0 {
					if n, err := repo.CloseStaleRunsAtomic(context.Background(), "", time.Now(), 10*time.Minute, 20); err != nil {
						logger.Warn("stale run sweep", zap.Error(err))
					} else if n > 0 {
						logger.Info("stale runs closed", zap.Int("count", n))
					}
				}
			}
		}()
		logger.Info("schedule materialize worker enabled")
	}

	// P3 采集派发循环(env 门控;调度中心只做协议转换,回放由探针执行)。
	// 每 3s 领取一条 SOURCE_ACTIVATE;多副本安全(SKIP LOCKED+CAS)。
	if os.Getenv("REPLAY_ENABLED") == "true" {
		brokers := strings.Split(strings.TrimSpace(os.Getenv("KAFKA_BROKERS")), ",")
		publisher, err := adapters.NewKafkaProbeCommandPublisher(brokers, kafkaCommon.SecurityConfig{
			SecurityProtocol: os.Getenv("KAFKA_SECURITY_PROTOCOL"),
			SASLMechanism:    os.Getenv("KAFKA_SASL_MECHANISM"),
			SASLUsername:     os.Getenv("KAFKA_SASL_USERNAME"),
			SASLPassword:     os.Getenv("KAFKA_SASL_PASSWORD"),
			TLSCAFile:        os.Getenv("KAFKA_TLS_CA_FILE"),
			TLSServerName:    os.Getenv("KAFKA_TLS_SERVER_NAME"),
		}, logger)
		if err != nil {
			logger.Fatal("probe command publisher assembly failed", zap.Error(err))
		}
		adapter := adapters.NewPcapReplayAdapter(publisher, repo)
		captureAdapter := adapters.NewCaptureWindowAdapter(publisher, repo)
		dispatcher := service.NewStageDispatcher(repo, adapter, logger)
		dispatcher.SetExecutor("PROBE_CAPTURE_WINDOW", captureAdapter)
		dispatcher.SetExecutor("LIVE_STREAM_WINDOW", captureAdapter)
		dispatcher.SetRunStateWalker(runStateWalker)
		go func() {
			ticker := time.NewTicker(3 * time.Second)
			defer ticker.Stop()
			ticks := 0
			for range ticker.C {
				if _, err := dispatcher.DispatchOnce(context.Background(), 1); err != nil {
					logger.Warn("source dispatch tick", zap.Error(err))
				}
				// §76.45.3 lease 过期回收:接受窗口超时的 DISPATCHED 回 PENDING 重入队
				ticks++
				if ticks%10 == 0 {
					if n, err := repo.ExpireStageLeasesAtomic(context.Background(), time.Now(), 20); err != nil {
						logger.Warn("lease expiry sweep", zap.Error(err))
					} else if n > 0 {
						logger.Info("leases expired and requeued", zap.Int("count", n))
					}
				}
			}
		}()
		logger.Info("replay command dispatch enabled (protocol conversion only)",
			zap.String("command_topic", "probe.control.v2"))
	}

	// 权威事件中继(env 门控):outbox → Kafka(analysis.run.events.v1 RunSubscription)。
	// RunScopeRouter 等数据面订阅方由此获得 run 上下文。
	if os.Getenv("RELAY_ENABLED") == "true" {
		brokers := strings.Split(strings.TrimSpace(os.Getenv("KAFKA_BROKERS")), ",")
		if len(brokers) == 0 || brokers[0] == "" {
			logger.Fatal("KAFKA_BROKERS required when RELAY_ENABLED=true")
		}
		publisher, err := adapters.NewKafkaEventPublisher(brokers, kafkaCommon.SecurityConfig{
			SecurityProtocol: os.Getenv("KAFKA_SECURITY_PROTOCOL"),
			SASLMechanism:    os.Getenv("KAFKA_SASL_MECHANISM"),
			SASLUsername:     os.Getenv("KAFKA_SASL_USERNAME"),
			SASLPassword:     os.Getenv("KAFKA_SASL_PASSWORD"),
			TLSCAFile:        os.Getenv("KAFKA_TLS_CA_FILE"),
			TLSServerName:    os.Getenv("KAFKA_TLS_SERVER_NAME"),
		}, logger)
		if err != nil {
			logger.Fatal("event publisher assembly failed", zap.Error(err))
		}
		relayer := service.NewRunEventRelayer(repo, publisher, logger)
		relayer.SetRunStateWalker(runStateWalker)
		go func() {
			ticker := time.NewTicker(3 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				pub, failed, err := relayer.RelayOnce(context.Background(), 20)
				if err != nil {
					logger.Warn("relay tick", zap.Error(err))
					continue
				}
				if pub > 0 || failed > 0 {
					logger.Info("relay tick", zap.Int("published", pub), zap.Int("failed", failed))
				}
			}
		}()
		logger.Info("run event relay enabled", zap.Strings("brokers", brokers))

		// 阶段闸门循环(每 3s 开一条):S1 后按 DAG 逐级开 S2/S3/S4(RUNNING+fencing)。
		gate := service.NewStageGateLoop(repo, logger)
		go func() {
			ticker := time.NewTicker(3 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				if gated, err := gate.GateOnce(context.Background()); err != nil {
					logger.Warn("stage gate tick", zap.Error(err))
				} else if gated {
					logger.Info("stage gate tick: gated one attempt")
				}
			}
		}()
		logger.Info("stage gate loop enabled")
	}

	// 回执回流(env 门控):analysis.receipts.v1 → ApplyStageReceiptAtomic。
	if os.Getenv("RECEIPTS_ENABLED") == "true" {
		brokers := strings.Split(strings.TrimSpace(os.Getenv("KAFKA_BROKERS")), ",")
		sec := kafkaCommon.SecurityConfig{
			SecurityProtocol: os.Getenv("KAFKA_SECURITY_PROTOCOL"),
			SASLMechanism:    os.Getenv("KAFKA_SASL_MECHANISM"),
			SASLUsername:     os.Getenv("KAFKA_SASL_USERNAME"),
			SASLPassword:     os.Getenv("KAFKA_SASL_PASSWORD"),
			TLSCAFile:        os.Getenv("KAFKA_TLS_CA_FILE"),
			TLSServerName:    os.Getenv("KAFKA_TLS_SERVER_NAME"),
		}
		dialer, err := sec.Dialer("analysis-service-receipts")
		if err != nil {
			logger.Fatal("receipts kafka dialer", zap.Error(err))
		}
		reader := kafka.NewReader(kafka.ReaderConfig{
			Brokers: brokers, Topic: commoncontracts.TopicAnalysisReceipts,
			GroupID: "analysis-receipts-v1", Dialer: dialer,
			MinBytes: 1, MaxBytes: 10e6, MaxWait: 2 * time.Second,
		})
		applier := service.NewReceiptApplier(repo, logger)
		go func() {
			for {
				msg, err := reader.ReadMessage(context.Background())
				if err != nil {
					logger.Warn("receipt read", zap.Error(err))
					time.Sleep(2 * time.Second)
					continue
				}
				if err := applier.Apply(context.Background(), msg.Value); err != nil {
					logger.Warn("receipt apply (will retry)", zap.Error(err))
				}
			}
		}()
		logger.Info("receipts consumer enabled", zap.Strings("brokers", brokers))
	}

	// 终局循环(env 门控):S1-S4 全终态 → S5 自产回执(对账+机器摘要)→ 三件套
	// 同事务 + 唯一终态(权威自身阶段,不依赖外部执行器)。
	if os.Getenv("FINALIZE_ENABLED") == "true" {
		finalizeLoop := service.NewFinalizeLoop(repo, logger)
		go func() {
			ticker := time.NewTicker(3 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				if finalized, err := finalizeLoop.FinalizeOnce(context.Background()); err != nil {
					logger.Warn("finalize tick", zap.Error(err))
				} else if finalized {
					logger.Info("finalize tick: finalized one run")
				}
			}
		}()
		logger.Info("finalize loop enabled")
	}

	// Orchestrator 影子接线(env 门控;只读+日志):对非终态 run 输出确定性编排决策,
	// 与现役双 loop 行为对照收集等价性证据;证据收敛前不切换写路径。
	if os.Getenv("ORCHESTRATOR_SHADOW") == "true" {
		shadow := service.NewOrchestratorShadow(repo)
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				ids, err := repo.ListNonTerminalRunIDs(context.Background(), 10)
				if err != nil {
					logger.Warn("orchestrator shadow list", zap.Error(err))
					continue
				}
				for _, runID := range ids {
					decision, err := shadow.ShadowOne(context.Background(), runID)
					if err != nil {
						logger.Warn("orchestrator shadow", zap.String("run_id", runID), zap.Error(err))
						continue
					}
					logger.Info("orchestrator shadow decision",
						zap.String("run_id", runID),
						zap.Strings("dispatchables", decision.Dispatchables),
						zap.String("wait", string(decision.Wait)))
				}
			}
		}()
		logger.Info("orchestrator shadow enabled (read-only)")
	}

	// 报告自动化(§8 AUTO_ASYNC):终态+摘要+策略 AUTO_ASYNC 且无报告行的 run
	// 自动发起报告请求;生成由 G05 worker 独立推进,失败不回退 Run。
	if os.Getenv("AUTO_REPORT_ENABLED") == "true" {
		go func() {
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				candidates, err := repo.NextAutoReportCandidates(context.Background(), 5)
				if err != nil {
					logger.Warn("auto report candidates", zap.Error(err))
					continue
				}
				for _, c := range candidates {
					reportID, replayed, err := reportSvc.RequestReport(context.Background(), c.TenantID, c.RunID, c.TemplateRevision, c.Locale)
					if err != nil {
						logger.Warn("auto report request", zap.String("run_id", c.RunID), zap.Error(err))
						continue
					}
					logger.Info("auto report requested",
						zap.String("run_id", c.RunID), zap.String("report_id", reportID), zap.Bool("replayed", replayed))
				}
			}
		}()
		logger.Info("auto report coordinator enabled")
	}

	// 权限体系第二层防御(共享鉴权中间件):JWKS 验签 + 租户绑定强制。
	// AUTHZ_MODE=shadow 只记录拒绝;enforce 时 401/403 fail-closed。
	// P2-c/ADR-6:AUTHZ_API_TOKEN_ENABLED=true 时机器凭证(api_tokens)经
	// 同一中间件回退校验,与人类 OIDC 凭证同一判定链(租户绑定同样强制)。
	authzCfg := authz.Config{
		JWKSURL:            getEnvDefault("AUTHZ_JWKS_URL", "http://10.0.5.8:30180/auth/realms/master/protocol/openid-connect/certs"),
		Issuer:             getEnvDefault("AUTHZ_ISSUER", ""),
		Mode:               getEnvDefault("AUTHZ_MODE", "shadow"),
		RequireTenantClaim: getBoolEnv("AUTHZ_REQUIRE_TENANT_CLAIM", false),
		AllowedAZP:         splitCSV(getEnvDefault("AUTHZ_ALLOWED_AZP", "")),
		DenyAuditor:        authzDenyAudit(db, logger),
		ExemptPaths:        []string{"/api/v1/analysis/health"},
	}
	if getBoolEnv("AUTHZ_API_TOKEN_ENABLED", false) {
		tokenRepo := authrepository.NewTokenRepository(db, logger)
		apiTokenValidator := apitoken.NewValidator(tokenRepo, logger)
		authzCfg.Fallback = apiTokenValidator.Validate
		logger.Info("API token fallback enabled (P2-c unified machine credential validation)")
	}
	authzMW := authz.New(authzCfg, logger)
	rootHandler := authzMW.Handler(mux)
	// 契约解释器:认证之后、业务路由之前逐操作判定(/v1/analysis/* 35 操作,
	// 深度测试发现 analysis 面此前无任何 scope 判定,viewer 可达写操作)。
	if mode := getEnvDefault("AUTHZ_CONTRACT_MODE", ""); mode == "enforce" || mode == "shadow" {
		rootHandler = authzMW.Handler(authz.EnforceContract(authz.PrincipalFromRequest, mode, nil, logger, analysisContractDenyAudit(db, logger))(mux))
		logger.Info("Contract interpreter enabled (analysis API)", zap.String("mode", mode))
	}

	server := &http.Server{Addr: cfg.ListenAddr, Handler: rootHandler, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		logger.Info("analysis-service listening", zap.String("addr", cfg.ListenAddr))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("http server", zap.Error(err))
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
	logger.Info("analysis-service stopped")
}

// repoAdapter 仓储到调度接口的适配(同进程直调)。
type repoAdapter struct {
	repo *repository.Repo
}

func (a repoAdapter) ListActiveSchedules(ctx context.Context, tenantID string) ([]repository.ScheduleRow, error) {
	return a.repo.ListActiveSchedules(ctx, tenantID)
}

func (a repoAdapter) InsertTriggerInstance(ctx context.Context, tenantID, identityKind, canonicalHash, requestSHA, triggerKind, windowID, taskDefinitionID string, planRevision int64, actor, effectiveClass, resourceRestrictions string, scheduleRevision int64) (string, bool, error) {
	return a.repo.InsertTriggerInstance(ctx, tenantID, identityKind, canonicalHash, requestSHA, triggerKind, windowID, taskDefinitionID, planRevision, actor, effectiveClass, resourceRestrictions, scheduleRevision)
}

// EnqueueMaterialize 物化由 MaterializeWorker 直接轮询 PENDING_MATERIALIZATION 领取
// (SKIP LOCKED,多副本安全);此处为兜底登记,不承担投递语义。
func (a repoAdapter) EnqueueMaterialize(_ context.Context, _ string) error {
	return nil
}

// HasActiveRunForDefinition FORBID_OVERLAP 判定(定义下存在非终态 run)。
func (a repoAdapter) HasActiveRunForDefinition(ctx context.Context, tenantID, taskDefinitionID string) (bool, error) {
	return a.repo.HasActiveRunForDefinition(ctx, tenantID, taskDefinitionID)
}

// SuppressTrigger 触发事实转 SUPPRESSED(原因由调用方登记;不创建 Task)。
func (a repoAdapter) SuppressTrigger(ctx context.Context, triggerID, reason string) (bool, error) {
	ok, err := a.repo.SuppressTrigger(ctx, triggerID, reason)
	if ok {
		_, _ = a.repo.RecordTriggerSuppression(ctx, triggerID, reason)
	}
	return ok, err
}

// getEnvDefault 读取环境变量,空则用缺省。
func getEnvDefault(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func getBoolEnv(key string, def bool) bool {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return def
	}
	parsed, err := strconv.ParseBool(val)
	if err != nil {
		return def
	}
	return parsed
}

// authzDenyAudit 认证拒绝留痕(审计三联):401/403 拒绝 best-effort 落 audit_logs。
func authzDenyAudit(db *sql.DB, logger *zap.Logger) authz.DenyAuditFunc {
	return func(r *http.Request, status int, reason string, principal *authz.Principal) {
		if db == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		detail, _ := json.Marshal(map[string]interface{}{
			"reason": reason, "path": r.URL.Path, "status": status, "result": "denied",
		})
		tenant := strings.TrimSpace(r.Header.Get("X-Tenant-ID"))
		if tenant == "" {
			tenant = "default"
		}
		userID := ""
		if principal != nil {
			userID = principal.Subject
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO audit_logs (tenant_id, user_id, action, object_type, object_id, detail, ip_addr, user_agent)
			VALUES ($1, NULLIF($2,'')::uuid, 'AUTHZ_ACCESS_DENIED', 'authz', '', $3::jsonb, $4, $5)`,
			tenant, userID, string(detail), httpx.GetClientIP(r), r.UserAgent()); err != nil {
			logger.Warn("authz deny audit persist failed", zap.String("path", r.URL.Path), zap.Error(err))
		}
	}
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// analysisContractDenyAudit 契约拒绝留痕(复用 PG 直落审计通道)。
func analysisContractDenyAudit(db *sql.DB, logger *zap.Logger) authz.ContractDenyAuditor {
	return func(r *http.Request, op *authz.Operation, principal *authz.Principal, status int) {
		reason := fmt.Sprintf("contract scope required: %s (operation=%s)", op.RequiredScope, op.OperationID)
		authzDenyAudit(db, logger)(r, status, reason, principal)
	}
}
