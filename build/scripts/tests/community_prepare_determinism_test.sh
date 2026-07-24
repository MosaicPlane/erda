#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
scratch="$(mktemp -d)"
cleanup() {
  rm -rf "${scratch}"
}
trap cleanup EXIT

mkdir -p "${scratch}/component-protocol"
cp -R \
  "${repo_root}/internal/core/openapi/legacy/component-protocol/." \
  "${scratch}/component-protocol/"

baseline=
for attempt in 1 2 3 4 5 6 7 8; do
  (
    cd "${scratch}/component-protocol/generate"
    go run gen_auto_register.go
  )
  generated="${scratch}/auto-register-${attempt}.go"
  cp \
    "${scratch}/component-protocol/generate/auto_register/auto_register.go" \
    "${generated}"
  if [[ -z "${baseline}" ]]; then
    baseline="${generated}"
    continue
  fi
  if ! cmp -s "${baseline}" "${generated}"; then
    echo "component registration generation changed on attempt ${attempt}" >&2
    exit 1
  fi
done

echo "community prepare generation determinism: PASS"
