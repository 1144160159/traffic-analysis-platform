package extractor

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/reassembly"
)

type Status string

const (
	StatusComplete    Status = "complete"
	StatusPartial     Status = "partial"
	StatusTruncated   Status = "truncated"
	StatusCorrupt     Status = "corrupt"
	StatusOversize    Status = "oversize"
	StatusUnsupported Status = "unsupported"
)

const (
	ProfileHTTP1Response = "http1-response-body-v1"
	ProfileFTPPassive    = "ftp-passive-retr-v1"
	ProfileSMTPDataMIME  = "smtp-data-mime-v1"
)

// ProtocolExtractor 单协议还原器(策略模式)。新增协议只需实现接口并通过
// RegisterProtocolExtractor 登记,不再修改 Extract 的分发逻辑。
type ProtocolExtractor interface {
	// Profile 返回处理的协议画像 ID。
	Profile() string
	// Extract 执行该协议的还原。
	Extract(input Input, limits Limits) Result
}

type protocolExtractorFunc struct {
	profile string
	fn      func(input Input, limits Limits) Result
}

func (p protocolExtractorFunc) Profile() string                           { return p.profile }
func (p protocolExtractorFunc) Extract(input Input, limits Limits) Result { return p.fn(input, limits) }

// protocolRegistry 已批准协议还原器注册表。仅允许在进程初始化阶段
// (init/RegisterProtocolExtractor)写入,运行期只读。
var protocolRegistry = map[string]ProtocolExtractor{
	ProfileHTTP1Response: protocolExtractorFunc{profile: ProfileHTTP1Response, fn: extractHTTP1},
	ProfileFTPPassive:    protocolExtractorFunc{profile: ProfileFTPPassive, fn: extractFTPPassive},
	ProfileSMTPDataMIME:  protocolExtractorFunc{profile: ProfileSMTPDataMIME, fn: extractSMTPData},
}

// RegisterProtocolExtractor 注册一个协议还原器(按 Profile 键)。请在进程启动
// 阶段调用;运行期并发调用需自行加锁。
func RegisterProtocolExtractor(e ProtocolExtractor) {
	if e == nil {
		return
	}
	if protocolRegistry == nil {
		protocolRegistry = make(map[string]ProtocolExtractor)
	}
	protocolRegistry[e.Profile()] = e
}

var safeFilenameCharacters = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

type Limits struct {
	MaxObjectBytes   int64
	MaxPartCount     int
	MaxMIMEDepth     int
	MaxExpansionRate float64
}

func (limits Limits) Validate() error {
	if limits.MaxObjectBytes <= 0 || limits.MaxPartCount <= 0 || limits.MaxMIMEDepth <= 0 || limits.MaxExpansionRate < 1 {
		return errors.New("all extractor limits must be positive and expansion rate must be at least one")
	}
	return nil
}

type Input struct {
	ProfileID string

	ClientToServer reassembly.Result
	ServerToClient reassembly.Result

	// FTP has a distinct data connection. These fields have no meaning for
	// HTTP or SMTP and are rejected when ambiguous.
	FTPDataServerToClient   reassembly.Result
	FTPDataConnections      int
	FTPDataCorrelated       bool
	FTPDataBoundaryComplete bool
	FTPDataReset            bool
	FTPTLSEnabled           bool

	ConnectionClosed bool
	ConnectionReset  bool
}

type Result struct {
	ProfileID           string
	ParserName          string
	ParserVersion       string
	AlgorithmVersion    string
	Status              Status
	StatusReason        string
	WireFilename        string
	SanitizedFilename   string
	DeclaredMIMEType    string
	DetectedMIMEType    string
	DeclaredSize        *int64
	VisibleSize         int64
	RestoredSize        int64
	WireSHA256          string
	ContentSHA256       string
	Content             []byte
	MissingRanges       []reassembly.SequenceRange
	TruncationOffset    *uint32
	ContentEncoding     string
	Executable          bool
	AutomaticOpen       bool
	AutomaticDecompress bool
	Quarantined         bool
	MalwareScanStatus   string
}

func sha256Hex(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func SanitizeFilename(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	value = filepath.Base(value)
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == 0 {
			return -1
		}
		return r
	}, value)
	value = safeFilenameCharacters.ReplaceAllString(value, "_")
	value = strings.Trim(value, " ._-")
	if len(value) > 128 {
		value = value[:128]
	}
	if value == "" || value == "." || value == ".." {
		return "restored.bin"
	}
	return value
}

func wireFilename(disposition string) string {
	_, params, err := mime.ParseMediaType(disposition)
	if err == nil {
		return params["filename"]
	}
	// Preserve an invalid-but-bounded wire filename as metadata. It is never
	// used as an object key and is sanitized before display/download metadata.
	if strings.ContainsAny(disposition, "\r\n\x00") {
		return ""
	}
	for _, parameter := range strings.Split(disposition, ";")[1:] {
		name, value, ok := strings.Cut(parameter, "=")
		if ok && strings.EqualFold(strings.TrimSpace(name), "filename") {
			return strings.Trim(strings.TrimSpace(value), `"`)
		}
	}
	return ""
}

func statusFromStream(stream reassembly.Result) (Status, bool) {
	switch stream.Status {
	case reassembly.StatusOversize:
		return StatusOversize, true
	case reassembly.StatusCorrupt:
		return StatusCorrupt, true
	case reassembly.StatusTruncated:
		return StatusTruncated, true
	case reassembly.StatusPartial:
		return StatusPartial, false
	default:
		return StatusComplete, false
	}
}

func combinedStreamStatus(streams ...reassembly.Result) (Status, []reassembly.SequenceRange, *uint32, bool) {
	status := StatusComplete
	var missing []reassembly.SequenceRange
	var truncation *uint32
	precedence := map[Status]int{
		StatusComplete: 0, StatusPartial: 1, StatusTruncated: 2, StatusCorrupt: 3, StatusOversize: 4,
	}
	for _, stream := range streams {
		candidate, _ := statusFromStream(stream)
		if precedence[candidate] > precedence[status] {
			status = candidate
		}
		missing = append(missing, stream.MissingRanges...)
		if stream.TruncationAt != nil && truncation == nil {
			value := *stream.TruncationAt
			truncation = &value
		}
	}
	return status, missing, truncation, status != StatusComplete
}

func baseResult(profile, parser string) Result {
	return Result{
		ProfileID: profile, ParserName: parser, ParserVersion: "1.0.0",
		AlgorithmVersion: "m03-restoration-v1", MalwareScanStatus: "not_scanned",
		Executable: false, AutomaticOpen: false, AutomaticDecompress: false, Quarantined: true,
	}
}

func finalize(result Result, wire, content []byte, limits Limits) Result {
	result.VisibleSize = int64(len(wire))
	result.WireSHA256 = sha256Hex(wire)
	if int64(len(content)) > limits.MaxObjectBytes {
		result.Status = StatusOversize
		result.StatusReason = "restored object exceeds max_object_bytes"
		result.RestoredSize = 0
		result.Content = nil
		result.ContentSHA256 = ""
		return result
	}
	result.Content = append([]byte(nil), content...)
	result.RestoredSize = int64(len(content))
	result.ContentSHA256 = sha256Hex(content)
	if result.SanitizedFilename == "" {
		result.SanitizedFilename = SanitizeFilename(result.WireFilename)
	}
	return result
}

func Extract(input Input, limits Limits) (Result, error) {
	if err := limits.Validate(); err != nil {
		return Result{}, err
	}
	if e, ok := protocolRegistry[input.ProfileID]; ok {
		return e.Extract(input, limits), nil
	}
	result := baseResult(input.ProfileID, "unsupported-profile")
	result.Status = StatusUnsupported
	result.StatusReason = fmt.Sprintf("protocol profile %q is not approved", input.ProfileID)
	return result, nil
}
