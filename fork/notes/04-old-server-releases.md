# 04 — Old server releases for upgrade tests, built from source

Plan 04, step 0.2 (CI as the PR gate). Date: 2026-11-19.

## What

`buildscripts/install-minio-release.sh <release-tag> <commit> <destination>`
builds an old MinIO server release from the fork `aursu/minio` at the given
commit, with the release's version flags. The tag and commit are pinned at the
call site, because each test wants its own old version.

| Test | Old release | Commit | Was |
|---|---|---|---|
| `minio-upgrade.sh` (`make test-upgrade`) | `RELEASE.2019-12-19T22-52-26Z` | `586614c7` | image `minio/minio:<tag>` from Docker Hub |

*(Updated 2026-11-27: `minio-upgrade.sh` now defaults to `RELEASE.2025-04-22T22-12-26Z`, and CI no longer runs the 2019 upgrade. Note 38.)*
| `minio-iam-ldap-upgrade-import-test.sh` | `RELEASE.2024-03-26T22-10-45Z` | `8bce123b` | binary from `dl.min.io` |
| `rewrite-old-new.sh` (`verify-healing-with-rewrite`) | `RELEASE.2020-10-28T08-16-50Z` | `f53c5a02` | binary from `dl.min.io` |

`minio-upgrade.sh` wraps the 2019 binary in an image. It builds the image from
the repo `Dockerfile`, tagged `minio/minio:<tag>`, so `compose.yml` stays
unchanged.

On 2026-11-19, 523 upstream tags were pushed to `aursu/minio`. Before that, the
fork held only three tags.

## Why

The old releases can no longer be obtained in any other way:

- **Docker Hub:** "pull access denied", the repository no longer exists.
- **Quay:** HTTP 401.
- **`dl.min.io`:** HTTP 410.

Building from source is original code at a pinned commit, the same approach as
`install-mc.sh` in note 03.

`rewrite-old-new.sh` downloaded with `curl --silent -O` and no `-f`, so it
stored the 410 page as `minio.RELEASE.2020-…` and made it executable. The
healing job that runs it was green on PR #10. That green result proved nothing:
the old server it claims to start was never there.

## Findings while building

- **All three build with Go 1.24.** The 2019 release declares Go 1.13.
- **2019 reports `DEVELOPMENT.GOGET`.** Its `cmd/build-constants.go`
  initialises the version variables from another variable
  (`Version = goGetTag`). Go's linker applies `-X` only to variables
  initialised with a string literal (or not at all); for these, package init
  overwrites the value at run time. No 2019 code path branches on the build
  version: `git grep` finds only policy and format `Version` fields. So the
  stamp is cosmetic there.
- **2020 and 2024 report their release tag**, as intended.
- **The 2019 image works.** Built the way the test builds it (repo
  `Dockerfile` on the pinned `ghcr` base), it starts and serves `server /data`.

## Why not the obvious alternatives

- **Disable the upgrade tests.** They are the only check that data written by
  old releases is still readable by our build. That check matters most for a
  fork that will change the server.
- **Store old binaries in `ghcr` or in the repo.** Opaque binaries with no
  build provenance, and storage to maintain. A source build is reproducible
  from a pinned commit.
- **Extend `install-mc.sh`.** Different module, repository and ldflags
  package. A shared script would carry a mode switch for no gain.

## Tests

- `install-minio-release.sh`:
  - shfmt and shellcheck: clean;
  - builds 2019, 2020 and 2024 in `golang:1.24` and reports their versions (2019 as noted above);
  - a malformed tag or commit exits 1 with a message.
- 2019 image: built as `build_old_release_image` does it, and the container
  starts.
- The three scripts: `bash -n` and `shfmt -s -d` are clean.
- **Integration:** this PR's CI runs `test-upgrade`, `test-iam-ldap-upgrade-import`
  and `verify-healing-with-rewrite`.
