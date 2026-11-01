#!/usr/bin/env bash
# Docker-only local application package. Never removes named data volumes.
set -euo pipefail
repo_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_dir"
config_path="${LEDGERX_APP_ENV:-$repo_dir/.env.app}"
action="${1:-start}"

case "$action" in
  start|stop|status|logs|backup|restore) ;;
  *) echo 'Usage: ./scripts/app.sh [start|stop|status|logs|backup|restore BACKUP_DIRECTORY]' >&2; exit 2 ;;
esac
command -v docker >/dev/null || { echo 'Install and start Docker Desktop first.' >&2; exit 1; }
docker compose version >/dev/null

if [[ ! -f "$config_path" ]]; then
  [[ "$action" == start || "$action" == restore ]] || { echo 'Start the app first.' >&2; exit 1; }
  command -v openssl >/dev/null || { echo 'OpenSSL is required to generate private local secrets.' >&2; exit 1; }
  # noclobber prevents replacing another startup process's newly created file.
  (umask 077; set -o noclobber
    printf 'LEDGERX_PORT=%s\nPOSTGRES_PASSWORD=%s\nJWT_SIGNING_KEY=%s\nMETRICS_TOKEN=%s\n' \
      "${LEDGERX_PORT:-8088}" "$(openssl rand -hex 32)" "$(openssl rand -hex 32)" "$(openssl rand -hex 32)" > "$config_path")
  echo 'Created private local configuration. Keep it with your server backups.'
fi

# A previously exported developer .env must not override this stack's secrets.
compose=(env -u POSTGRES_PASSWORD -u JWT_SIGNING_KEY -u METRICS_TOKEN docker compose --env-file "$config_path" -f deploy/compose.app.yml)
# Optional override for managed build CA trust or deployment-specific settings.
if [[ -n "${LEDGERX_COMPOSE_OVERRIDE:-}" ]]; then
  compose+=(-f "$LEDGERX_COMPOSE_OVERRIDE")
fi
case "$action" in
  start)
    # Build first so a failed build leaves the previous app available. Stop it
    # before migrations so old workers cannot write through a schema upgrade.
    "${compose[@]}" build app
    "${compose[@]}" stop -t 40 app
    "${compose[@]}" up --wait --wait-timeout 180
    port="$("${compose[@]}" port app 8080)"
    echo "LedgerX is ready at http://$port (this computer only)."
    ;;
  stop) "${compose[@]}" down ;;
  status) "${compose[@]}" ps -a ;;
  logs) "${compose[@]}" logs --tail 100 -f app ;;
  backup)
    umask 077
    mkdir -p tmp/server-backups
    destination="tmp/server-backups/ledgerx-$(date -u +%Y%m%dT%H%M%SZ)-$$"
    mkdir "$destination"
    running_app="$("${compose[@]}" ps --status running -q app)"
    running_redis="$("${compose[@]}" ps --status running -q redis)"
    resume() {
      [[ -z "$running_redis" ]] || "${compose[@]}" start redis
      [[ -z "$running_app" ]] || "${compose[@]}" start app
    }
    trap resume EXIT
    echo 'Pausing app and Redis for a consistent backup.'
    "${compose[@]}" stop -t 40 app redis
    "${compose[@]}" run --rm --no-deps -T maintenance pg_dump -Fc > "$destination/database.dump"
    "${compose[@]}" run --rm --no-deps -T maintenance tar -czf - -C /exports . > "$destination/exports.tar.gz"
    "${compose[@]}" run --rm --no-deps -T maintenance tar -czf - -C /redis-state . > "$destination/redis.tar.gz"
    cp "$config_path" "$destination/server.env"
    chmod 600 "$destination/server.env"
    printf 'LedgerX server backup v1\n' > "$destination/COMPLETE"
    echo "Server backup saved to $destination. It contains private data and secrets."
    ;;
  restore)
    source_dir="${2:?Pass a trusted server backup directory}"
    for file in COMPLETE database.dump exports.tar.gz redis.tar.gz; do
      [[ -f "$source_dir/$file" ]] || { echo "Incomplete backup: missing $file" >&2; exit 1; }
    done
    "${compose[@]}" up -d --wait postgres
    empty="$("${compose[@]}" exec -T postgres psql -U ledgerx -d ledgerx -Atc "SELECT NOT EXISTS(SELECT 1 FROM pg_tables WHERE schemaname='public')")"
    [[ "$empty" == t ]] || { echo 'Restore refused: destination database must be completely empty. Use a new Compose project.' >&2; exit 1; }
    "${compose[@]}" stop app redis
    "${compose[@]}" run --rm --no-deps -T maintenance pg_restore --dbname ledgerx --exit-on-error --no-owner --no-privileges < "$source_dir/database.dump"
    "${compose[@]}" run --rm --no-deps -T maintenance tar -xzf - -C /exports < "$source_dir/exports.tar.gz"
    "${compose[@]}" run --rm --no-deps -T maintenance tar -xzf - -C /redis-state < "$source_dir/redis.tar.gz"
    # Restored backups must not resurrect old refresh sessions.
    "${compose[@]}" exec -T postgres psql -U ledgerx -d ledgerx -v ON_ERROR_STOP=1 -c 'DELETE FROM refresh_tokens;'
    "${compose[@]}" up --build --wait --wait-timeout 180
    echo 'Restored the server into an empty stack. Log in again with your password.'
    ;;
esac
