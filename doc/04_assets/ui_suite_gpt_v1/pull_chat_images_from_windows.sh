#!/usr/bin/env bash
set -euo pipefail

# Pull GPT chat-window generated images from the Windows desktop download inbox
# into this repository's UI image inbox.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

WIN_USER="${WIN_USER:-11441}"
WIN_HOST="${WIN_HOST:-10.3.6.6}"
SSH_KEY="${SSH_KEY:-/root/.ssh/id_ed25519_traffic_ui_sync}"
WIN_INBOX="${WIN_INBOX:-C:/Users/11441/Downloads/traffic-ui-imagegen-inbox}"
WIN_PROCESSED="${WIN_PROCESSED:-C:/Users/11441/Downloads/traffic-ui-imagegen-processed}"
LOCAL_INBOX="${LOCAL_INBOX:-$SCRIPT_DIR/inbox}"
MOVE_REMOTE="${MOVE_REMOTE:-0}"

mkdir -p "$LOCAL_INBOX" "$SCRIPT_DIR/logs"

if [[ ! -f "$SSH_KEY" ]]; then
  echo "missing SSH key: $SSH_KEY" >&2
  exit 2
fi

ssh_base=(
  ssh
  -i "$SSH_KEY"
  -o BatchMode=yes
  -o StrictHostKeyChecking=accept-new
  -o ConnectTimeout=8
)

scp_base=(
  scp
  -i "$SSH_KEY"
  -o BatchMode=yes
  -o StrictHostKeyChecking=accept-new
  -o ConnectTimeout=8
)

remote="$WIN_USER@$WIN_HOST"
timestamp="$(date +%Y%m%d-%H%M%S)"
log_file="$SCRIPT_DIR/logs/pull-chat-images-$timestamp.log"

echo "remote=$remote" | tee -a "$log_file"
echo "win_inbox=$WIN_INBOX" | tee -a "$log_file"
echo "local_inbox=$LOCAL_INBOX" | tee -a "$log_file"

"${ssh_base[@]}" "$remote" "chcp 65001 >NUL & if not exist \"${WIN_INBOX//\//\\}\" mkdir \"${WIN_INBOX//\//\\}\" & if not exist \"${WIN_PROCESSED//\//\\}\" mkdir \"${WIN_PROCESSED//\//\\}\"" >>"$log_file" 2>&1

before_count="$(find "$LOCAL_INBOX" -maxdepth 1 -type f | wc -l)"

patterns=(
  '*.png'
  '*.jpg'
  '*.jpeg'
  '*.webp'
  '*.avif'
)

for pattern in "${patterns[@]}"; do
  # scp returns non-zero when a wildcard has no matches on Windows OpenSSH.
  "${scp_base[@]}" "$remote:$WIN_INBOX/$pattern" "$LOCAL_INBOX/" >>"$log_file" 2>&1 || true
done

after_count="$(find "$LOCAL_INBOX" -maxdepth 1 -type f | wc -l)"
pulled_count="$((after_count - before_count))"

if [[ "$MOVE_REMOTE" == "1" ]]; then
  move_script="$(cat <<PS
\$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [Text.UTF8Encoding]::UTF8
\$inbox = '$WIN_INBOX'
\$processed = '$WIN_PROCESSED'
New-Item -ItemType Directory -Force -Path \$processed | Out-Null
Get-ChildItem -Path (Join-Path \$inbox '*') -File |
  Where-Object { @('.png', '.jpg', '.jpeg', '.webp', '.avif') -contains \$_.Extension.ToLowerInvariant() } |
  Move-Item -Destination \$processed -Force
PS
)"
  encoded_move_script="$(
    python3 -c 'import base64,sys; print(base64.b64encode(sys.stdin.read().encode("utf-16le")).decode())' <<<"$move_script"
  )"
  "${ssh_base[@]}" "$remote" "powershell -NoProfile -ExecutionPolicy Bypass -EncodedCommand $encoded_move_script" >>"$log_file" 2>&1 || {
    echo "pulled files, but failed to move remote files; see $log_file" >&2
    exit 3
  }
fi

echo "pulled_count=$pulled_count" | tee -a "$log_file"
echo "log_file=$log_file"

find "$LOCAL_INBOX" -maxdepth 1 -type f \( -iname '*.png' -o -iname '*.jpg' -o -iname '*.jpeg' -o -iname '*.webp' -o -iname '*.avif' \) -printf '%f\n' | sort
