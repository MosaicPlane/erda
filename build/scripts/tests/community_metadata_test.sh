#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "${repo_root}"

revision=0123456789abcdef0123456789abcdef01234567
first="$(
  SOURCE_DATE_EPOCH=1700000000 VCS_REF="${revision}" VERSION=0.1.0-dev.0123456789ab \
    make --no-print-directory build-version MODULE_PATH=erda-server
)"
second="$(
  SOURCE_DATE_EPOCH=1700000000 VCS_REF="${revision}" VERSION=0.1.0-dev.0123456789ab \
    make --no-print-directory build-version MODULE_PATH=erda-server
)"

test "${first}" = "${second}"
grep -q 'BuildTime: 2023-11-14 22:13:20 UTC' <<<"${first}"
grep -q "CommitID: ${revision}" <<<"${first}"
options="$(
  SOURCE_DATE_EPOCH=1700000000 VCS_REF="${revision}" VERSION=0.1.0-dev.0123456789ab \
    make --no-print-directory -s print-go-build-options
)"
grep -q -- '-trimpath' <<<"${options}"
grep -q -- '-buildvcs=false' <<<"${options}"
if rg -n 'BUILD_TIME := \$\(shell date' Makefile; then
  echo "wall-clock BUILD_TIME remains in Makefile" >&2
  exit 1
fi
if rg -n -- '-buildvcs=true' Makefile; then
  echo "automatic Go VCS discovery remains enabled" >&2
  exit 1
fi
echo "community metadata test: PASS"
