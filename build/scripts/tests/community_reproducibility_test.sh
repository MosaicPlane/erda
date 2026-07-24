#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "${repo_root}"

builder_proxy="${COMMUNITY_BUILDER_PROXY_URL:-}"
if [[ -n "${builder_proxy}" && ! "${builder_proxy}" =~ ^https?://[^[:space:]]+$ ]]; then
  echo "COMMUNITY_BUILDER_PROXY_URL must be an HTTP(S) URL" >&2
  exit 1
fi

builders=()
created_builder=
cleanup() {
  local builder
  for builder in "${builders[@]}"; do
    docker buildx rm --force "${builder}" >/dev/null 2>&1 || true
  done
}
trap cleanup EXIT

create_builder() {
  local slot="$1"
  local builder="erda-community-repro-${slot}-$$-${RANDOM}"
  local args=(create --name "${builder}" --driver docker-container)
  if [[ -n "${builder_proxy}" ]]; then
    args+=(
      --driver-opt "network=host"
      --driver-opt "env.http_proxy=${builder_proxy}"
      --driver-opt "env.https_proxy=${builder_proxy}"
    )
  fi
  docker buildx "${args[@]}" >/dev/null
  builders+=("${builder}")
  docker buildx inspect --builder "${builder}" --bootstrap >/dev/null
  created_builder="${builder}"
}

revision="${VCS_REF:-$(git rev-parse HEAD)}"
source_date_epoch="${SOURCE_DATE_EPOCH:-$(git show -s --format=%ct "${revision}")}"
short_revision="${revision:0:12}"
version="${VERSION:-0.1.0-dev.${short_revision}}"
embedded_image="${EMBEDDED_IMAGE:-ghcr.io/mosaicplane/erda:sha-${short_revision}}"
image_prefix="${COMMUNITY_REPRO_IMAGE_PREFIX:-ghcr.io/mosaicplane/erda:repro-${short_revision}}"
image_a="${image_prefix}-a"
image_b="${image_prefix}-b"

build_once() {
  local builder="$1"
  local output_image="$2"
  local build_env=(
    "BUILDER_NAME=${builder}"
    "NO_CACHE=true"
    "VCS_REF=${revision}"
    "SOURCE_DATE_EPOCH=${source_date_epoch}"
    "VERSION=${version}"
    "EMBEDDED_IMAGE=${embedded_image}"
    "OUTPUT_IMAGE=${output_image}"
  )
  if [[ -n "${builder_proxy}" ]]; then
    build_env+=(
      "HTTP_PROXY=${builder_proxy}"
      "HTTPS_PROXY=${builder_proxy}"
    )
  fi
  env "${build_env[@]}" bash build/scripts/community_image.sh
}

create_builder a
builder_a="${created_builder}"
create_builder b
builder_b="${created_builder}"
build_once "${builder_a}" "${image_a}"
build_once "${builder_b}" "${image_b}"

id_a="$(docker image inspect --format '{{.Id}}' "${image_a}")"
id_b="$(docker image inspect --format '{{.Id}}' "${image_b}")"
test "${id_a}" = "${id_b}"
test "$(
  docker image inspect --format '{{json .RootFS.Layers}}' "${image_a}"
)" = "$(
  docker image inspect --format '{{json .RootFS.Layers}}' "${image_b}"
)"
echo "community backend image reproducibility: PASS ${id_a}"
