#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
scratch="$(mktemp -d)"
cleanup() {
  rm -rf "${scratch}"
}
trap cleanup EXIT

cp "${repo_root}/build/community/runtime-node/package.json" \
   "${repo_root}/build/community/runtime-node/package-lock.json" \
   "${scratch}/"

registry="${NPM_REGISTRY:-https://registry.npmjs.org}"
(
  cd "${scratch}"
  npm ci --omit=dev --ignore-scripts --registry="${registry}"
  npm audit --omit=dev --audit-level=moderate --registry="${registry}"
)

input='{"@id":"@id:root","name":"root","department":{"@id":"@id:dep","name":"Engineering"},"departmentAlias":"@id:dep"}'
output="$(
  "${scratch}/node_modules/.bin/jackson-path" \
    --json "${input}" \
    --path '$.departmentAlias.name' \
    --unwrap
)"
test "${output}" = '"Engineering"'
echo "community runtime node tools: PASS"
