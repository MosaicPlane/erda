#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
output="${repo_root}/build/community/base-images.env"

resolve() {
  local tagged_ref="$1"
  local repository="${tagged_ref%:*}"
  local digest
  digest="$(docker buildx imagetools inspect "${tagged_ref}" | awk '/^Digest:/ {print $2; exit}')"
  [[ "${digest}" =~ ^sha256:[0-9a-f]{64}$ ]] || {
    echo "could not resolve ${tagged_ref}" >&2
    exit 1
  }
  printf '%s@%s' "${repository}" "${digest}"
}

image="$(resolve docker.io/library/golang:1.24.6-bookworm)"
libgit2_commit="$(
  git ls-remote https://github.com/libgit2/libgit2.git \
    'refs/tags/v1.3.2' | awk '{print $1}'
)"
[[ "${libgit2_commit}" =~ ^[0-9a-f]{40}$ ]] || {
  echo "could not resolve libgit2 v1.3.2" >&2
  exit 1
}
{
  printf 'ERDA_BUILD_IMAGE=%s\n' "${image}"
  printf 'LIBGIT2_COMMIT=%s\n' "${libgit2_commit}"
} >"${output}"
echo "locked ${image}"
