package restoration

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sort"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"
	"github.com/klauspost/compress/zstd"

	forensicsindex "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/index"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/reassembly"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/s3client"
)

type VerifiedObjectReader interface {
	ReadVerifiedObject(
		context.Context,
		string, string, string, string, string,
		int64, int64,
	) ([]byte, s3client.ObjectAuthority, error)
}

type SegmentLoadQuery struct {
	TenantID string
	ProbeID  string
	Tuple    FiveTuple
	Start    time.Time
	End      time.Time
}

type SegmentLoadLimits struct {
	MaxSourceBytes int64
	MaxPackets     uint64
}

type LoadedSegments struct {
	ClientToServer      []reassembly.Segment
	ServerToClient      []reassembly.Segment
	ClientToServerStart *uint32
	ClientToServerEnd   *uint32
	ServerToClientStart *uint32
	ServerToClientEnd   *uint32
	ConnectionReset     bool
	SourceReceipts      []s3client.ObjectAuthority
	PacketRanges        []PacketRange
	PcapIndexIDs        []string
}

func recordTCPBoundary(target **uint32, value uint32) error {
	if *target != nil && **target != value {
		return errors.New("conflicting TCP connection boundary sequence")
	}
	if *target == nil {
		copyValue := value
		*target = &copyValue
	}
	return nil
}

func (query SegmentLoadQuery) Validate() error {
	if query.TenantID == "" || query.ProbeID == "" || query.Tuple.Protocol != uint8(layers.IPProtocolTCP) {
		return errors.New("segment loader requires tenant, probe and TCP five tuple")
	}
	if net.ParseIP(query.Tuple.SourceIP) == nil || net.ParseIP(query.Tuple.DestinationIP) == nil ||
		query.Tuple.SourcePort == 0 || query.Tuple.DestinationPort == 0 {
		return errors.New("segment loader five tuple is invalid")
	}
	if query.Start.IsZero() || !query.End.After(query.Start) {
		return errors.New("segment loader time window is invalid")
	}
	return nil
}

func (limits SegmentLoadLimits) Validate() error {
	if limits.MaxSourceBytes <= 0 || limits.MaxPackets == 0 {
		return errors.New("segment loader limits must be positive")
	}
	return nil
}

func decodePCAPSource(stored []byte, source forensicsindex.RestorationSource, remaining int64) ([]byte, error) {
	if remaining <= 0 || uint64(remaining) < source.OriginalSize {
		return nil, errors.New("decoded PCAP source exceeds cumulative max_source_bytes")
	}
	if source.Compression == "none" {
		if uint64(len(stored)) != source.OriginalSize {
			return nil, errors.New("uncompressed PCAP length differs from manifest authority")
		}
		return bytes.Clone(stored), nil
	}
	decoder, err := zstd.NewReader(bytes.NewReader(stored), zstd.WithDecoderMaxMemory(uint64(remaining)))
	if err != nil {
		return nil, fmt.Errorf("create bounded zstd PCAP decoder: %w", err)
	}
	defer decoder.Close()
	decoded, err := io.ReadAll(io.LimitReader(decoder, remaining+1))
	if err != nil {
		return nil, fmt.Errorf("decode zstd PCAP source: %w", err)
	}
	if int64(len(decoded)) > remaining || uint64(len(decoded)) != source.OriginalSize {
		return nil, errors.New("decoded PCAP length differs from manifest authority")
	}
	return decoded, nil
}

func packetEndpoints(packet gopacket.Packet) (net.IP, net.IP, *layers.TCP, bool) {
	var sourceIP, destinationIP net.IP
	if layer := packet.Layer(layers.LayerTypeIPv4); layer != nil {
		ipv4 := layer.(*layers.IPv4)
		sourceIP, destinationIP = ipv4.SrcIP, ipv4.DstIP
	} else if layer := packet.Layer(layers.LayerTypeIPv6); layer != nil {
		ipv6 := layer.(*layers.IPv6)
		sourceIP, destinationIP = ipv6.SrcIP, ipv6.DstIP
	} else {
		return nil, nil, nil, false
	}
	tcpLayer := packet.Layer(layers.LayerTypeTCP)
	if tcpLayer == nil {
		return nil, nil, nil, false
	}
	return sourceIP, destinationIP, tcpLayer.(*layers.TCP), true
}

func tupleDirection(query SegmentLoadQuery, sourceIP, destinationIP net.IP, tcp *layers.TCP) (bool, bool) {
	wantSource := net.ParseIP(query.Tuple.SourceIP)
	wantDestination := net.ParseIP(query.Tuple.DestinationIP)
	direct := sourceIP.Equal(wantSource) && destinationIP.Equal(wantDestination) &&
		uint16(tcp.SrcPort) == query.Tuple.SourcePort && uint16(tcp.DstPort) == query.Tuple.DestinationPort
	reverse := sourceIP.Equal(wantDestination) && destinationIP.Equal(wantSource) &&
		uint16(tcp.SrcPort) == query.Tuple.DestinationPort && uint16(tcp.DstPort) == query.Tuple.SourcePort
	return direct, reverse
}

func sourcePacketRange(source forensicsindex.RestorationSource) PacketRange {
	if source.OffsetStart != nil {
		return PacketRange{ObjectBucket: source.Bucket, ObjectKey: source.FileKey, ObjectVersion: source.ObjectVersion, Start: *source.OffsetStart, End: *source.OffsetEnd}
	}
	return PacketRange{ObjectBucket: source.Bucket, ObjectKey: source.FileKey, ObjectVersion: source.ObjectVersion, Start: 0, End: source.StoredSize}
}

// LoadVerifiedSegments reads only exact immutable object versions selected by
// the manifest-v2 index. It maps every emitted TCP payload back to a packet
// identity and a conservative source-object byte range; compressed objects are
// never represented as fictitious decompressed offsets.
func LoadVerifiedSegments(
	ctx context.Context,
	reader VerifiedObjectReader,
	sources []forensicsindex.RestorationSource,
	query SegmentLoadQuery,
	limits SegmentLoadLimits,
) (LoadedSegments, error) {
	if reader == nil {
		return LoadedSegments{}, errors.New("verified source object reader is required")
	}
	if err := query.Validate(); err != nil {
		return LoadedSegments{}, err
	}
	if err := limits.Validate(); err != nil {
		return LoadedSegments{}, err
	}
	if len(sources) == 0 {
		return LoadedSegments{}, errors.New("at least one manifest-v2 source is required")
	}
	result := LoadedSegments{}
	seen := make(map[string]struct{}, len(sources))
	var decodedTotal int64
	var packetIndex uint64
	for sourcePosition, source := range sources {
		if err := source.Validate(query.TenantID, query.ProbeID); err != nil {
			return LoadedSegments{}, fmt.Errorf("invalid source %d: %w", sourcePosition, err)
		}
		if _, duplicate := seen[source.ProjectionIdentity]; duplicate {
			return LoadedSegments{}, errors.New("duplicate PCAP projection identity")
		}
		seen[source.ProjectionIdentity] = struct{}{}
		if sourcePosition > 0 {
			prior := sources[sourcePosition-1]
			if source.TsStart.Before(prior.TsStart) || (source.TsStart.Equal(prior.TsStart) && source.ProjectionIdentity <= prior.ProjectionIdentity) {
				return LoadedSegments{}, errors.New("PCAP sources must retain canonical index order")
			}
		}
		if source.OriginalSize > uint64(limits.MaxSourceBytes) || source.StoredSize > uint64(limits.MaxSourceBytes) ||
			decodedTotal > limits.MaxSourceBytes-int64(source.OriginalSize) {
			return LoadedSegments{}, errors.New("PCAP sources exceed cumulative max_source_bytes")
		}
		stored, receipt, err := reader.ReadVerifiedObject(ctx, source.Bucket, source.FileKey, source.ObjectVersion,
			source.ETag, source.SHA256, int64(source.StoredSize), limits.MaxSourceBytes)
		if err != nil {
			return LoadedSegments{}, fmt.Errorf("read source %s: %w", source.ProjectionIdentity, err)
		}
		decoded, err := decodePCAPSource(stored, source, limits.MaxSourceBytes-decodedTotal)
		if err != nil {
			return LoadedSegments{}, fmt.Errorf("decode source %s: %w", source.ProjectionIdentity, err)
		}
		decodedTotal += int64(len(decoded))
		pcapReader, err := pcapgo.NewReader(bytes.NewReader(decoded))
		if err != nil {
			return LoadedSegments{}, fmt.Errorf("open source PCAP %s: %w", source.ProjectionIdentity, err)
		}
		objectRange := sourcePacketRange(source)
		decodedOffset := uint64(24)
		sourceMatched := false
		sourcePacketRanges := make([]PacketRange, 0)
		for {
			data, captureInfo, readErr := pcapReader.ReadPacketData()
			if errors.Is(readErr, io.EOF) {
				break
			}
			if readErr != nil {
				return LoadedSegments{}, fmt.Errorf("read source PCAP packet: %w", readErr)
			}
			recordStart := decodedOffset
			recordEnd := decodedOffset + 16 + uint64(captureInfo.CaptureLength)
			decodedOffset = recordEnd
			packetIndex++
			if packetIndex > limits.MaxPackets {
				return LoadedSegments{}, errors.New("PCAP packet count exceeds max_packets")
			}
			if captureInfo.Timestamp.Before(query.Start) || captureInfo.Timestamp.After(query.End) {
				continue
			}
			packet := gopacket.NewPacket(data, pcapReader.LinkType(), gopacket.DecodeOptions{Lazy: true, NoCopy: true})
			sourceIP, destinationIP, tcp, ok := packetEndpoints(packet)
			if !ok {
				continue
			}
			direct, reverse := tupleDirection(query, sourceIP, destinationIP, tcp)
			if !direct && !reverse {
				continue
			}
			sourceMatched = true
			if source.Compression == "none" {
				proofStart := objectRange.Start + recordStart
				proofEnd := objectRange.Start + recordEnd
				if proofEnd > objectRange.End {
					return LoadedSegments{}, errors.New("PCAP packet record exceeds authorized object range")
				}
				sourcePacketRanges = append(sourcePacketRanges, PacketRange{
					ObjectBucket: source.Bucket, ObjectKey: source.FileKey, ObjectVersion: source.ObjectVersion,
					Start: proofStart, End: proofEnd,
				})
			}
			sequence := tcp.Seq
			if tcp.SYN {
				sequence++
				if direct {
					if err := recordTCPBoundary(&result.ClientToServerStart, sequence); err != nil {
						return LoadedSegments{}, err
					}
				} else if err := recordTCPBoundary(&result.ServerToClientStart, sequence); err != nil {
					return LoadedSegments{}, err
				}
			}
			if tcp.FIN {
				endSequence := sequence + uint32(len(tcp.Payload))
				if direct {
					if err := recordTCPBoundary(&result.ClientToServerEnd, endSequence); err != nil {
						return LoadedSegments{}, err
					}
				} else if err := recordTCPBoundary(&result.ServerToClientEnd, endSequence); err != nil {
					return LoadedSegments{}, err
				}
			}
			result.ConnectionReset = result.ConnectionReset || tcp.RST
			if len(tcp.Payload) == 0 {
				continue
			}
			proofStart, proofEnd, proofExact := objectRange.Start, objectRange.End, false
			if source.Compression == "none" {
				proofStart = objectRange.Start + recordStart
				proofEnd = objectRange.Start + recordEnd
				proofExact = true
			}
			segment := reassembly.Segment{
				Sequence: sequence, Payload: tcp.Payload, CapturedLength: captureInfo.CaptureLength,
				OriginalLength: captureInfo.Length, PacketIndex: packetIndex, CapturedAt: captureInfo.Timestamp,
				ObjectBucket: source.Bucket, ObjectKey: source.FileKey, ObjectVersion: source.ObjectVersion,
				ObjectRangeStart: proofStart, ObjectRangeEnd: proofEnd, ObjectRangeExact: proofExact,
			}
			if direct {
				result.ClientToServer = append(result.ClientToServer, segment)
			} else {
				result.ServerToClient = append(result.ServerToClient, segment)
			}
		}
		if sourceMatched {
			result.SourceReceipts = append(result.SourceReceipts, receipt)
			if source.Compression == "none" {
				result.PacketRanges = append(result.PacketRanges, sourcePacketRanges...)
			} else {
				result.PacketRanges = append(result.PacketRanges, objectRange)
			}
			result.PcapIndexIDs = append(result.PcapIndexIDs, source.ProjectionIdentity)
		}
	}
	if len(result.ClientToServer) == 0 && len(result.ServerToClient) == 0 &&
		result.ClientToServerStart == nil && result.ServerToClientStart == nil {
		return LoadedSegments{}, errors.New("no TCP payload matched the authorized five tuple and time window")
	}
	sort.Slice(result.PacketRanges, func(i, j int) bool {
		if result.PacketRanges[i].ObjectBucket != result.PacketRanges[j].ObjectBucket {
			return result.PacketRanges[i].ObjectBucket < result.PacketRanges[j].ObjectBucket
		}
		if result.PacketRanges[i].ObjectKey == result.PacketRanges[j].ObjectKey {
			if result.PacketRanges[i].ObjectVersion != result.PacketRanges[j].ObjectVersion {
				return result.PacketRanges[i].ObjectVersion < result.PacketRanges[j].ObjectVersion
			}
			return result.PacketRanges[i].Start < result.PacketRanges[j].Start
		}
		return result.PacketRanges[i].ObjectKey < result.PacketRanges[j].ObjectKey
	})
	sort.Strings(result.PcapIndexIDs)
	return result, nil
}
