#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "${repo_root}"

required=(
  build/community/proto-tools.lock
  build/community/proto-inputs.lock
  build/community/proto-bootstrap.sh
  build/community/proto-generate.sh
  build/scripts/tests/community_proto_imports.go
  build/scripts/tests/community_proto_tree_hash.go
  build/scripts/tests/community_proto_coverage_test.sh
  build/scripts/tests/community_proto_closure_test.sh
  build/scripts/tests/community_proto_determinism_test.sh
  build/scripts/tests/community_proto_tamper_test.sh
)
for path in "${required[@]}"; do
  test -f "${path}" || { echo "missing ${path}" >&2; exit 1; }
done

proto_paths=(
  build/community/proto-tools.lock
  build/community/proto-inputs.lock
  build/community/proto-bootstrap.sh
  build/community/proto-generate.sh
)
if rg -n \
  'registry\.erda\.cloud|registry\.cn-hangzhou\.aliyuncs\.com/terminus|:latest|@latest|bitnami/git' \
  "${proto_paths[@]}" build/dockerfiles/Dockerfile.community; then
  echo "private or floating proto input found" >&2
  exit 1
fi
if rg -n 'git (clone|fetch|ls-remote|rev-parse|submodule)' "${proto_paths[@]}"; then
  echo "proto bootstrap/generation contains a Git dependency" >&2
  exit 1
fi

grep -Eq '^PROTO_BUILDER_IMAGE=docker\.io/library/golang@sha256:[0-9a-f]{64}$' \
  build/community/proto-tools.lock
grep -Eq '^PROTOC_VERSION=3\.15\.8$' build/community/proto-tools.lock
grep -Eq '^PROTOC_SHA256=[0-9a-f]{64}$' build/community/proto-tools.lock
grep -Eq '^ERDA_INFRA_COMMIT=[0-9a-f]{40}$' build/community/proto-tools.lock
grep -Eq '^ERDA_INFRA_SHA256=[0-9a-f]{64}$' build/community/proto-tools.lock
grep -Eq '^PROTOC_GEN_GO_VERSION=v[^[:space:]]+$' build/community/proto-tools.lock
grep -Eq '^PROTOC_GEN_GO_ARCHIVE_SHA256=[0-9a-f]{64}$' build/community/proto-tools.lock
grep -Eq '^PROTOC_GEN_GO_MOD_SUM=h1:[A-Za-z0-9+/=]+$' build/community/proto-tools.lock
grep -Eq '^PROTOC_GEN_GO_SUM=h1:[A-Za-z0-9+/=]+$' build/community/proto-tools.lock
grep -Eq '^PROTOC_GEN_VALIDATE_VERSION=v[^[:space:]]+$' build/community/proto-tools.lock
grep -Eq '^PROTOC_GEN_VALIDATE_ARCHIVE_SHA256=[0-9a-f]{64}$' build/community/proto-tools.lock
grep -Eq '^PROTOC_GEN_VALIDATE_MOD_SUM=h1:[A-Za-z0-9+/=]+$' build/community/proto-tools.lock
grep -Eq '^PROTOC_GEN_VALIDATE_SUM=h1:[A-Za-z0-9+/=]+$' build/community/proto-tools.lock
grep -Eq '^GOIMPORTS_VERSION=v[^[:space:]]+$' build/community/proto-tools.lock
grep -Eq '^GOIMPORTS_ARCHIVE_SHA256=[0-9a-f]{64}$' build/community/proto-tools.lock
grep -Eq '^GOIMPORTS_MOD_SUM=h1:[A-Za-z0-9+/=]+$' build/community/proto-tools.lock
grep -Eq '^GOIMPORTS_SUM=h1:[A-Za-z0-9+/=]+$' build/community/proto-tools.lock

grep -Eq '^OTEL_COMMIT=[0-9a-f]{40}$' build/community/proto-inputs.lock
grep -Eq '^OTEL_TREE=[0-9a-f]{40}$' build/community/proto-inputs.lock
grep -Eq '^OTEL_SHA256=[0-9a-f]{64}$' build/community/proto-inputs.lock

grep -q 'verify_sha256' build/community/proto-bootstrap.sh
grep -q 'verify_sha256' build/community/proto-generate.sh
grep -q '^FROM community-base AS proto-tools$' build/dockerfiles/Dockerfile.community
grep -q '^FROM proto-tools AS proto-inputs$' build/dockerfiles/Dockerfile.community
grep -q '^FROM proto-inputs AS proto-gen-a$' build/dockerfiles/Dockerfile.community
grep -q '^FROM proto-inputs AS proto-gen-b$' build/dockerfiles/Dockerfile.community
grep -q '^FROM proto-gen-a AS proto-verify$' build/dockerfiles/Dockerfile.community
grep -q 'COPY --from=proto-gen-b' build/dockerfiles/Dockerfile.community
grep -q 'COPY --from=proto-verify .*api/proto-go' build/dockerfiles/Dockerfile.community

bash -n build/community/proto-bootstrap.sh
bash -n build/community/proto-generate.sh
echo "community proto policy: PASS"
