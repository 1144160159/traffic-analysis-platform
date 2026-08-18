package restoration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/extractor"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/reassembly"
)

type goldenStream struct {
	Status        string                     `json:"status"`
	Bytes         string                     `json:"bytes"`
	MissingRanges []reassembly.SequenceRange `json:"missing_ranges"`
}

type goldenSegment struct {
	Sequence       uint32 `json:"sequence"`
	Payload        string `json:"payload"`
	CapturedLength int    `json:"captured_length"`
	OriginalLength int    `json:"original_length"`
	PacketIndex    uint64 `json:"packet_index"`
}

type goldenInput struct {
	MaxStreamBytes          uint64          `json:"max_stream_bytes"`
	Segments                []goldenSegment `json:"segments"`
	ProfileID               string          `json:"profile_id"`
	ClientToServer          goldenStream    `json:"client_to_server"`
	ServerToClient          goldenStream    `json:"server_to_client"`
	FTPDataServerToClient   goldenStream    `json:"ftp_data_server_to_client"`
	FTPDataConnections      int             `json:"ftp_data_connections"`
	FTPDataCorrelated       bool            `json:"ftp_data_correlated"`
	FTPDataBoundaryComplete bool            `json:"ftp_data_boundary_complete"`
	FTPDataReset            bool            `json:"ftp_data_reset"`
	FTPTLSEnabled           bool            `json:"ftp_tls_enabled"`
	ConnectionClosed        bool            `json:"connection_closed"`
}

type goldenExpected struct {
	Status            string                     `json:"status"`
	Content           string                     `json:"content"`
	MissingRanges     []reassembly.SequenceRange `json:"missing_ranges"`
	ConflictAt        *uint32                    `json:"conflict_at"`
	Truncated         bool                       `json:"truncated"`
	SanitizedFilename string                     `json:"sanitized_filename"`
	ContentEncoding   string                     `json:"content_encoding"`
	ObjectPolicy      string                     `json:"object_policy"`
	Inert             bool                       `json:"inert"`
}

type goldenCase struct {
	CaseID   string         `json:"case_id"`
	Stage    string         `json:"stage"`
	Input    goldenInput    `json:"input"`
	Expected goldenExpected `json:"expected"`
}

type goldenCorpus struct {
	Limits struct {
		MaxStreamBytes   uint64  `json:"max_stream_bytes"`
		MaxObjectBytes   int64   `json:"max_object_bytes"`
		MaxPartCount     int     `json:"max_part_count"`
		MaxMIMEDepth     int     `json:"max_mime_depth"`
		MaxExpansionRate float64 `json:"max_expansion_ratio"`
	} `json:"limits"`
	Cases []goldenCase `json:"cases"`
}

func goldenReassemblyStatus(value string) reassembly.Status {
	return reassembly.Status(value)
}

func goldenResultStream(value goldenStream) reassembly.Result {
	return reassembly.Result{
		Status: goldenReassemblyStatus(value.Status), Bytes: []byte(value.Bytes),
		MissingRanges: append([]reassembly.SequenceRange(nil), value.MissingRanges...),
	}
}

func actualObjectPolicy(result extractor.Result) string {
	switch result.Status {
	case extractor.StatusComplete:
		return "required"
	case extractor.StatusUnsupported:
		return "forbidden"
	case extractor.StatusPartial, extractor.StatusTruncated, extractor.StatusCorrupt:
		if len(result.Content) > 0 {
			return "optional_quarantine"
		}
		return "metadata_only"
	default:
		return "metadata_only"
	}
}

func loadGoldenCorpus(t *testing.T) goldenCorpus {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "..", "tests", "fixtures", "forensics", "file-restoration", "golden-corpus.v1.json")
	encoded, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var corpus goldenCorpus
	if err := json.Unmarshal(encoded, &corpus); err != nil {
		t.Fatal(err)
	}
	if len(corpus.Cases) != 18 {
		t.Fatalf("golden corpus cases = %d, want 18", len(corpus.Cases))
	}
	return corpus
}

func TestFileRestorationGoldenCorpusRunsProductionCode(t *testing.T) {
	corpus := loadGoldenCorpus(t)
	seen := make(map[string]struct{}, len(corpus.Cases))
	for _, testCase := range corpus.Cases {
		testCase := testCase
		t.Run(testCase.CaseID, func(t *testing.T) {
			if _, duplicate := seen[testCase.CaseID]; duplicate {
				t.Fatalf("duplicate case id %q", testCase.CaseID)
			}
			seen[testCase.CaseID] = struct{}{}
			switch testCase.Stage {
			case "reassembly":
				segments := make([]reassembly.Segment, 0, len(testCase.Input.Segments))
				for _, segment := range testCase.Input.Segments {
					segments = append(segments, reassembly.Segment{
						Sequence: segment.Sequence, Payload: []byte(segment.Payload),
						CapturedLength: segment.CapturedLength, OriginalLength: segment.OriginalLength,
						PacketIndex: segment.PacketIndex,
					})
				}
				maximum := corpus.Limits.MaxStreamBytes
				if testCase.Input.MaxStreamBytes > 0 {
					maximum = testCase.Input.MaxStreamBytes
				}
				result, err := reassembly.Reassemble(segments, maximum)
				if err != nil {
					t.Fatal(err)
				}
				if string(result.Status) != testCase.Expected.Status || string(result.Bytes) != testCase.Expected.Content {
					t.Fatalf("status/content = %s/%q, want %s/%q", result.Status, result.Bytes, testCase.Expected.Status, testCase.Expected.Content)
				}
				if !equalSequenceRanges(result.MissingRanges, testCase.Expected.MissingRanges) {
					t.Fatalf("missing ranges = %+v, want %+v", result.MissingRanges, testCase.Expected.MissingRanges)
				}
				if !equalOptionalUint32(result.ConflictAt, testCase.Expected.ConflictAt) || (result.TruncationAt != nil) != testCase.Expected.Truncated {
					t.Fatalf("conflict/truncation = %v/%v", result.ConflictAt, result.TruncationAt)
				}
			case "extractor":
				result, err := extractor.Extract(extractor.Input{
					ProfileID:               testCase.Input.ProfileID,
					ClientToServer:          goldenResultStream(testCase.Input.ClientToServer),
					ServerToClient:          goldenResultStream(testCase.Input.ServerToClient),
					FTPDataServerToClient:   goldenResultStream(testCase.Input.FTPDataServerToClient),
					FTPDataConnections:      testCase.Input.FTPDataConnections,
					FTPDataCorrelated:       testCase.Input.FTPDataCorrelated,
					FTPDataBoundaryComplete: testCase.Input.FTPDataBoundaryComplete,
					FTPDataReset:            testCase.Input.FTPDataReset,
					FTPTLSEnabled:           testCase.Input.FTPTLSEnabled,
					ConnectionClosed:        testCase.Input.ConnectionClosed,
				}, extractor.Limits{
					MaxObjectBytes: corpus.Limits.MaxObjectBytes, MaxPartCount: corpus.Limits.MaxPartCount,
					MaxMIMEDepth: corpus.Limits.MaxMIMEDepth, MaxExpansionRate: corpus.Limits.MaxExpansionRate,
				})
				if err != nil {
					t.Fatal(err)
				}
				if string(result.Status) != testCase.Expected.Status || string(result.Content) != testCase.Expected.Content {
					t.Fatalf("status/content = %s/%q, want %s/%q; reason=%s", result.Status, result.Content, testCase.Expected.Status, testCase.Expected.Content, result.StatusReason)
				}
				if result.SanitizedFilename != testCase.Expected.SanitizedFilename || result.ContentEncoding != testCase.Expected.ContentEncoding {
					t.Fatalf("filename/encoding = %q/%q, want %q/%q", result.SanitizedFilename, result.ContentEncoding, testCase.Expected.SanitizedFilename, testCase.Expected.ContentEncoding)
				}
				if actual := actualObjectPolicy(result); actual != testCase.Expected.ObjectPolicy {
					t.Fatalf("object policy = %q, want %q", actual, testCase.Expected.ObjectPolicy)
				}
				inert := result.Quarantined && !result.Executable && !result.AutomaticOpen && !result.AutomaticDecompress
				if inert != testCase.Expected.Inert {
					t.Fatalf("inert flags = quarantine:%v executable:%v open:%v decompress:%v", result.Quarantined, result.Executable, result.AutomaticOpen, result.AutomaticDecompress)
				}
			default:
				t.Fatalf("unknown corpus stage %q", testCase.Stage)
			}
		})
	}
}

func equalSequenceRanges(left, right []reassembly.SequenceRange) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func equalOptionalUint32(left, right *uint32) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
