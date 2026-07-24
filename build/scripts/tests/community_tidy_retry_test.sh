#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "${repo_root}"

tmp_dir="$(mktemp -d)"
cleanup() {
  rm -rf "${tmp_dir}"
}
trap cleanup EXIT

mkdir -p "${tmp_dir}/bin"
cat >"${tmp_dir}/bin/go" <<'STUB'
#!/usr/bin/env bash
set -euo pipefail

test "$#" -eq 2
test "$1" = mod
test "$2" = tidy

attempt=0
if [[ -f "${FAKE_GO_STATE}" ]]; then
  attempt="$(<"${FAKE_GO_STATE}")"
fi
attempt=$((attempt + 1))
printf '%s\n' "${attempt}" >"${FAKE_GO_STATE}"
echo "fake go tidy stderr attempt ${attempt}" >&2
if (( attempt <= FAKE_GO_FAILS_BEFORE_SUCCESS )); then
  exit 75
fi
echo "fake go tidy success attempt ${attempt}"
STUB
chmod +x "${tmp_dir}/bin/go"

retry_script="${repo_root}/build/community/go-mod-tidy-retry.sh"

transient_state="${tmp_dir}/transient-state"
transient_output="$(
  PATH="${tmp_dir}/bin:${PATH}" \
  FAKE_GO_STATE="${transient_state}" \
  FAKE_GO_FAILS_BEFORE_SUCCESS=1 \
  COMMUNITY_TIDY_RETRY_BACKOFF_SECONDS=0 \
    bash "${retry_script}" 2>&1
)"
test "$(<"${transient_state}")" -eq 2
grep -q 'community go mod tidy: attempt 1/3' <<<"${transient_output}"
grep -q 'fake go tidy stderr attempt 1' <<<"${transient_output}"
grep -q 'community go mod tidy: attempt 2/3' <<<"${transient_output}"
grep -q 'community go mod tidy: PASS on attempt 2/3' <<<"${transient_output}"

persistent_state="${tmp_dir}/persistent-state"
set +e
persistent_output="$(
  PATH="${tmp_dir}/bin:${PATH}" \
  FAKE_GO_STATE="${persistent_state}" \
  FAKE_GO_FAILS_BEFORE_SUCCESS=3 \
  COMMUNITY_TIDY_RETRY_BACKOFF_SECONDS=0 \
    bash "${retry_script}" 2>&1
)"
persistent_status=$?
set -e
test "${persistent_status}" -ne 0
test "$(<"${persistent_state}")" -eq 3
for attempt in 1 2 3; do
  grep -q "community go mod tidy: attempt ${attempt}/3" \
    <<<"${persistent_output}"
  grep -q "fake go tidy stderr attempt ${attempt}" <<<"${persistent_output}"
done
grep -q 'community go mod tidy: FAILED after 3 attempts' \
  <<<"${persistent_output}"
if grep -q 'attempt 4' <<<"${persistent_output}"; then
  echo "community tidy retry exceeded the three-attempt bound" >&2
  exit 1
fi

echo "community go mod tidy retry behavior: PASS"
