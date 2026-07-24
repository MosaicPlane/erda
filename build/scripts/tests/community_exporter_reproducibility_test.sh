#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
source "${repo_root}/build/community/image-output.sh"

scratch="$(mktemp -d)"
builder="erda-community-exporter-$$-${RANDOM}"
probe_load_image="erda-community-exporter-probe:load-$$"
builder_created=false
cleanup() {
  if [[ "${builder_created}" == true ]]; then
    docker buildx rm --force "${builder}" >/dev/null 2>&1 || true
  fi
  docker image rm --force "${probe_load_image}" >/dev/null 2>&1 || true
  community_cleanup_image_output
  rm -rf "${scratch}"
}
trap cleanup EXIT

for slot in a b; do
  mkdir -p "${scratch}/context-${slot}/payload"
  printf '%s\n' \
    'ARG SOURCE_DATE_EPOCH' \
    'FROM scratch' \
    'ARG SOURCE_DATE_EPOCH' \
    'COPY payload/ /payload/' \
    >"${scratch}/context-${slot}/Dockerfile"
  printf '%s\n' 'same content' \
    >"${scratch}/context-${slot}/payload/value.txt"
  touch -t 202311142213.20 \
    "${scratch}/context-${slot}/Dockerfile" \
    "${scratch}/context-${slot}/payload/value.txt"
done
touch -t 202401010000.00 "${scratch}/context-a/payload"
touch -t 202402020000.00 "${scratch}/context-b/payload"

docker buildx create --name "${builder}" --driver docker-container >/dev/null
builder_created=true
docker buildx inspect --builder "${builder}" --bootstrap >/dev/null

for output_type in docker oci tar; do
  for slot in a b; do
    output="${scratch}/${slot}.${output_type}.tar"
    community_configure_image_output \
      "${output_type}" "${output}" "erda-community-exporter-probe:stable"
    docker buildx build \
      --builder "${builder}" \
      --no-cache \
      --provenance=false \
      --platform linux/amd64 \
      --build-arg SOURCE_DATE_EPOCH=1700000000 \
      "${community_image_output_args[@]}" \
      "${scratch}/context-${slot}" >/dev/null
  done
  cmp -s \
    "${scratch}/a.${output_type}.tar" \
    "${scratch}/b.${output_type}.tar"
done

community_configure_image_output \
  docker "" "${probe_load_image}"
test "${community_image_output_should_load}" = true
test -f "${community_image_output_temporary_path}"
docker buildx build \
  --builder "${builder}" \
  --no-cache \
  --provenance=false \
  --platform linux/amd64 \
  --build-arg SOURCE_DATE_EPOCH=1700000000 \
  "${community_image_output_args[@]}" \
  "${scratch}/context-a" >/dev/null
community_load_image_output
test "$(
  docker image inspect --format '{{.Created}}' "${probe_load_image}"
)" = "2023-11-14T22:13:20Z"
community_cleanup_image_output

echo "community BuildKit exporters reproducibility: PASS"
