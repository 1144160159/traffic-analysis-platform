// Command proto_descriptor_locator resolves one Protobuf declaration against
// descriptors compiled entirely from a frozen Git candidate. It proves an
// exact locator only; it does not authorize design, execution, or rollout.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

const resolverSource = "scripts/alignment/proto_descriptor_locator/main.go"

var hex40 = regexp.MustCompile(`^[0-9a-f]{40}$`)

type hashRef struct {
	Commit         string `json:"commit"`
	ManifestPath   string `json:"manifest_path"`
	ManifestSHA256 string `json:"manifest_sha256"`
}

type resolverRef struct {
	ResolverID    string `json:"resolver_id"`
	Engine        string `json:"engine"`
	EngineVersion string `json:"engine_version"`
	SourcePath    string `json:"source_path"`
	SourceSHA256  string `json:"source_sha256"`
}

type position struct {
	ByteOffset int `json:"byte_offset"`
	Line       int `json:"line"`
	Column     int `json:"column"`
}

type locator struct {
	LocatorID         string   `json:"locator_id"`
	Language          string   `json:"language"`
	Path              string   `json:"path"`
	Package           string   `json:"package"`
	QualifiedSymbol   string   `json:"qualified_symbol"`
	DeclarationKind   string   `json:"declaration_kind"`
	Query             string   `json:"query"`
	MatchStrategy     string   `json:"match_strategy"`
	Signature         string   `json:"signature"`
	CandidateBlobSHA  string   `json:"candidate_blob_sha256"`
	DescriptorSetSHA  string   `json:"descriptor_set_sha256"`
	SourceSpanSHA     string   `json:"source_span_sha256"`
	NormalizedDescSHA string   `json:"normalized_descriptor_sha256"`
	Start             position `json:"start"`
	End               position `json:"end"`
}

type receipt struct {
	SchemaVersion  string      `json:"schema_version"`
	ArtifactKind   string      `json:"artifact_kind"`
	Status         string      `json:"status"`
	ProofLevel     string      `json:"proof_level"`
	Candidate      hashRef     `json:"candidate"`
	Resolver       resolverRef `json:"resolver"`
	Locator        locator     `json:"locator"`
	AmbiguityCount int         `json:"ambiguity_count"`
	ResolvedAt     string      `json:"resolved_at"`
	ProofCeiling   string      `json:"proof_ceiling"`
}

type manifest struct {
	Commit string            `json:"implementation_candidate_commit"`
	Blobs  map[string]string `json:"source_blob_sha256"`
}

type descriptorMatch struct {
	kind      string
	qualified string
	signature string
	path      []int32
	message   proto.Message
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func safeRelative(value string) error {
	if value == "" || filepath.IsAbs(value) || filepath.Clean(value) != filepath.FromSlash(value) {
		return fmt.Errorf("path is not canonical repository-relative: %q", value)
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("path contains an unsafe component: %q", value)
		}
	}
	return nil
}

func safeRegular(root, relative string) (string, error) {
	if err := safeRelative(relative); err != nil {
		return "", err
	}
	current := root
	for _, part := range strings.Split(relative, "/") {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("repository path contains a symlink: %s", relative)
		}
	}
	info, err := os.Stat(current)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("repository path is not a regular file: %s", relative)
	}
	return current, nil
}

func safeOutput(root, relative string) (string, error) {
	if err := safeRelative(relative); err != nil {
		return "", err
	}
	current := root
	parts := strings.Split(relative, "/")
	for index, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if index < len(parts)-1 {
				if err := os.Mkdir(current, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
					return "", err
				}
			}
			continue
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("output path contains a symlink: %s", relative)
		}
		if index < len(parts)-1 && !info.IsDir() {
			return "", fmt.Errorf("output parent is not a directory: %s", relative)
		}
		if index == len(parts)-1 && !info.Mode().IsRegular() {
			return "", fmt.Errorf("output path is not a regular file: %s", relative)
		}
	}
	return current, nil
}

func gitOutput(root string, args ...string) ([]byte, error) {
	command := exec.Command("git", args...)
	command.Dir = root
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s failed: %w", strings.Join(args, " "), err)
	}
	return output, nil
}

func stageCandidateProto(root, commit, temp string) error {
	listing, err := gitOutput(root, "ls-tree", "-r", "--name-only", commit, "--", "proto")
	if err != nil {
		return err
	}
	for _, repositoryPath := range strings.Fields(string(listing)) {
		if !strings.HasSuffix(repositoryPath, ".proto") || !strings.HasPrefix(repositoryPath, "proto/") {
			continue
		}
		if err := safeRelative(repositoryPath); err != nil {
			return err
		}
		content, err := gitOutput(root, "show", commit+":"+repositoryPath)
		if err != nil {
			return err
		}
		staged := filepath.Join(temp, filepath.FromSlash(strings.TrimPrefix(repositoryPath, "proto/")))
		if err := os.MkdirAll(filepath.Dir(staged), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(staged, content, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func fieldType(field *descriptorpb.FieldDescriptorProto) string {
	if field.GetTypeName() != "" {
		return strings.TrimPrefix(field.GetTypeName(), ".")
	}
	return strings.ToLower(strings.TrimPrefix(field.GetType().String(), "TYPE_"))
}

func collectMessage(
	result *[]descriptorMatch,
	message *descriptorpb.DescriptorProto,
	prefix string,
	path []int32,
) {
	qualified := prefix + "." + message.GetName()
	*result = append(*result, descriptorMatch{
		kind: "MESSAGE", qualified: qualified, signature: "message " + qualified,
		path: append([]int32(nil), path...), message: message,
	})
	for index, field := range message.GetField() {
		label := strings.ToLower(strings.TrimPrefix(field.GetLabel().String(), "LABEL_"))
		fieldQualified := qualified + "." + field.GetName()
		*result = append(*result, descriptorMatch{
			kind: "FIELD", qualified: fieldQualified,
			signature: fmt.Sprintf("%s %s %s = %d", label, fieldType(field), fieldQualified, field.GetNumber()),
			path:      append(append([]int32(nil), path...), 2, int32(index)), message: field,
		})
	}
	for index, nested := range message.GetNestedType() {
		collectMessage(result, nested, qualified, append(append([]int32(nil), path...), 3, int32(index)))
	}
	for index, enum := range message.GetEnumType() {
		enumQualified := qualified + "." + enum.GetName()
		*result = append(*result, descriptorMatch{
			kind: "ENUM", qualified: enumQualified, signature: "enum " + enumQualified,
			path: append(append([]int32(nil), path...), 4, int32(index)), message: enum,
		})
	}
}

func collectMatches(file *descriptorpb.FileDescriptorProto) []descriptorMatch {
	result := make([]descriptorMatch, 0)
	prefix := file.GetPackage()
	for index, message := range file.GetMessageType() {
		collectMessage(&result, message, prefix, []int32{4, int32(index)})
	}
	for index, enum := range file.GetEnumType() {
		qualified := prefix + "." + enum.GetName()
		result = append(result, descriptorMatch{
			kind: "ENUM", qualified: qualified, signature: "enum " + qualified,
			path: []int32{5, int32(index)}, message: enum,
		})
	}
	for serviceIndex, service := range file.GetService() {
		qualified := prefix + "." + service.GetName()
		result = append(result, descriptorMatch{
			kind: "SERVICE", qualified: qualified, signature: "service " + qualified,
			path: []int32{6, int32(serviceIndex)}, message: service,
		})
		for methodIndex, method := range service.GetMethod() {
			methodQualified := qualified + "." + method.GetName()
			result = append(result, descriptorMatch{
				kind: "METHOD", qualified: methodQualified,
				signature: fmt.Sprintf("rpc %s(%s) returns (%s)", methodQualified, strings.TrimPrefix(method.GetInputType(), "."), strings.TrimPrefix(method.GetOutputType(), ".")),
				path:      []int32{6, int32(serviceIndex), 2, int32(methodIndex)}, message: method,
			})
		}
	}
	return result
}

func equalPath(left, right []int32) bool {
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

func locate(file *descriptorpb.FileDescriptorProto, path []int32) ([]int32, error) {
	for _, location := range file.GetSourceCodeInfo().GetLocation() {
		if equalPath(location.GetPath(), path) {
			return location.GetSpan(), nil
		}
	}
	return nil, errors.New("descriptor source span is absent")
}

func lineOffsets(source []byte) []int {
	result := []int{0}
	for index, value := range source {
		if value == '\n' {
			result = append(result, index+1)
		}
	}
	return result
}

func positions(source []byte, span []int32) (position, position, error) {
	if len(span) != 3 && len(span) != 4 {
		return position{}, position{}, fmt.Errorf("unsupported descriptor source span: %v", span)
	}
	startLine, startColumn := int(span[0]), int(span[1])
	endLine, endColumn := startLine, int(span[2])
	if len(span) == 4 {
		endLine, endColumn = int(span[2]), int(span[3])
	}
	offsets := lineOffsets(source)
	if startLine < 0 || endLine < startLine || startLine >= len(offsets) || endLine >= len(offsets) {
		return position{}, position{}, errors.New("descriptor source span line is out of bounds")
	}
	startOffset := offsets[startLine] + startColumn
	endOffset := offsets[endLine] + endColumn
	if startOffset < 0 || startOffset >= endOffset || endOffset > len(source) {
		return position{}, position{}, errors.New("descriptor source span byte range is out of bounds")
	}
	return position{startOffset, startLine + 1, startColumn + 1}, position{endOffset, endLine + 1, endColumn + 1}, nil
}

func run() error {
	var sourcePath, symbol, locatorID, commit, manifestPath, manifestSHA, repoRoot, outputPath, resolvedAt string
	flag.StringVar(&sourcePath, "source", "", "repository-relative .proto source path")
	flag.StringVar(&symbol, "symbol", "", "exact Protobuf fully qualified name")
	flag.StringVar(&locatorID, "locator-id", "", "stable locator identity")
	flag.StringVar(&commit, "candidate-commit", "", "40-hex candidate commit")
	flag.StringVar(&manifestPath, "candidate-manifest", "", "repository-relative candidate manifest")
	flag.StringVar(&manifestSHA, "candidate-manifest-sha256", "", "candidate manifest SHA-256")
	flag.StringVar(&repoRoot, "repo-root", ".", "repository root")
	flag.StringVar(&outputPath, "output", "", "repository-relative output JSON; stdout when empty")
	flag.StringVar(&resolvedAt, "resolved-at", "", "RFC3339 UTC instant")
	flag.Parse()
	if sourcePath == "" || symbol == "" || locatorID == "" || commit == "" || manifestPath == "" || manifestSHA == "" {
		return errors.New("source, symbol, locator-id, candidate-commit, candidate-manifest and candidate-manifest-sha256 are required")
	}
	if !strings.HasPrefix(sourcePath, "proto/") || !strings.HasSuffix(sourcePath, ".proto") {
		return errors.New("source must be a repository-relative proto/*.proto path")
	}
	if !strings.HasPrefix(locatorID, "LOC-") || !hex40.MatchString(commit) {
		return errors.New("locator-id or candidate-commit is invalid")
	}
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return err
	}
	sourceAbsolute, err := safeRegular(root, sourcePath)
	if err != nil {
		return err
	}
	manifestAbsolute, err := safeRegular(root, manifestPath)
	if err != nil {
		return err
	}
	resolverAbsolute, err := safeRegular(root, resolverSource)
	if err != nil {
		return err
	}
	manifestBytes, err := os.ReadFile(manifestAbsolute)
	if err != nil {
		return err
	}
	if digest(manifestBytes) != manifestSHA {
		return errors.New("candidate manifest SHA-256 mismatch")
	}
	var candidate manifest
	if err := json.Unmarshal(manifestBytes, &candidate); err != nil {
		return err
	}
	if candidate.Commit != commit {
		return errors.New("candidate manifest commit mismatch")
	}
	current, err := os.ReadFile(sourceAbsolute)
	if err != nil {
		return err
	}
	frozen, err := gitOutput(root, "show", commit+":"+sourcePath)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, frozen) {
		return errors.New("worktree source differs from frozen candidate")
	}
	if candidate.Blobs[sourcePath] != digest(frozen) {
		return errors.New("candidate source SHA-256 differs from manifest")
	}
	temp, err := os.MkdirTemp("", "traffic-proto-descriptor-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	if err := stageCandidateProto(root, commit, temp); err != nil {
		return err
	}
	descriptorPath := filepath.Join(temp, "candidate.pb")
	target := strings.TrimPrefix(sourcePath, "proto/")
	command := exec.Command(
		"protoc", "--proto_path="+temp, "--include_imports", "--include_source_info",
		"--descriptor_set_out="+descriptorPath, target,
	)
	command.Dir = temp
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("candidate descriptor compilation failed: %s: %w", strings.TrimSpace(string(output)), err)
	}
	descriptorBytes, err := os.ReadFile(descriptorPath)
	if err != nil {
		return err
	}
	set := new(descriptorpb.FileDescriptorSet)
	if err := proto.Unmarshal(descriptorBytes, set); err != nil {
		return err
	}
	for _, item := range set.GetFile() {
		repositoryPath := "proto/" + item.GetName()
		input, err := gitOutput(root, "show", commit+":"+repositoryPath)
		if err != nil {
			return err
		}
		if candidate.Blobs[repositoryPath] != digest(input) {
			return fmt.Errorf("candidate manifest does not bind descriptor input: %s", repositoryPath)
		}
	}
	var file *descriptorpb.FileDescriptorProto
	for _, item := range set.GetFile() {
		if item.GetName() == target {
			file = item
			break
		}
	}
	if file == nil {
		return errors.New("compiled descriptor set lacks target source")
	}
	matches := make([]descriptorMatch, 0)
	for _, item := range collectMatches(file) {
		if item.qualified == symbol {
			matches = append(matches, item)
		}
	}
	if len(matches) != 1 {
		return fmt.Errorf("expected exactly one descriptor match for %q, got %d", symbol, len(matches))
	}
	match := matches[0]
	span, err := locate(file, match.path)
	if err != nil {
		return err
	}
	start, end, err := positions(frozen, span)
	if err != nil {
		return err
	}
	normalized, err := proto.MarshalOptions{Deterministic: true}.Marshal(match.message)
	if err != nil {
		return err
	}
	protocVersion, err := exec.Command("protoc", "--version").Output()
	if err != nil {
		return err
	}
	if resolvedAt == "" {
		return errors.New("resolved-at is required for an immutable receipt")
	}
	parsed, err := time.Parse(time.RFC3339Nano, resolvedAt)
	if err != nil || parsed.Location() != time.UTC || !strings.HasSuffix(resolvedAt, "Z") {
		return errors.New("resolved-at must be RFC3339 UTC ending in Z")
	}
	resolverBytes, err := os.ReadFile(resolverAbsolute)
	if err != nil {
		return err
	}
	payload := receipt{
		SchemaVersion: "1.0.0", ArtifactKind: "PROTO_DESCRIPTOR_LOCATOR_RESOLUTION_RECEIPT",
		Status: "RESOLVED", ProofLevel: "COMPILED_DESCRIPTOR",
		Candidate: hashRef{commit, manifestPath, manifestSHA},
		Resolver: resolverRef{
			ResolverID: "traffic-proto-descriptor-locator@1", Engine: "protoc/descriptorpb",
			EngineVersion: strings.TrimSpace(string(protocVersion)) + "; google.golang.org/protobuf-v1.36.11",
			SourcePath:    resolverSource, SourceSHA256: digest(resolverBytes),
		},
		Locator: locator{
			LocatorID: locatorID, Language: "protobuf", Path: sourcePath,
			Package: file.GetPackage(), QualifiedSymbol: symbol, DeclarationKind: match.kind,
			Query: symbol, MatchStrategy: "EXACT_COMPILED_DESCRIPTOR_FQN",
			Signature: match.signature, CandidateBlobSHA: digest(frozen),
			DescriptorSetSHA: digest(descriptorBytes), SourceSpanSHA: digest(frozen[start.ByteOffset:end.ByteOffset]),
			NormalizedDescSHA: digest(normalized), Start: start, End: end,
		},
		AmbiguityCount: 1, ResolvedAt: resolvedAt,
		ProofCeiling: "EXACT_LOCATOR_ONLY_NOT_SCHEMA_DESIGN_COMPATIBILITY_OR_EXECUTION_AUTHORIZATION",
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	if outputPath == "" {
		_, err = os.Stdout.Write(encoded)
		return err
	}
	outputAbsolute, err := safeOutput(root, outputPath)
	if err != nil {
		return err
	}
	if existing, readErr := os.ReadFile(outputAbsolute); readErr == nil {
		if bytes.Equal(existing, encoded) {
			return nil
		}
		return errors.New("immutable output already exists with different bytes")
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	fileHandle, err := os.OpenFile(outputAbsolute, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer fileHandle.Close()
	_, err = fileHandle.Write(encoded)
	return err
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
}
