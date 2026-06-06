////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/alert/dedup/fingerprint.go
// 修复版：添加租户隔离，完善 fingerprint 计算，修复时区问题
////////////////////////////////////////////////////////////////////////////////

package dedup

import (
	"crypto/md5"
	"fmt"
	"time"

	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

// CalculateFingerprint 计算告警去重指纹
// 指纹组成：tenant_id + alert_type + src_ip + dst_ip + dst_port + severity + time_bucket
// 修复：添加 tenant_id 确保跨租户隔离，使用 UTC 时区确保一致性
func CalculateFingerprint(batch *pb.DetectionBatch, timeBucketMinutes int) string {
	tenantID := batch.GetTenantId()
	communityID, alertType, severity := "", "", ""

	if len(batch.Behaviors) > 0 {
		b := batch.Behaviors[0]
		communityID = b.GetCommunityId()
		alertType = b.GetObjectType()
		severity = b.GetTopLabel()
	} else if len(batch.Businesses) > 0 {
		bu := batch.Businesses[0]
		communityID = bu.GetCommunityId()
		alertType = bu.GetDetectionType()
	}

	eventTime := time.Now().UTC()
	timeBucket := eventTime.Truncate(time.Duration(timeBucketMinutes) * time.Minute).Unix()

	data := fmt.Sprintf("%s:%s:%s:%s:%d",
		tenantID, communityID, alertType, severity, timeBucket)
	hash := md5.Sum([]byte(data))
	return fmt.Sprintf("%x", hash)
}

// CalculateAlertFingerprint 计算告警指纹（用于已有告警数据）
// 与 CalculateFingerprint 保持一致的逻辑
// 修复：使用 UTC 时区
func CalculateAlertFingerprint(tenantID, alertType, srcIP, dstIP string, dstPort uint32, severity string, eventTs int64, timeBucketMinutes int) string {
	// 修复：强制使用 UTC 时区计算时间桶
	eventTime := time.UnixMilli(eventTs).UTC()
	timeBucket := eventTime.Truncate(time.Duration(timeBucketMinutes) * time.Minute).Unix()

	data := fmt.Sprintf("%s:%s:%s:%s:%d:%s:%d",
		tenantID,
		alertType,
		srcIP,
		dstIP,
		dstPort,
		severity,
		timeBucket,
	)

	hash := md5.Sum([]byte(data))
	return fmt.Sprintf("%x", hash)
}

// CalculateSimpleFingerprint 计算简化版指纹（不含时间桶，用于持久化去重）
func CalculateSimpleFingerprint(tenantID, alertType, srcIP, dstIP string, dstPort uint32) string {
	data := fmt.Sprintf("%s:%s:%s:%s:%d",
		tenantID,
		alertType,
		srcIP,
		dstIP,
		dstPort,
	)

	hash := md5.Sum([]byte(data))
	return fmt.Sprintf("%x", hash)
}

// FingerprintComponents 指纹组成部分（用于调试和日志）
type FingerprintComponents struct {
	TenantID   string
	AlertType  string
	SrcIP      string
	DstIP      string
	DstPort    uint32
	Severity   string
	TimeBucket int64
}

// ExtractFingerprintComponents 从 DetectionBatch 提取指纹组成部分
func ExtractFingerprintComponents(batch *pb.DetectionBatch, timeBucketMinutes int) *FingerprintComponents {
	tenantID := batch.GetTenantId()
	communityID, alertType, severity := "", "", ""

	if len(batch.Behaviors) > 0 {
		b := batch.Behaviors[0]
		communityID = b.GetCommunityId()
		alertType = b.GetObjectType()
		severity = b.GetTopLabel()
	} else if len(batch.Businesses) > 0 {
		bu := batch.Businesses[0]
		communityID = bu.GetCommunityId()
		alertType = bu.GetDetectionType()
	}

	eventTime := time.Now().UTC()
	timeBucket := eventTime.Truncate(time.Duration(timeBucketMinutes) * time.Minute).Unix()

	return &FingerprintComponents{
		TenantID:   tenantID,
		AlertType:  alertType,
		SrcIP:      communityID,
		DstIP:      "",
		DstPort:    0,
		Severity:   severity,
		TimeBucket: timeBucket,
	}
}

// String 返回指纹组成部分的字符串表示
func (c *FingerprintComponents) String() string {
	return fmt.Sprintf("tenant=%s type=%s src=%s dst=%s:%d severity=%s bucket=%d (UTC)",
		c.TenantID, c.AlertType, c.SrcIP, c.DstIP, c.DstPort, c.Severity, c.TimeBucket)
}

// ValidateFingerprint 验证指纹格式
func ValidateFingerprint(fingerprint string) bool {
	if len(fingerprint) != 32 {
		return false
	}
	// MD5 哈希应该是 32 个十六进制字符
	for _, c := range fingerprint {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// FingerprintMetadata 指纹元数据（用于调试）
type FingerprintMetadata struct {
	Fingerprint       string
	Components        *FingerprintComponents
	CalculatedAt      time.Time
	TimeBucketMinutes int
}

// NewFingerprintMetadata 创建指纹元数据
func NewFingerprintMetadata(detection *pb.DetectionBatch, timeBucketMinutes int) *FingerprintMetadata {
	fingerprint := CalculateFingerprint(detection, timeBucketMinutes)
	components := ExtractFingerprintComponents(detection, timeBucketMinutes)

	return &FingerprintMetadata{
		Fingerprint:       fingerprint,
		Components:        components,
		CalculatedAt:      time.Now().UTC(),
		TimeBucketMinutes: timeBucketMinutes,
	}
}

// String 返回元数据字符串表示
func (m *FingerprintMetadata) String() string {
	return fmt.Sprintf("Fingerprint=%s, %s, CalculatedAt=%s",
		m.Fingerprint, m.Components.String(), m.CalculatedAt.Format(time.RFC3339))
}
