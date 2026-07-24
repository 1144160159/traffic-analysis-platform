package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/notification"
)

type notificationEscalationJob struct {
	JobID             int64
	TenantID          string
	AlertID           string
	RuleID            string
	PolicyID          string
	PolicyUpdatedAt   time.Time
	StageIndex        int
	StageAfterMinutes float64
	StageFingerprint  string
	TargetRole        string
	Channel           string
	DueAt             time.Time
	Payload           []byte
	Attempts          int
	LockToken         string
}

const notificationEscalationLease = 2 * time.Minute
const notificationEscalationMaxAttempts = 5

func (r *AdvancedRepository) scheduleNotificationEscalation(ctx context.Context, rule NotificationRuleRecord, policy NotificationEscalationPolicyRecord, alert *notification.AlertInfo, settings map[string]interface{}) error {
	if alert == nil {
		return errors.New("notification escalation requires alert context")
	}
	payload, err := json.Marshal(alert)
	if err != nil {
		return fmt.Errorf("marshal notification escalation alert: %w", err)
	}
	alertKey := strings.TrimSpace(alert.Fingerprint)
	if alertKey == "" {
		alertKey = alert.AlertID
	}
	base := alert.Timestamp
	if base.IsZero() {
		base = time.Now()
	}
	for stageIndex, stage := range policy.Stages {
		afterMinutes, valid := numericNotificationValue(stage["after_minutes"])
		targetRole := notificationText(stage["target_role"])
		// Persist every structurally valid stage. Conditions such as unconfirmed,
		// repeated, or remediation-failed describe future state and are evaluated
		// against the live alert again when the deadline is reached.
		if !valid || afterMinutes < 0 || targetRole == "" {
			continue
		}
		stageFingerprint, err := notificationEscalationStageFingerprint(stage)
		if err != nil {
			return fmt.Errorf("fingerprint notification escalation stage: %w", err)
		}
		for _, rawChannel := range rule.Channels {
			channel := strings.ToLower(strings.TrimSpace(rawChannel))
			if _, supported := notificationChannelNames[channel]; !supported || !notificationChannelEnabled(settings, channel) {
				continue
			}
			_, err := r.db.ExecContext(ctx, `
				INSERT INTO notification_escalation_jobs
				(tenant_id,alert_key,alert_id,rule_id,policy_id,policy_updated_at,stage_index,stage_after_minutes,stage_fingerprint,target_role,channel,due_at,alert_payload,status,trace_id)
				VALUES ($1,$2,$3,$4::uuid,$5::uuid,$6,$7,$8,$9,$10,$11,$12,$13::jsonb,'pending',$14)
				ON CONFLICT (tenant_id,alert_key,rule_id,stage_index,channel) DO UPDATE
				SET alert_id=EXCLUDED.alert_id,policy_id=EXCLUDED.policy_id,policy_updated_at=EXCLUDED.policy_updated_at,
					stage_after_minutes=EXCLUDED.stage_after_minutes,stage_fingerprint=EXCLUDED.stage_fingerprint,
					target_role=EXCLUDED.target_role,due_at=EXCLUDED.due_at,alert_payload=EXCLUDED.alert_payload,updated_at=now()
				WHERE notification_escalation_jobs.status='pending'`,
				alert.TenantID, alertKey, alert.AlertID, rule.RuleID, policy.PolicyID, policy.UpdatedAt, stageIndex,
				afterMinutes, stageFingerprint, targetRole, channel,
				base.Add(time.Duration(afterMinutes*float64(time.Minute))), string(payload), "escalation-"+uuid.NewString())
			if err != nil {
				return fmt.Errorf("schedule notification escalation: %w", err)
			}
		}
	}
	return nil
}

// RunNotificationEscalationWorker executes durable due jobs. Jobs are claimed
// before provider I/O, so repeated Kafka detections and concurrent ticks cannot
// deliver the same alert/rule/stage/channel tuple more than once.
func (r *AdvancedRepository) RunNotificationEscalationWorker(ctx context.Context, notifier *notification.NotificationService, interval time.Duration) {
	if r == nil || r.db == nil || notifier == nil {
		return
	}
	if interval <= 0 {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := r.processDueNotificationEscalations(ctx, notifier, 50); err != nil && !errors.Is(err, context.Canceled) && r.logger != nil {
			r.logger.Warn("Notification escalation worker tick failed", zap.Error(err))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *AdvancedRepository) processDueNotificationEscalations(ctx context.Context, notifier *notification.NotificationService, limit int) error {
	rows, err := r.db.QueryContext(ctx, `
		SELECT job_id,tenant_id,alert_id,rule_id::text,COALESCE(policy_id::text,''),COALESCE(policy_updated_at,to_timestamp(0)),
		       stage_index,COALESCE(stage_after_minutes,-1),stage_fingerprint,target_role,channel,due_at,alert_payload,attempts
		FROM notification_escalation_jobs
		WHERE (status='pending' AND due_at<=now())
		   OR (status='processing' AND locked_at<now()-$2::interval)
		ORDER BY due_at,job_id LIMIT $1`, limit, notificationEscalationLeaseInterval())
	if err != nil {
		return fmt.Errorf("list due notification escalations: %w", err)
	}
	jobs := make([]notificationEscalationJob, 0)
	for rows.Next() {
		var job notificationEscalationJob
		if err := rows.Scan(&job.JobID, &job.TenantID, &job.AlertID, &job.RuleID, &job.PolicyID, &job.PolicyUpdatedAt,
			&job.StageIndex, &job.StageAfterMinutes, &job.StageFingerprint, &job.TargetRole, &job.Channel, &job.DueAt, &job.Payload, &job.Attempts); err != nil {
			rows.Close()
			return err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, job := range jobs {
		job.LockToken = uuid.NewString()
		result, err := r.db.ExecContext(ctx, `
			UPDATE notification_escalation_jobs
			SET status='processing',attempts=attempts+1,locked_at=now(),lock_token=$2,updated_at=now()
			WHERE job_id=$1 AND ((status='pending' AND due_at<=now()) OR (status='processing' AND locked_at<now()-$3::interval))`,
			job.JobID, job.LockToken, notificationEscalationLeaseInterval())
		if err != nil {
			return err
		}
		claimed, _ := result.RowsAffected()
		if claimed != 1 {
			continue
		}
		jobCtx, cancelJob := context.WithCancel(ctx)
		stopHeartbeat := r.startNotificationEscalationLeaseHeartbeat(jobCtx, cancelJob, job)
		var alert notification.AlertInfo
		if err := json.Unmarshal(job.Payload, &alert); err != nil {
			_ = stopHeartbeat()
			_ = r.completeNotificationEscalationJob(ctx, job, "failed", "invalid alert payload: "+err.Error())
			continue
		}
		executable, reason, err := r.revalidateNotificationEscalation(jobCtx, job, &alert)
		if err != nil {
			_ = stopHeartbeat()
			if releaseErr := r.releaseNotificationEscalationJob(ctx, job, err); releaseErr != nil {
				return releaseErr
			}
			continue
		}
		if !executable {
			_ = stopHeartbeat()
			if err := r.completeNotificationEscalationJob(ctx, job, "cancelled", reason); err != nil {
				return err
			}
			continue
		}
		alert.TargetName = job.TargetRole
		destinations, resolveErr := r.notificationRoleDestinations(jobCtx, job.TenantID, job.TargetRole, job.Channel)
		if resolveErr != nil {
			err = resolveErr
		} else if len(destinations) == 0 {
			err = fmt.Errorf("target role %q has no active %s destinations", job.TargetRole, job.Channel)
		} else if job.Channel == "email" {
			alert.Recipients = destinations
		} else {
			alert.Destinations = destinations
		}
		if err == nil {
			err = notifier.SendChannel(jobCtx, job.Channel, &alert)
		}
		status, errorMessage := "sent", ""
		if err != nil {
			status, errorMessage = "failed", err.Error()
		}
		recordErr := r.RecordAutomaticNotificationDelivery(jobCtx, notification.DeliveryResult{Alert: &alert, Route: notification.ChannelRoute{Channel: job.Channel, RuleID: job.RuleID, TargetName: job.TargetRole}, Status: status, ErrorMessage: errorMessage})
		if recordErr != nil {
			status, errorMessage = "failed", "persist delivery: "+recordErr.Error()
		}
		heartbeatErr := stopHeartbeat()
		if heartbeatErr != nil {
			return heartbeatErr
		}
		if status == "failed" {
			if err := r.releaseNotificationEscalationJob(ctx, job, errors.New(errorMessage)); err != nil {
				return err
			}
			continue
		}
		if err := r.completeNotificationEscalationJob(ctx, job, status, errorMessage); err != nil {
			return err
		}
	}
	return nil
}

func (r *AdvancedRepository) revalidateNotificationEscalation(ctx context.Context, job notificationEscalationJob, alert *notification.AlertInfo) (bool, string, error) {
	settings := defaultNotificationSettings()
	if saved, ok, err := r.GetNotificationSettings(ctx, job.TenantID); err != nil {
		return false, "", err
	} else if ok {
		settings = mergeSettings(settings, saved)
	}
	if enabled, ok := settings["enabled"].(bool); !ok || !enabled {
		return false, "notification settings disabled before escalation deadline", nil
	}
	if !notificationChannelEnabled(settings, job.Channel) {
		return false, "notification channel disabled before escalation deadline", nil
	}
	if r.notificationStateResolver == nil {
		return false, "", errors.New("live alert state resolver is not configured")
	}
	liveAlert, status, err := r.notificationStateResolver(ctx, job.TenantID, job.AlertID)
	if err != nil {
		return false, "", fmt.Errorf("resolve live alert: %w", err)
	}
	if liveAlert == nil {
		return false, "", errors.New("live alert resolver returned an empty alert")
	}
	mergeNotificationAlertPresentation(liveAlert, alert)
	*alert = *liveAlert
	if !notificationSeverityAtLeast(alert.Severity, notificationText(settings["min_severity"])) {
		return false, "alert no longer meets notification minimum severity", nil
	}
	if err := r.enrichNotificationAlertDimensions(ctx, alert); err != nil {
		return false, "", err
	}
	rule, found, err := r.getNotificationRule(ctx, job.TenantID, job.RuleID)
	if err != nil {
		return false, "", err
	}
	if !found || !rule.Enabled {
		return false, "notification rule disabled or removed before escalation deadline", nil
	}
	channelStillConfigured := false
	for _, rawChannel := range rule.Channels {
		if strings.EqualFold(strings.TrimSpace(rawChannel), job.Channel) {
			channelStillConfigured = true
			break
		}
	}
	if !channelStillConfigured {
		return false, "notification channel removed from rule before escalation deadline", nil
	}
	policies, err := r.ListNotificationEscalationPolicies(ctx, job.TenantID, 200)
	if err != nil {
		return false, "", err
	}
	enabledPolicies := make(map[string]NotificationEscalationPolicyRecord, len(policies))
	for _, policy := range policies {
		if policy.Enabled {
			enabledPolicies[policy.Name] = policy
		}
	}
	policyName := notificationText(rule.Conditions["escalation_policy"])
	policy, exists := enabledPolicies[policyName]
	if policyName == "" || !exists || policy.PolicyID != job.PolicyID || !policy.UpdatedAt.Equal(job.PolicyUpdatedAt) || job.StageIndex < 0 || job.StageIndex >= len(policy.Stages) {
		return false, "escalation policy or stage disabled or removed before deadline", nil
	}
	stage := policy.Stages[job.StageIndex]
	stageAfterMinutes, valid := numericNotificationValue(stage["after_minutes"])
	stageFingerprint, fingerprintErr := notificationEscalationStageFingerprint(stage)
	if fingerprintErr != nil {
		return false, "", fingerprintErr
	}
	if !valid || stageAfterMinutes != job.StageAfterMinutes || stageFingerprint != job.StageFingerprint || notificationText(stage["target_role"]) != job.TargetRole {
		return false, "escalation policy stage changed before deadline", nil
	}
	if !notificationRuleMatchesAlert(rule, alert, enabledPolicies) {
		return false, "alert no longer matches notification rule", nil
	}
	silences, err := r.ListNotificationSilenceRules(ctx, job.TenantID, 200)
	if err != nil {
		return false, "", err
	}
	if notificationRuleIsSilenced(rule, alert, silences) {
		return false, "notification entered an active silence window before escalation deadline", nil
	}
	if !notificationEscalationConditionMatchesAtExecution(notificationText(stage["condition"]), alert, status) {
		return false, fmt.Sprintf("escalation condition no longer matches live alert status %q", status), nil
	}
	return true, "", nil
}

func (r *AdvancedRepository) getNotificationRule(ctx context.Context, tenantID, ruleID string) (NotificationRuleRecord, bool, error) {
	rule, err := scanNotificationRule(r.db.QueryRowContext(ctx, `SELECT rule_id::text,tenant_id,name,conditions,channels,enabled,COALESCE(created_by::text,''),created_at,updated_at FROM notification_rules WHERE tenant_id=$1 AND rule_id::text=$2`, tenantID, ruleID))
	if err == sql.ErrNoRows {
		return NotificationRuleRecord{}, false, nil
	}
	if err != nil {
		return NotificationRuleRecord{}, false, err
	}
	return rule, true, nil
}

func notificationEscalationConditionMatchesAtExecution(condition string, alert *notification.AlertInfo, liveStatus string) bool {
	condition = strings.ToLower(strings.TrimSpace(condition))
	status := strings.ToLower(strings.TrimSpace(liveStatus))
	terminal := status == "closed" || status == "resolved" || status == "confirmed" || status == "ignored" || status == "false_positive" || status == "alert_status_closed" || status == "alert_status_resolved" || status == "已关闭"
	if terminal {
		return false
	}
	switch {
	case condition == "", condition == "sla 超时":
		return status != ""
	case condition == "未确认":
		return status == "new" || status == "open" || status == "unhandled" || status == "alert_status_new" || status == "未处理"
	case strings.Contains(condition, "重复"):
		return alert.Count > 1 || strings.Contains(strings.ToLower(strings.Join(alert.Labels, " ")), "repeat") || strings.Contains(strings.Join(alert.Labels, " "), "重复")
	case strings.Contains(condition, "失败"):
		liveText := strings.ToLower(strings.Join(append(append([]string{}, alert.Labels...), alert.Description), " "))
		return strings.Contains(status, "fail") || strings.Contains(status, "失败") || strings.Contains(liveText, "fail") || strings.Contains(liveText, "失败")
	default:
		return notificationEscalationConditionMatches(condition, alert)
	}
}

func notificationEscalationLeaseInterval() string {
	return fmt.Sprintf("%.0f seconds", notificationEscalationLease.Seconds())
}

func notificationEscalationStageFingerprint(stage map[string]interface{}) (string, error) {
	canonical, err := json.Marshal(stage)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

func mergeNotificationAlertPresentation(live, snapshot *notification.AlertInfo) {
	if live == nil || snapshot == nil {
		return
	}
	if live.Title == "" {
		live.Title = snapshot.Title
	}
	if live.Description == "" {
		live.Description = strings.Join(live.Labels, " ")
	}
	if live.Description == "" {
		live.Description = snapshot.Description
	}
	if live.AssetName == "" {
		live.AssetName = snapshot.AssetName
	}
	if live.AssetScope == "" {
		live.AssetScope = snapshot.AssetScope
	}
	if live.Campus == "" {
		live.Campus = snapshot.Campus
	}
	if live.ObjectType == "" {
		live.ObjectType = snapshot.ObjectType
	}
	if live.ObjectID == "" {
		live.ObjectID = snapshot.ObjectID
	}
	if live.ThreatIntel == "" {
		live.ThreatIntel = snapshot.ThreatIntel
	}
}

func (r *AdvancedRepository) startNotificationEscalationLeaseHeartbeat(ctx context.Context, cancel context.CancelFunc, job notificationEscalationJob) func() error {
	done := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(notificationEscalationLease / 3)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				done <- nil
				return
			case <-ticker.C:
				result, err := r.db.ExecContext(ctx, `UPDATE notification_escalation_jobs SET locked_at=now(),updated_at=now() WHERE job_id=$1 AND status='processing' AND lock_token=$2`, job.JobID, job.LockToken)
				if err != nil {
					if ctx.Err() != nil {
						done <- nil
						return
					}
					cancel()
					done <- fmt.Errorf("refresh notification escalation job %d lease: %w", job.JobID, err)
					return
				}
				updated, _ := result.RowsAffected()
				if updated != 1 {
					cancel()
					done <- fmt.Errorf("notification escalation job %d lease ownership was lost", job.JobID)
					return
				}
			}
		}
	}()
	return func() error {
		cancel()
		return <-done
	}
}

func (r *AdvancedRepository) completeNotificationEscalationJob(ctx context.Context, job notificationEscalationJob, status, errorMessage string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE notification_escalation_jobs SET status=$3,last_error=NULLIF($4,''),completed_at=now(),locked_at=NULL,lock_token='',updated_at=now() WHERE job_id=$1 AND status='processing' AND lock_token=$2`, job.JobID, job.LockToken, status, errorMessage)
	if err != nil {
		return err
	}
	updated, _ := result.RowsAffected()
	if updated != 1 {
		return fmt.Errorf("notification escalation job %d lease was lost before completion", job.JobID)
	}
	return nil
}

func (r *AdvancedRepository) releaseNotificationEscalationJob(ctx context.Context, job notificationEscalationJob, cause error) error {
	status := "pending"
	delay := time.Duration(job.Attempts+1) * 15 * time.Second
	if job.Attempts+1 >= notificationEscalationMaxAttempts {
		status = "failed"
		delay = 0
	}
	result, err := r.db.ExecContext(ctx, `UPDATE notification_escalation_jobs SET status=$3,last_error=$4,due_at=CASE WHEN $3='pending' THEN now()+$5::interval ELSE due_at END,completed_at=CASE WHEN $3='failed' THEN now() ELSE NULL END,locked_at=NULL,lock_token='',updated_at=now() WHERE job_id=$1 AND status='processing' AND lock_token=$2`, job.JobID, job.LockToken, status, cause.Error(), fmt.Sprintf("%.0f seconds", delay.Seconds()))
	if err != nil {
		return err
	}
	updated, _ := result.RowsAffected()
	if updated != 1 {
		return fmt.Errorf("notification escalation job %d lease was lost before retry release", job.JobID)
	}
	return nil
}

func (r *AdvancedRepository) notificationRoleDestinations(ctx context.Context, tenantID, roleName, channel string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT COALESCE(u.email,''),COALESCE((us.settings->$3)::text,'null') FROM users u
		JOIN user_roles ur ON ur.user_id=u.user_id
		JOIN roles role ON role.role_id=ur.role_id
		LEFT JOIN user_settings us ON us.user_id=u.user_id AND us.tenant_id=u.tenant_id AND us.category='notifications'
		WHERE u.tenant_id=$1 AND role.tenant_id=$1 AND u.status='active' AND role.name=$2
		ORDER BY u.email`, tenantID, roleName, channel)
	if err != nil {
		return nil, fmt.Errorf("resolve notification target role: %w", err)
	}
	defer rows.Close()
	destinations := make([]string, 0)
	seen := map[string]struct{}{}
	for rows.Next() {
		var email, configured string
		if err := rows.Scan(&email, &configured); err != nil {
			return nil, err
		}
		values := make([]string, 0)
		if channel == "email" {
			values = append(values, email)
		} else {
			var single string
			if err := json.Unmarshal([]byte(configured), &single); err == nil {
				values = append(values, single)
			} else {
				var many []string
				if err := json.Unmarshal([]byte(configured), &many); err == nil {
					values = append(values, many...)
				}
			}
		}
		for _, raw := range values {
			value := strings.TrimSpace(raw)
			if value == "" {
				continue
			}
			if _, exists := seen[value]; exists {
				continue
			}
			seen[value] = struct{}{}
			destinations = append(destinations, value)
		}
	}
	return destinations, rows.Err()
}
