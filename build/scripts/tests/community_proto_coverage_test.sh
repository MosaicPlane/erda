#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
generated_root="${1:-${repo_root}/api/proto-go}"

go run "${repo_root}/build/scripts/tests/community_proto_imports.go" \
  --source-root "${repo_root}" \
  --generated-root "${generated_root}" \
  --expected-active 168 \
  --expected-lexical 169
echo "community proto AST coverage: PASS"
