////////////////////////////////////////////////////////////////////////////////
// Alert Notification Service — 告警通知 (Email / Webhook / Slack)
// 缺失业务逻辑 #1: 关键告警实时通知
////////////////////////////////////////////////////////////////////////////////

package notification

import (
	"bytes"
	"context"
	"encoding/json"
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

	MinSeverity      string        `env:"NOTIFY_MIN_SEVERITY" envDefault:"high"`
	RateLimitPerMin  int           `env:"NOTIFY_RATE_LIMIT" envDefault:"10"`
	TemplateDir      string        `env:"NOTIFY_TEMPLATE_DIR" envDefault:"/etc/traffic/templates"`
}

// =============================================================================
// Alert Notification
// =============================================================================

type AlertInfo struct {
	AlertID     string    `json:"alert_id"`
	Title       string    `json:"title"`
	Severity    string    `json:"severity"`
	Score       float64   `json:"score"`
	SourceIP    string    `json:"source_ip"`
	DestIP      string    `json:"dest_ip"`
	AlertType   string    `json:"alert_type"`
	Description string    `json:"description"`
	TenantID    string    `json:"tenant_id"`
	Timestamp   time.Time `json:"timestamp"`
	CampaignID  string    `json:"campaign_id,omitempty"`
	AssetName   string    `json:"asset_name,omitempty"`
	ThreatIntel string    `json:"threat_intel,omitempty"`
}

type NotificationService struct {
	config  NotifyConfig
	logger  *zap.Logger
	limiter *rateLimiter
}

func NewNotificationService(cfg NotifyConfig, logger *zap.Logger) *NotificationService {
	return &NotificationService{
		config:  cfg,
		logger:  logger,
		limiter: newRateLimiter(cfg.RateLimitPerMin),
	}
}

// Notify 发送告警通知（自动选择渠道）
func (s *NotificationService) Notify(ctx context.Context, alert *AlertInfo) error {
	if !s.shouldNotify(alert) {
		return nil
	}
	if !s.limiter.Allow() {
		s.logger.Warn("Notification rate limit exceeded", zap.String("alert_id", alert.AlertID))
		return nil
	}

	var errs []string

	// Email 通知 (severity >= high)
	if s.config.SMTPHost != "" && s.isSeverityAtLeast(alert.Severity, "high") {
		if err := s.sendEmail(ctx, alert); err != nil {
			errs = append(errs, "email: "+err.Error())
		}
	}

	// Slack 通知 (severity >= critical)
	if s.config.SlackWebhook != "" && s.isSeverityAtLeast(alert.Severity, "critical") {
		if err := s.sendSlack(ctx, alert); err != nil {
			errs = append(errs, "slack: "+err.Error())
		}
	}

	// 通用 Webhook (所有级别)
	if s.config.WebhookURL != "" {
		if err := s.sendWebhook(ctx, alert); err != nil {
			errs = append(errs, "webhook: "+err.Error())
		}
	}

	// 企业微信 (severity >= high)
	if s.config.WechatWebhook != "" && s.isSeverityAtLeast(alert.Severity, "high") {
		if err := s.sendWechat(ctx, alert); err != nil {
			errs = append(errs, "wechat: "+err.Error())
		}
	}

	// 钉钉 (severity >= high)
	if s.config.DingtalkWebhook != "" && s.isSeverityAtLeast(alert.Severity, "high") {
		if err := s.sendDingtalk(ctx, alert); err != nil {
			errs = append(errs, "dingtalk: "+err.Error())
		}
	}

	// 飞书 (severity >= high)
	if s.config.FeishuWebhook != "" && s.isSeverityAtLeast(alert.Severity, "high") {
		if err := s.sendFeishu(ctx, alert); err != nil {
			errs = append(errs, "feishu: "+err.Error())
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("notification errors: %s", strings.Join(errs, "; "))
	}
	return nil
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
			"footer":     "Traffic Analysis Platform",
			"ts":         alert.Timestamp.Unix(),
		}},
	}

	body, _ := json.Marshal(payload)
	resp, err := http.Post(s.config.SlackWebhook, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
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
	return s.postWebhook(s.config.WechatWebhook, payload)
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
	return s.postWebhook(s.config.DingtalkWebhook, payload)
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
	return s.postWebhook(s.config.FeishuWebhook, payload)
}

// =============================================================================
// 通用 Webhook POST
// =============================================================================

func (s *NotificationService) postWebhook(url string, payload map[string]interface{}) error {
	body, _ := json.Marshal(payload)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("webhook returned %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// =============================================================================
// Webhook (generic)
// =============================================================================

func (s *NotificationService) sendWebhook(ctx context.Context, alert *AlertInfo) error {
	body, _ := json.Marshal(alert)
	resp, err := http.Post(s.config.WebhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("webhook returned %d: %s", resp.StatusCode, string(respBody))
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
	case "critical": return "#FF0000"
	case "high":     return "#FF6600"
	case "medium":   return "#FFCC00"
	default:         return "#3399FF"
	}
}

func (s *NotificationService) assetLabel(name string) string {
	if name != "" {
		return " (" + name + ")"
	}
	return ""
}

func (s *NotificationService) getRecipients(alert *AlertInfo) []string {
	// 未来可扩展：按 tenant_id 查找管理员邮箱
	return nil
}

// =============================================================================
// Rate Limiter
// =============================================================================

type rateLimiter struct {
	mu       sync.Mutex
	window   []time.Time
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
