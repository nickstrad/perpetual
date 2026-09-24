#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

# Capture ownership while each disposable container exists. Checking the exact
# IDs afterward must never become a global dangling-volume prune.
evidence_dir="$(mktemp -d /tmp/perpetual-cleanup-check.XXXXXX)"
echo "Cleanup ownership evidence: $evidence_dir"
for expected_status in 0 23; do
  metadata="$evidence_dir/exit-$expected_status.txt"
  if scripts/test-postgres.sh bash -c '
    set -euo pipefail
    printf "%s\n" "$PERPETUAL_TEST_CONTAINER" > "$1"
    timeout 5s docker inspect --format "{{.Id}}" "$PERPETUAL_TEST_CONTAINER" >> "$1"
    volume="$(timeout 5s docker inspect --format "{{range .Mounts}}{{if eq .Destination \"/var/lib/postgresql\"}}{{if eq .Type \"volume\"}}{{.Name}}{{end}}{{end}}{{end}}" "$PERPETUAL_TEST_CONTAINER")"
    test -n "$volume"
    timeout 5s docker volume inspect "$volume" >/dev/null
    printf "%s\n" "$volume" >> "$1"
    exit "$2"
  ' fixture-cleanup "$metadata" "$expected_status"; then
    actual_status=0
  else
    actual_status=$?
  fi
  if (( actual_status != expected_status )); then
    echo "Fixture returned $actual_status, expected $expected_status; evidence: $metadata" >&2
    exit 1
  fi
  mapfile -t owned < "$metadata"
  if (( ${#owned[@]} != 3 )) || [[ "${owned[0]}" != perpetual-test-* ]]; then
    echo "Fixture ownership metadata incomplete: $metadata" >&2
    exit 1
  fi
  # Listing must succeed; a daemon/inspection error is not proof of absence.
  surviving_container="$(timeout 5s docker container ls --all --filter "id=${owned[1]}" --format '{{.ID}}')"
  surviving_volume="$(timeout 5s docker volume ls --filter "name=${owned[2]}" --format '{{.Name}}')"
  if [[ -n "$surviving_container" || -n "$surviving_volume" ]]; then
    echo "Owned fixture resource survived exit $expected_status: container=${owned[0]} volume=${owned[2]}; evidence: $metadata" >&2
    exit 1
  fi
  echo "Fixture exit $expected_status preserved; owned container and data volume removed."
done
