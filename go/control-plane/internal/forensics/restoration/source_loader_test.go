package restoration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"

	forensicsindex "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/index"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/s3client"
)

type fakeVerifiedObjectReader struct {
	body    []byte
	receipt s3client.ObjectAuthority
	err     error
}

func (reader fakeVerifiedObjectReader) ReadVerifiedObject(
	_ context.Context,
	_, _, _, _, _ string,
	_, _ int64,
) ([]byte, s3client.ObjectAuthority, error) {
	if reader.err != nil {
		return nil, s3client.ObjectAuthority{}, reader.err
	}
	return bytes.Clone(reader.body), reader.receipt, nil
}

func sha256String(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func serializeTCPPacket(t *testing.T, timestamp time.Time, sourceIP, destinationIP [4]byte, sourcePort, destinationPort uint16, sequence uint32, payload []byte, fin bool) (gopacket.CaptureInfo, []byte) {
	t.Helper()
	ethernet := &layers.Ethernet{
		SrcMAC: []byte{0, 1, 2, 3, 4, 5}, DstMAC: []byte{6, 7, 8, 9, 10, 11}, EthernetType: layers.EthernetTypeIPv4,
	}
	ipv4 := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: sourceIP[:], DstIP: destinationIP[:]}
	tcp := &layers.TCP{SrcPort: layers.TCPPort(sourcePort), DstPort: layers.TCPPort(destinationPort), Seq: sequence, ACK: true, PSH: len(payload) > 0, FIN: fin}
	if err := tcp.SetNetworkLayerForChecksum(ipv4); err != nil {
		t.Fatal(err)
	}
	buffer := gopacket.NewSerializeBuffer()
	if err := gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, ethernet, ipv4, tcp, gopacket.Payload(payload)); err != nil {
		t.Fatal(err)
	}
	data := buffer.Bytes()
	return gopacket.CaptureInfo{Timestamp: timestamp, CaptureLength: len(data), Length: len(data)}, data
}

func restorationPCAP(t *testing.T, now time.Time) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := pcapgo.NewWriter(&buffer)
	if err := writer.WriteFileHeader(65535, layers.LinkTypeEthernet); err != nil {
		t.Fatal(err)
	}
	client := [4]byte{192, 0, 2, 1}
	server := [4]byte{198, 51, 100, 2}
	packets := []struct {
		capture gopacket.CaptureInfo
		data    []byte
	}{
		func() struct {
			capture gopacket.CaptureInfo
			data    []byte
		} {
			ci, data := serializeTCPPacket(t, now, client, server, 51000, 80, 100, []byte("GET / HTTP/1.1\r\n\r\n"), false)
			return struct {
				capture gopacket.CaptureInfo
				data    []byte
			}{ci, data}
		}(),
		func() struct {
			capture gopacket.CaptureInfo
			data    []byte
		} {
			ci, data := serializeTCPPacket(t, now.Add(time.Millisecond), server, client, 80, 51000, 500, []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nOK"), false)
			return struct {
				capture gopacket.CaptureInfo
				data    []byte
			}{ci, data}
		}(),
		func() struct {
			capture gopacket.CaptureInfo
			data    []byte
		} {
			ci, data := serializeTCPPacket(t, now.Add(2*time.Millisecond), server, client, 80, 51000, 545, nil, true)
			return struct {
				capture gopacket.CaptureInfo
				data    []byte
			}{ci, data}
		}(),
	}
	for _, packet := range packets {
		if err := writer.WritePacket(packet.capture, packet.data); err != nil {
			t.Fatal(err)
		}
	}
	return buffer.Bytes()
}

func TestLoadVerifiedSegmentsSeparatesDirectionsAndRetainsAuthority(t *testing.T) {
	now := time.Date(2026, 8, 14, 13, 0, 0, 0, time.UTC)
	pcap := restorationPCAP(t, now)
	digest := sha256String(pcap)
	source := forensicsindex.RestorationSource{
		TenantID: "tenant-a", ProbeID: "probe-a", ProjectionIdentity: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		FileKey: "tenant-a/probe-a/source.pcap", Bucket: "pcap", ObjectVersion: "version-1", ETag: "etag-1", SHA256: digest,
		OriginalSize: uint64(len(pcap)), StoredSize: uint64(len(pcap)), Compression: "none", ManifestVersion: 2,
		CommunityID: "1:test", FlowID: "flow-a",
		TsStart: now.Add(-time.Second), TsEnd: now.Add(time.Second),
	}
	receipt := s3client.ObjectAuthority{
		Bucket: source.Bucket, Key: source.FileKey, VersionID: source.ObjectVersion, ETag: source.ETag,
		SizeBytes: int64(len(pcap)), SHA256: digest, ObservedAt: now,
	}
	loaded, err := LoadVerifiedSegments(context.Background(), fakeVerifiedObjectReader{body: pcap, receipt: receipt},
		[]forensicsindex.RestorationSource{source}, SegmentLoadQuery{
			TenantID: "tenant-a", ProbeID: "probe-a", Tuple: FiveTuple{SourceIP: "192.0.2.1", DestinationIP: "198.51.100.2", SourcePort: 51000, DestinationPort: 80, Protocol: 6},
			Start: now.Add(-time.Second), End: now.Add(time.Second),
		}, SegmentLoadLimits{MaxSourceBytes: 1 << 20, MaxPackets: 10})
	if err != nil {
		t.Fatalf("LoadVerifiedSegments() error = %v", err)
	}
	if len(loaded.ClientToServer) != 1 || len(loaded.ServerToClient) != 1 {
		t.Fatalf("directional segment counts = %d/%d", len(loaded.ClientToServer), len(loaded.ServerToClient))
	}
	if string(loaded.ServerToClient[0].Payload) != "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nOK" || loaded.ServerToClientEnd == nil || loaded.ConnectionReset {
		t.Fatalf("server stream or close evidence missing: %+v", loaded)
	}
	if len(loaded.SourceReceipts) != 1 || loaded.SourceReceipts[0].VersionID != "version-1" || len(loaded.PacketRanges) != 3 {
		t.Fatalf("source authority missing: %+v", loaded)
	}
	for _, packetRange := range loaded.PacketRanges {
		if packetRange.Start == 0 || packetRange.End > uint64(len(pcap)) || packetRange.End <= packetRange.Start {
			t.Fatalf("uncompressed packet proof is not an exact PCAP record range: %+v", packetRange)
		}
	}
}

func TestLoadVerifiedSegmentsRejectsReaderAuthorityFailure(t *testing.T) {
	now := time.Date(2026, 8, 14, 13, 0, 0, 0, time.UTC)
	pcap := restorationPCAP(t, now)
	source := forensicsindex.RestorationSource{
		TenantID: "tenant-a", ProbeID: "probe-a", ProjectionIdentity: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		FileKey: "tenant-a/probe-a/source.pcap", Bucket: "pcap", ObjectVersion: "version-1", ETag: "etag-1",
		SHA256: sha256String(pcap), OriginalSize: uint64(len(pcap)), StoredSize: uint64(len(pcap)), Compression: "none", ManifestVersion: 2,
		CommunityID: "1:test", FlowID: "flow-a",
		TsStart: now.Add(-time.Second), TsEnd: now.Add(time.Second),
	}
	_, err := LoadVerifiedSegments(context.Background(), fakeVerifiedObjectReader{err: errors.New("HEAD differs from index authority")},
		[]forensicsindex.RestorationSource{source}, SegmentLoadQuery{
			TenantID: "tenant-a", ProbeID: "probe-a", Tuple: FiveTuple{SourceIP: "192.0.2.1", DestinationIP: "198.51.100.2", SourcePort: 51000, DestinationPort: 80, Protocol: 6},
			Start: now.Add(-time.Second), End: now.Add(time.Second),
		}, SegmentLoadLimits{MaxSourceBytes: 1 << 20, MaxPackets: 10})
	if err == nil || !errors.Is(err, context.Canceled) && err.Error() != "read source aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa: HEAD differs from index authority" {
		t.Fatalf("LoadVerifiedSegments() error = %v", err)
	}
}
