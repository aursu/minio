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

**Reference: the community image of the last releases with the full
console.** Console v1.7.6 shipped up to `RELEASE.2025-04-22T22-12-26Z`;
that image is no longer published anywhere (ghcr fork copy, quay.io,
Docker Hub checked 2026-11-23). Between `RELEASE.2025-03-12T18-04-18Z`
and it, upstream's `Dockerfile.release` changed only the build stage's Go
image (`2b34e5b9a`), so the fork's copy of 2025-03-12
(`ghcr.io/aursu/minio/minio`, base of the plain `Dockerfile`) has the same
runtime layout and is used as the reference.

Alexander's rule (2026-11-23): the security decisions below stand; every
other question about the image is answered the way the reference answers
it. Taken from it unchanged:

- runs as root (no `USER`), `WorkingDir /`;
- `EXPOSE 9000` (the console port is set by `--console-address`);
- `ENV`, less the minisign key;
- `/usr/bin/docker-entrypoint.sh` 0775, unchanged script;
- the CA bundle at `/etc/ssl/certs/ca-certificates.crt` 0644;
- `/licenses/{CREDITS,LICENSE}` 0664 in a 0755 directory;
- `/usr/bin/{minio,mc}.sha256sum` in the same format,
  `<sha256> <binary>.<release tag>`;
- `VOLUME /data`, `ENTRYPOINT`, `CMD ["minio"]`;
- the `name`, `version`, `release`, `summary`, `description` labels.

Removed, for security:

- `chmod -R 777 /usr/bin`: added upstream in September 2024
  (`f05641c3c` as `1777`, then `229f04ab7` "fixes minio/operator#2319");
  the commits give no reason. The one runtime writer of that directory is
  the server's self-update (`mc admin update`: `selfupdate` replaces
  `os.Executable()`, downloaded from dl.min.io unless a URL is given). In
  a container that replacement lives in the container layer and is lost
  on re-creation; the price was every binary in the image
  world-writable. `/usr/bin` keeps the base image's mode (0555).
- `MINIO_UPDATE_MINISIGN_PUBKEY` and the `*.minisig` files: MinIO's
  signing key; these binaries are not signed with it.
- The static curl: downloaded unpinned (`releases/latest`) from a
  third-party repository. A healthcheck can run `mc ready local` inside
  the container (tested below).
- `vendor`/`maintainer` labels naming MinIO, Inc.: this image is not
  theirs. OCI `source`, `licenses`, `version` labels added.

Other differences: base Rocky `9-ubi-micro` 9.9 instead of Red Hat
`ubi9/ubi-micro` 9.5, Rocky's CA bundle instead of Alpine's (plan 04,
"Decisions taken (2026-11-23)"); `mc` is aursu/mc
`RELEASE.2025-08-13T08-35-41Z`.

Checked by diffing `docker image inspect .Config` and `stat` of the
image's paths against the reference: the only differences are the ones
removed above.

### Build stages

| Stage | Image (tag + digest) | Gives |
|---|---|---|
| `ca` | `quay.io/rockylinux/rockylinux:9.9-minimal` | the CA bundle |
| `build` | `golang:1.27.4` | `minio` (static, `-tags kqueue -trimpath`, as `make build`), `mc`, checksum files, files with their final modes |
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

- **A non-root image** (first draft: `USER 1000:1000`, `/data` owned by
  it, `EXPOSE 9000 9001`): not the reference's answer. Existing
  deployments of the community image keep working unchanged.
- **The reference copied in full** (second draft: also `chmod -R 777
  /usr/bin`, the minisign key, a static curl pinned to 8.21.0): the
  security decisions above take precedence.
- **Commit ID without `.git`:**
  - ldflags computed on the host and passed as a build argument:
    gen-ldflags would also stamp the host's `GOPATH`/`GOROOT`, and a local
    `docker build` would differ from CI's;
  - source cloned from GitHub inside the build: an unpushed tree could
    not be built, and the Dockerfile and the code could come from
    different commits;
  - gen-ldflags reading the commit from an environment variable: own code
    for something BuildKit already solves.
- Runtime base `ghcr.io/aursu/rockylinux:*-base` and Rocky 10: plan 04
  "Decisions taken (2026-11-23)".

## Tests (media-gw, 2026-11-23)

Build with `RELEASE=RELEASE.2026-11-23T00-00-00Z.aursu.0`; the first,
uncached build took 1 min 22 s, context 218 MB (`.git` is 137 MB of it
locally; a CI checkout is shallow).

- `minio --version` in the build and in the container:
  `RELEASE.2026-11-23T00-00-00Z.aursu.0 (commit-id=…)`; the start-up
  banner shows the same tag and `go1.27.4 linux/amd64`.
- `minio.sha256sum`: `… minio.RELEASE.2026-11-23T00-00-00Z.aursu.0`;
  `mc.sha256sum`: `… mc.RELEASE.2025-08-13T08-35-41Z`.
- Config and file modes diffed against the reference: see above.
- A named volume on `/data`: `mc ready`, `mc mb`, `mc cp`, `mc cat`, the
  object read back unchanged.
- `mc admin info`: `Version: 2026-11-23T00:00:00Z`; `--json` gives the
  full `commitID`.
- Console `:9001/` → `200 OK`.
- Inside the container: `id` = root; `mc ready local` → ready, exit 0.
- Go's TLS with the Rocky bundle: `mc alias set` against
  `https://s3.amazonaws.com` with invented keys fails on the access key,
  not on the certificate.

Not tested yet: console login through Keycloak (3.1, next).
