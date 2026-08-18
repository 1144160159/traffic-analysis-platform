import com.sun.source.tree.AnnotationTree;
import com.sun.source.tree.ClassTree;
import com.sun.source.tree.CompilationUnitTree;
import com.sun.source.tree.ExpressionTree;
import com.sun.source.tree.MethodInvocationTree;
import com.sun.source.tree.MethodTree;
import com.sun.source.tree.NewClassTree;
import com.sun.source.tree.Tree;
import com.sun.source.tree.TypeParameterTree;
import com.sun.source.tree.VariableTree;
import com.sun.source.util.JavacTask;
import com.sun.source.util.SourcePositions;
import com.sun.source.util.TreeScanner;
import com.sun.source.util.Trees;

import javax.tools.DiagnosticCollector;
import javax.tools.JavaCompiler;
import javax.tools.JavaFileObject;
import javax.tools.SimpleJavaFileObject;
import javax.tools.StandardJavaFileManager;
import javax.tools.ToolProvider;
import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.OutputStream;
import java.net.URI;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.LinkOption;
import java.nio.file.OpenOption;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.nio.file.StandardOpenOption;
import java.security.MessageDigest;
import java.time.Instant;
import java.time.format.DateTimeParseException;
import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Comparator;
import java.util.Deque;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Set;
import java.util.regex.Pattern;

/**
 * Resolves one Java type, method, or constructor from a Git-frozen candidate.
 * The receipt proves only an exact javac syntax-tree location. It grants no
 * function-design, implementation, review, or execution authority.
 */
public final class JavaAstLocator {
    private static final String RESOLVER_PATH = "scripts/alignment/java_ast_locator/JavaAstLocator.java";
    private static final Pattern HEX40 = Pattern.compile("^[0-9a-f]{40}$");
    private static final Pattern HEX64 = Pattern.compile("^[0-9a-f]{64}$");
    private static final Pattern LOCATOR_ID = Pattern.compile("^LOC-[A-Za-z0-9._-]+$");

    private JavaAstLocator() {}

    private record Position(int byteOffset, long line, long column) {}
    private record CallRef(String expression, long line) {}
    private record Match(
            Tree tree,
            String classPath,
            String memberName,
            String declarationKind,
            String signature,
            String qualifiedSymbol,
            long startChar,
            long endChar,
            List<CallRef> calls) {}

    private static final class SourceObject extends SimpleJavaFileObject {
        private final String source;

        SourceObject(String path, String source) {
            super(URI.create("string:///" + path.replace('\\', '/')), Kind.SOURCE);
            this.source = source;
        }

        @Override
        public CharSequence getCharContent(boolean ignoreEncodingErrors) {
            return source;
        }
    }

    private static final class JsonParser {
        private final String input;
        private int offset;

        JsonParser(String input) { this.input = input; }

        Object parse() {
            Object value = parseValue();
            whitespace();
            if (offset != input.length()) throw fail("trailing JSON content");
            return value;
        }

        private Object parseValue() {
            whitespace();
            if (offset >= input.length()) throw fail("unexpected end of JSON");
            char c = input.charAt(offset);
            if (c == '{') return object();
            if (c == '[') return array();
            if (c == '"') return string();
            if (input.startsWith("true", offset)) { offset += 4; return Boolean.TRUE; }
            if (input.startsWith("false", offset)) { offset += 5; return Boolean.FALSE; }
            if (input.startsWith("null", offset)) { offset += 4; return null; }
            return number();
        }

        private Map<String, Object> object() {
            LinkedHashMap<String, Object> result = new LinkedHashMap<>();
            expect('{'); whitespace();
            if (peek('}')) { offset++; return result; }
            while (true) {
                whitespace();
                if (!peek('"')) throw fail("object key must be a string");
                String key = string();
                whitespace(); expect(':');
                result.put(key, parseValue());
                whitespace();
                if (peek('}')) { offset++; return result; }
                expect(',');
            }
        }

        private List<Object> array() {
            ArrayList<Object> result = new ArrayList<>();
            expect('['); whitespace();
            if (peek(']')) { offset++; return result; }
            while (true) {
                result.add(parseValue()); whitespace();
                if (peek(']')) { offset++; return result; }
                expect(',');
            }
        }

        private String string() {
            expect('"');
            StringBuilder result = new StringBuilder();
            while (offset < input.length()) {
                char c = input.charAt(offset++);
                if (c == '"') return result.toString();
                if (c != '\\') { result.append(c); continue; }
                if (offset >= input.length()) throw fail("unterminated JSON escape");
                char escaped = input.charAt(offset++);
                switch (escaped) {
                    case '"', '\\', '/' -> result.append(escaped);
                    case 'b' -> result.append('\b');
                    case 'f' -> result.append('\f');
                    case 'n' -> result.append('\n');
                    case 'r' -> result.append('\r');
                    case 't' -> result.append('\t');
                    case 'u' -> {
                        if (offset + 4 > input.length()) throw fail("short JSON unicode escape");
                        result.append((char) Integer.parseInt(input.substring(offset, offset + 4), 16));
                        offset += 4;
                    }
                    default -> throw fail("unsupported JSON escape");
                }
            }
            throw fail("unterminated JSON string");
        }

        private Number number() {
            int start = offset;
            while (offset < input.length() && "-+0123456789.eE".indexOf(input.charAt(offset)) >= 0) offset++;
            if (start == offset) throw fail("invalid JSON token");
            String token = input.substring(start, offset);
            try {
                return token.contains(".") || token.contains("e") || token.contains("E")
                        ? Double.parseDouble(token) : Long.parseLong(token);
            } catch (NumberFormatException error) {
                throw fail("invalid JSON number");
            }
        }

        private void whitespace() {
            while (offset < input.length() && Character.isWhitespace(input.charAt(offset))) offset++;
        }

        private boolean peek(char expected) { return offset < input.length() && input.charAt(offset) == expected; }
        private void expect(char expected) {
            whitespace();
            if (!peek(expected)) throw fail("expected '" + expected + "'");
            offset++;
        }
        private IllegalArgumentException fail(String message) {
            return new IllegalArgumentException(message + " at JSON offset " + offset);
        }
    }

    public static void main(String[] args) {
        try {
            run(args);
        } catch (Exception error) {
            System.err.println(error.getMessage());
            System.exit(1);
        }
    }

    private static void run(String[] args) throws Exception {
        Map<String, String> options = parseArgs(args);
        String sourcePath = required(options, "source");
        String symbol = required(options, "symbol");
        String locatorId = required(options, "locator-id");
        String commit = required(options, "candidate-commit");
        String manifestPath = required(options, "candidate-manifest");
        String manifestHash = required(options, "candidate-manifest-sha256");
        String repoRoot = options.getOrDefault("repo-root", ".");
        String outputPath = options.get("output");
        String resolvedAt = options.get("resolved-at");

        if (!sourcePath.startsWith("java/") || !sourcePath.endsWith(".java")) {
            throw new IllegalArgumentException("source must be a repository-relative java/*.java path: " + sourcePath);
        }
        validateSafeRelative(sourcePath, "source");
        validateSafeRelative(manifestPath, "candidate manifest");
        if (outputPath != null) validateSafeRelative(outputPath, "output");
        if (!HEX40.matcher(commit).matches()) throw new IllegalArgumentException("candidate commit must be 40 lowercase hex characters");
        if (!HEX64.matcher(manifestHash).matches()) throw new IllegalArgumentException("candidate manifest SHA-256 must be 64 lowercase hex characters");
        if (!LOCATOR_ID.matcher(locatorId).matches()) throw new IllegalArgumentException("locator-id format is invalid");

        Path root = Paths.get(repoRoot).toAbsolutePath().normalize();
        Path source = safeExistingFile(root, sourcePath, "source");
        Path manifest = safeExistingFile(root, manifestPath, "candidate manifest");
        Path resolver = safeExistingFile(root, RESOLVER_PATH, "resolver source");
        byte[] manifestBytes = Files.readAllBytes(manifest);
        if (!sha256(manifestBytes).equals(manifestHash)) {
            throw new IllegalArgumentException("candidate manifest SHA-256 mismatch: " + manifestPath);
        }
        Object parsedManifest = new JsonParser(new String(manifestBytes, StandardCharsets.UTF_8)).parse();
        if (!(parsedManifest instanceof Map<?, ?> manifestMap)) {
            throw new IllegalArgumentException("candidate manifest must be a JSON object");
        }
        if (!commit.equals(manifestMap.get("implementation_candidate_commit"))) {
            throw new IllegalArgumentException("candidate manifest does not bind commit " + commit);
        }
        Object rawBlobs = manifestMap.get("source_blob_sha256");
        if (!(rawBlobs instanceof Map<?, ?> blobs) || !(blobs.get(sourcePath) instanceof String declaredBlob)) {
            throw new IllegalArgumentException("candidate manifest does not bind source " + sourcePath);
        }

        byte[] currentBytes = Files.readAllBytes(source);
        byte[] candidateBytes = gitShow(root, commit, sourcePath);
        if (!Arrays.equals(currentBytes, candidateBytes)) {
            throw new IllegalArgumentException("worktree source differs from frozen candidate: " + sourcePath);
        }
        if (!sha256(candidateBytes).equals(declaredBlob)) {
            throw new IllegalArgumentException("candidate source SHA-256 differs from manifest: " + sourcePath);
        }

        String sourceText = new String(candidateBytes, StandardCharsets.UTF_8);
        ParsedJava parsed = parseJava(sourcePath, sourceText);
        List<Match> matches = collectMatches(parsed, sourceText, symbol);
        if (matches.size() != 1) {
            throw new IllegalArgumentException("expected exactly one Java AST match for \"" + symbol + "\", got " + matches.size());
        }
        Match match = matches.get(0);
        int startChar = Math.toIntExact(match.startChar());
        int endChar = Math.toIntExact(match.endChar());
        if (startChar < 0 || endChar <= startChar || endChar > sourceText.length()) {
            throw new IllegalArgumentException("invalid javac source span " + startChar + ":" + endChar);
        }
        int startByte = sourceText.substring(0, startChar).getBytes(StandardCharsets.UTF_8).length;
        int endByte = sourceText.substring(0, endChar).getBytes(StandardCharsets.UTF_8).length;
        byte[] spanBytes = sourceText.substring(startChar, endChar).getBytes(StandardCharsets.UTF_8);
        Position start = new Position(startByte, parsed.unit().getLineMap().getLineNumber(startChar), parsed.unit().getLineMap().getColumnNumber(startChar));
        Position end = new Position(endByte, parsed.unit().getLineMap().getLineNumber(endChar), parsed.unit().getLineMap().getColumnNumber(endChar));

        if (resolvedAt == null || resolvedAt.isBlank()) {
            String epoch = System.getenv("SOURCE_DATE_EPOCH");
            resolvedAt = epoch == null || epoch.isBlank() ? Instant.now().toString() : Instant.ofEpochSecond(Long.parseLong(epoch)).toString();
        }
        try {
            Instant parsedTime = Instant.parse(resolvedAt);
            if (!resolvedAt.endsWith("Z")) throw new DateTimeParseException("not UTC", resolvedAt, resolvedAt.length());
            resolvedAt = parsedTime.toString();
        } catch (DateTimeParseException error) {
            throw new IllegalArgumentException("resolved-at must be a valid RFC3339 UTC timestamp ending in Z: " + resolvedAt);
        }

        LinkedHashMap<String, Object> payload = map(
                "schema_version", "1.0.0",
                "artifact_kind", "JAVA_LOCATOR_RESOLUTION_RECEIPT",
                "status", "RESOLVED",
                "proof_level", "LANGUAGE_AST",
                "candidate", map("commit", commit, "manifest_path", manifestPath, "manifest_sha256", manifestHash),
                "resolver", map(
                        "resolver_id", "traffic-java-javac-locator@1",
                        "engine", "jdk.compiler:JavacTask+TreePathScanner",
                        "engine_version", System.getProperty("java.version"),
                        "source_path", RESOLVER_PATH,
                        "source_sha256", sha256(Files.readAllBytes(resolver))),
                "locator", map(
                        "locator_id", locatorId,
                        "language", "java",
                        "path", sourcePath,
                        "package", parsed.packageName(),
                        "qualified_symbol", match.qualifiedSymbol(),
                        "declaration_kind", match.declarationKind(),
                        "query", symbol,
                        "match_strategy", "EXACT_JAVAC_TREE_TYPE_OR_MEMBER_WITH_OPTIONAL_PARAMETER_TYPES",
                        "signature", match.signature(),
                        "candidate_blob_sha256", sha256(candidateBytes),
                        "source_span_sha256", sha256(spanBytes),
                        "normalized_ast_sha256", sha256(normalizeTree(match.tree()).getBytes(StandardCharsets.UTF_8)),
                        "start", positionMap(start),
                        "end", positionMap(end),
                        "calls", callMaps(match.calls())),
                "ambiguity_count", 1,
                "resolved_at", resolvedAt,
                "proof_ceiling", "EXACT_LOCATOR_ONLY_NOT_FUNCTION_DESIGN_OR_EXECUTION_AUTHORIZATION");
        byte[] encoded = (json(payload, 0) + "\n").getBytes(StandardCharsets.UTF_8);
        if (outputPath == null) {
            System.out.write(encoded);
        } else {
            writeImmutable(root, outputPath, encoded);
        }
    }

    private record ParsedJava(CompilationUnitTree unit, Trees trees, SourcePositions positions, String packageName) {}

    private static ParsedJava parseJava(String sourcePath, String sourceText) throws IOException {
        JavaCompiler compiler = ToolProvider.getSystemJavaCompiler();
        if (compiler == null) throw new IllegalStateException("JDK compiler is unavailable; a JRE is insufficient");
        DiagnosticCollector<JavaFileObject> diagnostics = new DiagnosticCollector<>();
        try (StandardJavaFileManager manager = compiler.getStandardFileManager(diagnostics, Locale.ROOT, StandardCharsets.UTF_8)) {
            JavacTask task = (JavacTask) compiler.getTask(null, manager, diagnostics, List.of("-proc:none"), null, List.of(new SourceObject(sourcePath, sourceText)));
            List<CompilationUnitTree> units = new ArrayList<>();
            task.parse().forEach(units::add);
            if (units.size() != 1 || diagnostics.getDiagnostics().stream().anyMatch(item -> item.getKind() == javax.tools.Diagnostic.Kind.ERROR)) {
                throw new IllegalArgumentException("javac syntax parse failed: " + diagnostics.getDiagnostics());
            }
            CompilationUnitTree unit = units.get(0);
            String packageName = unit.getPackageName() == null ? "" : unit.getPackageName().toString();
            if (packageName.isBlank()) throw new IllegalArgumentException("Java source must declare a package");
            Trees trees = Trees.instance(task);
            return new ParsedJava(unit, trees, trees.getSourcePositions(), packageName);
        }
    }

    private static List<Match> collectMatches(ParsedJava parsed, String sourceText, String query) {
        Query wanted = Query.parse(query);
        ArrayList<Match> matches = new ArrayList<>();
        new TreeScanner<Void, Void>() {
            private final Deque<String> classes = new ArrayDeque<>();

            @Override
            public Void visitClass(ClassTree node, Void unused) {
                String name = node.getSimpleName().toString();
                if (name.isBlank()) return super.visitClass(node, unused);
                classes.addLast(name);
                String classPath = String.join(".", classes);
                if (!wanted.isMember() && wanted.matchesType(classPath, name)) {
                    matches.add(toMatch(parsed, sourceText, node, classPath, null, classKind(node), classSignature(node, classPath), parsed.packageName() + "." + classPath));
                }
                super.visitClass(node, unused);
                classes.removeLast();
                return null;
            }

            @Override
            public Void visitMethod(MethodTree node, Void unused) {
                if (classes.isEmpty()) return super.visitMethod(node, unused);
                String classPath = String.join(".", classes);
                String rawName = node.getName().toString();
                boolean constructor = rawName.equals("<init>");
                String member = constructor ? "<init>" : rawName;
                List<String> parameterTypes = node.getParameters().stream().map(item -> normalizeType(item.getType().toString())).toList();
                if (wanted.isMember() && wanted.matchesMember(classPath, classes.getLast(), member, parameterTypes)) {
                    String qualified = parsed.packageName() + "." + classPath + (constructor ? ".<init>" : "." + member);
                    matches.add(toMatch(parsed, sourceText, node, classPath, member, constructor ? "CONSTRUCTOR" : "METHOD", methodSignature(node, classes.getLast()), qualified));
                }
                return super.visitMethod(node, unused);
            }
        }.scan(parsed.unit(), null);
        return matches;
    }

    private record Query(String base, List<String> parameters, boolean parameterQualified, boolean member) {
        static Query parse(String raw) {
            String value = raw.trim();
            if (value.isEmpty()) throw new IllegalArgumentException("symbol must not be empty");
            int open = value.indexOf('(');
            if (open < 0) return new Query(value, List.of(), false, value.contains(".") && Character.isLowerCase(value.charAt(value.lastIndexOf('.') + 1)) || value.endsWith(".<init>"));
            if (!value.endsWith(")") || value.indexOf('(', open + 1) >= 0) throw new IllegalArgumentException("invalid Java member query: " + raw);
            String base = value.substring(0, open).trim();
            String body = value.substring(open + 1, value.length() - 1).trim();
            List<String> params = body.isEmpty() ? List.of() : Arrays.stream(body.split(",")).map(String::trim).map(JavaAstLocator::normalizeType).toList();
            return new Query(base, params, true, true);
        }

        boolean isMember() { return member; }
        boolean matchesType(String classPath, String simple) { return base.equals(classPath) || base.equals(simple); }
        boolean matchesMember(String classPath, String simpleClass, String memberName, List<String> actualParams) {
            String exact = classPath + "." + memberName;
            String simple = simpleClass + "." + memberName;
            boolean nameMatches = base.equals(exact) || base.equals(simple) || base.equals(memberName);
            return nameMatches && (!parameterQualified || parameters.equals(actualParams));
        }
    }

    private static Match toMatch(ParsedJava parsed, String sourceText, Tree tree, String classPath, String memberName, String kind, String signature, String qualified) {
        long start = parsed.positions().getStartPosition(parsed.unit(), tree);
        long end = parsed.positions().getEndPosition(parsed.unit(), tree);
        ArrayList<CallRef> calls = new ArrayList<>();
        new TreeScanner<Void, Void>() {
            @Override public Void visitMethodInvocation(MethodInvocationTree node, Void unused) {
                calls.add(new CallRef(node.getMethodSelect().toString(), parsed.unit().getLineMap().getLineNumber(parsed.positions().getStartPosition(parsed.unit(), node))));
                return super.visitMethodInvocation(node, unused);
            }
            @Override public Void visitNewClass(NewClassTree node, Void unused) {
                calls.add(new CallRef("new " + node.getIdentifier(), parsed.unit().getLineMap().getLineNumber(parsed.positions().getStartPosition(parsed.unit(), node))));
                return super.visitNewClass(node, unused);
            }
        }.scan(tree, null);
        calls.sort(Comparator.comparingLong(CallRef::line).thenComparing(CallRef::expression));
        return new Match(tree, classPath, memberName, kind, signature, qualified, start, end, calls);
    }

    private static String classKind(ClassTree tree) {
        return switch (tree.getKind()) {
            case CLASS -> "CLASS";
            case INTERFACE -> "INTERFACE";
            case ENUM -> "ENUM";
            case RECORD -> "RECORD";
            case ANNOTATION_TYPE -> "ANNOTATION_TYPE";
            default -> throw new IllegalArgumentException("unsupported Java type declaration: " + tree.getKind());
        };
    }

    private static String classSignature(ClassTree tree, String classPath) {
        StringBuilder value = new StringBuilder();
        appendModifiers(value, tree.getModifiers().getFlags().stream().map(Object::toString).sorted().toList());
        value.append(classKind(tree).toLowerCase(Locale.ROOT).replace("annotation_type", "@interface")).append(' ').append(classPath);
        if (!tree.getTypeParameters().isEmpty()) value.append(tree.getTypeParameters());
        if (tree.getExtendsClause() != null) value.append(" extends ").append(tree.getExtendsClause());
        if (!tree.getImplementsClause().isEmpty()) value.append(" implements ").append(joinTrees(tree.getImplementsClause()));
        return value.toString().trim();
    }

    private static String methodSignature(MethodTree tree, String className) {
        StringBuilder value = new StringBuilder();
        appendModifiers(value, tree.getModifiers().getFlags().stream().map(Object::toString).sorted().toList());
        if (!tree.getTypeParameters().isEmpty()) value.append(joinTrees(tree.getTypeParameters())).append(' ');
        if (tree.getReturnType() != null) value.append(normalizeType(tree.getReturnType().toString())).append(' ');
        value.append(tree.getName().contentEquals("<init>") ? className : tree.getName()).append('(');
        for (int index = 0; index < tree.getParameters().size(); index++) {
            if (index > 0) value.append(", ");
            VariableTree parameter = tree.getParameters().get(index);
            value.append(normalizeType(parameter.getType().toString())).append(' ').append(parameter.getName());
        }
        value.append(')');
        if (!tree.getThrows().isEmpty()) value.append(" throws ").append(joinTrees(tree.getThrows()));
        return value.toString().trim();
    }

    private static void appendModifiers(StringBuilder value, List<String> flags) {
        if (!flags.isEmpty()) value.append(String.join(" ", flags)).append(' ');
    }

    private static String joinTrees(List<? extends Tree> trees) {
        return String.join(", ", trees.stream().map(Object::toString).toList());
    }

    private static String normalizeType(String value) { return value.replaceAll("\\s+", "").replace("...", "[]"); }
    private static String normalizeTree(Tree tree) { return tree.toString().replaceAll("\\s+", " ").trim(); }

    private static Map<String, String> parseArgs(String[] args) {
        LinkedHashMap<String, String> result = new LinkedHashMap<>();
        Set<String> known = Set.of("source", "symbol", "locator-id", "candidate-commit", "candidate-manifest", "candidate-manifest-sha256", "repo-root", "output", "resolved-at");
        for (int index = 0; index < args.length; index += 2) {
            if (!args[index].startsWith("--") || index + 1 >= args.length) throw new IllegalArgumentException("arguments must be --name value pairs");
            String key = args[index].substring(2);
            if (!known.contains(key)) throw new IllegalArgumentException("unknown argument --" + key);
            if (result.put(key, args[index + 1]) != null) throw new IllegalArgumentException("duplicate argument --" + key);
        }
        return result;
    }

    private static String required(Map<String, String> options, String key) {
        String value = options.get(key);
        if (value == null || value.isBlank()) throw new IllegalArgumentException("--" + key + " is required");
        return value;
    }

    private static void validateSafeRelative(String value, String label) {
        Path path = Paths.get(value);
        if (path.isAbsolute() || value.contains("\\") || path.normalize().startsWith("..") || !path.normalize().toString().replace('\\', '/').equals(value)) {
            throw new IllegalArgumentException(label + " path contains an unsafe component: " + value);
        }
    }

    private static Path safeExistingFile(Path root, String relative, String label) throws IOException {
        Path current = root;
        for (Path component : Paths.get(relative)) {
            current = current.resolve(component);
            if (Files.isSymbolicLink(current)) throw new IllegalArgumentException("repository path contains a symlink: " + relative);
        }
        Path normalized = current.toAbsolutePath().normalize();
        if (!normalized.startsWith(root) || !Files.isRegularFile(normalized, LinkOption.NOFOLLOW_LINKS)) {
            throw new IllegalArgumentException(label + " is not a regular repository file: " + relative);
        }
        return normalized;
    }

    private static byte[] gitShow(Path root, String commit, String sourcePath) throws IOException, InterruptedException {
        Process process = new ProcessBuilder("git", "show", commit + ":" + sourcePath).directory(root.toFile()).start();
        byte[] stdout = process.getInputStream().readAllBytes();
        byte[] stderr = process.getErrorStream().readAllBytes();
        int code = process.waitFor();
        if (code != 0) throw new IllegalArgumentException("read candidate blob failed: " + new String(stderr, StandardCharsets.UTF_8).trim());
        return stdout;
    }

    private static String sha256(byte[] value) {
        try {
            return java.util.HexFormat.of().formatHex(MessageDigest.getInstance("SHA-256").digest(value));
        } catch (Exception error) {
            throw new IllegalStateException(error);
        }
    }

    private static void writeImmutable(Path root, String relative, byte[] encoded) throws IOException {
        Path output = root.resolve(relative).normalize();
        if (!output.startsWith(root)) throw new IllegalArgumentException("output path escapes repository: " + relative);
        Path parent = output.getParent();
        Files.createDirectories(parent);
        Path cursor = root;
        for (Path component : root.relativize(parent)) {
            cursor = cursor.resolve(component);
            if (Files.isSymbolicLink(cursor)) throw new IllegalArgumentException("output path contains a symlink: " + relative);
        }
        if (Files.exists(output, LinkOption.NOFOLLOW_LINKS)) {
            if (!Files.isRegularFile(output, LinkOption.NOFOLLOW_LINKS) || Files.isSymbolicLink(output)) throw new IllegalArgumentException("output exists and is not a regular file: " + relative);
            if (Arrays.equals(Files.readAllBytes(output), encoded)) return;
            throw new IllegalArgumentException("immutable output already exists with different bytes: " + relative);
        }
        Files.write(output, encoded, StandardOpenOption.CREATE_NEW, StandardOpenOption.WRITE);
    }

    private static LinkedHashMap<String, Object> map(Object... entries) {
        LinkedHashMap<String, Object> result = new LinkedHashMap<>();
        for (int index = 0; index < entries.length; index += 2) result.put((String) entries[index], entries[index + 1]);
        return result;
    }

    private static LinkedHashMap<String, Object> positionMap(Position value) {
        return map("byte_offset", value.byteOffset(), "line", value.line(), "column", value.column());
    }

    private static List<Object> callMaps(List<CallRef> calls) {
        return calls.stream().map(item -> (Object) map("expression", item.expression(), "line", item.line())).toList();
    }

    private static String json(Object value, int indent) {
        if (value == null) return "null";
        if (value instanceof String text) return "\"" + escape(text) + "\"";
        if (value instanceof Number || value instanceof Boolean) return value.toString();
        String pad = " ".repeat(indent);
        String childPad = " ".repeat(indent + 2);
        if (value instanceof Map<?, ?> object) {
            if (object.isEmpty()) return "{}";
            ArrayList<String> fields = new ArrayList<>();
            for (Map.Entry<?, ?> entry : object.entrySet()) fields.add(childPad + json(entry.getKey().toString(), 0) + ": " + json(entry.getValue(), indent + 2));
            return "{\n" + String.join(",\n", fields) + "\n" + pad + "}";
        }
        if (value instanceof List<?> array) {
            if (array.isEmpty()) return "[]";
            return "[\n" + String.join(",\n", array.stream().map(item -> childPad + json(item, indent + 2)).toList()) + "\n" + pad + "]";
        }
        throw new IllegalArgumentException("unsupported JSON value " + value.getClass());
    }

    private static String escape(String value) {
        StringBuilder result = new StringBuilder();
        for (int index = 0; index < value.length(); index++) {
            char c = value.charAt(index);
            switch (c) {
                case '"' -> result.append("\\\"");
                case '\\' -> result.append("\\\\");
                case '\b' -> result.append("\\b");
                case '\f' -> result.append("\\f");
                case '\n' -> result.append("\\n");
                case '\r' -> result.append("\\r");
                case '\t' -> result.append("\\t");
                default -> {
                    if (c < 0x20) result.append(String.format("\\u%04x", (int) c)); else result.append(c);
                }
            }
        }
        return result.toString();
    }
}
