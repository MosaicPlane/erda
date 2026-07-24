# Community backend image verification

`make build-community-image` builds the public, pinned Linux/amd64 backend image.
`make verify-community-image-reproducibility` performs two no-cache builds with
separate temporary Buildx builders and requires identical image IDs and root
filesystem layers.

The linked-worktree and reproducibility tests create their own Buildx builders
and always remove them on exit. They do not assume a user-specific proxy. When
the builders need a proxy, pass it explicitly:

```sh
COMMUNITY_BUILDER_PROXY_URL=http://host.internal:7890 \
  make verify-community-image-reproducibility
```

The value is supplied to the temporary BuildKit daemon through parameterized
`env.http_proxy` and `env.https_proxy` driver options and to build steps through
Docker's predefined proxy build arguments. Proxy-enabled temporary builders use
host networking so aliases such as `host.internal` resolve inside BuildKit. No
proxy address or port is the default.

On OrbStack installations where an isolated BuildKit daemon cannot resolve
`host.internal`, use its host-network loopback explicitly:

```sh
COMMUNITY_BUILDER_PROXY_URL=http://127.0.0.1:7890 \
  make verify-community-image-reproducibility
```

Community builds default to conservative Go concurrency for stable cross-arch
builders: `COMMUNITY_GO_BUILD_PARALLELISM=1` controls Go package build
parallelism and `COMMUNITY_GO_MAX_PROCS=2` controls compiler runtime
parallelism. Both are positive-integer parameters and can be raised explicitly
on native amd64 builders. Existing caller `GOFLAGS` are preserved before the
required deterministic and parallelism flags are appended.

The community-only project-level `go mod tidy` step retries at most three times
with a two-second backoff and logs every attempt without hiding Go's stderr.
Persistent failures still stop the build with a non-zero exit. The legacy
`build-all` and `build-one` targets keep their existing single-attempt `tidy`
dependency.

Community image exporters rewrite every layer timestamp to
`SOURCE_DATE_EPOCH`. The default `COMMUNITY_IMAGE_OUTPUT_TYPE=docker` exports a
temporary Docker archive and loads it into the local image store. Set
`COMMUNITY_IMAGE_OUTPUT_PATH` to retain that Docker archive instead. Reproducible
OCI image archives and root filesystem tarballs are available with:

```sh
COMMUNITY_IMAGE_OUTPUT_TYPE=oci \
COMMUNITY_IMAGE_OUTPUT_PATH=/tmp/erda-community.oci.tar \
  bash build/scripts/community_image.sh

COMMUNITY_IMAGE_OUTPUT_TYPE=tar \
COMMUNITY_IMAGE_OUTPUT_PATH=/tmp/erda-community.rootfs.tar \
  bash build/scripts/community_image.sh
```

OCI and tar output require `COMMUNITY_IMAGE_OUTPUT_PATH`. The community
Dockerfile also removes package-manager caches and time-stamped install logs,
and its component registration generator sorts scenario names before emitting
Go source.
