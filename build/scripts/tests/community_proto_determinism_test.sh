#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
generated_a="${1:-${repo_root}/build/community/.proto-gen-a-missing}"
generated_b="${2:-${repo_root}/build/community/.proto-gen-b-missing}"

hash_a="$(
  go run "${repo_root}/build/scripts/tests/community_proto_tree_hash.go" \
    "${generated_a}"
)"
hash_b="$(
  go run "${repo_root}/build/scripts/tests/community_proto_tree_hash.go" \
    "${generated_b}"
)"
test "${hash_a}" = "${hash_b}" || {
  echo "generated proto trees differ" >&2
  echo "a: ${hash_a}" >&2
  echo "b: ${hash_b}" >&2
  exit 1
}
echo "community proto determinism: PASS ${hash_a}"
