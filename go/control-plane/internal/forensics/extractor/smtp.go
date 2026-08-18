package extractor

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/http"
	"net/mail"
	"strings"
)

type smtpPart struct {
	wire        []byte
	content     []byte
	contentType string
	disposition string
}

func extractSMTPData(input Input, limits Limits) Result {
	result := baseResult(ProfileSMTPDataMIME, "smtp-data-mime")
	if status, missing, truncation, unsafe := combinedStreamStatus(input.ClientToServer, input.ServerToClient); unsafe {
		result.Status = status
		result.StatusReason = "SMTP control/data stream is incomplete or unsafe"
		result.MissingRanges = missing
		result.TruncationOffset = truncation
		return result
	}
	client := input.ClientToServer.Bytes
	server := strings.ToUpper(string(input.ServerToClient.Bytes))
	if bytes.Contains(bytes.ToUpper(client), []byte("\r\nBDAT ")) || bytes.HasPrefix(bytes.ToUpper(client), []byte("BDAT ")) {
		result.Status = StatusUnsupported
		result.StatusReason = "SMTP BDAT chunking is excluded"
		return result
	}
	dataStart := smtpCommandEnd(client, "DATA")
	if dataStart < 0 || !smtpHasReply(server, "354") {
		result.Status = StatusUnsupported
		result.StatusReason = "SMTP session lacks DATA command with 354 reply"
		return result
	}
	dataEnd := bytes.Index(client[dataStart:], []byte("\r\n.\r\n"))
	if dataEnd < 0 {
		result.Status = StatusTruncated
		result.StatusReason = "SMTP DATA message lacks dot terminator"
		return result
	}
	wire := append([]byte(nil), client[dataStart:dataStart+dataEnd]...)
	wire = smtpDotUnstuff(wire)
	if int64(len(wire)) > limits.MaxObjectBytes*int64(limits.MaxExpansionRate) {
		result.Status = StatusOversize
		result.StatusReason = "SMTP wire message exceeds bounded expansion input"
		return result
	}
	message, err := mail.ReadMessage(bufio.NewReader(bytes.NewReader(wire)))
	if err != nil {
		result.Status = StatusCorrupt
		result.StatusReason = "SMTP MIME headers are malformed"
		return result
	}
	body, err := io.ReadAll(io.LimitReader(message.Body, limits.MaxObjectBytes*int64(limits.MaxExpansionRate)+1))
	if err != nil || int64(len(body)) > limits.MaxObjectBytes*int64(limits.MaxExpansionRate) {
		result.Status = StatusOversize
		result.StatusReason = "SMTP MIME body exceeds bounded parser input"
		return result
	}
	part, status, reason := parseSMTPMIMEPart(textprotoHeader(message.Header), body, limits, 1, new(int))
	if status != StatusComplete {
		result.Status, result.StatusReason = status, reason
		return result
	}
	result.WireFilename = wireFilename(part.disposition)
	if result.WireFilename == "" {
		result.WireFilename = "message.eml"
	}
	result.SanitizedFilename = SanitizeFilename(result.WireFilename)
	result.DeclaredMIMEType = part.contentType
	result.DetectedMIMEType = http.DetectContentType(part.content)
	result.Status = StatusComplete
	result.StatusReason = "approved SMTP DATA MIME message completed"
	return finalize(result, part.wire, part.content, limits)
}

// mail.Header is intentionally copied to a plain map so recursive parsing does
// not retain reader state or permit implicit content execution.
func textprotoHeader(header mail.Header) map[string][]string {
	result := make(map[string][]string, len(header))
	for key, values := range header {
		result[key] = append([]string(nil), values...)
	}
	return result
}

func headerGet(header map[string][]string, key string) string {
	for name, values := range header {
		if strings.EqualFold(name, key) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

func parseSMTPMIMEPart(header map[string][]string, wire []byte, limits Limits, depth int, parts *int) (smtpPart, Status, string) {
	if depth > limits.MaxMIMEDepth {
		return smtpPart{}, StatusOversize, "SMTP MIME nesting exceeds max_mime_depth"
	}
	*parts++
	if *parts > limits.MaxPartCount {
		return smtpPart{}, StatusOversize, "SMTP MIME part count exceeds max_part_count"
	}
	contentType := headerGet(header, "Content-Type")
	if contentType == "" {
		contentType = "text/plain"
	}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return smtpPart{}, StatusCorrupt, "SMTP Content-Type is malformed"
	}
	if strings.HasPrefix(strings.ToLower(mediaType), "message/") {
		return smtpPart{}, StatusUnsupported, "nested SMTP message containers are excluded"
	}
	if strings.HasPrefix(strings.ToLower(mediaType), "multipart/") {
		boundary := params["boundary"]
		if boundary == "" {
			return smtpPart{}, StatusCorrupt, "SMTP multipart lacks boundary"
		}
		reader := multipart.NewReader(bytes.NewReader(wire), boundary)
		var selected *smtpPart
		for {
			part, nextErr := reader.NextPart()
			if errors.Is(nextErr, io.EOF) {
				break
			}
			if nextErr != nil {
				return smtpPart{}, StatusCorrupt, "SMTP multipart framing is corrupt"
			}
			partWire, readErr := io.ReadAll(io.LimitReader(part, limits.MaxObjectBytes*int64(limits.MaxExpansionRate)+1))
			if readErr != nil || int64(len(partWire)) > limits.MaxObjectBytes*int64(limits.MaxExpansionRate) {
				return smtpPart{}, StatusOversize, "SMTP MIME part exceeds bounded parser input"
			}
			partHeader := make(map[string][]string, len(part.Header))
			for key, values := range part.Header {
				partHeader[key] = append([]string(nil), values...)
			}
			parsed, status, reason := parseSMTPMIMEPart(partHeader, partWire, limits, depth+1, parts)
			if status != StatusComplete {
				return smtpPart{}, status, reason
			}
			if selected != nil {
				return smtpPart{}, StatusUnsupported, "multiple restorable SMTP MIME parts require separate manifests"
			}
			copyPart := parsed
			selected = &copyPart
		}
		if selected == nil {
			return smtpPart{}, StatusCorrupt, "SMTP multipart contains no body part"
		}
		return *selected, StatusComplete, ""
	}
	encoding := strings.ToLower(strings.TrimSpace(headerGet(header, "Content-Transfer-Encoding")))
	decoded, status, reason := decodeSMTPBody(wire, encoding, limits)
	if status != StatusComplete {
		return smtpPart{}, status, reason
	}
	return smtpPart{
		wire: wire, content: decoded, contentType: mediaType,
		disposition: headerGet(header, "Content-Disposition"),
	}, StatusComplete, ""
}

func decodeSMTPBody(wire []byte, encoding string, limits Limits) ([]byte, Status, string) {
	var reader io.Reader
	switch encoding {
	case "", "7bit", "8bit", "binary":
		reader = bytes.NewReader(wire)
	case "base64":
		reader = base64.NewDecoder(base64.StdEncoding, bytes.NewReader(wire))
	case "quoted-printable":
		reader = quotedprintable.NewReader(bytes.NewReader(wire))
	default:
		return nil, StatusUnsupported, "unknown SMTP Content-Transfer-Encoding"
	}
	decoded, err := io.ReadAll(io.LimitReader(reader, limits.MaxObjectBytes+1))
	if err != nil {
		return nil, StatusCorrupt, "declared SMTP transfer encoding cannot be decoded"
	}
	if int64(len(decoded)) > limits.MaxObjectBytes {
		return nil, StatusOversize, "decoded SMTP body exceeds max_object_bytes"
	}
	if len(wire) > 0 && float64(len(decoded))/float64(len(wire)) > limits.MaxExpansionRate {
		return nil, StatusOversize, "decoded SMTP body exceeds max_expansion_ratio"
	}
	return decoded, StatusComplete, ""
}

func smtpCommandEnd(value []byte, command string) int {
	upper := bytes.ToUpper(value)
	target := []byte(command + "\r\n")
	if bytes.HasPrefix(upper, target) {
		return len(target)
	}
	index := bytes.Index(upper, append([]byte("\r\n"), target...))
	if index < 0 {
		return -1
	}
	return index + 2 + len(target)
}

func smtpHasReply(server, code string) bool {
	server = strings.ReplaceAll(server, "\r\n", "\n")
	for _, line := range strings.Split(server, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), code) {
			return true
		}
	}
	return false
}

func smtpDotUnstuff(value []byte) []byte {
	value = bytes.ReplaceAll(value, []byte("\r\n.."), []byte("\r\n."))
	if bytes.HasPrefix(value, []byte("..")) {
		value = value[1:]
	}
	return value
}
