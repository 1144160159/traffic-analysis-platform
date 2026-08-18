package cutter

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
	"go.uber.org/zap"

	forensicsindex "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/index"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/s3client"
)

type fakeVerifiedCutIndex struct {
	sources []forensicsindex.RestorationSource
	err     error
}

func (index fakeVerifiedCutIndex) LookupVerifiedCutSources(context.Context, forensicsindex.VerifiedCutSourceQuery) ([]forensicsindex.RestorationSource, error) {
	return index.sources, index.err
}

type fakeVerifiedCutReader struct {
	body  []byte
	err   error
	reads int
}

func (reader *fakeVerifiedCutReader) ReadVerifiedObject(
	context.Context, string, string, string, string, string, int64, int64,
) ([]byte, s3client.ObjectAuthority, error) {
	reader.reads++
	if reader.err != nil {
		return nil, s3client.ObjectAuthority{}, reader.err
	}
	digest := sha256.Sum256(reader.body)
	return append([]byte(nil), reader.body...), s3client.ObjectAuthority{
		Bucket: "pcap", Key: "tenant-a/probe-a/source.pcap", VersionID: "version-1", ETag: "etag-1",
		SizeBytes: int64(len(reader.body)), SHA256: hex.EncodeToString(digest[:]), ObservedAt: time.Now().UTC(),
	}, nil
}

func versionedCutPCAP(t *testing.T, timestamp time.Time) []byte {
	t.Helper()
	packetBuffer := gopacket.NewSerializeBuffer()
	ipv4 := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: []byte{192, 0, 2, 1}, DstIP: []byte{198, 51, 100, 2}}
	tcp := &layers.TCP{SrcPort: 51000, DstPort: 80, Seq: 1, SYN: true}
	if err := tcp.SetNetworkLayerForChecksum(ipv4); err != nil {
		t.Fatal(err)
	}
	if err := gopacket.SerializeLayers(packetBuffer, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true},
		&layers.Ethernet{SrcMAC: []byte{0, 1, 2, 3, 4, 5}, DstMAC: []byte{6, 7, 8, 9, 10, 11}, EthernetType: layers.EthernetTypeIPv4},
		ipv4, tcp,
		gopacket.Payload([]byte("evidence")),
	); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	writer := pcapgo.NewWriter(&output)
	if err := writer.WriteFileHeader(65535, layers.LinkTypeEthernet); err != nil {
		t.Fatal(err)
	}
	packet := packetBuffer.Bytes()
	if err := writer.WritePacket(gopacket.CaptureInfo{Timestamp: timestamp, CaptureLength: len(packet), Length: len(packet)}, packet); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func validVerifiedCutSource(body []byte, now time.Time) forensicsindex.RestorationSource {
	digest := sha256.Sum256(body)
	return forensicsindex.RestorationSource{
		TenantID: "tenant-a", ProbeID: "probe-a", ProjectionIdentity: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		FileKey: "tenant-a/probe-a/source.pcap", Bucket: "pcap", ObjectVersion: "version-1", ETag: "etag-1",
		SHA256: hex.EncodeToString(digest[:]), OriginalSize: uint64(len(body)), StoredSize: uint64(len(body)),
		Compression: "none", ManifestVersion: 2, CommunityID: "1:test", FlowID: "flow-a",
		TsStart: now.Add(-time.Minute), TsEnd: now.Add(time.Minute),
	}
}

func TestCutPCAPVersionedReadsExactAuthorityAndReturnsReceipts(t *testing.T) {
	now := time.Now().UTC()
	body := versionedCutPCAP(t, now)
	reader := &fakeVerifiedCutReader{body: body}
	worker := &Cutter{
		verifiedIndex:   fakeVerifiedCutIndex{sources: []forensicsindex.RestorationSource{validVerifiedCutSource(body, now)}},
		verifiedObjects: reader, maxPackets: 100, bufferSize: 64 << 10, logger: zap.NewNop(),
	}
	var output bytes.Buffer
	result, err := worker.CutPCAPVersioned(context.Background(), &CutQuery{
		TenantID: "tenant-a", ProbeID: "probe-a", SrcIP: "192.0.2.1", DstIP: "198.51.100.2",
		SrcPort: 51000, DstPort: 80, Protocol: 6, CommunityID: "1:test",
		StartTime: now.Add(-time.Second).UnixMilli(), EndTime: now.Add(time.Second).UnixMilli(),
	}, []string{"probe-a"}, VerifiedCutLimits{MaxSourceObjects: 10, MaxSourceBytes: 1 << 20, SourceRetention: 90 * 24 * time.Hour}, &output, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalPackets != 1 || result.FilesScanned != 1 || len(result.PcapIndexIDs) != 1 || len(result.SourceReceipts) != 1 || reader.reads != 1 {
		t.Fatalf("unexpected verified cut result: %+v reads=%d", result, reader.reads)
	}
	pcapReader, err := pcapgo.NewReader(bytes.NewReader(output.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := pcapReader.ReadPacketData(); err != nil {
		t.Fatalf("published PCAP is unreadable: %v", err)
	}
}

func TestCutPCAPVersionedFailsClosedForBadHashExpiredAndCrossTenantSource(t *testing.T) {
	now := time.Now().UTC()
	body := versionedCutPCAP(t, now)
	query := &CutQuery{TenantID: "tenant-a", StartTime: now.Add(-time.Second).UnixMilli(), EndTime: now.Add(time.Second).UnixMilli()}
	limits := VerifiedCutLimits{MaxSourceObjects: 10, MaxSourceBytes: 1 << 20, SourceRetention: 90 * 24 * time.Hour}
	for _, test := range []struct {
		name   string
		source forensicsindex.RestorationSource
		reader *fakeVerifiedCutReader
	}{
		{"bad-hash", validVerifiedCutSource(body, now), &fakeVerifiedCutReader{err: errors.New("source object SHA-256 differs from authority")}},
		{"expired", func() forensicsindex.RestorationSource {
			value := validVerifiedCutSource(body, now)
			value.TsStart = now.Add(-91 * 24 * time.Hour)
			return value
		}(), &fakeVerifiedCutReader{body: body}},
		{"cross-tenant", func() forensicsindex.RestorationSource {
			value := validVerifiedCutSource(body, now)
			value.TenantID = "tenant-b"
			return value
		}(), &fakeVerifiedCutReader{body: body}},
	} {
		t.Run(test.name, func(t *testing.T) {
			worker := &Cutter{verifiedIndex: fakeVerifiedCutIndex{sources: []forensicsindex.RestorationSource{test.source}}, verifiedObjects: test.reader, maxPackets: 100, bufferSize: 64 << 10, logger: zap.NewNop()}
			if _, err := worker.CutPCAPVersioned(context.Background(), query, []string{"probe-a"}, limits, &bytes.Buffer{}, nil); err == nil {
				t.Fatal("unsafe source was accepted")
			}
		})
	}
}
