#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
tools_root="${1:-${repo_root}/build/community/.proto-tools-missing}"
inputs_root="${2:-${repo_root}/build/community/.proto-inputs-missing}"

protoc="${tools_root}/bin/protoc"
test -x "${protoc}" || {
  echo "missing locked protoc: ${protoc}" >&2
  exit 1
}
test -d "${inputs_root}/proto" || {
  echo "missing staged proto input closure: ${inputs_root}/proto" >&2
  exit 1
}
test -d "${tools_root}/include" || {
  echo "missing locked protoc include closure: ${tools_root}/include" >&2
  exit 1
}

scratch="$(mktemp -d)"
cleanup() {
  rm -rf "${scratch}"
}
trap cleanup EXIT

proto_files=()
while IFS= read -r path; do
  proto_files+=("${path}")
done < <(find "${inputs_root}/proto" -type f -name '*.proto' | LC_ALL=C sort)
test "${#proto_files[@]}" -gt 0 || {
  echo "no staged proto files" >&2
  exit 1
}

for proto_file in "${proto_files[@]}"; do
  "${protoc}" \
    --proto_path="$(dirname "${proto_file}")" \
    --proto_path="${inputs_root}/proto" \
    --proto_path="${tools_root}/include" \
    --include_imports \
    --descriptor_set_out="${scratch}/closure.pb" \
    "${proto_file}"
  test -s "${scratch}/closure.pb"
done
echo "community proto descriptor closure: PASS"
