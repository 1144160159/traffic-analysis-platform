// //////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/common/protobuf/conversion.go
// 新增：Protobuf 到 ClickHouse 类型安全转换工具
// //////////////////////////////////////////////////////////////////////////////
package protobuf

import (
	"fmt"
	"math"

	"go.uber.org/zap"
)

// SafeUint32ToUint16 安全转换 uint32 到 uint16（带溢出检查）
func SafeUint32ToUint16(value uint32, fieldName string, logger *zap.Logger) uint16 {
	if value > math.MaxUint16 {
		if logger != nil {
			logger.Warn("Value exceeds UInt16 range, truncating",
				zap.String("field", fieldName),
				zap.Uint32("original", value),
				zap.Uint16("truncated", uint16(value)))
		}
	}
	return uint16(value)
}

// SafeUint32ToUint8 安全转换 uint32 到 uint8（带溢出检查）
func SafeUint32ToUint8(value uint32, fieldName string, logger *zap.Logger) uint8 {
	if value > math.MaxUint8 {
		if logger != nil {
			logger.Warn("Value exceeds UInt8 range, truncating",
				zap.String("field", fieldName),
				zap.Uint32("original", value),
				zap.Uint8("truncated", uint8(value)))
		}
	}
	return uint8(value)
}

// ValidatePortNumber 验证端口号（应在 0-65535 范围内）
func ValidatePortNumber(port uint32, fieldName string) error {
	if port > 65535 {
		return fmt.Errorf("%s exceeds valid port range (0-65535): %d", fieldName, port)
	}
	return nil
}

// ValidateProtocolNumber 验证协议号（应在 0-255 范围内）
func ValidateProtocolNumber(protocol uint32, fieldName string) error {
	if protocol > 255 {
		return fmt.Errorf("%s exceeds valid protocol range (0-255): %d", fieldName, protocol)
	}
	return nil
}

// TruncateTCPFlags 截断 TCP 标志位到 16 位
func TruncateTCPFlags(flags uint32) uint16 {
	// TCP 标准标志位只有 9 个（FIN, SYN, RST, PSH, ACK, URG, ECE, CWR, NS）
	// 保留低 16 位足够
	return uint16(flags & 0xFFFF)
}

// TruncateTOS 截断 TOS/DSCP 到 8 位
func TruncateTOS(tos uint32) uint8 {
	// TOS/DSCP 字段是 8 位
	// 如果包含 IPv6 Flow Label，需要单独处理
	return uint8(tos & 0xFF)
}

// ExtractIPv6FlowLabel 从 TOS 字段提取 IPv6 Flow Label（如果存在）
func ExtractIPv6FlowLabel(tos uint32) uint32 {
	// IPv6 Flow Label 是 20 位
	// 假设存储在 bits 8-27
	return (tos >> 8) & 0xFFFFF
}

// PacketSizeStats 包大小统计（安全类型）
type PacketSizeStats struct {
	Min  uint16
	Max  uint16
	Mean float32
	Std  float32
}

// NewPacketSizeStatsFromProto 从 Protobuf 创建包大小统计（带类型转换）
func NewPacketSizeStatsFromProto(min, max uint32, mean, std float32, logger *zap.Logger) PacketSizeStats {
	return PacketSizeStats{
		Min:  SafeUint32ToUint16(min, "pktlen_min", logger),
		Max:  SafeUint32ToUint16(max, "pktlen_max", logger),
		Mean: mean,
		Std:  std,
	}
}

// FiveTuple 五元组（ClickHouse 安全类型）
type FiveTuple struct {
	SrcIP    string
	DstIP    string
	SrcPort  uint16
	DstPort  uint16
	Protocol uint8
}

// NewFiveTupleFromProto 从 Protobuf 创建五元组（带类型转换）
func NewFiveTupleFromProto(srcIP, dstIP string, srcPort, dstPort, protocol uint32, logger *zap.Logger) FiveTuple {
	return FiveTuple{
		SrcIP:    srcIP,
		DstIP:    dstIP,
		SrcPort:  SafeUint32ToUint16(srcPort, "src_port", logger),
		DstPort:  SafeUint32ToUint16(dstPort, "dst_port", logger),
		Protocol: SafeUint32ToUint8(protocol, "protocol", logger),
	}
}

// ProtoMetadata Protobuf 元数据
type ProtoMetadata struct {
	ContentType        string
	MessageType        string
	SchemaVersion      string
	Package            string
	SerializationCodec string // "protobuf", "json", "avro"
}

// NewProtoMetadata 创建 Protobuf 元数据
func NewProtoMetadata(messageType, schemaVersion string) ProtoMetadata {
	return ProtoMetadata{
		ContentType:        "application/x-protobuf",
		MessageType:        messageType,
		SchemaVersion:      schemaVersion,
		Package:            extractPackage(messageType),
		SerializationCodec: "protobuf",
	}
}

// ToHeaders 转换为 Kafka Headers map
func (m ProtoMetadata) ToHeaders() map[string]string {
	return map[string]string{
		"content_type":         m.ContentType,
		"proto_message_type":   m.MessageType,
		"proto_schema_version": m.SchemaVersion,
		"proto_package":        m.Package,
		"serialization_codec":  m.SerializationCodec,
	}
}

// extractPackage 从消息类型提取包名
// 示例：traffic.v1.FlowEvent -> traffic.v1
func extractPackage(messageType string) string {
	for i := len(messageType) - 1; i >= 0; i-- {
		if messageType[i] == '.' {
			return messageType[:i]
		}
	}
	return ""
}

// MessageTypeRegistry 消息类型注册表（用于自动生成元数据）
var MessageTypeRegistry = map[string]ProtoMetadata{
	"FlowEvent":          NewProtoMetadata("traffic.v1.FlowEvent", "v1"),
	"SessionEvent":       NewProtoMetadata("traffic.v1.SessionEvent", "v1"),
	"PcapIndexMeta":      NewProtoMetadata("traffic.v1.PcapIndexMeta", "v1"),
	"FeatureStat":        NewProtoMetadata("traffic.v1.FeatureStat", "v1"),
	"FeatureSeq":         NewProtoMetadata("traffic.v1.FeatureSeq", "v1"),
	"FeatureFingerprint": NewProtoMetadata("traffic.v1.FeatureFingerprint", "v1"),
	"DetectionBehavior":  NewProtoMetadata("traffic.v1.DetectionBehavior", "v1"),
	"DetectionBusiness":  NewProtoMetadata("traffic.v1.DetectionBusiness", "v1"),
	"Alert":              NewProtoMetadata("traffic.v1.Alert", "v1"),
	"Evidence":           NewProtoMetadata("traffic.v1.Evidence", "v1"),
	"Campaign":           NewProtoMetadata("traffic.v1.Campaign", "v1"),
	"RunReport":          NewProtoMetadata("traffic.v1.RunReport", "v1"),
}

// GetMetadata 获取消息类型元数据
func GetMetadata(messageTypeName string) (ProtoMetadata, bool) {
	meta, ok := MessageTypeRegistry[messageTypeName]
	return meta, ok
}
