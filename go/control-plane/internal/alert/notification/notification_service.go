////////////////////////////////////////////////////////////////////////////////
// Alert Notification Service — 告警通知 (Email / Webhook / Slack)
// 缺失业务逻辑 #1: 关键告警实时通知
////////////////////////////////////////////////////////////////////////////////

package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/smtp"
	"strings"
	"sync"
	"text/template"
	"time"

	"go.uber.org/zap"
)

// =============================================================================
// Config
// =============================================================================

type NotifyConfig struct {
	SMTPHost     string `env:"NOTIFY_SMTP_HOST"`
	SMTPPort     int    `env:"NOTIFY_SMTP_PORT" envDefault:"587"`
	SMTPUser     string `env:"NOTIFY_SMTP_USER"`
	SMTPPassword string `env:"NOTIFY_SMTP_PASSWORD"`
	FromEmail    string `env:"NOTIFY_FROM_EMAIL" envDefault:"alerts@traffic-analysis.local"`

	SlackWebhook    string `env:"NOTIFY_SLACK_WEBHOOK"`
	WebhookURL      string `env:"NOTIFY_WEBHOOK_URL"`
	WechatWebhook   string `env:"NOTIFY_WECHAT_WEBHOOK"`
	DingtalkWebhook string `env:"NOTIFY_DINGTALK_WEBHOOK"`
	FeishuWebhook   string `env:"NOTIFY_FEISHU_WEBHOOK"`

	MinSeverity     string `env:"NOTIFY_MIN_SEVERITY" envDefault:"high"`
	RateLimitPerMin int    `env:"NOTIFY_RATE_LIMIT" envDefault:"10"`
	TemplateDir     string `env:"NOTIFY_TEMPLATE_DIR" envDefault:"/etc/traffic/templates"`
}

// =============================================================================
// Alert Notification
// =============================================================================

type AlertInfo struct {
	AlertID      string    `json:"alert_id"`
	Title        string    `json:"title"`
	Severity     string    `json:"severity"`
	Score        float64   `json:"score"`
	SourceIP     string    `json:"source_ip"`
	DestIP       string    `json:"dest_ip"`
	AlertType    string    `json:"alert_type"`
	Description  string    `json:"description"`
	Labels       []string  `json:"labels,omitempty"`
	Count        int32     `json:"count,omitempty"`
	TenantID     string    `json:"tenant_id"`
	Timestamp    time.Time `json:"timestamp"`
	CampaignID   string    `json:"campaign_id,omitempty"`
	AssetName    string    `json:"asset_name,omitempty"`
	ThreatIntel  string    `json:"threat_intel,omitempty"`
	AssetScope   string    `json:"asset_scope,omitempty"`
	Campus       string    `json:"campus,omitempty"`
	ObjectType   string    `json:"object_type,omitempty"`
	ObjectID     string    `json:"object_id,omitempty"`
	Fingerprint  string    `json:"fingerprint,omitempty"`
	TargetName   string    `json:"target_name,omitempty"`
	Recipients   []string  `json:"recipients,omitempty"`
	Destinations []string  `json:"destinations,omitempty"`
}

// ChannelRoute is the concrete result of notification-governance evaluation.
// Keeping the matched rule and escalation target with the channel lets the
// execution path persist an honest, attributable delivery history.
type ChannelRoute struct {
	Channel    string `json:"channel"`
	RuleID     string `json:"rule_id,omitempty"`
	TargetName string `json:"target_name,omitempty"`
}

type DeliveryResult struct {
	Alert        *AlertInfo
	Route        ChannelRoute
	Status       string
	ErrorMessage string
}

type NotificationService struct {
	config           NotifyConfig
	logger           *zap.Logger
	limiter          *rateLimiter
	channelResolver  func(context.Context, *AlertInfo) ([]ChannelRoute, error)
	deliveryRecorder func(context.Context, DeliveryResult) error
}

func (s *NotificationService) SetChannelResolver(resolver func(context.Context, *AlertInfo) ([]ChannelRoute, error)) {
	if s != nil {
		s.channelResolver = resolver
	}
}

func (s *NotificationService) SetDeliveryRecorder(recorder func(context.Context, DeliveryResult) error) {
	if s != nil {
		s.deliveryRecorder = recorder
	}
}

var (
	ErrChannelNotConfigured = errors.New("notification channel is not configured")
	ErrChannelUnsupported   = errors.New("notification channel is not implemented")
	ErrRateLimited          = errors.New("notification rate limit exceeded")
)

func NewNotificationService(cfg NotifyConfig, logger *zap.Logger) *NotificationService {
	return &NotificationService{
		config:  cfg,
		logger:  logger,
		limiter: newRateLimiter(cfg.RateLimitPerMin),
	}
}

// Notify 发送告警通知（自动选择渠道）
func (s *NotificationService) Notify(ctx context.Context, alert *AlertInfo) error {
	if alert == nil {
		return errors.New("alert is required")
	}
	alert.Severity = NormalizeSeverity(alert.Severity, alert.Score)
	alert.AlertType = NormalizeAlertType(alert.AlertType, alert.Description)
	if s.channelResolver != nil {
		routes, err := s.channelResolver(ctx, alert)
		if err != nil {
			return fmt.Errorf("resolve governed notification channels: %w", err)
		}
		if len(routes) == 0 {
			return nil
		}
		if !s.limiter.Allow() {
			for _, route := range routes {
				s.recordDelivery(ctx, DeliveryResult{Alert: alert, Route: route, Status: "failed", ErrorMessage: ErrRateLimited.Error()})
			}
			return ErrRateLimited
		}
		var errs []string
		for _, route := range routes {
			dispatchErr := s.sendChannel(ctx, route.Channel, alert)
			result := DeliveryResult{Alert: alert, Route: route, Status: "sent"}
			if dispatchErr != nil {
				result.Status = "failed"
				result.ErrorMessage = dispatchErr.Error()
				errs = append(errs, dispatchErr.Error())
			}
			if recordErr := s.recordDelivery(ctx, result); recordErr != nil {
				errs = append(errs, "persist delivery: "+recordErr.Error())
			}
		}
		if len(errs) > 0 {
			return fmt.Errorf("notification errors: %s", strings.Join(errs, "; "))
		}
		return nil
	}
	if !s.shouldNotify(alert) {
		return nil
	}
	if !s.limiter.Allow() {
		s.logger.Warn("Notification rate limit exceeded", zap.String("alert_id", alert.AlertID))
		return ErrRateLimited
	}

	var errs []string
	attempted := 0

	// Email 通知 (severity >= high)
	if s.config.SMTPHost != "" && s.isSeverityAtLeast(alert.Severity, "high") {
		attempted++
		if err := s.sendEmail(ctx, alert); err != nil {
			errs = append(errs, "email: "+err.Error())
		}
	}

	// Slack 通知 (severity >= critical)
	if s.config.SlackWebhook != "" && s.isSeverityAtLeast(alert.Severity, "critical") {
		attempted++
		if err := s.sendSlack(ctx, alert); err != nil {
			errs = append(errs, "slack: "+err.Error())
		}
	}

	// 通用 Webhook (所有级别)
	if s.config.WebhookURL != "" {
		attempted++
		if err := s.sendWebhook(ctx, alert); err != nil {
			errs = append(errs, "webhook: "+err.Error())
		}
	}

	// 企业微信 (severity >= high)
	if s.config.WechatWebhook != "" && s.isSeverityAtLeast(alert.Severity, "high") {
		attempted++
		if err := s.sendWechat(ctx, alert); err != nil {
			errs = append(errs, "wechat: "+err.Error())
		}
	}

	// 钉钉 (severity >= high)
	if s.config.DingtalkWebhook != "" && s.isSeverityAtLeast(alert.Severity, "high") {
		attempted++
		if err := s.sendDingtalk(ctx, alert); err != nil {
			errs = append(errs, "dingtalk: "+err.Error())
		}
	}

	// 飞书 (severity >= high)
	if s.config.FeishuWebhook != "" && s.isSeverityAtLeast(alert.Severity, "high") {
		attempted++
		if err := s.sendFeishu(ctx, alert); err != nil {
			errs = append(errs, "feishu: "+err.Error())
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("notification errors: %s", strings.Join(errs, "; "))
	}
	if attempted == 0 {
		return ErrChannelNotConfigured
	}
	return nil
}

func (s *NotificationService) recordDelivery(ctx context.Context, result DeliveryResult) error {
	if s == nil || s.deliveryRecorder == nil {
		return nil
	}
	return s.deliveryRecorder(ctx, result)
}

// NormalizeSeverity accepts protobuf enum strings, localized labels and the
// historical top-label field. Unknown labels fall back to the numeric score.
func NormalizeSeverity(raw string, score float64) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	value = strings.TrimPrefix(value, "severity_")
	switch value {
	case "critical", "严重", "紧急":
		return "critical"
	case "high", "高危", "高":
		return "high"
	case "medium", "中危", "中":
		return "medium"
	case "low", "低危", "低":
		return "low"
	}
	// Detector scores are normally 0..1; some historical writers persist
	// percentages. Convert once so both contracts use the same thresholds.
	if score > 1 {
		score /= 100
	}
	switch {
	case score >= 0.9:
		return "critical"
	case score >= 0.7:
		return "high"
	case score >= 0.4:
		return "medium"
	default:
		return "low"
	}
}

// NormalizeAlertType maps detector/protobuf labels to the business categories
// configured by the notification workbench. The original value remains the
// fallback so custom rule types continue to work.
func NormalizeAlertType(raw string, labels string) string {
	classify := func(value string) string {
		value = strings.ToLower(strings.TrimSpace(value))
		switch {
		case strings.Contains(value, "data_exfil"), strings.Contains(value, "exfiltration"), strings.Contains(value, "数据泄露"), strings.Contains(value, "外传"):
			return "数据泄露"
		case strings.Contains(value, "login"), strings.Contains(value, "brute_force"), strings.Contains(value, "异常登录"), strings.Contains(value, "暴力破解"):
			return "异常登录"
		case strings.Contains(value, "task_failure"), strings.Contains(value, "data_quality"), strings.Contains(value, "任务失败"), strings.Contains(value, "数据质量"):
			return "任务失败"
		case strings.Contains(value, "scan"), strings.Contains(value, "attack"), strings.Contains(value, "intrusion"), strings.Contains(value, "攻击"), strings.Contains(value, "扫描"):
			return "攻击告警"
		}
		return ""
	}
	if normalized := classify(raw); normalized != "" {
		return normalized
	}
	if normalized := classify(labels); normalized != "" {
		return normalized
	}
	return strings.TrimSpace(raw)
}

// SendChannel delivers through exactly one requested channel. It fails closed
// when a provider is missing or unsupported, so callers cannot report a send
// merely because the handler itself completed.
func (s *NotificationService) SendChannel(ctx context.Context, channel string, alert *AlertInfo) error {
	if s == nil {
		return ErrChannelNotConfigured
	}
	if alert == nil {
		return errors.New("alert is required")
	}
	if !s.limiter.Allow() {
		return ErrRateLimited
	}
	return s.sendChannel(ctx, channel, alert)
}

func (s *NotificationService) sendChannel(ctx context.Context, channel string, alert *AlertInfo) error {
	switch strings.ToLower(strings.TrimSpace(channel)) {
	case "email":
		if s.config.SMTPHost == "" || s.config.SMTPUser == "" {
			return fmt.Errorf("email: %w", ErrChannelNotConfigured)
		}
		return s.sendEmail(ctx, alert)
	case "webhook":
		if s.config.WebhookURL == "" && len(alert.Destinations) == 0 {
			return fmt.Errorf("webhook: %w", ErrChannelNotConfigured)
		}
		return s.sendWebhook(ctx, alert)
	case "slack":
		if s.config.SlackWebhook == "" && len(alert.Destinations) == 0 {
			return fmt.Errorf("slack: %w", ErrChannelNotConfigured)
		}
		return s.sendSlack(ctx, alert)
	case "wechat":
		if s.config.WechatWebhook == "" && len(alert.Destinations) == 0 {
			return fmt.Errorf("wechat: %w", ErrChannelNotConfigured)
		}
		return s.sendWechat(ctx, alert)
	case "dingtalk":
		if s.config.DingtalkWebhook == "" && len(alert.Destinations) == 0 {
			return fmt.Errorf("dingtalk: %w", ErrChannelNotConfigured)
		}
		return s.sendDingtalk(ctx, alert)
	case "feishu":
		if s.config.FeishuWebhook == "" && len(alert.Destinations) == 0 {
			return fmt.Errorf("feishu: %w", ErrChannelNotConfigured)
		}
		return s.sendFeishu(ctx, alert)
	case "sms", "ticket":
		return fmt.Errorf("%s: %w", channel, ErrChannelUnsupported)
	default:
		return fmt.Errorf("%s: %w", channel, ErrChannelUnsupported)
	}
}

// =============================================================================
// Email
// =============================================================================

func (s *NotificationService) sendEmail(ctx context.Context, alert *AlertInfo) error {
	subject := fmt.Sprintf("[%s] %s Alert: %s — %s → %s",
		strings.ToUpper(alert.Severity), alert.AlertType, alert.Title, alert.SourceIP, alert.DestIP)

	body := s.buildEmailBody(alert)
	msg := fmt.Sprintf("From: %s\r\nTo: %%s\r\nSubject: %s\r\nMIME-Version: 1.0\r\n"+
		"Content-Type: text/html; charset=UTF-8\r\n\r\n%s",
		s.config.FromEmail, subject, body)

	auth := smtp.PlainAuth("", s.config.SMTPUser, s.config.SMTPPassword,
		strings.Split(s.config.SMTPHost, ":")[0])

	addr := fmt.Sprintf("%s:%d", s.config.SMTPHost, s.config.SMTPPort)
	recipients := s.getRecipients(alert)
	if len(recipients) == 0 {
		recipients = []string{s.config.SMTPUser}
	}

	return smtp.SendMail(addr, auth, s.config.FromEmail, recipients,
		[]byte(fmt.Sprintf(msg, strings.Join(recipients, ","))))
}

func (s *NotificationService) buildEmailBody(alert *AlertInfo) string {
	return fmt.Sprintf(`<html><body style="font-family:Arial,sans-serif">
<h2 style="color:%s">🚨 %s Alert: %s</h2>
<table border="1" cellpadding="8" cellspacing="0" style="border-collapse:collapse">
<tr><td><b>Alert ID</b></td><td>%s</td></tr>
<tr><td><b>Severity</b></td><td style="color:%s"><b>%s</b></td></tr>
<tr><td><b>Score</b></td><td>%.2f</td></tr>
<tr><td><b>Source</b></td><td>%s%s</td></tr>
<tr><td><b>Destination</b></td><td>%s%s</td></tr>
<tr><td><b>Type</b></td><td>%s</td></tr>
<tr><td><b>Campaign</b></td><td>%s</td></tr>
<tr><td><b>Threat Intel</b></td><td>%s</td></tr>
</table>
<p><i>Traffic Analysis Platform — %s</i></p>
</body></html>`,
		s.severityColor(alert.Severity), strings.ToUpper(alert.Severity), alert.Title,
		alert.AlertID, s.severityColor(alert.Severity), strings.ToUpper(alert.Severity),
		alert.Score, alert.SourceIP, s.assetLabel(alert.AssetName),
		alert.DestIP, "", alert.AlertType, alert.CampaignID,
		alert.ThreatIntel, time.Now().Format(time.RFC3339))
}

// =============================================================================
// Slack
// =============================================================================

func (s *NotificationService) sendSlack(ctx context.Context, alert *AlertInfo) error {
	color := s.severityColor(alert.Severity)

	payload := map[string]interface{}{
		"attachments": []map[string]interface{}{{
			"color": color,
			"title": fmt.Sprintf("🚨 [%s] %s: %s", alert.Severity, alert.AlertType, alert.Title),
			"fields": []map[string]interface{}{
				{"title": "Alert ID", "value": alert.AlertID, "short": true},
				{"title": "Severity", "value": alert.Severity, "short": true},
				{"title": "Score", "value": fmt.Sprintf("%.2f", alert.Score), "short": true},
				{"title": "Source → Dest", "value": fmt.Sprintf("%s → %s", alert.SourceIP, alert.DestIP), "short": true},
				{"title": "Campaign", "value": alert.CampaignID, "short": true},
				{"title": "Threat Intel", "value": alert.ThreatIntel, "short": true},
			},
			"footer": "Traffic Analysis Platform",
			"ts":     alert.Timestamp.Unix(),
		}},
	}

	return s.postWebhookDestinations(ctx, alert, s.config.SlackWebhook, payload, "slack")
}

// =============================================================================
// 企业微信
// =============================================================================

func (s *NotificationService) sendWechat(ctx context.Context, alert *AlertInfo) error {
	color := s.severityColor(alert.Severity)
	payload := map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"content": fmt.Sprintf(
				"## <font color=\"%s\">[%s] %s Alert</font>\n"+
					"> Alert ID: %s\n> Severity: **%s**\n> Score: %.2f\n"+
					"> Source: %s → Dest: %s\n> Type: %s\n> Campaign: %s",
				color, strings.ToUpper(alert.Severity), alert.Title,
				alert.AlertID, strings.ToUpper(alert.Severity), alert.Score,
				alert.SourceIP, alert.DestIP, alert.AlertType, alert.CampaignID),
		},
	}
	return s.postWebhookDestinations(ctx, alert, s.config.WechatWebhook, payload, "wechat")
}

// =============================================================================
// 钉钉
// =============================================================================

func (s *NotificationService) sendDingtalk(ctx context.Context, alert *AlertInfo) error {
	payload := map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": fmt.Sprintf("[%s] %s Alert", alert.Severity, alert.Title),
			"text": fmt.Sprintf(
				"## [%s] %s Alert\n\n"+
					"- Alert ID: %s\n- Severity: **%s**\n- Score: %.2f\n"+
					"- Source: %s\n- Destination: %s\n- Type: %s\n- Campaign: %s\n\n"+
					"> Traffic Analysis Platform",
				strings.ToUpper(alert.Severity), alert.Title,
				alert.AlertID, strings.ToUpper(alert.Severity), alert.Score,
				alert.SourceIP, alert.DestIP, alert.AlertType, alert.CampaignID),
		},
	}
	return s.postWebhookDestinations(ctx, alert, s.config.DingtalkWebhook, payload, "dingtalk")
}

// =============================================================================
// 飞书
// =============================================================================

func (s *NotificationService) sendFeishu(ctx context.Context, alert *AlertInfo) error {
	color := s.severityColor(alert.Severity)
	elements := []map[string]interface{}{
		{"tag": "div", "text": map[string]string{"tag": "lark_md",
			"content": fmt.Sprintf("**🚨 [%s] %s Alert**", strings.ToUpper(alert.Severity), alert.Title)}},
		{"tag": "hr"},
		{"tag": "div", "text": map[string]string{"tag": "lark_md",
			"content": fmt.Sprintf("Alert ID: %s\nSeverity: **%s**\nScore: %.2f\nSource: %s → Dest: %s\nType: %s",
				alert.AlertID, alert.Severity, alert.Score, alert.SourceIP, alert.DestIP, alert.AlertType)}},
	}
	payload := map[string]interface{}{
		"msg_type": "interactive",
		"card": map[string]interface{}{
			"header": map[string]interface{}{
				"title":    map[string]string{"tag": "plain_text", "content": fmt.Sprintf("[%s] Alert", alert.Severity)},
				"template": color,
			},
			"elements": elements,
		},
	}
	return s.postWebhookDestinations(ctx, alert, s.config.FeishuWebhook, payload, "feishu")
}

// =============================================================================
// 通用 Webhook POST
// =============================================================================

func (s *NotificationService) postWebhook(ctx context.Context, url string, payload map[string]interface{}, provider string) error {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned %d: %s", resp.StatusCode, string(respBody))
	}
	if provider == "slack" {
		value := strings.TrimSpace(string(respBody))
		if !strings.EqualFold(value, "ok") {
			return fmt.Errorf("slack business response is not an explicit success: %q", value)
		}
		return nil
	}
	var response map[string]interface{}
	if len(bytes.TrimSpace(respBody)) == 0 {
		return fmt.Errorf("%s business response is empty", provider)
	}
	if err := json.Unmarshal(respBody, &response); err != nil {
		return fmt.Errorf("%s business response is not JSON: %w", provider, err)
	}
	code := notificationProviderCode(response, provider)
	if code == "" {
		return fmt.Errorf("%s business response has no success code", provider)
	}
	if code != "0" {
		message := notificationProviderMessage(response)
		return fmt.Errorf("%s business error code=%s message=%s", provider, code, message)
	}
	return nil
}

func (s *NotificationService) postWebhookDestinations(ctx context.Context, alert *AlertInfo, fallback string, payload map[string]interface{}, provider string) error {
	destinations := notificationDestinations(alert, fallback)
	if len(destinations) == 0 {
		return fmt.Errorf("%s: %w", provider, ErrChannelNotConfigured)
	}
	for _, destination := range destinations {
		if err := s.postWebhook(ctx, destination, payload, provider); err != nil {
			return err
		}
	}
	return nil
}

func notificationProviderCode(response map[string]interface{}, provider string) string {
	keys := []string{"errcode", "code"}
	if provider == "feishu" {
		keys = []string{"code", "StatusCode", "errcode"}
	}
	for _, key := range keys {
		if value, ok := response[key]; ok {
			return strings.TrimSpace(fmt.Sprint(value))
		}
	}
	return ""
}

func notificationProviderMessage(response map[string]interface{}) string {
	for _, key := range []string{"errmsg", "msg", "message", "StatusMessage"} {
		if value, ok := response[key]; ok {
			return strings.TrimSpace(fmt.Sprint(value))
		}
	}
	return "provider rejected notification"
}

// =============================================================================
// Webhook (generic)
// =============================================================================

func (s *NotificationService) sendWebhook(ctx context.Context, alert *AlertInfo) error {
	body, _ := json.Marshal(alert)
	destinations := notificationDestinations(alert, s.config.WebhookURL)
	if len(destinations) == 0 {
		return fmt.Errorf("webhook: %w", ErrChannelNotConfigured)
	}
	for _, destination := range destinations {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, destination, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
		if err != nil {
			return err
		}
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("webhook returned %d: %s", resp.StatusCode, string(respBody))
		}
	}
	return nil
}

func notificationDestinations(alert *AlertInfo, fallback string) []string {
	if alert != nil && len(alert.Destinations) > 0 {
		result := make([]string, 0, len(alert.Destinations))
		seen := map[string]struct{}{}
		for _, raw := range alert.Destinations {
			destination := strings.TrimSpace(raw)
			if destination == "" {
				continue
			}
			if _, exists := seen[destination]; exists {
				continue
			}
			seen[destination] = struct{}{}
			result = append(result, destination)
		}
		return result
	}
	if destination := strings.TrimSpace(fallback); destination != "" {
		return []string{destination}
	}
	return nil
}

// =============================================================================
// Helpers
// =============================================================================

func (s *NotificationService) shouldNotify(alert *AlertInfo) bool {
	switch alert.Severity {
	case "critical":
		return true
	case "high":
		return true
	case "medium":
		return s.config.MinSeverity == "medium" || s.config.MinSeverity == "low"
	case "low":
		return s.config.MinSeverity == "low"
	}
	return false
}

func (s *NotificationService) isSeverityAtLeast(severity, min string) bool {
	levels := map[string]int{"low": 0, "medium": 1, "high": 2, "critical": 3}
	return levels[severity] >= levels[min]
}

func (s *NotificationService) severityColor(severity string) string {
	switch severity {
	case "critical":
		return "#FF0000"
	case "high":
		return "#FF6600"
	case "medium":
		return "#FFCC00"
	default:
		return "#3399FF"
	}
}

func (s *NotificationService) assetLabel(name string) string {
	if name != "" {
		return " (" + name + ")"
	}
	return ""
}

func (s *NotificationService) getRecipients(alert *AlertInfo) []string {
	return append([]string(nil), alert.Recipients...)
}

// =============================================================================
// Rate Limiter
// =============================================================================

type rateLimiter struct {
	mu        sync.Mutex
	window    []time.Time
	maxPerMin int
}

func newRateLimiter(maxPerMin int) *rateLimiter {
	return &rateLimiter{maxPerMin: maxPerMin}
}

func (r *rateLimiter) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-1 * time.Minute)
	filtered := make([]time.Time, 0)
	for _, t := range r.window {
		if t.After(cutoff) {
			filtered = append(filtered, t)
		}
	}
	if len(filtered) >= r.maxPerMin {
		return false
	}
	r.window = append(filtered, now)
	return true
}

// Suppress unused imports
var _ = template.New
