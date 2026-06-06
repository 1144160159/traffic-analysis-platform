package protobuf

import (
	"fmt"
	"math"

	"go.uber.org/zap"
)

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

func ValidatePortNumber(port uint32, fieldName string) error {
	if port > 65535 {
		return fmt.Errorf("%s exceeds valid port range (0-65535): %d", fieldName, port)
	}
	return nil
}

func ValidateProtocolNumber(protocol uint32, fieldName string) error {
	if protocol > 255 {
		return fmt.Errorf("%s exceeds valid protocol range (0-255): %d", fieldName, protocol)
	}
	return nil
}

func TruncateTCPFlags(flags uint32) uint16 {

	return uint16(flags & 0xFFFF)
}

func TruncateTOS(tos uint32) uint8 {

	return uint8(tos & 0xFF)
}

func ExtractIPv6FlowLabel(tos uint32) uint32 {

	return (tos >> 8) & 0xFFFFF
}

type PacketSizeStats struct {
	Min  uint16
	Max  uint16
	Mean float32
	Std  float32
}

func NewPacketSizeStatsFromProto(min, max uint32, mean, std float32, logger *zap.Logger) PacketSizeStats {
	return PacketSizeStats{
		Min:  SafeUint32ToUint16(min, "pktlen_min", logger),
		Max:  SafeUint32ToUint16(max, "pktlen_max", logger),
		Mean: mean,
		Std:  std,
	}
}

type FiveTuple struct {
	SrcIP    string
	DstIP    string
	SrcPort  uint16
	DstPort  uint16
	Protocol uint8
}

func NewFiveTupleFromProto(srcIP, dstIP string, srcPort, dstPort, protocol uint32, logger *zap.Logger) FiveTuple {
	return FiveTuple{
		SrcIP:    srcIP,
		DstIP:    dstIP,
		SrcPort:  SafeUint32ToUint16(srcPort, "src_port", logger),
		DstPort:  SafeUint32ToUint16(dstPort, "dst_port", logger),
		Protocol: SafeUint32ToUint8(protocol, "protocol", logger),
	}
}

type ProtoMetadata struct {
	ContentType        string
	MessageType        string
	SchemaVersion      string
	Package            string
	SerializationCodec string
}

func NewProtoMetadata(messageType, schemaVersion string) ProtoMetadata {
	return ProtoMetadata{
		ContentType:        "application/x-protobuf",
		MessageType:        messageType,
		SchemaVersion:      schemaVersion,
		Package:            extractPackage(messageType),
		SerializationCodec: "protobuf",
	}
}

func (m ProtoMetadata) ToHeaders() map[string]string {
	return map[string]string{
		"content_type":         m.ContentType,
		"proto_message_type":   m.MessageType,
		"proto_schema_version": m.SchemaVersion,
		"proto_package":        m.Package,
		"serialization_codec":  m.SerializationCodec,
	}
}

func extractPackage(messageType string) string {
	for i := len(messageType) - 1; i >= 0; i-- {
		if messageType[i] == '.' {
			return messageType[:i]
		}
	}
	return ""
}

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

func GetMetadata(messageTypeName string) (ProtoMetadata, bool) {
	meta, ok := MessageTypeRegistry[messageTypeName]
	return meta, ok
}
