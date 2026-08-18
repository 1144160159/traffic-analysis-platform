package extractor

import (
	"bytes"
	"encoding/hex"
	"mime"
	"net/http"
	"strconv"
	"strings"
)

const maxHTTPHeaderBytes = 64 * 1024

func extractHTTP1(input Input, limits Limits) Result {
	result := baseResult(ProfileHTTP1Response, "http1-response-body")
	streamStatus, terminal := statusFromStream(input.ServerToClient)
	if terminal {
		result.Status = streamStatus
		result.StatusReason = "TCP stream is not safe for HTTP framing"
		result.MissingRanges = input.ServerToClient.MissingRanges
		result.TruncationOffset = input.ServerToClient.TruncationAt
		return result
	}
	wire := input.ServerToClient.Bytes
	if len(wire) > maxHTTPHeaderBytes && !bytes.Contains(wire[:maxHTTPHeaderBytes], []byte("\r\n\r\n")) {
		result.Status = StatusOversize
		result.StatusReason = "HTTP header exceeds bounded parser limit"
		return result
	}
	headerEnd := bytes.Index(wire, []byte("\r\n\r\n"))
	if headerEnd < 0 {
		result.Status = StatusTruncated
		result.StatusReason = "HTTP response headers are incomplete"
		return result
	}
	lines := strings.Split(string(wire[:headerEnd]), "\r\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "HTTP/1.") {
		result.Status = StatusUnsupported
		result.StatusReason = "only HTTP/1.x response framing is approved"
		return result
	}
	headers := make(http.Header)
	for _, line := range lines[1:] {
		name, value, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(name) == "" {
			result.Status = StatusCorrupt
			result.StatusReason = "malformed HTTP response header"
			return result
		}
		headers.Add(http.CanonicalHeaderKey(strings.TrimSpace(name)), strings.TrimSpace(value))
	}
	if strings.EqualFold(headers.Get("Upgrade"), "websocket") {
		result.Status = StatusUnsupported
		result.StatusReason = "WebSocket upgrade is excluded"
		return result
	}
	contentLengths := headers.Values("Content-Length")
	transferEncoding := strings.ToLower(strings.Join(headers.Values("Transfer-Encoding"), ","))
	if len(contentLengths) > 0 && transferEncoding != "" {
		result.Status = StatusCorrupt
		result.StatusReason = "Content-Length and Transfer-Encoding are contradictory"
		return result
	}
	var declaredLength *int64
	for _, raw := range contentLengths {
		for _, item := range strings.Split(raw, ",") {
			length, err := strconv.ParseInt(strings.TrimSpace(item), 10, 64)
			if err != nil || length < 0 {
				result.Status = StatusCorrupt
				result.StatusReason = "invalid Content-Length"
				return result
			}
			if declaredLength != nil && *declaredLength != length {
				result.Status = StatusCorrupt
				result.StatusReason = "conflicting Content-Length headers"
				return result
			}
			copyLength := length
			declaredLength = &copyLength
		}
	}
	bodyWire := wire[headerEnd+4:]
	body := bodyWire
	complete := false
	switch {
	case declaredLength != nil:
		if *declaredLength > limits.MaxObjectBytes {
			result.Status = StatusOversize
			result.StatusReason = "declared HTTP body exceeds max_object_bytes"
			result.DeclaredSize = declaredLength
			return result
		}
		if int64(len(bodyWire)) < *declaredLength {
			result.Status = StatusTruncated
			result.StatusReason = "capture ended before declared HTTP body length"
			result.DeclaredSize = declaredLength
			return finalize(result, bodyWire, bodyWire, limits)
		}
		bodyWire = bodyWire[:*declaredLength]
		body = bodyWire
		complete = true
	case transferEncoding != "":
		if transferEncoding != "chunked" {
			result.Status = StatusUnsupported
			result.StatusReason = "unsupported HTTP transfer encoding"
			return result
		}
		decoded, consumed, status, reason := decodeHTTPChunks(bodyWire, limits.MaxObjectBytes)
		if status != StatusComplete {
			result.Status, result.StatusReason = status, reason
			return finalize(result, bodyWire, decoded, limits)
		}
		bodyWire = bodyWire[:consumed]
		body = decoded
		complete = true
	default:
		if input.ConnectionReset {
			result.Status = StatusTruncated
			result.StatusReason = "connection-close HTTP body ended with TCP reset"
			return finalize(result, bodyWire, bodyWire, limits)
		}
		if !input.ConnectionClosed {
			result.Status = StatusTruncated
			result.StatusReason = "connection-close HTTP body lacks an observed close"
			return finalize(result, bodyWire, bodyWire, limits)
		}
		complete = true
	}
	mediaType := headers.Get("Content-Type")
	if parsed, _, err := mime.ParseMediaType(mediaType); err == nil {
		result.DeclaredMIMEType = parsed
	} else {
		result.DeclaredMIMEType = mediaType
	}
	result.DetectedMIMEType = http.DetectContentType(body)
	result.ContentEncoding = headers.Get("Content-Encoding")
	result.WireFilename = wireFilename(headers.Get("Content-Disposition"))
	result.SanitizedFilename = SanitizeFilename(result.WireFilename)
	result.DeclaredSize = declaredLength
	if complete {
		result.Status = StatusComplete
		result.StatusReason = "approved HTTP/1 response body framing completed"
	}
	return finalize(result, bodyWire, body, limits)
}

func decodeHTTPChunks(wire []byte, maximum int64) ([]byte, int, Status, string) {
	var decoded []byte
	offset := 0
	for {
		lineEnd := bytes.Index(wire[offset:], []byte("\r\n"))
		if lineEnd < 0 {
			return decoded, offset, StatusTruncated, "chunk size line is incomplete"
		}
		line := string(wire[offset : offset+lineEnd])
		offset += lineEnd + 2
		sizeText := strings.TrimSpace(strings.SplitN(line, ";", 2)[0])
		if sizeText == "" || len(sizeText) > 16 {
			return decoded, offset, StatusCorrupt, "invalid HTTP chunk size"
		}
		size, err := strconv.ParseUint(sizeText, 16, 63)
		if err != nil {
			return decoded, offset, StatusCorrupt, "invalid HTTP chunk size: " + hex.EncodeToString([]byte(sizeText))
		}
		if int64(len(decoded))+int64(size) > maximum {
			return decoded, offset, StatusOversize, "decoded chunks exceed max_object_bytes"
		}
		if size == 0 {
			if len(wire[offset:]) < 2 {
				return decoded, offset, StatusTruncated, "final HTTP chunk terminator is incomplete"
			}
			if bytes.HasPrefix(wire[offset:], []byte("\r\n")) {
				return decoded, offset + 2, StatusComplete, ""
			}
			trailerEnd := bytes.Index(wire[offset:], []byte("\r\n\r\n"))
			if trailerEnd < 0 {
				return decoded, offset, StatusTruncated, "HTTP chunk trailers are incomplete"
			}
			return decoded, offset + trailerEnd + 4, StatusComplete, ""
		}
		if uint64(len(wire)-offset) < size+2 {
			return decoded, offset, StatusTruncated, "HTTP chunk payload is incomplete"
		}
		decoded = append(decoded, wire[offset:offset+int(size)]...)
		offset += int(size)
		if !bytes.HasPrefix(wire[offset:], []byte("\r\n")) {
			return decoded, offset, StatusCorrupt, "HTTP chunk lacks CRLF"
		}
		offset += 2
	}
}
