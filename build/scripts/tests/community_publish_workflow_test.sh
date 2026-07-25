#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "${repo_root}"

workflow=.github/workflows/community-image-acr.yml
test -f "${workflow}"

grep -q '^  push:$' "${workflow}"
grep -q '^      - master$' "${workflow}"
grep -q '^  workflow_dispatch:$' "${workflow}"
grep -q '^  contents: read$' "${workflow}"
grep -q '^  ACR_REGISTRY: registry.cn-beijing.aliyuncs.com$' "${workflow}"
grep -q '^  IMAGE_REPOSITORY: registry.cn-beijing.aliyuncs.com/myerda/erda$' "${workflow}"
grep -Fq 'username: ${{ secrets.ACR_USERNAME }}' "${workflow}"
grep -Fq 'password: ${{ secrets.ACR_PASSWORD }}' "${workflow}"
grep -q 'sudo apt-get install --no-install-recommends -y ripgrep' "${workflow}"
grep -Fq 'OUTPUT_IMAGE: ${{ steps.image.outputs.image }}' "${workflow}"
grep -Fq 'EMBEDDED_IMAGE: ${{ steps.image.outputs.image }}' "${workflow}"
grep -Fq 'IMAGE_REPOSITORY }}@${{ steps.push.outputs.digest }}' "${workflow}"
grep -q 'platform: "linux/amd64"' "${workflow}"
grep -q 'sbom.spdx.json' "${workflow}"
grep -q 'trivy.json' "${workflow}"
grep -q 'evidence.json' "${workflow}"
grep -q 'digest.txt' "${workflow}"
grep -q 'exit-code: "0"' "${workflow}"

for action in \
  'actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1' \
  'docker/setup-buildx-action@4d04d5d9486b7bd6fa91e7baf45bbb4f8b9deedd' \
  'docker/login-action@abd2ef45e78c5afb21d64d4ca52ee8550d9572c7' \
  'anchore/sbom-action@e22c389904149dbc22b58101806040fa8d37a610' \
  'aquasecurity/trivy-action@ed142fd0673e97e23eac54620cfb913e5ce36c25' \
  'actions/upload-artifact@043fb46d1a93c77aae656e7c1c64a875d1fc6a0a'; do
  grep -Fq "uses: ${action}" "${workflow}"
done

if rg -n ':latest|ACR_PASSWORD: [^$]|password: [^$]' "${workflow}"; then
  echo "floating image tag or plaintext ACR password found in workflow" >&2
  exit 1
fi

echo "community backend ACR workflow policy: PASS"
