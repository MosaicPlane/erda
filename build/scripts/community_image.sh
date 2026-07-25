#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${repo_root}"
source build/community/base-images.env
source build/community/image-output.sh

revision="${VCS_REF:-$(git rev-parse HEAD)}"
source_date_epoch="${SOURCE_DATE_EPOCH:-$(git show -s --format=%ct "${revision}")}"
[[ "${revision}" =~ ^[0-9a-f]{40}$ ]]
[[ "${source_date_epoch}" =~ ^[0-9]+$ ]]
short_revision="${revision:0:12}"
version="${VERSION:-0.1.0-dev.${short_revision}}"
[[ "${version}" =~ ^[0-9A-Za-z._-]+$ ]]
output_image="${OUTPUT_IMAGE:-registry.cn-beijing.aliyuncs.com/myerda/erda:sha-${short_revision}}"
embedded_image="${EMBEDDED_IMAGE:-registry.cn-beijing.aliyuncs.com/myerda/erda:sha-${short_revision}}"
go_build_parallelism="${COMMUNITY_GO_BUILD_PARALLELISM:-1}"
go_max_procs="${COMMUNITY_GO_MAX_PROCS:-2}"
go_extra_flags="${GOFLAGS:-}"
output_type="${COMMUNITY_IMAGE_OUTPUT_TYPE:-docker}"
output_path="${COMMUNITY_IMAGE_OUTPUT_PATH:-}"
[[ "${go_build_parallelism}" =~ ^[1-9][0-9]*$ ]]
[[ "${go_max_procs}" =~ ^[1-9][0-9]*$ ]]
community_configure_image_output \
  "${output_type}" "${output_path}" "${output_image}"
trap community_cleanup_image_output EXIT

buildx_args=(--platform linux/amd64 --provenance=false)
[[ -z "${BUILDER_NAME:-}" ]] || buildx_args+=(--builder "${BUILDER_NAME}")
[[ "${NO_CACHE:-false}" != true ]] || buildx_args+=(--no-cache)
[[ -z "${HTTP_PROXY:-}" ]] || buildx_args+=(--build-arg "HTTP_PROXY=${HTTP_PROXY}")
[[ -z "${HTTPS_PROXY:-}" ]] || buildx_args+=(--build-arg "HTTPS_PROXY=${HTTPS_PROXY}")
[[ -z "${NO_PROXY:-}" ]] || buildx_args+=(--build-arg "NO_PROXY=${NO_PROXY}")

docker buildx build \
  "${buildx_args[@]}" \
  "${community_image_output_args[@]}" \
  --build-arg "ERDA_BUILD_IMAGE=${ERDA_BUILD_IMAGE}" \
  --build-arg "LIBGIT2_COMMIT=${LIBGIT2_COMMIT}" \
  --build-arg "COMMUNITY_GO_BUILD_PARALLELISM=${go_build_parallelism}" \
  --build-arg "COMMUNITY_GO_MAX_PROCS=${go_max_procs}" \
  --build-arg "COMMUNITY_GO_EXTRA_FLAGS=${go_extra_flags}" \
  --build-arg "SOURCE_DATE_EPOCH=${source_date_epoch}" \
  --build-arg "VCS_REF=${revision}" \
  --build-arg "VERSION=${version}" \
  --build-arg "EMBEDDED_IMAGE=${embedded_image}" \
  --file build/dockerfiles/Dockerfile.community \
  .
community_load_image_output
echo "${output_image}"
