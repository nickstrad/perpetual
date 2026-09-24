#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
# Pinned digest comes from setup's committed versions.env.
source scripts/versions.env
fixture_name="perpetual-test-$$-$RANDOM"
fixture_logs="$(mktemp -d /tmp/perpetual-pg-logs.XXXXXX)"
cleanup() {
  result=$?
  timeout 10s docker logs "$fixture_name" > "$fixture_logs/postgres.log" 2>&1 || true
  if ! timeout 15s docker rm -f "$fixture_name" >/dev/null; then
    echo "Fixture cleanup failed: $fixture_name; logs: $fixture_logs" >&2
    result=1
  fi
  if (( result != 0 )); then echo "Fixture logs: $fixture_logs" >&2; fi
  exit "$result"
}
timeout 10s docker info >/dev/null
# Only this script owns this server; arbitrary user databases are never restarted.
# Docker may assign a new ephemeral host port on restart. Bind an explicitly
# selected host port so the adapter's original DSN still addresses the server.
fixture_host_port="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()')"
trap cleanup EXIT
timeout 180s docker run --detach --name "$fixture_name" --publish "127.0.0.1:${fixture_host_port}:5432" --env POSTGRES_PASSWORD=fixture-only "$POSTGRES_IMAGE" > /dev/null
ready=0
fixture_ready_deadline=$((SECONDS + 30))
for ((attempt=0; attempt<200 && SECONDS<fixture_ready_deadline; attempt++)); do
  if timeout 2s docker exec "$fixture_name" pg_isready -h 127.0.0.1 -U postgres >/dev/null 2>&1; then ready=1; break; fi
  sleep 0.1
done
if (( ready == 0 )); then echo "PostgreSQL readiness exceeded bounded attempts/30s" >&2; exit 1; fi
fixture_port="$(timeout 5s docker port "$fixture_name" 5432/tcp)"
export PERPETUAL_TEST_DSN="postgres://postgres:fixture-only@${fixture_port}/postgres?sslmode=disable"
export PERPETUAL_TEST_CONTAINER="$fixture_name"
# Restart-owning packages share this server, so execute packages serially.
# Concurrency within each package and the race detector remain enabled.
export GOFLAGS="${GOFLAGS:-} -p=1"
timeout 5s docker exec "$fixture_name" psql -U postgres -Atc "SELECT version(); SHOW transaction_isolation; SHOW fsync; SHOW synchronous_commit; SHOW full_page_writes;"
if (( $# == 0 )); then set -- go test -p 1 -tags=integration ./... -count=1 -timeout=180s; fi
timeout --kill-after=5s 360s "$@"
