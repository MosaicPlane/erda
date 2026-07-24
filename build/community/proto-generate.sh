#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "${repo_root}/build/community/proto-inputs.lock"

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

check_output() {
  local output="$1"
  case "${output}" in
    ""|"/"|".")
      echo "refusing unsafe proto output: ${output}" >&2
      exit 1
      ;;
  esac
}

normalize_times() {
  local root="$1"
  local epoch="${SOURCE_DATE_EPOCH:-0}"
  [[ "${epoch}" =~ ^[0-9]+$ ]]
  find "${root}" -exec touch -h -d "@${epoch}" {} +
}

stage_inputs() {
  local source_root="$1"
  local output="$2"
  local cache="${PROTO_DOWNLOAD_DIR:-/tmp/erda-community-proto-downloads}"
  local output_parent
  local staging
  local archive
  local extract_root
  local archive_root
  local proto_count
  local proto_file
  local relative
  local proto_directory
  local expected_go_package
  local go_package_count
  check_output "${output}"
  test -d "${source_root}/api/proto" || {
    echo "tracked proto source missing: ${source_root}/api/proto" >&2
    exit 1
  }
  test -f "${source_root}/api/proto-go/go.mod" || {
    echo "proto-go scaffold missing: ${source_root}/api/proto-go/go.mod" >&2
    exit 1
  }
  output_parent="$(dirname "${output}")"
  mkdir -p "${cache}" "${output_parent}"
  staging="$(mktemp -d "${output_parent}/.proto-inputs.XXXXXX")"
  extract_root="$(mktemp -d "${output_parent}/.otel-extract.XXXXXX")"
  cleanup_stage_inputs() {
    rm -rf "${staging}" "${extract_root}"
  }
  trap cleanup_stage_inputs RETURN

  archive="${cache}/${OTEL_ARCHIVE}"
  if [[ ! -f "${archive}" ]]; then
    curl --fail --location --silent --show-error \
      --retry 3 --retry-all-errors \
      "${OTEL_URL}" --output "${archive}.partial"
    mv "${archive}.partial" "${archive}"
  fi
  verify_sha256 opentelemetry "${OTEL_SHA256}" "${archive}"
  tar -xzf "${archive}" -C "${extract_root}"
  archive_root="${extract_root}/opentelemetry-proto-${OTEL_COMMIT}"
  test -d "${archive_root}/opentelemetry" || {
    echo "locked OpenTelemetry archive root missing" >&2
    exit 1
  }

  mkdir -p "${staging}/proto" "${staging}/proto-go-scaffold"
  cp -a "${source_root}/api/proto/." "${staging}/proto/"
  cp -a "${source_root}/api/proto-go/." "${staging}/proto-go-scaffold/"
  rm -rf \
    "${staging}/proto/externalrepo" \
    "${staging}/proto/opentelemetry"
  cp -a "${archive_root}/opentelemetry" "${staging}/proto/opentelemetry"

  while IFS= read -r proto_file; do
    awk '
      {
        if ($0 ~ /^message /) {
          print "// +SKIP_GO-FORM"
        }
        print
      }
    ' "${proto_file}" >"${proto_file}.tmp"
    mv "${proto_file}.tmp" "${proto_file}"
  done < <(
    find "${staging}/proto/opentelemetry" -type f -name '*.proto' |
      LC_ALL=C sort
  )

  while IFS= read -r proto_file; do
    relative="${proto_file#${staging}/proto/}"
    if [[ "${relative}" == opentelemetry/* ]]; then
      continue
    fi
    proto_directory="${relative%/*}"
    test "${relative}" != "${proto_directory}" || {
      echo "proto file is not in a package directory: ${relative}" >&2
      exit 1
    }
    expected_go_package="${PROTO_GO_MODULE}/${proto_directory}/pb"
    go_package_count="$(
      awk '
        /^[[:space:]]*option go_package[[:space:]]*=/ { count++ }
        END { print count + 0 }
      ' "${proto_file}"
    )"
    test "${go_package_count}" -eq 1 || {
      echo "expected one go_package option in ${relative}, got ${go_package_count}" >&2
      exit 1
    }
    awk \
      -v go_package="${expected_go_package}" \
      -v proto_directory="${proto_directory}" '
      /^[[:space:]]*import[[:space:]]+"[^/"]+\.proto";[[:space:]]*$/ {
        quote = index($0, "\"")
        print substr($0, 1, quote) proto_directory "/" substr($0, quote + 1)
        next
      }
      /^[[:space:]]*option go_package[[:space:]]*=/ {
        print "option go_package = \"" go_package "\";"
        next
      }
      { print }
    ' "${proto_file}" >"${proto_file}.tmp"
    mv "${proto_file}.tmp" "${proto_file}"
  done < <(
    find "${staging}/proto" -type f -name '*.proto' |
      LC_ALL=C sort
  )

  proto_count="$(
    find "${staging}/proto/opentelemetry" -type f -name '*.proto' |
      wc -l |
      tr -d ' '
  )"
  test "${proto_count}" -eq "${OTEL_PROTO_COUNT}" || {
    echo "OpenTelemetry proto count: got ${proto_count}, want ${OTEL_PROTO_COUNT}" >&2
    exit 1
  }
  normalize_times "${staging}"
  rm -rf "${output}"
  mv "${staging}" "${output}"
  staging=""
  rm -rf "${extract_root}"
  extract_root=""
  trap - RETURN
  echo "community proto inputs: PASS"
}

run_protoc() {
  local tools_root="$1"
  local proto_root="$2"
  local proto_directory="$3"
  shift 3
  PATH="${tools_root}/bin:${PATH}" \
    "${tools_root}/bin/protoc" \
      -I="${proto_root}" \
      -I="${proto_directory}" \
      -I="${tools_root}/include" \
      "$@"
}

flatten_generated_directory() {
  local output_root="$1"
  local relative="$2"
  local nested="${output_root}/${relative}"
  local first_component="${relative%%/*}"
  if [[ -d "${nested}" ]]; then
    cp -a "${nested}/." "${output_root}/"
    rm -rf "${output_root:?}/${first_component}"
  fi
}

generate() {
  local tools_root="$1"
  local inputs_root="$2"
  local output="$3"
  local output_parent
  local staging
  local proto_root="${inputs_root}/proto"
  local proto_directory
  local relative
  local pb_output
  local client_output
  check_output "${output}"
  test -x "${tools_root}/bin/protoc" || {
    echo "locked protoc missing: ${tools_root}/bin/protoc" >&2
    exit 1
  }
  test -d "${tools_root}/include" || {
    echo "locked proto includes missing: ${tools_root}/include" >&2
    exit 1
  }
  test -d "${proto_root}" || {
    echo "staged proto inputs missing: ${proto_root}" >&2
    exit 1
  }
  test -f "${inputs_root}/proto-go-scaffold/go.mod" || {
    echo "staged proto-go scaffold missing" >&2
    exit 1
  }

  output_parent="$(dirname "${output}")"
  mkdir -p "${output_parent}"
  staging="$(mktemp -d "${output_parent}/.proto-generate.XXXXXX")"
  cleanup_generate() {
    rm -rf "${staging}"
  }
  trap cleanup_generate RETURN
  cp -a "${inputs_root}/proto-go-scaffold/." "${staging}/"

  while IFS= read -r proto_directory; do
    relative="${proto_directory#${proto_root}/}"
    if [[ "${relative}" == "${proto_directory}" || -z "${relative}" ]]; then
      continue
    fi
    if [[ "${relative}" == opentelemetry/* ]]; then
      continue
    fi
    pb_output="${staging}/${relative}/pb"
    client_output="${staging}/${relative}/client"
    mkdir -p "${pb_output}" "${client_output}"
    proto_files=()
    while IFS= read -r proto_file; do
      proto_files+=("${proto_file}")
    done < <(find "${proto_directory}" -maxdepth 1 -type f -name '*.proto' | sort)

    run_protoc "${tools_root}" "${proto_root}" "${proto_directory}" \
      --go_out="${pb_output}" \
      --go_opt=paths=source_relative \
      "${proto_files[@]}"
    run_protoc "${tools_root}" "${proto_root}" "${proto_directory}" \
      --validate_out="paths=source_relative,lang=go:${pb_output}" \
      "${proto_files[@]}"
    run_protoc "${tools_root}" "${proto_root}" "${proto_directory}" \
      --go-json_out="${pb_output}" \
      --go-json_opt=paths=source_relative \
      --go-json_opt=emit_defaults=true \
      --go-json_opt=allow_unknown_fields=true \
      "${proto_files[@]}"
    run_protoc "${tools_root}" "${proto_root}" "${proto_directory}" \
      --go-grpc_out="${pb_output}" \
      --go-grpc_opt=paths=source_relative \
      --go-client_out="${client_output}" \
      --go-client_opt=paths=source_relative \
      "${proto_files[@]}"
    run_protoc "${tools_root}" "${proto_root}" "${proto_directory}" \
      --go-http_out="${pb_output}" \
      --go-http_opt=paths=source_relative \
      --go-form_out="${pb_output}" \
      --go-form_opt=paths=source_relative \
      "${proto_files[@]}"
    run_protoc "${tools_root}" "${proto_root}" "${proto_directory}" \
      --go-register_out="${pb_output}" \
      --go-register_opt=paths=source_relative \
      --go-register_opt=grpc=true \
      --go-register_opt=http=true \
      "${proto_files[@]}"
    flatten_generated_directory "${pb_output}" "${relative}"
    flatten_generated_directory "${client_output}" "${relative}"
  done < <(
    find "${proto_root}" -type f -name '*.proto' -exec dirname {} \; |
      LC_ALL=C sort -u
  )

  {
    echo "// Code generated by proto-generate.sh. DO NOT EDIT."
    echo
    echo "package proto"
    echo
    echo "import ("
    while IFS= read -r proto_directory; do
      relative="${proto_directory#${proto_root}/}"
      if [[ "${relative}" != "${proto_directory}" &&
            -n "${relative}" &&
            "${relative}" != opentelemetry/* &&
            "${relative}" != openapiv1/testplatform/* ]]; then
        printf '\t_ "github.com/erda-project/erda-proto-go/%s/pb"\n' \
          "${relative}"
      fi
    done < <(
      find "${proto_root}" -type f -name '*.proto' -exec dirname {} \; |
        LC_ALL=C sort -u
    )
    echo ")"
  } >"${staging}/all.go"

  while IFS= read -r go_file; do
    gofmt -w "${go_file}"
    GOPROXY=off GOWORK=off \
      "${tools_root}/bin/goimports" \
        -local github.com/erda-project/erda-proto-go \
        -w "${go_file}"
  done < <(find "${staging}" -type f -name '*.go' | LC_ALL=C sort)

  normalize_times "${staging}"
  rm -rf "${output}"
  mv "${staging}" "${output}"
  staging=""
  trap - RETURN
  echo "community proto generation: PASS"
}

case "${1:-}" in
  stage-inputs)
    test "$#" -eq 3 || {
      echo "usage: proto-generate.sh stage-inputs SOURCE_ROOT OUTPUT_ROOT" >&2
      exit 1
    }
    stage_inputs "$2" "$3"
    ;;
  generate)
    test "$#" -eq 4 || {
      echo "usage: proto-generate.sh generate TOOLS_ROOT INPUTS_ROOT OUTPUT_ROOT" >&2
      exit 1
    }
    generate "$2" "$3" "$4"
    ;;
  *)
    echo "usage: proto-generate.sh {stage-inputs|generate} ..." >&2
    exit 1
    ;;
esac
