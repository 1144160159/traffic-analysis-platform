#!/usr/bin/env bash
set -euo pipefail

# T-CH-001 authoritative ClickHouse migration runner.
#
# Required environment:
#   CLICKHOUSE_PASSWORD
# Optional environment:
#   CLICKHOUSE_HOSTS      whitespace-separated nodes (default: localhost)
#   CLICKHOUSE_USER       default
#   CLICKHOUSE_MIGRATIONS repository migration directory
#   CLICKHOUSE_CLIENT     clickhouse-client
#
# The runner applies every migration to every node. It never edits a migration
# at runtime and refuses to continue if an applied version's checksum changes.

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(cd -- "$script_dir/../.." && pwd)
migration_dir=${CLICKHOUSE_MIGRATIONS:-"$repo_root/deployments/clickhouse/migrations"}
client=${CLICKHOUSE_CLIENT:-clickhouse-client}
hosts=${CLICKHOUSE_HOSTS:-localhost}
user=${CLICKHOUSE_USER:-default}

if [[ ! -d "$migration_dir" ]]; then
  echo "ClickHouse migration directory does not exist: $migration_dir" >&2
  exit 2
fi

if [[ -z "${CLICKHOUSE_PASSWORD+x}" ]]; then
  echo "CLICKHOUSE_PASSWORD must be set (an empty password is allowed explicitly)." >&2
  exit 2
fi

mapfile -t migration_files < <(find "$migration_dir" -maxdepth 1 -type f -name '*.sql' -print | LC_ALL=C sort)
if [[ ${#migration_files[@]} -eq 0 ]]; then
  echo "No ClickHouse migrations found in $migration_dir" >&2
  exit 2
fi

previous_version=""
for migration_file in "${migration_files[@]}"; do
  filename=$(basename -- "$migration_file")
  version=${filename%%_*}
  description=${filename#*_}
  description=${description%.sql}
  if [[ ! "$version" =~ ^[0-9]{12}$ ]] || [[ ! "$description" =~ ^[a-z0-9_]+$ ]]; then
    echo "Invalid migration filename: $filename" >&2
    exit 2
  fi
  if [[ -n "$previous_version" && "$version" < "$previous_version" ]]; then
    echo "Migration versions are not ordered: $previous_version then $version" >&2
    exit 2
  fi
  previous_version=$version

  checksum=$(sha256sum "$migration_file" | awk '{print $1}')
  for host in $hosts; do
    client_args=(
      --host "$host"
      --user "$user"
      --password "$CLICKHOUSE_PASSWORD"
    )

    applied_checksum=""
    if "$client" "${client_args[@]}" --query \
      "EXISTS TABLE traffic.alignment_schema_migrations_local" 2>/dev/null | grep -qx '1'; then
      applied_checksum=$("$client" "${client_args[@]}" --query \
        "SELECT argMax(checksum, applied_at) FROM traffic.alignment_schema_migrations_local WHERE version={version:String}" \
        --param_version "$version")
    fi

    if [[ -n "$applied_checksum" ]]; then
      if [[ "$applied_checksum" != "$checksum" ]]; then
        echo "Checksum mismatch for applied migration $version on $host" >&2
        echo "recorded=$applied_checksum candidate=$checksum" >&2
        exit 3
      fi
      echo "[$host] already applied $filename ($checksum)"
      continue
    fi

    echo "[$host] applying $filename ($checksum)"
    "$client" "${client_args[@]}" --multiquery < "$migration_file"
    "$client" "${client_args[@]}" --query \
      "INSERT INTO traffic.alignment_schema_migrations_local (version,checksum,description,applied_by) VALUES ({version:String},{checksum:String},{description:String},{applied_by:String})" \
      --param_version "$version" \
      --param_checksum "$checksum" \
      --param_description "$description" \
      --param_applied_by "${USER:-clickhouse-migration-runner}"
  done
done

echo "ClickHouse migrations applied successfully to: $hosts"
