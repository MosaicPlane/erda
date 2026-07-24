#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "${repo_root}"

required=(
  build/community/base-images.env
  build/community/debian.sources
  build/community/python-requirements.in
  build/community/python-requirements.lock
  build/community/runtime-node/package.json
  build/community/runtime-node/package-lock.json
  build/community/proto-tools.lock
  build/community/proto-inputs.lock
  build/community/proto-bootstrap.sh
  build/community/proto-generate.sh
  build/community/go-mod-tidy-retry.sh
  build/community/image-output.sh
  build/community/README.md
  build/dockerfiles/Dockerfile.community
  build/scripts/community_image.sh
  build/scripts/tests/community_linked_worktree_test.sh
  build/scripts/tests/community_reproducibility_test.sh
  build/scripts/tests/community_exporter_reproducibility_test.sh
  build/scripts/tests/community_prepare_determinism_test.sh
  build/scripts/tests/community_tidy_retry_test.sh
  build/scripts/tests/community_runtime_node_test.sh
  build/scripts/tests/community_proto_policy_test.sh
  build/scripts/tests/community_proto_coverage_test.sh
  build/scripts/tests/community_proto_closure_test.sh
  build/scripts/tests/community_proto_determinism_test.sh
  build/scripts/tests/community_proto_runtime_test.sh
  build/scripts/tests/community_proto_tamper_test.sh
)
for path in "${required[@]}"; do
  test -f "${path}" || { echo "missing ${path}" >&2; exit 1; }
done

source build/community/base-images.env
[[ "${ERDA_BUILD_IMAGE}" =~ ^docker\.io/library/golang@sha256:[0-9a-f]{64}$ ]]
[[ "${LIBGIT2_COMMIT}" =~ ^[0-9a-f]{40}$ ]]
test "$(wc -l < <(rg --files cmd | rg '/main\.go$'))" -eq 24

scan_paths=(
  build/community
  build/dockerfiles/Dockerfile.community
  build/scripts/community_image.sh
)
if rg -n 'registry\.erda\.cloud|registry\.cn-hangzhou\.aliyuncs\.com/terminus|:latest|bitnami/git' "${scan_paths[@]}"; then
  echo "private or floating backend build input found" >&2
  exit 1
fi

grep -q '^URIs: https://snapshot.debian.org/archive/debian/' build/community/debian.sources
grep -q '^dicttoxml==1.7.16$' build/community/python-requirements.in
grep -q '^xmindparser==1.0.9$' build/community/python-requirements.in
grep -q -- '--hash=sha256:' build/community/python-requirements.lock
grep -q '"integrity": "sha512-' build/community/runtime-node/package-lock.json
grep -q '"jackson-path": "bin/jackson-path.js"' \
  build/community/runtime-node/package-lock.json
grep -q 'COPY --from=build /go/src/github.com/erda-project/erda/bin/erda-cli /erda/bin/erda-cli' \
  build/dockerfiles/Dockerfile.community
grep -q 'node_modules/.bin/jackson-path /usr/local/bin/jackson-path' \
  build/dockerfiles/Dockerfile.community
grep -q 'ARG EMBEDDED_IMAGE' build/dockerfiles/Dockerfile.community
! grep -q 'ARG DOCKER_IMAGE' build/dockerfiles/Dockerfile.community
grep -q '^\.git$' build/dockerfiles/Dockerfile.community.dockerignore
grep -q 'RUN make build-all-community' build/dockerfiles/Dockerfile.community
grep -q '^FROM community-base AS build-source$' build/dockerfiles/Dockerfile.community
grep -q '^FROM build-source AS build$' build/dockerfiles/Dockerfile.community
grep -q '^FROM build-source AS community-modules$' build/dockerfiles/Dockerfile.community
grep -q '^RUN go mod download all$' build/dockerfiles/Dockerfile.community
grep -q '^FROM community-modules AS erda-server-build$' build/dockerfiles/Dockerfile.community
grep -q '^ARG COMMUNITY_DIAGNOSTIC_SLOT=default$' build/dockerfiles/Dockerfile.community
grep -q 'make build-one-community' build/dockerfiles/Dockerfile.community
grep -q '^FROM scratch AS erda-server-output$' build/dockerfiles/Dockerfile.community
grep -q '^FROM community-base AS runtime-base$' build/dockerfiles/Dockerfile.community
grep -q '^FROM runtime-base$' build/dockerfiles/Dockerfile.community
grep -q -- '-buildvcs=false' build/dockerfiles/Dockerfile.community
grep -q '^ARG COMMUNITY_GO_BUILD_PARALLELISM=1$' \
  build/dockerfiles/Dockerfile.community
grep -q '^ARG COMMUNITY_GO_MAX_PROCS=2$' build/dockerfiles/Dockerfile.community
grep -q '^ARG COMMUNITY_GO_EXTRA_FLAGS=""$' build/dockerfiles/Dockerfile.community
grep -q 'GOFLAGS="${COMMUNITY_GO_EXTRA_FLAGS} -buildvcs=false -p=${COMMUNITY_GO_BUILD_PARALLELISM}"' \
  build/dockerfiles/Dockerfile.community
grep -q 'GOMAXPROCS=${COMMUNITY_GO_MAX_PROCS}' \
  build/dockerfiles/Dockerfile.community
grep -q 'COMMUNITY_GO_BUILD_PARALLELISM' build/scripts/community_image.sh
grep -q 'COMMUNITY_GO_MAX_PROCS' build/scripts/community_image.sh
grep -Fq 'go_extra_flags="${GOFLAGS:-}"' build/scripts/community_image.sh
grep -q 'COMMUNITY_GO_EXTRA_FLAGS=${go_extra_flags}' \
  build/scripts/community_image.sh
grep -Fq 'type=docker,dest=${output_path},rewrite-timestamp=true' \
  build/community/image-output.sh
if grep -Fq -- '--load' build/scripts/community_image.sh; then
  echo "community image must use the timestamp-rewriting Docker exporter" >&2
  exit 1
fi
grep -Fq 'type=oci,dest=${output_path},rewrite-timestamp=true' \
  build/community/image-output.sh
grep -Fq 'type=tar,dest=${output_path},rewrite-timestamp=true' \
  build/community/image-output.sh
grep -Fq 'rm -f /var/cache/ldconfig/aux-cache' \
  build/dockerfiles/Dockerfile.community
grep -Fq '/var/log/alternatives.log /var/log/dpkg.log' \
  build/dockerfiles/Dockerfile.community
grep -Fq 'rm -rf /root/.npm /tmp/community-npm-cache' \
  build/dockerfiles/Dockerfile.community
grep -q 'org.opencontainers.image.revision="${VCS_REF}"' \
  build/dockerfiles/Dockerfile.community
if rg -n 'git (rev-parse|submodule)' build/dockerfiles/Dockerfile.community; then
  echo "container-side Git metadata dependency found" >&2
  exit 1
fi
grep -q '^build-all: build-version submodule prepare tidy$' Makefile
grep -q '^build-one: build-version submodule prepare tidy$' Makefile
grep -q '^build-all-community: build-version community-source-tree prepare community-tidy$' Makefile
grep -q '^build-one-community: build-version community-source-tree prepare community-tidy$' Makefile
grep -q '^community-tidy:$' Makefile
grep -q 'bash build/community/go-mod-tidy-retry.sh' Makefile
grep -q '^max_attempts=3$' build/community/go-mod-tidy-retry.sh
grep -q '^retry_backoff_seconds="${COMMUNITY_TIDY_RETRY_BACKOFF_SECONDS:-2}"$' \
  build/community/go-mod-tidy-retry.sh
grep -q '^verify-community-image-exporters: verify-community-image$' Makefile
grep -q '^verify-community-image-reproducibility: verify-community-image-exporters$' Makefile
for test_script in \
  build/scripts/tests/community_linked_worktree_test.sh \
  build/scripts/tests/community_reproducibility_test.sh; do
  grep -q 'COMMUNITY_BUILDER_PROXY_URL' "${test_script}"
  grep -q 'env.http_proxy=' "${test_script}"
  grep -q 'env.https_proxy=' "${test_script}"
  grep -q -- '--driver-opt "network=host"' "${test_script}"
  grep -q 'docker buildx rm' "${test_script}"
done
grep -q 'NO_CACHE=true' build/scripts/tests/community_reproducibility_test.sh
if grep -Fq '="$(create_builder ' build/scripts/tests/community_reproducibility_test.sh; then
  echo "builder creation must not run in a subshell that loses cleanup state" >&2
  exit 1
fi
grep -q 'COMMUNITY_BUILDER_PROXY_URL' build/community/README.md
grep -q 'COMMUNITY_GO_BUILD_PARALLELISM' build/community/README.md
grep -q 'COMMUNITY_GO_MAX_PROCS' build/community/README.md
grep -q 'COMMUNITY_IMAGE_OUTPUT_TYPE' build/community/README.md
grep -q 'COMMUNITY_IMAGE_OUTPUT_PATH' build/community/README.md
grep -q 'three times' build/community/README.md
if COMMUNITY_GO_BUILD_PARALLELISM=0 \
  bash build/scripts/community_image.sh >/dev/null 2>&1; then
  echo "zero Go build parallelism must be rejected before Docker starts" >&2
  exit 1
fi
if COMMUNITY_GO_MAX_PROCS=invalid \
  bash build/scripts/community_image.sh >/dev/null 2>&1; then
  echo "invalid GOMAXPROCS must be rejected before Docker starts" >&2
  exit 1
fi
if COMMUNITY_IMAGE_OUTPUT_TYPE=invalid \
  bash build/scripts/community_image.sh >/dev/null 2>&1; then
  echo "invalid community image output type must be rejected" >&2
  exit 1
fi
for archive_type in oci tar; do
  if COMMUNITY_IMAGE_OUTPUT_TYPE="${archive_type}" \
    bash build/scripts/community_image.sh >/dev/null 2>&1; then
    echo "${archive_type} output without a path must be rejected" >&2
    exit 1
  fi
done
bash -n build/community/resolve-base-images.sh
bash -n build/community/go-mod-tidy-retry.sh
bash -n build/community/image-output.sh
bash -n build/scripts/community_image.sh
bash -n build/scripts/tests/community_linked_worktree_test.sh
bash -n build/scripts/tests/community_reproducibility_test.sh
bash -n build/scripts/tests/community_exporter_reproducibility_test.sh
bash -n build/scripts/tests/community_prepare_determinism_test.sh
bash -n build/scripts/tests/community_tidy_retry_test.sh
bash -n build/scripts/tests/community_runtime_node_test.sh
echo "community backend image policy: PASS"
