#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
generated_root="${1:-${repo_root}/api/proto-go}"

if find "${generated_root}" -type f -name '*.pb.go' \
  -exec grep -HnE '^// source: [^/]+\.proto$' {} +; then
  echo "generated descriptor uses a basename-only source path" >&2
  exit 1
fi
if grep -Eq 'erda-proto-go/(opentelemetry|openapiv1/testplatform)/' \
  "${generated_root}/all.go"; then
  echo "all.go imports a transitive or explicitly inactive proto namespace" >&2
  exit 1
fi
if [[ -d "${generated_root}/opentelemetry" ]]; then
  echo "generated tree contains a duplicate local OpenTelemetry module" >&2
  exit 1
fi
grep -q '"go.opentelemetry.io/proto/otlp/metrics/v1"' \
  "${generated_root}/oap/collector/receiver/opentelemetry/pb/opentelemetry.pb.go"

(
  cd "${repo_root}"
  go run -mod=readonly ./build/scripts/tests/community_proto_runtime.go
)
echo "community proto runtime registry: PASS"
