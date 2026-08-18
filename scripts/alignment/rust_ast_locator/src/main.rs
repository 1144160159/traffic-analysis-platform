//! Resolve one Rust code unit against an exact frozen candidate blob.
//!
//! The emitted receipt proves only an AST locator. It never grants function
//! review, implementation, execution, merge, deployment, or acceptance.

use proc_macro2::{LineColumn, Span};
use quote::{quote, ToTokens};
use serde::Serialize;
use serde_json::Value;
use sha2::{Digest, Sha256};
use std::collections::BTreeMap;
use std::env;
use std::ffi::OsStr;
use std::fs::{self, OpenOptions};
use std::io::Write;
use std::path::{Component, Path, PathBuf};
use std::process::{Command, ExitCode};
use std::time::{SystemTime, UNIX_EPOCH};
use syn::spanned::Spanned;
use syn::visit::{self, Visit};
use syn::{ImplItem, Item, ItemImpl, Signature, Type};

const RESOLVER_SOURCE: &str = "scripts/alignment/rust_ast_locator/src/main.rs";
const PROOF_CEILING: &str = "EXACT_LOCATOR_ONLY_NOT_FUNCTION_DESIGN_OR_EXECUTION_AUTHORIZATION";

#[derive(Debug)]
struct Args {
    source: String,
    query: String,
    locator_id: String,
    candidate_commit: String,
    candidate_manifest: String,
    candidate_manifest_sha256: String,
    repo_root: PathBuf,
    output: Option<String>,
    resolved_at: Option<String>,
}

#[derive(Serialize)]
struct HashCandidate {
    commit: String,
    manifest_path: String,
    manifest_sha256: String,
}

#[derive(Serialize)]
struct ResolverRef {
    resolver_id: &'static str,
    engine: &'static str,
    engine_version: String,
    source_path: &'static str,
    source_sha256: String,
}

#[derive(Serialize)]
struct Position {
    byte_offset: usize,
    line: usize,
    column: usize,
}

#[derive(Clone, Serialize)]
struct CallRef {
    expression: String,
    line: usize,
}

#[derive(Serialize)]
struct Locator {
    locator_id: String,
    language: &'static str,
    path: String,
    module_path: String,
    qualified_symbol: String,
    declaration_kind: String,
    query: String,
    match_strategy: &'static str,
    signature: String,
    candidate_blob_sha256: String,
    source_span_sha256: String,
    normalized_ast_sha256: String,
    start: Position,
    end: Position,
    calls: Vec<CallRef>,
}

#[derive(Serialize)]
struct Receipt {
    schema_version: &'static str,
    artifact_kind: &'static str,
    status: &'static str,
    proof_level: &'static str,
    candidate: HashCandidate,
    resolver: ResolverRef,
    locator: Locator,
    ambiguity_count: usize,
    resolved_at: String,
    proof_ceiling: &'static str,
}

#[derive(Clone)]
struct Match {
    identity: String,
    module_path: String,
    declaration_kind: String,
    signature: String,
    query_signature: Option<String>,
    normalized_ast: String,
    span: Span,
    calls: Vec<CallRef>,
}

fn fail(message: impl Into<String>) -> Result<(), String> {
    Err(message.into())
}

fn parse_args() -> Result<Args, String> {
    let mut values = env::args().skip(1);
    let mut parsed: BTreeMap<String, String> = BTreeMap::new();
    while let Some(flag) = values.next() {
        if !flag.starts_with("--") {
            return Err(format!("unexpected positional argument: {flag}"));
        }
        let value = values
            .next()
            .ok_or_else(|| format!("missing value for {flag}"))?;
        if parsed.insert(flag.clone(), value).is_some() {
            return Err(format!("duplicate argument: {flag}"));
        }
    }
    let required = |name: &str| {
        parsed
            .get(name)
            .cloned()
            .ok_or_else(|| format!("missing required argument: {name}"))
    };
    let args = Args {
        source: required("--source")?,
        query: required("--symbol")?,
        locator_id: required("--locator-id")?,
        candidate_commit: required("--candidate-commit")?,
        candidate_manifest: required("--candidate-manifest")?,
        candidate_manifest_sha256: required("--candidate-manifest-sha256")?,
        repo_root: PathBuf::from(
            parsed
                .get("--repo-root")
                .cloned()
                .unwrap_or_else(|| ".".to_string()),
        ),
        output: parsed.get("--output").cloned(),
        resolved_at: parsed.get("--resolved-at").cloned(),
    };
    let known = [
        "--source",
        "--symbol",
        "--locator-id",
        "--candidate-commit",
        "--candidate-manifest",
        "--candidate-manifest-sha256",
        "--repo-root",
        "--output",
        "--resolved-at",
    ];
    if let Some(unknown) = parsed.keys().find(|item| !known.contains(&item.as_str())) {
        return Err(format!("unknown argument: {unknown}"));
    }
    if !args.locator_id.starts_with("LOC-") {
        return Err("locator ID must start with LOC-".to_string());
    }
    if args.candidate_commit.len() != 40
        || !args
            .candidate_commit
            .bytes()
            .all(|item| item.is_ascii_hexdigit())
    {
        return Err("candidate commit must be exactly 40 hexadecimal characters".to_string());
    }
    if args.candidate_manifest_sha256.len() != 64
        || !args
            .candidate_manifest_sha256
            .bytes()
            .all(|item| item.is_ascii_hexdigit())
    {
        return Err(
            "candidate manifest SHA-256 must be exactly 64 hexadecimal characters".to_string(),
        );
    }
    Ok(args)
}

fn digest(bytes: &[u8]) -> String {
    format!("{:x}", Sha256::digest(bytes))
}

fn validate_relative_path(relative: &str, suffix: Option<&str>) -> Result<(), String> {
    let path = Path::new(relative);
    if path.is_absolute() || relative.is_empty() {
        return Err(format!(
            "path must be non-empty and repository-relative: {relative:?}"
        ));
    }
    if path
        .components()
        .any(|component| !matches!(component, Component::Normal(_)))
    {
        return Err(format!("path contains an unsafe component: {relative:?}"));
    }
    if let Some(required_suffix) = suffix {
        if path.extension() != Some(OsStr::new(required_suffix)) {
            return Err(format!("path must end in .{required_suffix}: {relative:?}"));
        }
    }
    Ok(())
}

fn reject_symlink_components(root: &Path, relative: &str) -> Result<PathBuf, String> {
    validate_relative_path(relative, None)?;
    let mut current = root.to_path_buf();
    for component in Path::new(relative).components() {
        let Component::Normal(name) = component else {
            return Err(format!("path contains an unsafe component: {relative:?}"));
        };
        current.push(name);
        let metadata = fs::symlink_metadata(&current)
            .map_err(|error| format!("inspect repository path {relative:?}: {error}"))?;
        if metadata.file_type().is_symlink() {
            return Err(format!("repository path contains a symlink: {relative:?}"));
        }
    }
    Ok(current)
}

fn safe_regular_file(root: &Path, relative: &str, suffix: Option<&str>) -> Result<PathBuf, String> {
    validate_relative_path(relative, suffix)?;
    let path = reject_symlink_components(root, relative)?;
    let metadata = fs::metadata(&path)
        .map_err(|error| format!("inspect repository file {relative:?}: {error}"))?;
    if !metadata.is_file() {
        return Err(format!(
            "repository path is not a regular file: {relative:?}"
        ));
    }
    let canonical_root = root
        .canonicalize()
        .map_err(|error| format!("canonicalize repository root: {error}"))?;
    let canonical_path = path
        .canonicalize()
        .map_err(|error| format!("canonicalize repository file {relative:?}: {error}"))?;
    if !canonical_path.starts_with(&canonical_root) {
        return Err(format!("repository file escapes root: {relative:?}"));
    }
    Ok(path)
}

fn git_candidate_blob(root: &Path, commit: &str, source: &str) -> Result<Vec<u8>, String> {
    let output = Command::new("git")
        .args(["show", &format!("{commit}:{source}")])
        .current_dir(root)
        .output()
        .map_err(|error| format!("run git show: {error}"))?;
    if !output.status.success() {
        return Err(format!(
            "read candidate blob failed: {}",
            String::from_utf8_lossy(&output.stderr).trim()
        ));
    }
    Ok(output.stdout)
}

fn token_string(value: &impl ToTokens) -> String {
    value.to_token_stream().to_string()
}

fn compact(value: &str) -> String {
    let mut result: String = value.chars().filter(|item| !item.is_whitespace()).collect();
    while result.contains(",)") {
        result = result.replace(",)", ")");
    }
    result
}

fn type_name(value: &Type) -> String {
    token_string(value).trim().to_string()
}

fn signature_text(signature: &Signature) -> String {
    token_string(signature)
}

fn query_signature(identity: &str, signature: &Signature) -> String {
    let inputs = &signature.inputs;
    let output = &signature.output;
    let generics = &signature.generics;
    let qualifiers = quote!(#generics (#inputs) #output).to_string();
    format!("{identity}{qualifiers}")
}

struct CallCollector {
    calls: Vec<CallRef>,
}

impl<'ast> Visit<'ast> for CallCollector {
    fn visit_expr_call(&mut self, node: &'ast syn::ExprCall) {
        self.calls.push(CallRef {
            expression: token_string(&node.func),
            line: node.span().start().line,
        });
        visit::visit_expr_call(self, node);
    }

    fn visit_expr_method_call(&mut self, node: &'ast syn::ExprMethodCall) {
        self.calls.push(CallRef {
            expression: format!(".{}", node.method),
            line: node.span().start().line,
        });
        visit::visit_expr_method_call(self, node);
    }
}

fn calls_for(value: &impl ToTokens, visit: impl FnOnce(&mut CallCollector)) -> Vec<CallRef> {
    let _ = value;
    let mut collector = CallCollector { calls: Vec::new() };
    visit(&mut collector);
    collector
        .calls
        .sort_by(|left, right| (left.line, &left.expression).cmp(&(right.line, &right.expression)));
    collector.calls
}

fn module_join(module: &[String]) -> String {
    if module.is_empty() {
        "crate".to_string()
    } else {
        format!("crate::{}", module.join("::"))
    }
}

fn qualified(module: &[String], identity: &str) -> String {
    if module.is_empty() {
        format!("crate::{identity}")
    } else {
        format!("crate::{}::{identity}", module.join("::"))
    }
}

fn qualified_match(value: &Match) -> String {
    if value.module_path == "crate" {
        return format!("crate::{}", value.identity);
    }
    let module_tail = value.module_path.strip_prefix("crate::").unwrap_or("");
    if !module_tail.is_empty() && value.identity.starts_with(&format!("{module_tail}::")) {
        format!("crate::{}", value.identity)
    } else {
        format!("{}::{}", value.module_path, value.identity)
    }
}

fn push_function(result: &mut Vec<Match>, item: &syn::ItemFn, module: &[String]) {
    let local = item.sig.ident.to_string();
    let identity = if module.is_empty() {
        local.clone()
    } else {
        format!("{}::{local}", module.join("::"))
    };
    let declaration_kind = if item.sig.asyncness.is_some() {
        "ASYNC_FUNCTION"
    } else {
        "FUNCTION"
    };
    let calls = calls_for(item, |collector| collector.visit_block(&item.block));
    result.push(Match {
        identity: identity.clone(),
        module_path: module_join(module),
        declaration_kind: declaration_kind.to_string(),
        signature: signature_text(&item.sig),
        query_signature: Some(query_signature(&identity, &item.sig)),
        normalized_ast: token_string(item),
        span: item.span(),
        calls,
    });
}

fn impl_identity(item: &ItemImpl) -> (String, String) {
    let self_type = type_name(&item.self_ty);
    if let Some((_, path, _)) = &item.trait_ {
        (
            format!("impl {} for {self_type}", token_string(path)),
            "TRAIT_IMPL".to_string(),
        )
    } else {
        (format!("impl {self_type}"), "INHERENT_IMPL".to_string())
    }
}

fn push_impl(result: &mut Vec<Match>, item: &ItemImpl, module: &[String]) {
    let (block_identity, block_kind) = impl_identity(item);
    let block_calls = calls_for(item, |collector| collector.visit_item_impl(item));
    result.push(Match {
        identity: block_identity.clone(),
        module_path: module_join(module),
        declaration_kind: block_kind,
        signature: block_identity.clone(),
        query_signature: None,
        normalized_ast: token_string(item),
        span: item.span(),
        calls: block_calls,
    });
    let self_type = type_name(&item.self_ty);
    let trait_name = item.trait_.as_ref().map(|(_, path, _)| token_string(path));
    for member in &item.items {
        let ImplItem::Fn(method) = member else {
            continue;
        };
        let simple_identity = format!("{self_type}::{}", method.sig.ident);
        let identity = trait_name
            .as_ref()
            .map(|trait_path| format!("impl {trait_path} for {simple_identity}"))
            .unwrap_or_else(|| simple_identity.clone());
        let calls = calls_for(method, |collector| collector.visit_block(&method.block));
        result.push(Match {
            identity: identity.clone(),
            module_path: module_join(module),
            declaration_kind: if trait_name.is_some() {
                "TRAIT_METHOD".to_string()
            } else if method.sig.asyncness.is_some() {
                "ASYNC_INHERENT_METHOD".to_string()
            } else {
                "INHERENT_METHOD".to_string()
            },
            signature: signature_text(&method.sig),
            query_signature: Some(query_signature(&identity, &method.sig)),
            normalized_ast: token_string(method),
            span: method.span(),
            calls,
        });
    }
}

fn push_named_item(
    result: &mut Vec<Match>,
    identity: String,
    module: &[String],
    kind: &str,
    span: Span,
    normalized_ast: String,
) {
    result.push(Match {
        identity,
        module_path: module_join(module),
        declaration_kind: kind.to_string(),
        signature: normalized_ast.clone(),
        query_signature: None,
        normalized_ast,
        span,
        calls: Vec::new(),
    });
}

fn collect_items(items: &[Item], module: &mut Vec<String>, result: &mut Vec<Match>) {
    for item in items {
        match item {
            Item::Fn(value) => push_function(result, value, module),
            Item::Impl(value) => push_impl(result, value, module),
            Item::Struct(value) => push_named_item(
                result,
                qualified(module, &value.ident.to_string())
                    .trim_start_matches("crate::")
                    .to_string(),
                module,
                "STRUCT",
                value.span(),
                token_string(value),
            ),
            Item::Enum(value) => push_named_item(
                result,
                qualified(module, &value.ident.to_string())
                    .trim_start_matches("crate::")
                    .to_string(),
                module,
                "ENUM",
                value.span(),
                token_string(value),
            ),
            Item::Type(value) => push_named_item(
                result,
                qualified(module, &value.ident.to_string())
                    .trim_start_matches("crate::")
                    .to_string(),
                module,
                "TYPE_ALIAS",
                value.span(),
                token_string(value),
            ),
            Item::Trait(value) => push_named_item(
                result,
                qualified(module, &value.ident.to_string())
                    .trim_start_matches("crate::")
                    .to_string(),
                module,
                "TRAIT",
                value.span(),
                token_string(value),
            ),
            Item::Mod(value) => {
                if let Some((_, children)) = &value.content {
                    module.push(value.ident.to_string());
                    collect_items(children, module, result);
                    module.pop();
                }
            }
            _ => {}
        }
    }
}

fn identity_matches(query: &str, candidate: &Match) -> bool {
    let normalized_query = compact(query);
    let normalized_identity = compact(&candidate.identity);
    if normalized_query == normalized_identity {
        return true;
    }
    if candidate.identity.contains("::") {
        let tail = candidate
            .identity
            .rsplit_once("::")
            .map(|(_, value)| value)
            .unwrap_or(&candidate.identity);
        if normalized_query == compact(tail) && !candidate.identity.starts_with("impl ") {
            return true;
        }
    }
    candidate
        .query_signature
        .as_ref()
        .is_some_and(|signature| normalized_query == compact(signature))
}

fn byte_offset(source: &[u8], position: LineColumn) -> Result<usize, String> {
    if position.line == 0 {
        return Err("AST span line is zero".to_string());
    }
    let mut line = 1usize;
    let mut offset = 0usize;
    while line < position.line {
        let Some(next) = source[offset..].iter().position(|item| *item == b'\n') else {
            return Err("AST span line exceeds source".to_string());
        };
        offset += next + 1;
        line += 1;
    }
    let result = offset + position.column;
    if result > source.len() {
        return Err("AST span column exceeds source".to_string());
    }
    Ok(result)
}

fn position(source: &[u8], value: LineColumn) -> Result<Position, String> {
    Ok(Position {
        byte_offset: byte_offset(source, value)?,
        line: value.line,
        column: value.column + 1,
    })
}

fn resolved_at(value: Option<&str>) -> Result<String, String> {
    let selected = if let Some(explicit) = value {
        explicit.to_string()
    } else if let Ok(epoch) = env::var("SOURCE_DATE_EPOCH") {
        let seconds: u64 = epoch
            .parse()
            .map_err(|_| "SOURCE_DATE_EPOCH must be an unsigned integer".to_string())?;
        // The resolver does not depend on a time crate. Keep deterministic
        // test/build epochs restricted to the Unix epoch until a reviewed
        // RFC3339 implementation is added.
        if seconds != 0 {
            return Err("non-zero SOURCE_DATE_EPOCH requires --resolved-at".to_string());
        }
        "1970-01-01T00:00:00Z".to_string()
    } else {
        let seconds = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .map_err(|error| format!("system clock precedes Unix epoch: {error}"))?
            .as_secs();
        return Err(format!(
            "current time ({seconds}) cannot be rendered without ambiguity; pass --resolved-at RFC3339 UTC"
        ));
    };
    let bytes = selected.as_bytes();
    let structurally_valid = bytes.len() >= 20
        && selected.ends_with('Z')
        && bytes.get(4) == Some(&b'-')
        && bytes.get(7) == Some(&b'-')
        && bytes.get(10) == Some(&b'T')
        && bytes.get(13) == Some(&b':')
        && bytes.get(16) == Some(&b':');
    if !structurally_valid {
        return Err("resolved-at must be RFC3339 UTC ending in Z".to_string());
    }
    Ok(selected)
}

fn output_path(root: &Path, relative: &str) -> Result<PathBuf, String> {
    validate_relative_path(relative, Some("json"))?;
    let parent = Path::new(relative)
        .parent()
        .ok_or_else(|| "output has no parent directory".to_string())?;
    let mut current = root.to_path_buf();
    for component in parent.components() {
        let Component::Normal(name) = component else {
            return Err("output path contains an unsafe component".to_string());
        };
        current.push(name);
        if current.exists() {
            let metadata = fs::symlink_metadata(&current)
                .map_err(|error| format!("inspect output parent: {error}"))?;
            if metadata.file_type().is_symlink() || !metadata.is_dir() {
                return Err("output parent contains a symlink or non-directory".to_string());
            }
        } else {
            fs::create_dir(&current)
                .map_err(|error| format!("create output directory: {error}"))?;
        }
    }
    Ok(root.join(relative))
}

fn run() -> Result<(), String> {
    let args = parse_args()?;
    validate_relative_path(&args.source, Some("rs"))?;
    if !args.source.starts_with("rust/") {
        return fail("Rust source must remain under rust/");
    }
    let root = args
        .repo_root
        .canonicalize()
        .map_err(|error| format!("canonicalize repository root: {error}"))?;
    let source_path = safe_regular_file(&root, &args.source, Some("rs"))?;
    let manifest_path = safe_regular_file(&root, &args.candidate_manifest, Some("json"))?;
    let resolver_path = safe_regular_file(&root, RESOLVER_SOURCE, Some("rs"))?;

    let manifest_bytes =
        fs::read(&manifest_path).map_err(|error| format!("read candidate manifest: {error}"))?;
    if digest(&manifest_bytes) != args.candidate_manifest_sha256 {
        return fail("candidate manifest SHA-256 mismatch");
    }
    let manifest: Value = serde_json::from_slice(&manifest_bytes)
        .map_err(|error| format!("decode candidate manifest: {error}"))?;
    if manifest
        .get("implementation_candidate_commit")
        .and_then(Value::as_str)
        != Some(&args.candidate_commit)
    {
        return fail("candidate manifest commit mismatch");
    }
    let declared_blob = manifest
        .get("source_blob_sha256")
        .and_then(Value::as_object)
        .and_then(|values| values.get(&args.source))
        .and_then(Value::as_str)
        .ok_or_else(|| "candidate manifest does not bind source path".to_string())?;
    let current = fs::read(&source_path).map_err(|error| format!("read source: {error}"))?;
    let candidate = git_candidate_blob(&root, &args.candidate_commit, &args.source)?;
    if current != candidate {
        return fail("worktree source differs from frozen candidate");
    }
    if digest(&candidate) != declared_blob {
        return fail("candidate source SHA-256 differs from manifest");
    }
    let source_text = std::str::from_utf8(&candidate)
        .map_err(|error| format!("Rust source is not UTF-8: {error}"))?;
    let syntax =
        syn::parse_file(source_text).map_err(|error| format!("parse Rust AST: {error}"))?;
    let mut candidates = Vec::new();
    collect_items(&syntax.items, &mut Vec::new(), &mut candidates);
    let candidate_debug: Vec<String> = candidates
        .iter()
        .map(|item| {
            item.query_signature
                .clone()
                .unwrap_or_else(|| item.identity.clone())
        })
        .collect();
    let matches: Vec<_> = candidates
        .into_iter()
        .filter(|item| identity_matches(&args.query, item))
        .collect();
    if matches.len() != 1 {
        return fail(format!(
            "expected exactly one Rust AST match for {:?}, got {}; candidates={:?}",
            args.query,
            matches.len(),
            candidate_debug,
        ));
    }
    let selected = &matches[0];
    let start = byte_offset(&candidate, selected.span.start())?;
    let end = byte_offset(&candidate, selected.span.end())?;
    if start >= end || end > candidate.len() {
        return fail("Rust AST span is empty or outside the candidate blob");
    }
    let rustc = Command::new("rustc")
        .arg("--version")
        .output()
        .map_err(|error| format!("read rustc version: {error}"))?;
    if !rustc.status.success() {
        return fail("rustc --version failed");
    }
    let receipt = Receipt {
        schema_version: "1.0.0",
        artifact_kind: "RUST_LOCATOR_RESOLUTION_RECEIPT",
        status: "RESOLVED",
        proof_level: "LANGUAGE_AST",
        candidate: HashCandidate {
            commit: args.candidate_commit.clone(),
            manifest_path: args.candidate_manifest.clone(),
            manifest_sha256: args.candidate_manifest_sha256.clone(),
        },
        resolver: ResolverRef {
            resolver_id: "traffic-rust-syn-locator@1",
            engine: "syn",
            engine_version: format!(
                "syn-2.0.119; {}",
                String::from_utf8_lossy(&rustc.stdout).trim()
            ),
            source_path: RESOLVER_SOURCE,
            source_sha256: digest(
                &fs::read(resolver_path)
                    .map_err(|error| format!("read resolver source: {error}"))?,
            ),
        },
        locator: Locator {
            locator_id: args.locator_id,
            language: "rust",
            path: args.source,
            module_path: selected.module_path.clone(),
            qualified_symbol: qualified_match(selected),
            declaration_kind: selected.declaration_kind.clone(),
            query: args.query,
            match_strategy: "EXACT_SYN_ITEM_IMPL_OR_SIGNATURE",
            signature: selected.signature.clone(),
            candidate_blob_sha256: digest(&candidate),
            source_span_sha256: digest(&candidate[start..end]),
            normalized_ast_sha256: digest(selected.normalized_ast.as_bytes()),
            start: position(&candidate, selected.span.start())?,
            end: position(&candidate, selected.span.end())?,
            calls: selected.calls.clone(),
        },
        ambiguity_count: 1,
        resolved_at: resolved_at(args.resolved_at.as_deref())?,
        proof_ceiling: PROOF_CEILING,
    };
    let mut encoded =
        serde_json::to_vec_pretty(&receipt).map_err(|error| format!("encode receipt: {error}"))?;
    encoded.push(b'\n');
    if let Some(relative) = args.output {
        let output = output_path(&root, &relative)?;
        match fs::symlink_metadata(&output) {
            Ok(metadata) => {
                if metadata.file_type().is_symlink() || !metadata.is_file() {
                    return fail("immutable output exists and is not a regular file");
                }
                let existing =
                    fs::read(&output).map_err(|error| format!("read immutable output: {error}"))?;
                if existing != encoded {
                    return fail("immutable output already exists with different bytes");
                }
            }
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => {
                let mut file = OpenOptions::new()
                    .write(true)
                    .create_new(true)
                    .open(&output)
                    .map_err(|error| format!("create immutable output: {error}"))?;
                file.write_all(&encoded)
                    .map_err(|error| format!("write immutable output: {error}"))?;
                file.sync_all()
                    .map_err(|error| format!("sync immutable output: {error}"))?;
            }
            Err(error) => return fail(format!("inspect immutable output: {error}")),
        }
    } else {
        std::io::stdout()
            .write_all(&encoded)
            .map_err(|error| format!("write stdout: {error}"))?;
    }
    Ok(())
}

fn main() -> ExitCode {
    match run() {
        Ok(()) => ExitCode::SUCCESS,
        Err(error) => {
            eprintln!("{error}");
            ExitCode::FAILURE
        }
    }
}
