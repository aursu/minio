# 03 — mc for the test scripts: one helper, one pinned release

Plan 04, step 0.2 (CI as the PR gate). Date: 2026-11-19.

## What

`buildscripts/install-mc.sh <mc-destination> [functional-tests-destination]`
builds `mc` from the fork `aursu/mc` at commit `7394ce0d`, which is
`RELEASE.2025-08-13T08-35-41Z`, the last community release. It uses the
release's own ldflags, so `mc --version` reports that release and commit. The
optional second path receives `functional-tests.sh` from the same commit.

The scripts in `buildscripts/` now call the helper instead of getting `mc` in
one of three other ways:

| Before | Scripts |
|---|---|
| `git clone minio/mc` master + `go build` | heal-inconsistent-versions, multipart-quorum-test, resolve-right-versions, rewrite-old-new, test-timeout, verify-build |
| `go install github.com/minio/mc@latest` | minio-upgrade, minio-iam-ldap-upgrade-import-test |
| `wget dl.min.io/client/mc/release/...` | disable-root, verify-healing, verify-healing-empty-erasure-set |

`verify-build.sh` used to fetch `functional-tests.sh` from `mc` master. It now
gets it from the same commit as the binary.

The scripts in `docs/` and `.github/workflows/multipart/` still download from
`dl.min.io`. They move to the helper in a follow-up PR.

## Why

- **`dl.min.io` returns HTTP 410** for every community `mc` and server
  binary (checked 2026-11-19): "These files are no longer served from this
  site."
- **`minio/mc` is archived.** Its `master` (`77f82e18`, 2025-11-06) is
  unreleased code past the last release. Building it gave CI a different `mc`
  from the one the release was tested with.
- **`@latest` and `master` move**, so a PR's result depended on the day it
  ran. A pinned commit makes each bump a reviewed change.

The binary built this way matches `ghcr.io/aursu/minio/mc:RELEASE.2025-08-13T08-35-41Z`.
Both report `RELEASE.2025-08-13T08-35-41Z (commit-id=7394ce0dd2a8…)`.

## Why not the obvious alternatives

- **Copy the binary out of the `ghcr` `mc` image.** It's faster, but amd64
  only, needs Docker inside every script, and is a prebuilt binary we cannot
  patch. Building from source takes about 30 s.
- **`go install github.com/minio/mc@<pseudo-version>` from the Go proxy.** It
  works and the checksum database verifies it. But it builds as
  `DEVELOPMENT.GOGET` without release ldflags, cannot deliver
  `functional-tests.sh`, and cannot take a fix of ours: the module path is
  upstream's.
- **Keep `master`, but from the fork.** That is still unreleased code.

## Tests

- `install-mc.sh` checks:
  - shfmt and shellcheck: clean;
  - a build in `golang:1.24` reports the release version line;
  - a missing destination directory is created;
  - a usage error and an unreachable repository each exit 1 with a message;
  - no temporary directory is left behind.
- The changed scripts: `bash -n` and `shfmt -s -d` over `buildscripts/` are clean.
- Integration: the CI jobs that run these scripts (healing, `test-timeout`,
  multipart, root-disable, upgrade) run on this PR.
