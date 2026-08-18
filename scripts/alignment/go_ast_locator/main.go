// Command go_ast_locator resolves one Go top-level declaration, receiver
// method, or direct struct field against a frozen candidate blob. It emits a
// deterministic LANGUAGE_AST receipt; it never grants function-design or
// execution authorization.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

type candidateRef struct {
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

type callRef struct {
	Expression string `json:"expression"`
	Line       int    `json:"line"`
}

type locator struct {
	LocatorID        string    `json:"locator_id"`
	Language         string    `json:"language"`
	Path             string    `json:"path"`
	Package          string    `json:"package"`
	QualifiedSymbol  string    `json:"qualified_symbol"`
	DeclarationKind  string    `json:"declaration_kind"`
	Query            string    `json:"query"`
	MatchStrategy    string    `json:"match_strategy"`
	Signature        string    `json:"signature"`
	CandidateBlobSHA string    `json:"candidate_blob_sha256"`
	SourceSpanSHA    string    `json:"source_span_sha256"`
	NormalizedASTSHA string    `json:"normalized_ast_sha256"`
	Start            position  `json:"start"`
	End              position  `json:"end"`
	Calls            []callRef `json:"calls"`
}

type receipt struct {
	SchemaVersion  string       `json:"schema_version"`
	ArtifactKind   string       `json:"artifact_kind"`
	Status         string       `json:"status"`
	ProofLevel     string       `json:"proof_level"`
	Candidate      candidateRef `json:"candidate"`
	Resolver       resolverRef  `json:"resolver"`
	Locator        locator      `json:"locator"`
	AmbiguityCount int          `json:"ambiguity_count"`
	ResolvedAt     string       `json:"resolved_at"`
	ProofCeiling   string       `json:"proof_ceiling"`
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func cleanRelative(relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) {
		return "", fmt.Errorf("path must be repository-relative: %q", relative)
	}
	cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(relative)))
	if cleaned != relative || cleaned == "." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("path must be canonical and repository-relative: %q", relative)
	}
	return cleaned, nil
}

func safeRegularFile(root, relative string) (string, error) {
	cleaned, err := cleanRelative(relative)
	if err != nil {
		return "", err
	}
	cursor := root
	parts := strings.Split(cleaned, "/")
	for index, part := range parts {
		cursor = filepath.Join(cursor, filepath.FromSlash(part))
		info, statErr := os.Lstat(cursor)
		if statErr != nil {
			return "", statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("repository path contains a symlink: %q", relative)
		}
		if index < len(parts)-1 && !info.IsDir() {
			return "", fmt.Errorf("repository path parent is not a directory: %q", relative)
		}
	}
	info, err := os.Lstat(cursor)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("repository path is not a regular file: %q", relative)
	}
	return cursor, nil
}

func safeOutput(root, relative string) (string, error) {
	cleaned, err := cleanRelative(relative)
	if err != nil {
		return "", err
	}
	parts := strings.Split(cleaned, "/")
	cursor := root
	for _, part := range parts[:len(parts)-1] {
		cursor = filepath.Join(cursor, filepath.FromSlash(part))
		info, statErr := os.Lstat(cursor)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				break
			}
			return "", statErr
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", fmt.Errorf("output parent is not a regular directory: %q", relative)
		}
	}
	return filepath.Join(root, filepath.FromSlash(cleaned)), nil
}

func receiverTypeName(expr ast.Expr) (string, bool) {
	switch value := expr.(type) {
	case *ast.Ident:
		return value.Name, true
	case *ast.IndexExpr:
		return receiverTypeName(value.X)
	case *ast.IndexListExpr:
		return receiverTypeName(value.X)
	default:
		return "", false
	}
}

func receiverName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) != 1 {
		return ""
	}
	expr := fn.Recv.List[0].Type
	if star, ok := expr.(*ast.StarExpr); ok {
		if name, ok := receiverTypeName(star.X); ok {
			return "(*" + name + ")"
		}
	}
	if name, ok := receiverTypeName(expr); ok {
		return "(" + name + ")"
	}
	return ""
}

func receiverBaseName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) != 1 {
		return ""
	}
	expr := fn.Recv.List[0].Type
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	name, _ := receiverTypeName(expr)
	return name
}

func expressionString(fset *token.FileSet, expr ast.Expr) string {
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, expr); err != nil {
		return "<unprintable>"
	}
	return buf.String()
}

func signatureString(fset *token.FileSet, fn *ast.FuncDecl) (string, error) {
	clone := *fn
	clone.Body = nil
	clone.Doc = nil
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, &clone); err != nil {
		return "", err
	}
	return strings.TrimSpace(buf.String()), nil
}

func nodeString(fset *token.FileSet, node ast.Node) (string, error) {
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, node); err != nil {
		return "", err
	}
	return strings.TrimSpace(buf.String()), nil
}

type declaration struct {
	node       ast.Node
	kind       string
	query      string
	qualified  string
	signature  string
	normalized string
	callRoot   ast.Node
}

func fieldString(fset *token.FileSet, field *ast.Field) (string, error) {
	typeName, err := nodeString(fset, field.Type)
	if err != nil {
		return "", err
	}
	names := make([]string, 0, len(field.Names))
	for _, name := range field.Names {
		names = append(names, name.Name)
	}
	rendered := strings.TrimSpace(strings.Join(names, ", ") + " " + typeName)
	if field.Tag != nil {
		rendered += " " + field.Tag.Value
	}
	return rendered, nil
}

func addFunctionDeclaration(result *[]declaration, fset *token.FileSet, packageName string, fn *ast.FuncDecl) error {
	signature, err := signatureString(fset, fn)
	if err != nil {
		return err
	}
	rendered, err := nodeString(fset, fn)
	if err != nil {
		return err
	}
	query := fn.Name.Name
	qualified := packageName + "." + fn.Name.Name
	kind := "FUNCTION"
	if base := receiverBaseName(fn); base != "" {
		query = base + "." + fn.Name.Name
		qualified = packageName + "." + query
		kind = "METHOD"
	}
	*result = append(*result, declaration{
		node: fn, kind: kind, query: query, qualified: qualified,
		signature: signature, normalized: rendered, callRoot: fn.Body,
	})
	return nil
}

func collectDeclarations(parsed *ast.File, fset *token.FileSet) ([]declaration, error) {
	result := make([]declaration, 0)
	for _, decl := range parsed.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			if err := addFunctionDeclaration(&result, fset, parsed.Name.Name, fn); err != nil {
				return nil, err
			}
			continue
		}
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			switch value := spec.(type) {
			case *ast.TypeSpec:
				rendered, err := nodeString(fset, value)
				if err != nil {
					return nil, err
				}
				result = append(result, declaration{
					node: value, kind: "TYPE", query: value.Name.Name,
					qualified: parsed.Name.Name + "." + value.Name.Name,
					signature: "type " + rendered, normalized: "type " + rendered, callRoot: value,
				})
				structure, ok := value.Type.(*ast.StructType)
				if !ok {
					continue
				}
				for _, field := range structure.Fields.List {
					fieldRendered, err := fieldString(fset, field)
					if err != nil {
						return nil, err
					}
					for _, name := range field.Names {
						query := value.Name.Name + "." + name.Name
						result = append(result, declaration{
							node: field, kind: "STRUCT_FIELD", query: query,
							qualified: parsed.Name.Name + "." + query,
							signature: "field " + fieldRendered, normalized: fieldRendered, callRoot: field,
						})
					}
				}
			case *ast.ValueSpec:
				rendered, err := nodeString(fset, value)
				if err != nil {
					return nil, err
				}
				kind := "VAR"
				prefix := "var "
				if gen.Tok == token.CONST {
					kind, prefix = "CONST", "const "
				}
				for _, name := range value.Names {
					result = append(result, declaration{
						node: value, kind: kind, query: name.Name,
						qualified: parsed.Name.Name + "." + name.Name,
						signature: prefix + rendered, normalized: prefix + rendered, callRoot: value,
					})
				}
			}
		}
	}
	return result, nil
}

func run() error {
	var sourcePath, symbol, locatorID, commit, manifestPath, manifestSHA, repoRoot, outputPath, resolvedAt string
	flag.StringVar(&sourcePath, "source", "", "repository-relative Go source path")
	flag.StringVar(&symbol, "symbol", "", "function name or receiver-qualified method, for example (*T).M")
	flag.StringVar(&locatorID, "locator-id", "", "stable locator identity")
	flag.StringVar(&commit, "candidate-commit", "", "40-hex candidate commit")
	flag.StringVar(&manifestPath, "candidate-manifest", "", "repository-relative candidate manifest")
	flag.StringVar(&manifestSHA, "candidate-manifest-sha256", "", "candidate manifest digest")
	flag.StringVar(&repoRoot, "repo-root", ".", "repository root")
	flag.StringVar(&outputPath, "output", "", "output JSON path; stdout when empty")
	flag.StringVar(&resolvedAt, "resolved-at", "", "RFC3339 UTC time; SOURCE_DATE_EPOCH or current UTC when empty")
	flag.Parse()
	if sourcePath == "" || symbol == "" || locatorID == "" || commit == "" || manifestPath == "" || manifestSHA == "" {
		return fmt.Errorf("source, symbol, locator-id, candidate-commit, candidate-manifest and candidate-manifest-sha256 are required")
	}
	if !strings.HasPrefix(sourcePath, "go/") || !strings.HasSuffix(sourcePath, ".go") {
		return fmt.Errorf("source must be a repository-relative go/*.go path: %q", sourcePath)
	}
	if _, err := cleanRelative(sourcePath); err != nil {
		return fmt.Errorf("unsafe source: %w", err)
	}
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return err
	}
	resolverBytes, err := os.ReadFile(filepath.Join(root, "scripts/alignment/go_ast_locator/main.go"))
	if err != nil {
		return fmt.Errorf("read resolver source: %w", err)
	}
	manifestAbsolute, err := safeRegularFile(root, manifestPath)
	if err != nil {
		return fmt.Errorf("unsafe candidate manifest: %w", err)
	}
	manifestBytes, err := os.ReadFile(manifestAbsolute)
	if err != nil {
		return fmt.Errorf("read candidate manifest: %w", err)
	}
	if digest(manifestBytes) != manifestSHA {
		return fmt.Errorf("candidate manifest sha256 mismatch: %s", manifestPath)
	}
	var manifest map[string]any
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return fmt.Errorf("decode candidate manifest: %w", err)
	}
	manifestCommit, ok := manifest["implementation_candidate_commit"].(string)
	if !ok || manifestCommit != commit {
		return fmt.Errorf("candidate manifest does not bind commit %s", commit)
	}
	declaredBlobs, ok := manifest["source_blob_sha256"].(map[string]any)
	if !ok {
		return fmt.Errorf("candidate manifest lacks source_blob_sha256")
	}
	declaredBlob, ok := declaredBlobs[sourcePath].(string)
	if !ok || declaredBlob == "" {
		return fmt.Errorf("candidate manifest does not bind source %s", sourcePath)
	}
	sourceAbsolute, err := safeRegularFile(root, sourcePath)
	if err != nil {
		return fmt.Errorf("unsafe source: %w", err)
	}
	current, err := os.ReadFile(sourceAbsolute)
	if err != nil {
		return err
	}
	cmd := exec.Command("git", "show", commit+":"+sourcePath)
	cmd.Dir = root
	candidateBytes, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("read candidate blob: %w", err)
	}
	if !bytes.Equal(current, candidateBytes) {
		return fmt.Errorf("worktree source differs from frozen candidate: %s", sourcePath)
	}
	if digest(candidateBytes) != declaredBlob {
		return fmt.Errorf("candidate source sha256 differs from manifest: %s", sourcePath)
	}
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, sourcePath, candidateBytes, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return err
	}
	declarations, err := collectDeclarations(parsed, fset)
	if err != nil {
		return err
	}
	normalizedSymbol := symbol
	if strings.HasPrefix(symbol, "(*") || strings.HasPrefix(symbol, "(") {
		if close := strings.Index(symbol, ")."); close > 1 {
			receiver := strings.TrimPrefix(symbol[:close], "(")
			receiver = strings.TrimPrefix(receiver, "*")
			normalizedSymbol = receiver + symbol[close+1:]
		}
	}
	matches := make([]declaration, 0)
	for _, item := range declarations {
		if item.query == normalizedSymbol {
			matches = append(matches, item)
		}
	}
	if len(matches) != 1 {
		return fmt.Errorf("expected exactly one Go AST declaration match for %q, got %d", symbol, len(matches))
	}
	selected := matches[0]
	start := fset.PositionFor(selected.node.Pos(), false)
	end := fset.PositionFor(selected.node.End(), false)
	if start.Offset < 0 || end.Offset > len(candidateBytes) || start.Offset >= end.Offset {
		return fmt.Errorf("invalid AST byte span %d:%d", start.Offset, end.Offset)
	}
	normalized := []byte(selected.normalized)
	calls := make([]callRef, 0)
	ast.Inspect(selected.callRoot, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		calls = append(calls, callRef{Expression: expressionString(fset, call.Fun), Line: fset.PositionFor(call.Pos(), false).Line})
		return true
	})
	sort.Slice(calls, func(i, j int) bool {
		if calls[i].Line != calls[j].Line {
			return calls[i].Line < calls[j].Line
		}
		return calls[i].Expression < calls[j].Expression
	})
	if resolvedAt == "" {
		if epoch := os.Getenv("SOURCE_DATE_EPOCH"); epoch != "" {
			var seconds int64
			if _, err := fmt.Sscan(epoch, &seconds); err != nil {
				return fmt.Errorf("invalid SOURCE_DATE_EPOCH: %w", err)
			}
			resolvedAt = time.Unix(seconds, 0).UTC().Format(time.RFC3339)
		} else {
			resolvedAt = time.Now().UTC().Format(time.RFC3339)
		}
	}
	parsedTime, err := time.Parse(time.RFC3339, resolvedAt)
	if err != nil || !strings.HasSuffix(resolvedAt, "Z") {
		return fmt.Errorf("resolved-at must be a valid RFC3339 UTC timestamp ending in Z: %q", resolvedAt)
	}
	resolvedAt = parsedTime.UTC().Format(time.RFC3339)
	payload := receipt{
		SchemaVersion: "1.0.0", ArtifactKind: "LOCATOR_RESOLUTION_RECEIPT", Status: "RESOLVED", ProofLevel: "LANGUAGE_AST",
		Candidate: candidateRef{Commit: commit, ManifestPath: manifestPath, ManifestSHA256: manifestSHA},
		Resolver:  resolverRef{ResolverID: "traffic-go-ast-locator@1", Engine: "go/parser", EngineVersion: runtime.Version(), SourcePath: "scripts/alignment/go_ast_locator/main.go", SourceSHA256: digest(resolverBytes)},
		Locator: locator{
			LocatorID: locatorID, Language: "go", Path: sourcePath, Package: parsed.Name.Name, QualifiedSymbol: selected.qualified,
			DeclarationKind: selected.kind, Query: symbol, MatchStrategy: "EXACT_GO_TOP_LEVEL_OR_RECEIVER_MEMBER_DECLARATION", Signature: selected.signature, CandidateBlobSHA: digest(candidateBytes),
			SourceSpanSHA: digest(candidateBytes[start.Offset:end.Offset]), NormalizedASTSHA: digest(normalized),
			Start: position{ByteOffset: start.Offset, Line: start.Line, Column: start.Column},
			End:   position{ByteOffset: end.Offset, Line: end.Line, Column: end.Column}, Calls: calls,
		},
		AmbiguityCount: 1, ResolvedAt: resolvedAt,
		ProofCeiling: "EXACT_LOCATOR_ONLY_NOT_FUNCTION_DESIGN_OR_EXECUTION_AUTHORIZATION",
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
	output, err := safeOutput(root, outputPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	if _, err := safeOutput(root, outputPath); err != nil {
		return err
	}
	if info, statErr := os.Lstat(output); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("output exists and is not a regular file: %q", outputPath)
		}
		existing, readErr := os.ReadFile(output)
		if readErr != nil {
			return readErr
		}
		if bytes.Equal(existing, encoded) {
			return nil
		}
		return fmt.Errorf("immutable output already exists with different content: %q", outputPath)
	} else if !os.IsNotExist(statErr) {
		return statErr
	}
	file, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(encoded); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
