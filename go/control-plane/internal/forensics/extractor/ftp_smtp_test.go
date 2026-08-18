package extractor

import (
	"encoding/base64"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/reassembly"
)

func TestFTPPassiveRetrRequiresExactControlAndSingleDataConnection(t *testing.T) {
	result, err := Extract(Input{
		ProfileID:               ProfileFTPPassive,
		ClientToServer:          stream("USER analyst\r\nPASV\r\nRETR ../../report.bin\r\n"),
		ServerToClient:          stream("230 Login ok\r\n227 Entering Passive Mode (198,51,100,2,195,80)\r\n150 Opening data\r\n226 Transfer complete\r\n"),
		FTPDataServerToClient:   stream("payload"),
		FTPDataConnections:      1,
		FTPDataCorrelated:       true,
		FTPDataBoundaryComplete: true,
	}, testLimits)
	if err != nil || result.Status != StatusComplete || string(result.Content) != "payload" || result.SanitizedFilename != "report.bin" {
		t.Fatalf("unexpected FTP result: %#v err=%v", result, err)
	}
	ambiguous, _ := Extract(Input{
		ProfileID:               ProfileFTPPassive,
		ClientToServer:          stream("PASV\r\nRETR report.bin\r\n"),
		ServerToClient:          stream("227 Entering Passive Mode (198,51,100,2,195,80)\r\n150 Opening\r\n226 Done\r\n"),
		FTPDataServerToClient:   stream("payload"),
		FTPDataConnections:      2,
		FTPDataCorrelated:       true,
		FTPDataBoundaryComplete: true,
	}, testLimits)
	if ambiguous.Status != StatusUnsupported || len(ambiguous.Content) != 0 {
		t.Fatalf("ambiguous FTP data escaped: %#v", ambiguous)
	}
}

func TestFTPCommandParsingUsesOneNormalizedCoordinateSystem(t *testing.T) {
	result, _ := Extract(Input{
		ProfileID:               ProfileFTPPassive,
		ClientToServer:          stream("USER a\r\nEPSV\nRETR nested\\mixed.txt\r\n"),
		ServerToClient:          stream("229 Entering Extended Passive Mode (|||50000|)\n150 Opening\n226 Done\r\n"),
		FTPDataServerToClient:   stream("payload"),
		FTPDataConnections:      1,
		FTPDataCorrelated:       true,
		FTPDataBoundaryComplete: true,
	}, testLimits)
	if result.Status != StatusComplete || result.SanitizedFilename != "mixed.txt" {
		t.Fatalf("mixed newline parsing failed: %#v", result)
	}
}

func TestFTPTLSAndMissingCompletionAreNotComplete(t *testing.T) {
	tlsResult, _ := Extract(Input{ProfileID: ProfileFTPPassive, FTPTLSEnabled: true}, testLimits)
	if tlsResult.Status != StatusUnsupported {
		t.Fatalf("FTP TLS must be unsupported: %#v", tlsResult)
	}
	truncated, _ := Extract(Input{
		ProfileID:               ProfileFTPPassive,
		ClientToServer:          stream("EPSV\r\nRETR report.bin\r\n"),
		ServerToClient:          stream("229 Entering Extended Passive Mode (|||50000|)\r\n150 Opening\r\n"),
		FTPDataServerToClient:   stream("payload"),
		FTPDataConnections:      1,
		FTPDataCorrelated:       true,
		FTPDataBoundaryComplete: true,
	}, testLimits)
	if truncated.Status != StatusTruncated || len(truncated.Content) != 0 {
		t.Fatalf("missing 226 claimed complete: %#v", truncated)
	}
}

func TestSMTPDataDecodesDeclaredBase64AndDotUnstuffs(t *testing.T) {
	payload := []byte("hello attachment")
	encoded := base64.StdEncoding.EncodeToString(payload)
	message := "DATA\r\nContent-Type: application/octet-stream\r\nContent-Disposition: attachment; filename=../../mail.bin\r\nContent-Transfer-Encoding: base64\r\n\r\n" + encoded + "\r\n.\r\nQUIT\r\n"
	result, err := Extract(Input{
		ProfileID:      ProfileSMTPDataMIME,
		ClientToServer: stream(message),
		ServerToClient: stream("354 End data with <CR><LF>.<CR><LF>\r\n250 accepted\r\n"),
	}, testLimits)
	if err != nil || result.Status != StatusComplete || string(result.Content) != string(payload) || result.SanitizedFilename != "mail.bin" {
		t.Fatalf("unexpected SMTP result: %#v err=%v", result, err)
	}
}

func TestSMTPRejectsUnknownEncodingBDATAndMissingDotTerminator(t *testing.T) {
	unknown, _ := Extract(Input{
		ProfileID:      ProfileSMTPDataMIME,
		ClientToServer: stream("DATA\r\nContent-Type: text/plain\r\nContent-Transfer-Encoding: x-evil\r\n\r\nbody\r\n.\r\n"),
		ServerToClient: stream("354 go\r\n"),
	}, testLimits)
	if unknown.Status != StatusUnsupported || len(unknown.Content) != 0 {
		t.Fatalf("unknown encoding escaped: %#v", unknown)
	}
	bdat, _ := Extract(Input{
		ProfileID:      ProfileSMTPDataMIME,
		ClientToServer: stream("BDAT 4 LAST\r\nbody"),
		ServerToClient: stream("250 ok\r\n"),
	}, testLimits)
	if bdat.Status != StatusUnsupported {
		t.Fatalf("BDAT escaped: %#v", bdat)
	}
	truncated, _ := Extract(Input{
		ProfileID:      ProfileSMTPDataMIME,
		ClientToServer: stream("DATA\r\nContent-Type: text/plain\r\n\r\nbody"),
		ServerToClient: stream("354 go\r\n"),
	}, testLimits)
	if truncated.Status != StatusTruncated {
		t.Fatalf("missing terminator escaped: %#v", truncated)
	}
}

func TestSMTPMultipartMoreThanOneRestorablePartIsUnsupportedNotMerged(t *testing.T) {
	body := "DATA\r\nContent-Type: multipart/mixed; boundary=x\r\n\r\n--x\r\nContent-Type: text/plain\r\n\r\none\r\n--x\r\nContent-Type: text/plain\r\n\r\ntwo\r\n--x--\r\n\r\n.\r\n"
	result, _ := Extract(Input{
		ProfileID:      ProfileSMTPDataMIME,
		ClientToServer: stream(body),
		ServerToClient: stream("354 go\r\n"),
	}, testLimits)
	if result.Status != StatusUnsupported || len(result.Content) != 0 {
		t.Fatalf("multiple parts were unsafely merged: %#v", result)
	}
}

func TestPartialFTPDataNeverInventsAfterGap(t *testing.T) {
	partial := reassembly.Result{Status: reassembly.StatusPartial, Bytes: []byte("visible"), MissingRanges: []reassembly.SequenceRange{{Start: 10, End: 20}}}
	result, _ := Extract(Input{
		ProfileID:               ProfileFTPPassive,
		ClientToServer:          stream("PASV\r\nRETR report.bin\r\n"),
		ServerToClient:          stream("227 Entering Passive Mode (198,51,100,2,195,80)\r\n150 Opening\r\n226 Done\r\n"),
		FTPDataServerToClient:   partial,
		FTPDataConnections:      1,
		FTPDataCorrelated:       true,
		FTPDataBoundaryComplete: true,
	}, testLimits)
	if result.Status != StatusPartial || string(result.Content) != "visible" || len(result.MissingRanges) != 1 {
		t.Fatalf("partial FTP evidence lost: %#v", result)
	}
}

func TestFTPPassiveDataCorrelationRequiresExactAdvertisedEndpoint(t *testing.T) {
	client := []byte("PASV\r\nRETR report.bin\r\n")
	server := []byte("227 Entering Passive Mode (198,51,100,2,195,80)\r\n150 Opening\r\n")
	if !CorrelateFTPPassiveData(client, server, "198.51.100.2", "192.0.2.1", 52000, "198.51.100.2", 50000) {
		t.Fatal("exact PASV endpoint was not correlated")
	}
	if CorrelateFTPPassiveData(client, server, "198.51.100.2", "192.0.2.1", 52000, "198.51.100.2", 50001) {
		t.Fatal("wrong PASV port was correlated")
	}
	epsvClient := []byte("EPSV\r\nRETR report.bin\r\n")
	epsvServer := []byte("229 Entering Extended Passive Mode (|||50000|)\r\n150 Opening\r\n")
	if !CorrelateFTPPassiveData(epsvClient, epsvServer, "2001:db8::2", "2001:db8::1", 52000, "2001:db8::2", 50000) {
		t.Fatal("exact EPSV endpoint was not correlated")
	}
}

func TestFTPAndSMTPRejectUnsafeControlStreamsBeforeFraming(t *testing.T) {
	partialControl := reassembly.Result{Status: reassembly.StatusPartial, Bytes: []byte("PASV\r\n"), MissingRanges: []reassembly.SequenceRange{{Start: 10, End: 20}}}
	ftp, _ := Extract(Input{ProfileID: ProfileFTPPassive, ClientToServer: partialControl, ServerToClient: stream("227 Entering Passive Mode (198,51,100,2,195,80)\r\n")}, testLimits)
	if ftp.Status != StatusPartial || len(ftp.MissingRanges) != 1 {
		t.Fatalf("unsafe FTP control stream escaped: %#v", ftp)
	}
	corruptControl := reassembly.Result{Status: reassembly.StatusCorrupt}
	smtp, _ := Extract(Input{ProfileID: ProfileSMTPDataMIME, ClientToServer: corruptControl, ServerToClient: stream("354 go\r\n")}, testLimits)
	if smtp.Status != StatusCorrupt {
		t.Fatalf("unsafe SMTP stream escaped: %#v", smtp)
	}
}
