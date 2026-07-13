#!/usr/bin/env bash
set -Eeuo pipefail

PROJECT_DIR="${PROJECT_DIR:-/home/wangwt/phase_2/code/traffic-analysis-platform}"
BRANCH="${BRANCH:-main}"
REMOTE="${REMOTE:-origin}"
REMOTE_BRANCH="${REMOTE_BRANCH:-main}"
STABLE_SECONDS="${STABLE_SECONDS:-300}"
AUTO_PUSH="${AUTO_PUSH:-1}"
LOG_FILE="${LOG_FILE:-$PROJECT_DIR/logs/git-auto-commit.log}"
STATE_DIR="${STATE_DIR:-$PROJECT_DIR/.git/auto-commit}"
LOCK_FILE="${LOCK_FILE:-/run/traffic-analysis-auto-commit.lock}"
DRY_RUN=0
FORCE=0

for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=1 ;;
    --force) FORCE=1 ;;
    --run) ;;
    *) echo "unknown argument: $arg" >&2; exit 2 ;;
  esac
done

GLOBAL_EXCLUDES=(
  ':(exclude,glob).git/**'
  ':(exclude,glob)**/node_modules/**'
  ':(exclude,glob)**/target/**'
  ':(exclude,glob)**/dist/**'
  ':(exclude,glob)**/build/**'
  ':(exclude,glob)tmp/**'
  ':(exclude,glob)evidence/**'
  ':(exclude,glob)web/ui/evidence/**'
  ':(exclude,glob)web/ui/dist-assets-v2/**'
  ':(exclude,glob)doc/02_acceptance/runs/**'
  ':(exclude,glob)common/config/**'
  ':(exclude,glob)rust/**/config.yaml'
  ':(exclude,glob)**/.env'
  ':(exclude,glob)**/.env.*'
  ':(exclude,glob,icase)**/*secret*'
  ':(exclude,glob,icase)**/*credential*'
  ':(exclude,glob,icase)**/*password*'
  ':(exclude,glob,icase)**/*.pem'
  ':(exclude,glob,icase)**/*.key'
  ':(exclude,glob,icase)**/*.p12'
  ':(exclude,glob,icase)**/*.jks'
  ':(exclude,glob,icase)**/*.keystore'
  ':(exclude,glob,icase)**/*.truststore'
)

log() {
  local ts
  ts="$(date '+%Y-%m-%d %H:%M:%S%z')"
  printf '[%s] %s\n' "$ts" "$*" | tee -a "$LOG_FILE"
}

run_git() {
  GIT_SSH_COMMAND='ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new' git "$@"
}

fail_if_repo_not_ready() {
  cd "$PROJECT_DIR"
  mkdir -p "$(dirname "$LOG_FILE")" "$STATE_DIR"

  git rev-parse --is-inside-work-tree >/dev/null
  local current_branch
  current_branch="$(git branch --show-current)"
  if [[ "$current_branch" != "$BRANCH" ]]; then
    log "skip: current branch is $current_branch, expected $BRANCH"
    exit 0
  fi

  for marker in MERGE_HEAD rebase-merge rebase-apply CHERRY_PICK_HEAD REVERT_HEAD; do
    if [[ -e ".git/$marker" ]]; then
      log "skip: git operation in progress: $marker"
      exit 0
    fi
  done

  if ! git diff --quiet || ! git diff --cached --quiet; then
    log "skip: tracked working tree or index is dirty before automation"
    git status -sb | tee -a "$LOG_FILE"
    exit 0
  fi
}

sync_remote_before_commit() {
  run_git fetch "$REMOTE" "$REMOTE_BRANCH"
  local ahead behind
  read -r ahead behind < <(git rev-list --left-right --count "HEAD...$REMOTE/$REMOTE_BRANCH")
  if [[ "$ahead" -gt 0 && "$behind" -gt 0 ]]; then
    log "skip: local and remote branches diverged; manual merge required"
    exit 0
  fi
  if [[ "$behind" -gt 0 ]]; then
    log "remote is ahead by $behind commits; pulling with fast-forward only"
    run_git pull --ff-only "$REMOTE" "$REMOTE_BRANCH"
  fi
}

candidate_fingerprint() {
  git status --porcelain=v1 -z -- . "${GLOBAL_EXCLUDES[@]}" | sha256sum | awk '{print $1}'
}

candidate_count() {
  git status --porcelain=v1 -z -- . "${GLOBAL_EXCLUDES[@]}" | tr '\0' '\n' | sed '/^$/d' | wc -l
}

wait_for_stability() {
  local count hash hash_file ts_file now first_seen
  count="$(candidate_count | tr -d ' ')"
  if [[ "$count" == "0" ]]; then
    log "no committable changes"
    rm -f "$STATE_DIR/status.sha256" "$STATE_DIR/status.first_seen" 2>/dev/null || true
    exit 0
  fi

  hash="$(candidate_fingerprint)"
  hash_file="$STATE_DIR/status.sha256"
  ts_file="$STATE_DIR/status.first_seen"
  now="$(date +%s)"

  if [[ "$DRY_RUN" == "1" ]]; then
    log "dry-run: $count committable status entries detected; hash=$hash"
    git status -sb -- . "${GLOBAL_EXCLUDES[@]}" | sed -n '1,120p' | tee -a "$LOG_FILE"
    exit 0
  fi

  if [[ "$FORCE" == "1" ]]; then
    log "force mode: bypassing stability window for $count status entries"
    return
  fi

  if [[ ! -f "$hash_file" || ! -f "$ts_file" || "$(cat "$hash_file")" != "$hash" ]]; then
    printf '%s\n' "$hash" > "$hash_file"
    printf '%s\n' "$now" > "$ts_file"
    log "changes detected; waiting for ${STABLE_SECONDS}s stable window"
    exit 0
  fi

  first_seen="$(cat "$ts_file")"
  if (( now - first_seen < STABLE_SECONDS )); then
    log "changes not stable yet: $((now - first_seen))/${STABLE_SECONDS}s"
    exit 0
  fi

  log "changes stable for $((now - first_seen))s; starting batch commits"
}

ensure_safe_index() {
  local bad_large bad_sensitive
  bad_large=""
  while IFS= read -r -d '' file; do
    if [[ -f "$file" ]]; then
      local size
      size="$(stat -c '%s' -- "$file")"
      if (( size > 100000000 )); then
        bad_large+="$file $size"$'\n'
      fi
    fi
  done < <(git diff --cached --name-only -z)

  if [[ -n "$bad_large" ]]; then
    log "blocked: staged files exceed GitHub 100MB limit"
    printf '%s' "$bad_large" | tee -a "$LOG_FILE"
    git reset -q
    exit 1
  fi

  bad_sensitive="$(git diff --cached --name-only | grep -Ei '(^|/)(\.env($|\.)|.*(secret|credential|password).*)|\.(pem|key|p12|jks|keystore|truststore)$' || true)"
  if [[ -n "$bad_sensitive" ]]; then
    log "blocked: sensitive-looking files staged"
    printf '%s\n' "$bad_sensitive" | tee -a "$LOG_FILE"
    git reset -q
    exit 1
  fi

  if ! git diff --cached --check >>"$LOG_FILE" 2>&1; then
    log "blocked: staged diff failed whitespace/check validation"
    git reset -q
    exit 1
  fi
}

commit_batch() {
  local message="$1"
  shift
  local paths=("$@")
  local pathspec_file="$STATE_DIR/pathspec.$$"

  git reset -q
  git ls-files -z -m -d -o --exclude-standard -- "${paths[@]}" "${GLOBAL_EXCLUDES[@]}" > "$pathspec_file"
  if [[ ! -s "$pathspec_file" ]]; then
    rm -f "$pathspec_file"
    return
  fi
  git add -A --pathspec-from-file="$pathspec_file" --pathspec-file-nul
  rm -f "$pathspec_file"

  if git diff --cached --quiet; then
    git reset -q
    return
  fi

  ensure_safe_index
  log "committing: $message"
  git commit -m "$message" | tee -a "$LOG_FILE"
  git reset -q
}

commit_batches() {
  commit_batch "Update shared project scaffolding" \
    .gitignore README.md Makefile agent.md .github common deployments proto rules scripts mlops

  commit_batch "Update Go control plane changes" go
  commit_batch "Update Flink job changes" java
  commit_batch "Update Rust probe agent changes" rust
  commit_batch "Update web UI changes" web
  commit_batch "Update validation and test changes" tests
  commit_batch "Update project documentation" doc
}

push_if_needed() {
  run_git fetch "$REMOTE" "$REMOTE_BRANCH"
  local ahead behind
  read -r ahead behind < <(git rev-list --left-right --count "HEAD...$REMOTE/$REMOTE_BRANCH")

  if [[ "$behind" -gt 0 ]]; then
    log "skip push: remote advanced during automation; manual fast-forward/rebase required"
    exit 0
  fi

  if [[ "$ahead" -eq 0 ]]; then
    log "no local commits to push"
    return
  fi

  if [[ "$AUTO_PUSH" == "1" ]]; then
    log "pushing $ahead commits to $REMOTE/$REMOTE_BRANCH"
    run_git push "$REMOTE" "$BRANCH:$REMOTE_BRANCH" | tee -a "$LOG_FILE"
  else
    log "AUTO_PUSH disabled; $ahead local commits left unpushed"
  fi
}

main() {
  exec 9>"$LOCK_FILE"
  if ! flock -n 9; then
    echo "another auto-commit run is active"
    exit 0
  fi

  fail_if_repo_not_ready
  sync_remote_before_commit
  wait_for_stability
  commit_batches
  push_if_needed
  rm -f "$STATE_DIR/status.sha256" "$STATE_DIR/status.first_seen" 2>/dev/null || true
  log "done"
}

main
