#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
scratch="$(mktemp -d)"
cleanup() {
  rm -rf "${scratch}"
}
trap cleanup EXIT

cache="${scratch}/cache"
tools="${scratch}/tools"
mkdir -p "${cache}"
printf 'tampered archive\n' >"${cache}/protoc-3.15.8-linux-x86_64.zip"

if PROTO_DOWNLOAD_DIR="${cache}" \
  bash "${repo_root}/build/community/proto-bootstrap.sh" "${tools}" \
  >"${scratch}/bootstrap.log" 2>&1; then
  echo "tampered protoc archive was accepted" >&2
  exit 1
fi
if ! grep -Eq 'checksum mismatch.*protoc' "${scratch}/bootstrap.log"; then
  cat "${scratch}/bootstrap.log" >&2
  echo "tamper rejection did not come from the protoc checksum gate" >&2
  exit 1
fi
test ! -e "${tools}/bin/protoc"
echo "community proto tamper rejection: PASS"
