# 17 — the release image (plan 04, step 3.1)

Date: 2026-11-23.

## Why

The fork has no image of its own. Upstream's `Dockerfile.release`
downloads `minio` and `mc` from dl.min.io, which answers 410, and the
`minio/minio` images are gone. The plain `Dockerfile` (CI, resiliency
tests) puts a local `./minio` on top of a 2025-03 community image and
stays as it is.

## What

`Dockerfile.release` is rewritten to build the binaries from source; the
build interface keeps upstream's `ARG RELEASE`. Only linux/amd64 is built
(Alexander, 2026-11-23: no arm64).

**The image is the community image with our binaries** (Alexander,
2026-11-23: user ids, settings and configuration as in the current
image). The reference is the last community release,
`minio/minio:RELEASE.2025-03-12T18-04-18Z` (fork copy
`ghcr.io/aursu/minio/minio`, the base of the plain `Dockerfile`). Taken
from it unchanged:

- runs as root (no `USER`), `WorkingDir /`;
- the same `ENV`, including `MINIO_UPDATE_MINISIGN_PUBKEY`;
- `RUN chmod -R 777 /usr/bin` before the binaries are copied, so the base
  image's files are 0777 and `minio`, `mc`, `curl` 0755, as there;
- `/usr/bin/docker-entrypoint.sh` 0775, unchanged script;
- the CA bundle at `/etc/ssl/certs/ca-certificates.crt` 0644;
- `/licenses/{CREDITS,LICENSE}` 0664 in a 0755 directory;
- `/usr/bin/{minio,mc}.sha256sum` in the same format,
  `<sha256> <binary>.<release tag>`;
- a static curl in `/usr/bin` (healthchecks call it);
- `VOLUME /data`, `ENTRYPOINT`, `CMD ["minio"]`;
- the `name`, `version`, `release`, `summary`, `description` labels.

Differences, each deliberate:

| What | Community image | Here | Why |
|---|---|---|---|
| Base | Red Hat `ubi9/ubi-micro` 9.5 | Rocky `9-ubi-micro` 9.9 | plan 04, "Decisions taken (2026-11-23)" |
| `EXPOSE` | 9000 | 9000 9001 | the console is the reason for the fork (accepted 2026-11-23) |
| `minio`, `mc` | dl.min.io, minisign-verified | built here; mc = aursu/mc at `RELEASE.2025-08-13T08-35-41Z` | dl.min.io is gone |
| `*.minisig` | present | absent | made with MinIO's signing key, which we do not have |
| curl | 8.11.0, `releases/latest` at build time | 8.21.0, pinned version and sha256 | pinned, current; same source (moparisthebest/static-curl) |
| CA bundle | Alpine's | Rocky's (`9.9-minimal`) | plan 04 |
| `vendor`, `maintainer` labels | MinIO Inc | absent | this image is not theirs |
| OCI labels | none | `source`, `licenses`, `version` | provenance |

Checked by diffing `docker image inspect .Config` and `stat` of every path
above between the two images: the only differences are `9001/tcp` and
the missing `.minisig` files.

### Build stages

| Stage | Image (tag + digest) | Gives |
|---|---|---|
| `ca` | `quay.io/rockylinux/rockylinux:9.9-minimal` | the CA bundle |
| `build` | `golang:1.27.4` | `minio` (static, `-tags kqueue -trimpath`, as `make build`), `mc`, curl, checksum files, files with their final modes |
| runtime | `quay.io/rockylinux/rockylinux:9-ubi-micro` (Rocky 9.9) | the image |

- **Version.** `RELEASE=RELEASE.<UTC time>.aursu.N` is split into the
  time (converted to RFC 3339) and the suffix, and handed to
  `buildscripts/gen-ldflags.go` as upstream's hotfix builds did:
  `MINIO_RELEASE=RELEASE MINIO_HOTFIX=aursu.N`. `cmd.Version` gets the
  bare time, `cmd.ReleaseTag` the full tag (plan 04, "Version string").
  Without `RELEASE` the binary is a development build.
- **Commit ID.** gen-ldflags runs `git log`, so the build needs `.git`.
  The root `.dockerignore` excludes it; `Dockerfile.release.dockerignore`
  (BuildKit reads `<Dockerfile>.dockerignore` before the root one) is the
  root list without that line. `.git` reaches the build stage only.
- **Modes are set in the build stage.** `COPY --chmod=0664` onto
  `/licenses/` gave the directory it created mode 0664 as well, and
  `--chmod=0644` on the CA file made `/etc/ssl/certs` 0644. The build
  stage prepares `/out/licenses` and `/out/certs` with the right modes,
  and the runtime stage copies the directories.

## Alternatives rejected

- **A non-root image** (first draft: `USER 1000:1000`, no `chmod -R 777
  /usr/bin`, no curl, no upstream signing key): replaced by the reference
  layout on Alexander's decision, so that existing deployments of the
  community image take this one without changes. The first draft's
  arguments stay true and are recorded here: `chmod -R 777 /usr/bin`
  (upstream `f05641c3c`, `229f04ab7`, September 2024, no reason given)
  leaves every binary world-writable; its one runtime user is the
  server's self-update, which in a container is lost on re-creation.
- **Commit ID without `.git`:**
  - ldflags computed on the host and passed as a build argument:
    gen-ldflags would also stamp the host's `GOPATH`/`GOROOT`, and a local
    `docker build` would differ from CI's;
  - source cloned from GitHub inside the build: an unpushed tree could
    not be built, and the Dockerfile and the code could come from
    different commits;
  - gen-ldflags reading the commit from an environment variable: own code
    for something BuildKit already solves.
- **curl 8.11.0**, the reference's version: identical, but two years of
  curl fixes older; the image's purpose is security upgrades.
- Runtime base `ghcr.io/aursu/rockylinux:*-base` and Rocky 10: plan 04
  "Decisions taken (2026-11-23)".

## Tests (media-gw, 2026-11-23)

Build with `RELEASE=RELEASE.2026-11-23T00-00-00Z.aursu.0`; the first,
uncached build took 1 min 22 s, context 218 MB (`.git` is 137 MB of it
locally; a CI checkout is shallow).

- `minio --version` in the build and in the container:
  `RELEASE.2026-11-23T00-00-00Z.aursu.0 (commit-id=7e5ca3bc…)`; the start-up
  banner shows the same tag and `go1.27.4 linux/amd64`.
- `minio.sha256sum`: `… minio.RELEASE.2026-11-23T00-00-00Z.aursu.0`;
  `mc.sha256sum`: `… mc.RELEASE.2025-08-13T08-35-41Z`.
- curl download checked against the pinned sha256 (`OK`).
- Config and file modes diffed against the reference: see above.
- A named volume on `/data`: `mc ready`, `mc mb`, `mc cp`, `mc cat`, the
  object read back unchanged.
- `mc admin info`: `Version: 2026-11-23T00:00:00Z`; `--json` gives the
  full `commitID` (first draft's run).
- Inside the container: `id` = root; `curl` to
  `/minio/health/live` → 200, console `:9001/` → 200, `https://github.com/`
  → 200 (the CA bundle works for curl); Go's TLS: `mc alias set` against
  `https://s3.amazonaws.com` with invented keys fails on the access key,
  not on the certificate.

Not tested yet: console login through Keycloak (3.1, next).
