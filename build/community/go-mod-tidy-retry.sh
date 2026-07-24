#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${repo_root}"

max_attempts=3
retry_backoff_seconds="${COMMUNITY_TIDY_RETRY_BACKOFF_SECONDS:-2}"
if [[ ! "${retry_backoff_seconds}" =~ ^[0-9]+$ ]]; then
  echo "COMMUNITY_TIDY_RETRY_BACKOFF_SECONDS must be a non-negative integer" >&2
  exit 2
fi

for ((attempt = 1; attempt <= max_attempts; attempt++)); do
  echo "community go mod tidy: attempt ${attempt}/${max_attempts}" >&2
  if go mod tidy; then
    echo "community go mod tidy: PASS on attempt ${attempt}/${max_attempts}" >&2
    exit 0
  else
    status=$?
  fi

  echo "community go mod tidy: attempt ${attempt}/${max_attempts} failed with exit ${status}" >&2
  if (( attempt == max_attempts )); then
    echo "community go mod tidy: FAILED after ${max_attempts} attempts" >&2
    exit "${status}"
  fi
  echo "community go mod tidy: retrying in ${retry_backoff_seconds}s" >&2
  sleep "${retry_backoff_seconds}"
done
