package extractor

import (
	"bytes"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/reassembly"
)

var testLimits = Limits{MaxObjectBytes: 1024, MaxPartCount: 8, MaxMIMEDepth: 3, MaxExpansionRate: 10}

func stream(value string) reassembly.Result {
	return reassembly.Result{Status: reassembly.StatusComplete, Bytes: []byte(value)}
}

func TestHTTPContentLengthAndFilenameAreBoundedMetadata(t *testing.T) {
	result, err := Extract(Input{
		ProfileID:      ProfileHTTP1Response,
		ServerToClient: stream("HTTP/1.1 200 OK\r\nContent-Length: 5\r\nContent-Type: text/plain\r\nContent-Disposition: attachment; filename=../../evil.sh\r\n\r\nhello"),
	}, testLimits)
	if err != nil || result.Status != StatusComplete || !bytes.Equal(result.Content, []byte("hello")) {
		t.Fatalf("unexpected extraction: %#v err=%v", result, err)
	}
	if result.SanitizedFilename != "evil.sh" || result.Executable || result.AutomaticOpen || result.AutomaticDecompress {
		t.Fatalf("unsafe filename or execution flags: %#v", result)
	}
}

func TestHTTPChunkedDecodesWithoutAutomaticContentDecompression(t *testing.T) {
	result, err := Extract(Input{
		ProfileID:      ProfileHTTP1Response,
		ServerToClient: stream("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\nContent-Encoding: gzip\r\n\r\n5\r\nhello\r\n6\r\n world\r\n0\r\n\r\n"),
	}, testLimits)
	if err != nil || result.Status != StatusComplete || string(result.Content) != "hello world" {
		t.Fatalf("unexpected chunk extraction: %#v err=%v", result, err)
	}
	if result.ContentEncoding != "gzip" || result.AutomaticDecompress {
		t.Fatalf("content encoding policy violated: %#v", result)
	}
}

func TestHTTPContradictoryFramingAndUnsupportedUpgradeFailClosed(t *testing.T) {
	contradictory, _ := Extract(Input{
		ProfileID:      ProfileHTTP1Response,
		ServerToClient: stream("HTTP/1.1 200 OK\r\nContent-Length: 5\r\nTransfer-Encoding: chunked\r\n\r\n0\r\n\r\n"),
	}, testLimits)
	if contradictory.Status != StatusCorrupt {
		t.Fatalf("expected corrupt: %#v", contradictory)
	}
	websocket, _ := Extract(Input{
		ProfileID:      ProfileHTTP1Response,
		ServerToClient: stream("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\n\r\n"),
	}, testLimits)
	if websocket.Status != StatusUnsupported || len(websocket.Content) != 0 {
		t.Fatalf("expected unsupported: %#v", websocket)
	}
}

func TestHTTPDeclaredOversizeAndIncompleteBody(t *testing.T) {
	oversize, _ := Extract(Input{
		ProfileID:      ProfileHTTP1Response,
		ServerToClient: stream("HTTP/1.1 200 OK\r\nContent-Length: 2048\r\n\r\n"),
	}, testLimits)
	if oversize.Status != StatusOversize || len(oversize.Content) != 0 {
		t.Fatalf("expected oversize metadata only: %#v", oversize)
	}
	truncated, _ := Extract(Input{
		ProfileID:      ProfileHTTP1Response,
		ServerToClient: stream("HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nhel"),
	}, testLimits)
	if truncated.Status != StatusTruncated || string(truncated.Content) != "hel" {
		t.Fatalf("expected bounded visible bytes: %#v", truncated)
	}
}

func TestUnknownProfileNeverWritesObjectContent(t *testing.T) {
	result, err := Extract(Input{ProfileID: "http2-response-v1", ServerToClient: stream("secret")}, testLimits)
	if err != nil || result.Status != StatusUnsupported || len(result.Content) != 0 {
		t.Fatalf("unsupported profile escaped: %#v err=%v", result, err)
	}
}
