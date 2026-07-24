#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "${repo_root}/build/community/proto-tools.lock"

output="${1:?usage: proto-bootstrap.sh OUTPUT_ROOT}"
case "${output}" in
  ""|"/"|".")
    echo "refusing unsafe proto tools output: ${output}" >&2
    exit 1
    ;;
esac

cache="${PROTO_DOWNLOAD_DIR:-/tmp/erda-community-proto-downloads}"
output_parent="$(dirname "${output}")"
mkdir -p "${cache}" "${output_parent}"
staging="$(mktemp -d "${output_parent}/.proto-tools.XXXXXX")"
cleanup() {
  if [[ -n "${staging}" && -d "${staging}" ]]; then
    rm -rf "${staging}"
  fi
}
trap cleanup EXIT

file_sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

verify_sha256() {
  local label="$1"
  local expected="$2"
  local path="$3"
  local actual
  actual="$(file_sha256 "${path}")"
  if [[ "${actual}" != "${expected}" ]]; then
    echo "checksum mismatch for ${label}: expected ${expected}, got ${actual}" >&2
    exit 1
  fi
}

download_checked() {
  local label="$1"
  local url="$2"
  local expected="$3"
  local filename="$4"
  local path="${cache}/${filename}"
  if [[ ! -f "${path}" ]]; then
    curl --fail --location --silent --show-error \
      --retry 3 --retry-all-errors \
      "${url}" --output "${path}.partial"
    mv "${path}.partial" "${path}"
  fi
  verify_sha256 "${label}" "${expected}" "${path}"
  printf '%s' "${path}"
}

verify_go_module() {
  local label="$1"
  local module="$2"
  local version="$3"
  local expected_sum="$4"
  local expected_mod_sum="$5"
  local metadata
  local actual_sum
  local actual_mod_sum
  local directory
  metadata="$(go mod download -json "${module}@${version}")"
  actual_sum="$(jq -r '.Sum // empty' <<<"${metadata}")"
  actual_mod_sum="$(jq -r '.GoModSum // empty' <<<"${metadata}")"
  directory="$(jq -r '.Dir // empty' <<<"${metadata}")"
  if [[ "${actual_sum}" != "${expected_sum}" ]]; then
    echo "${label} module sum mismatch: expected ${expected_sum}, got ${actual_sum}" >&2
    exit 1
  fi
  if [[ "${actual_mod_sum}" != "${expected_mod_sum}" ]]; then
    echo "${label} go.mod sum mismatch: expected ${expected_mod_sum}, got ${actual_mod_sum}" >&2
    exit 1
  fi
  test -d "${directory}" || {
    echo "${label} module directory missing" >&2
    exit 1
  }
  printf '%s' "${directory}"
}

assert_go_binary_module() {
  local path="$1"
  local module="$2"
  local version="$3"
  local sum="$4"
  go version -m "${path}" |
    awk -v module="${module}" -v version="${version}" -v sum="${sum}" \
      '$1 == "mod" && $2 == module && $3 == version && $4 == sum { found = 1 }
       END { exit(found ? 0 : 1) }' || {
    echo "binary module identity mismatch: ${path} ${module}@${version}" >&2
    exit 1
  }
}

protoc_archive="$(
  download_checked protoc "${PROTOC_URL}" "${PROTOC_SHA256}" "${PROTOC_ARCHIVE}"
)"
erda_infra_archive="$(
  download_checked erda-infra \
    "${ERDA_INFRA_URL}" "${ERDA_INFRA_SHA256}" "${ERDA_INFRA_ARCHIVE}"
)"
download_checked protoc-gen-go-module \
  "${PROTOC_GEN_GO_ARCHIVE_URL}" \
  "${PROTOC_GEN_GO_ARCHIVE_SHA256}" \
  "${PROTOC_GEN_GO_ARCHIVE}" >/dev/null
download_checked protoc-gen-validate-module \
  "${PROTOC_GEN_VALIDATE_ARCHIVE_URL}" \
  "${PROTOC_GEN_VALIDATE_ARCHIVE_SHA256}" \
  "${PROTOC_GEN_VALIDATE_ARCHIVE}" >/dev/null
download_checked goimports-module \
  "${GOIMPORTS_ARCHIVE_URL}" \
  "${GOIMPORTS_ARCHIVE_SHA256}" \
  "${GOIMPORTS_ARCHIVE}" >/dev/null

mkdir -p "${staging}/bin" "${staging}/include" "${staging}/src"
python3 -m zipfile -e "${protoc_archive}" "${staging}"
chmod 0755 "${staging}/bin/protoc"
tar -xzf "${erda_infra_archive}" -C "${staging}/src"

export GOENV=off
export GOPATH="${staging}/gopath"
export GOMODCACHE="${staging}/gomodcache"
export GOBIN="${staging}/bin"
export GOPROXY=https://proxy.golang.org
export GOSUMDB=sum.golang.org
export GOTOOLCHAIN=local
export CGO_ENABLED=0
export GOOS=linux
export GOARCH=amd64
export GOFLAGS="-trimpath -buildvcs=false -ldflags=-buildid="

verify_go_module protoc-gen-go \
  "${PROTOC_GEN_GO_MODULE}" \
  "${PROTOC_GEN_GO_VERSION}" \
  "${PROTOC_GEN_GO_SUM}" \
  "${PROTOC_GEN_GO_MOD_SUM}" >/dev/null
pgv_module_dir="$(
  verify_go_module protoc-gen-validate \
    "${PROTOC_GEN_VALIDATE_MODULE}" \
    "${PROTOC_GEN_VALIDATE_VERSION}" \
    "${PROTOC_GEN_VALIDATE_SUM}" \
    "${PROTOC_GEN_VALIDATE_MOD_SUM}"
)"
verify_go_module goimports \
  "${GOIMPORTS_MODULE}" \
  "${GOIMPORTS_VERSION}" \
  "${GOIMPORTS_SUM}" \
  "${GOIMPORTS_MOD_SUM}" >/dev/null

go install "${PROTOC_GEN_GO_PACKAGE}@${PROTOC_GEN_GO_VERSION}"
go install "${PROTOC_GEN_VALIDATE_PACKAGE}@${PROTOC_GEN_VALIDATE_VERSION}"
go install "${GOIMPORTS_PACKAGE}@${GOIMPORTS_VERSION}"

erda_infra_root="${staging}/src/erda-infra-${ERDA_INFRA_COMMIT}"
test -d "${erda_infra_root}/tools/protoc" || {
  echo "locked erda-infra protoc source missing" >&2
  exit 1
}
plugins=(
  protoc-gen-go-grpc
  protoc-gen-go-client
  protoc-gen-go-http
  protoc-gen-go-form
  protoc-gen-go-json
  protoc-gen-go-register
)
for plugin in "${plugins[@]}"; do
  (
    cd "${erda_infra_root}/tools"
    go build \
      -mod=readonly \
      -o "${staging}/bin/${plugin}" \
      "./protoc/${plugin}"
  )
done

cp -a "${erda_infra_root}/tools/protoc/include/." "${staging}/include/"
mkdir -p \
  "${staging}/include/github.com/envoyproxy/protoc-gen-validate/validate"
cp -a "${pgv_module_dir}/validate/validate.proto" \
  "${staging}/include/github.com/envoyproxy/protoc-gen-validate/validate/validate.proto"

test "$("${staging}/bin/protoc" --version)" = "libprotoc ${PROTOC_VERSION}"
test "$("${staging}/bin/protoc-gen-go" --version)" = \
  "protoc-gen-go ${PROTOC_GEN_GO_VERSION}"
assert_go_binary_module \
  "${staging}/bin/protoc-gen-go" \
  "${PROTOC_GEN_GO_MODULE}" \
  "${PROTOC_GEN_GO_VERSION}" \
  "${PROTOC_GEN_GO_SUM}"
assert_go_binary_module \
  "${staging}/bin/protoc-gen-validate" \
  "${PROTOC_GEN_VALIDATE_MODULE}" \
  "${PROTOC_GEN_VALIDATE_VERSION}" \
  "${PROTOC_GEN_VALIDATE_SUM}"
assert_go_binary_module \
  "${staging}/bin/goimports" \
  "${GOIMPORTS_MODULE}" \
  "${GOIMPORTS_VERSION}" \
  "${GOIMPORTS_SUM}"
for plugin in "${plugins[@]}"; do
  go version -m "${staging}/bin/${plugin}" |
    grep -F "github.com/erda-project/erda-infra/tools/protoc/${plugin}" \
      >/dev/null
done

rm -rf \
  "${staging}/src" \
  "${staging}/gopath" \
  "${staging}/gomodcache"
if [[ -n "${SOURCE_DATE_EPOCH:-}" ]]; then
  [[ "${SOURCE_DATE_EPOCH}" =~ ^[0-9]+$ ]]
  find "${staging}" -exec touch -h -d "@${SOURCE_DATE_EPOCH}" {} +
fi

rm -rf "${output}"
mv "${staging}" "${output}"
staging=""
echo "community proto tools: PASS"
