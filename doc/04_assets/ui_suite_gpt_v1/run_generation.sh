#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "${ROOT_DIR}/../../.." && pwd)"
CLI="/root/.codex/skills/.system/imagegen/scripts/image_gen.py"
MANIFEST="${ROOT_DIR}/manifest.json"
REFERENCE_IMAGE="${REFERENCE_IMAGE:-${PROJECT_DIR}/doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-generation-reference.png}"
MODEL="${MODEL:-gpt-image-2}"
QUALITY="${QUALITY:-medium}"
TARGET_SIZE="${SIZE:-1920x1080}"
REQUEST_SIZE="$TARGET_SIZE"
ONLY_ID="${ONLY_ID:-}"
START_AT="${START_AT:-}"
LIMIT="${LIMIT:-0}"
DRY_RUN="${DRY_RUN:-0}"
LOG_FILE="${ROOT_DIR}/generation.log"

if [[ "$MODEL" == "gpt-image-2" && "$TARGET_SIZE" == "1920x1080" ]]; then
  REQUEST_SIZE="1920x1088"
fi

load_env_file() {
  local file="$1"
  if [[ -f "$file" ]]; then
    set -a
    # shellcheck disable=SC1090
    source "$file"
    set +a
  fi
}

load_env_file "${PROJECT_DIR}/.env"
load_env_file "${PROJECT_DIR}/.env.local"
load_env_file "${PROJECT_DIR}/web/ui/.env"
load_env_file "${PROJECT_DIR}/web/ui/.env.local"

if [[ ! -f "$CLI" ]]; then
  echo "imagegen CLI not found: $CLI" >&2
  exit 2
fi

if [[ ! -f "$MANIFEST" ]]; then
  echo "manifest not found, run build_prompt_manifest.mjs first: $MANIFEST" >&2
  exit 2
fi

if [[ ! -f "$REFERENCE_IMAGE" ]]; then
  echo "reference image not found: $REFERENCE_IMAGE" >&2
  exit 2
fi

if [[ "${DRY_RUN}" != "1" && -z "${OPENAI_API_KEY:-}" ]]; then
  echo "OPENAI_API_KEY is not set" >&2
  exit 2
fi

cd "$PROJECT_DIR"

node --input-type=module -e '
  import fs from "node:fs";
  const manifest = JSON.parse(fs.readFileSync(process.argv[1], "utf8"));
  for (const item of manifest.items) {
    console.log([item.id, item.type, item.promptFile, item.targetFile].join("\t"));
  }
' "$MANIFEST" | {
  started=0
  generated=0
  while IFS=$'\t' read -r id type prompt_file target_file; do
    if [[ -n "$ONLY_ID" && "$id" != "$ONLY_ID" ]]; then
      continue
    fi
    if [[ -n "$START_AT" && "$started" == "0" ]]; then
      if [[ "$id" != "$START_AT" ]]; then
        continue
      fi
      started=1
    fi
    mkdir -p "$(dirname "$target_file")"
    if [[ "$DRY_RUN" == "1" ]]; then
      echo "DRY_RUN ${id} -> ${target_file} reference=${REFERENCE_IMAGE} request_size=${REQUEST_SIZE} target_size=${TARGET_SIZE}"
    else
      {
        echo "[$(date -Is)] START ${id} ${type} -> ${target_file}"
        python3 "$CLI" edit \
          --model "$MODEL" \
          --image "$REFERENCE_IMAGE" \
          --prompt-file "$prompt_file" \
          --quality "$QUALITY" \
          --size "$REQUEST_SIZE" \
          --out "$target_file" \
          --force
        if [[ "$REQUEST_SIZE" != "$TARGET_SIZE" && "$TARGET_SIZE" == "1920x1080" ]]; then
          python3 - "$target_file" <<'PY'
import sys
from pathlib import Path
from PIL import Image

target = Path(sys.argv[1])
with Image.open(target) as image:
    if image.size == (1920, 1088):
        image.crop((0, 0, 1920, 1080)).save(target)
    elif image.size != (1920, 1080):
        raise SystemExit(f"unexpected generated size: {image.size[0]}x{image.size[1]}")
PY
        fi
        echo "[$(date -Is)] DONE ${id} ${type} -> ${target_file}"
      } 2>&1 | tee -a "$LOG_FILE"
    fi
    generated=$((generated + 1))
    if [[ "$LIMIT" != "0" && "$generated" -ge "$LIMIT" ]]; then
      break
    fi
  done
}
