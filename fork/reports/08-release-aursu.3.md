# 08 — Release RELEASE.2026-11-26T12-18-43Z.aursu.3

Date: 2026-11-26. Plan 04 phase 4, item 7.

| | |
|---|---|
| Tag | `RELEASE.2026-11-26T12-18-43Z.aursu.3` (annotated) on `master` `26bedbe33` (#67's merge) |
| Image | `ghcr.io/aursu/minio:RELEASE.2026-11-26T12-18-43Z.aursu.3`, also `:latest` |
| Digest | `sha256:a97428abea4b81350c215ba6d598165721eac0f0aa0715cbdf000510e2ef330a` (both tags) |
| Package | public: the manifest is served to an anonymous token |
| `minio --version` | `RELEASE.2026-11-26T12-18-43Z.aursu.3 (commit-id=26bedbe33f90…)`, `go1.27.4 linux/amd64` |
| Workflow | "Release image" run `45818520994`: build, version stamp, console login through Keycloak, publish; all green |
| Since `aursu.2` | #65 (the `aursu.2` report) and #67 |
| Dependencies | `go.mod` and `go.sum` unchanged since `aursu.1`; console still `aursu/console` `7832665c91d3` |

## What is in it

**Storage RPC volumes and paths are confined to the drive** (#67, note 36).
This is the follow-up of CVE-2026-42600, with no advisory of its own.

- **The flaw.** The request validity middleware never checked what the storage calls read from a request body:
  - the grid calls take volumes and paths from the msgpack body;
  - several HTTP storage calls take paths from the body.

  `xlStorage` joined them to the drive path, resolving `..` and confining nothing, and the volume `/` named the drive root.
- **Who could exploit it.** A holder of the internode token: every node, and anyone with the root credentials.
- **What it allowed.** Reading, writing, renaming and deleting files outside the drive, creating directories there, and acting on the whole drive at once.
- **The fix.** `xlStorage` now refuses any volume or path with a `.` or `..` segment, and any volume made of separators only. This is the middleware's own rule, so nothing a client can name is refused.

The review of #67 found the drive-root volume and corrected the account of which HTTP calls were exposed (note 36).

## Changes that clients and operators see

None. Only the storage RPCs between nodes are affected, and no release sends the refused names.

## Vulnerability state at the tag

| Check | Result |
|---|---|
| `govulncheck` v1.10.0, `./...`, Go 1.27.4, on `26bedbe33` | **No vulnerabilities found** |
| UI audit | not repeated: the console is the commit audited for `aursu.2` today, **no audit suggestions** |
| OSV, `github.com/minio/minio`, published since 2025-10-15 | the same 8 CVEs, **all fixed** (report 07) |

## Check of the published image

Run on media-gw with the image pulled from ghcr (scratch `item7/e2e.sh`). Two nodes; their drives sit on one tmpfs at `/mnt`. A client mints the internode token from the root credentials and sends `DeleteBulk` to node 2 through its real router and middlewares. The volume goes in the query, the paths in the body.

| request | `aursu.2` | `aursu.3` |
|---|---|---|
| volume `.minio.sys`, path `../../victim` | `/mnt/victim` deleted | `file access denied`, still there |
| volume `/`, path `rootplant` | `/mnt/data1/rootplant` deleted | `volume access denied`, still there |

## CI on the way

- **#67: "Upgrade from RELEASE.2019-12-19" failed once.** Anonymous reads alternated between 200 and 403, because one node booted before quorum. *(Corrected 2026-11-27: the cause is a race in the migration of 2019 bucket configs; one node keeps the bucket without its policy. Note 38.)* This is the fourth time this has happened. The rerun passed, and the flake is recorded in the fork's CI notes.

## Next

- **Plan 04 step 2.4 (dependencies within their majors)** is in progress. Group A is minio #66.
- **Not done:**
  - malformed `FileInfo` from a peer (erasure geometry, part sizes), note 36;
  - the items offered earlier, report 07.
