#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
scratch="$(mktemp -d)"
linked="${scratch}/erda-linked"
builder="erda-community-linked-$$-${RANDOM}"
builder_created=false
cleanup() {
  if [[ "${builder_created}" == true ]]; then
    docker buildx rm --force "${builder}" >/dev/null 2>&1 || true
  fi
  git -C "${repo_root}" worktree remove --force "${linked}" >/dev/null 2>&1 || true
  rmdir "${scratch}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

builder_proxy="${COMMUNITY_BUILDER_PROXY_URL:-}"
if [[ -n "${builder_proxy}" && ! "${builder_proxy}" =~ ^https?://[^[:space:]]+$ ]]; then
  echo "COMMUNITY_BUILDER_PROXY_URL must be an HTTP(S) URL" >&2
  exit 1
fi
builder_args=(create --name "${builder}" --driver docker-container)
if [[ -n "${builder_proxy}" ]]; then
  builder_args+=(
    --driver-opt "network=host"
    --driver-opt "env.http_proxy=${builder_proxy}"
    --driver-opt "env.https_proxy=${builder_proxy}"
  )
fi
docker buildx "${builder_args[@]}" >/dev/null
builder_created=true
docker buildx inspect --builder "${builder}" --bootstrap >/dev/null

git -C "${repo_root}" worktree add --detach "${linked}" HEAD
# Overlay the current tracked/untracked implementation files so this pre-commit
# verification exercises the exact candidate tree while preserving linked/.git.
git -C "${repo_root}" ls-files --cached --others --exclude-standard -z |
  tar --null -C "${repo_root}" -T - -cf - |
  tar -C "${linked}" -xf -
test -f "${linked}/.git"
grep -q '^gitdir: ' "${linked}/.git"

revision="$(git -C "${linked}" rev-parse HEAD)"
epoch=1700000000
version="0.1.0-linked.${revision:0:12}"
image="ghcr.io/mosaicplane/erda:sha-${revision:0:12}"
(
  cd "${linked}"
  build_env=(
    "BUILDER_NAME=${builder}"
    "VCS_REF=${revision}"
    "SOURCE_DATE_EPOCH=${epoch}"
    "VERSION=${version}"
    "EMBEDDED_IMAGE=${image}"
    "OUTPUT_IMAGE=${image}"
  )
  if [[ -n "${builder_proxy}" ]]; then
    build_env+=(
      "HTTP_PROXY=${builder_proxy}"
      "HTTPS_PROXY=${builder_proxy}"
    )
  fi
  env "${build_env[@]}" bash build/scripts/community_image.sh
)

test "$(docker image inspect --format '{{ index .Config.Labels "org.opencontainers.image.revision" }}' \
  "${image}")" = "${revision}"
if docker history --no-trunc --format '{{.CreatedBy}}' "${image}" |
  rg 'git (rev-parse|submodule)'; then
  echo "container build history contains a Git metadata command" >&2
  exit 1
fi
docker run --rm --entrypoint grep "${image}" \
  -aF "${revision}" /erda/cmd/erda-server/bin >/dev/null
docker run --rm --entrypoint grep "${image}" \
  -aF '2023-11-14 22:13:20 UTC' /erda/cmd/erda-server/bin >/dev/null
test "${image}" = "ghcr.io/mosaicplane/erda:sha-${revision:0:12}"
echo "community linked-worktree backend image: PASS"
