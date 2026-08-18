// Command shell_ast_locator resolves one literal topic entry from a frozen
// shell syntax tree. It emits locator evidence only, never execution authority.
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

	"mvdan.cc/sh/v3/syntax"
)

const resolverSource = "scripts/alignment/shell_ast_locator/main.go"

var hex40 = regexp.MustCompile(`^[0-9a-f]{40}$`)

type manifest struct {
	Commit string            `json:"implementation_candidate_commit"`
	Blobs  map[string]string `json:"source_blob_sha256"`
}

type wordMatch struct {
	word     *syntax.Word
	rendered string
	literal  string
}

func digest(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func safeRelative(value string) error {
	if value == "" || filepath.IsAbs(value) || filepath.Clean(value) != filepath.FromSlash(value) {
		return fmt.Errorf("path contains an unsafe component: %s", value)
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("path contains an unsafe component: %s", value)
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

func printNode(node syntax.Node) (string, error) {
	var output bytes.Buffer
	if err := syntax.NewPrinter().Print(&output, node); err != nil {
		return "", err
	}
	return output.String(), nil
}

func staticWordText(word *syntax.Word) (string, bool) {
	var result strings.Builder
	for _, part := range word.Parts {
		switch value := part.(type) {
		case *syntax.Lit:
			result.WriteString(value.Value)
		case *syntax.DblQuoted:
			for _, quotedPart := range value.Parts {
				literal, ok := quotedPart.(*syntax.Lit)
				if !ok {
					return "", false
				}
				result.WriteString(literal.Value)
			}
		default:
			return "", false
		}
	}
	return result.String(), true
}

func run() error {
	var sourcePath, query, locatorID, commit, manifestPath, manifestSHA, repoRoot, outputPath, resolvedAt string
	flag.StringVar(&sourcePath, "source", "", "repository-relative shell source")
	flag.StringVar(&query, "query", "", "exact first colon-delimited literal field")
	flag.StringVar(&locatorID, "locator-id", "", "stable locator identity")
	flag.StringVar(&commit, "candidate-commit", "", "40-hex candidate commit")
	flag.StringVar(&manifestPath, "candidate-manifest", "", "repository-relative candidate manifest")
	flag.StringVar(&manifestSHA, "candidate-manifest-sha256", "", "candidate manifest SHA-256")
	flag.StringVar(&repoRoot, "repo-root", ".", "repository root")
	flag.StringVar(&outputPath, "output", "", "repository-relative output JSON; stdout when empty")
	flag.StringVar(&resolvedAt, "resolved-at", "", "RFC3339 UTC instant")
	flag.Parse()
	if sourcePath == "" || query == "" || locatorID == "" || commit == "" || manifestPath == "" || manifestSHA == "" {
		return errors.New("source, query, locator-id, candidate-commit, candidate-manifest and candidate-manifest-sha256 are required")
	}
	if !strings.HasSuffix(sourcePath, ".sh") || !strings.HasPrefix(locatorID, "LOC-") || !hex40.MatchString(commit) {
		return errors.New("source, locator-id or candidate-commit is invalid")
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
	parser := syntax.NewParser(syntax.Variant(syntax.LangBash))
	file, err := parser.Parse(bytes.NewReader(frozen), sourcePath)
	if err != nil {
		return fmt.Errorf("shell AST parse failed: %w", err)
	}
	matches := make([]wordMatch, 0)
	syntax.Walk(file, func(node syntax.Node) bool {
		word, ok := node.(*syntax.Word)
		if !ok {
			return true
		}
		literal, static := staticWordText(word)
		if !static {
			return true
		}
		if !strings.Contains(literal, ":") || strings.SplitN(literal, ":", 2)[0] != query {
			return true
		}
		rendered, printErr := printNode(word)
		if printErr == nil {
			matches = append(matches, wordMatch{word: word, rendered: rendered, literal: literal})
		}
		return true
	})
	if len(matches) != 1 {
		return fmt.Errorf("expected exactly one shell AST word match for %q, got %d", query, len(matches))
	}
	match := matches[0]
	start := int(match.word.Pos().Offset())
	end := int(match.word.End().Offset())
	if start < 0 || start >= end || end > len(frozen) {
		return errors.New("shell AST byte span is out of bounds")
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
	payload := map[string]any{
		"schema_version": "1.0.0",
		"artifact_kind":  "SHELL_AST_LOCATOR_RESOLUTION_RECEIPT",
		"status":         "RESOLVED",
		"proof_level":    "LANGUAGE_AST",
		"candidate":      map[string]any{"commit": commit, "manifest_path": manifestPath, "manifest_sha256": manifestSHA},
		"resolver": map[string]any{
			"resolver_id": "traffic-shell-ast-locator@1", "engine": "mvdan.cc/sh/syntax",
			"engine_version": "mvdan.cc/sh/v3-v3.13.1", "source_path": resolverSource,
			"source_sha256": digest(resolverBytes),
		},
		"locator": map[string]any{
			"locator_id": locatorID, "language": "shell", "path": sourcePath, "query": query,
			"match_strategy": "EXACT_AST_WORD_FIRST_COLON_FIELD", "rendered_word": match.rendered,
			"literal_value": match.literal, "candidate_blob_sha256": digest(frozen),
			"source_span_sha256":    digest(frozen[start:end]),
			"normalized_ast_sha256": digest([]byte(match.rendered)),
			"start":                 map[string]any{"byte_offset": start, "line": int(match.word.Pos().Line()), "column": int(match.word.Pos().Col())},
			"end":                   map[string]any{"byte_offset": end, "line": int(match.word.End().Line()), "column": int(match.word.End().Col())},
		},
		"ambiguity_count": 1,
		"resolved_at":     resolvedAt,
		"proof_ceiling":   "EXACT_LOCATOR_ONLY_NOT_SHELL_BEHAVIOR_DEPLOYMENT_OR_EXECUTION_AUTHORIZATION",
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
	handle, err := os.OpenFile(outputAbsolute, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer handle.Close()
	_, err = handle.Write(encoded)
	return err
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
}
