# 06 — Release RELEASE.2026-11-24T06-24-34Z.aursu.1

Date: 2026-11-24. The fork's first release, plan 04 step 3.2.

| | |
|---|---|
| Tag | `RELEASE.2026-11-24T06-24-34Z.aursu.1` (annotated) on `master` `37fa04ea4` (#44's merge) |
| Image | `ghcr.io/aursu/minio:RELEASE.2026-11-24T06-24-34Z.aursu.1`, also `:latest` |
| Digest | `sha256:9fadef64e45df30ee55465cfd3877ede338d59c12fe1d75828202fe669dae6ef` (both tags) |
| Package | public: both tags pull anonymously |
| `minio --version` | `RELEASE.2026-11-24T06-24-34Z.aursu.1 (commit-id=37fa04ea4e9d…)`, `go1.27.4 linux/amd64` |
| Workflow | "Release image" run `45340238924`: build, version stamp, console login through Keycloak, publish; all green, 5 min |
| Upstream base | `minio/minio` `master` `7aac2a2c5` (2026-02-12), the last upstream commit |
| Console | `github.com/aursu/console` `7832665c91d3` (via `replace`, note 12) |

## What is in it

Against the upstream base: #1–#46 (154 commits). In groups:

- **Security fixes (phase 4).**
  - CVE-2026-33322, the OIDC JWT algorithm confusion. Fixed by #43, a cherry-pick of relvo-labs/granary (note 19).
  - CVE-2026-41145 and CVE-2026-40344, uploads of unsigned-trailer objects without a signature. Fixed by #44, a cherry-pick of relvo-labs/granary (note 20).
- **Fixes found on the way.**
  - #45 removes a data race in the Snowball extract: parallel entries share request state, and a denied entry wrote a second response (note 21).
  - #46 updates `github.com/gobwas/ws` to 1.5.0 for GO-2026-6940 (note 22).
- **Toolchain and dependencies (phase 2).**
  - Go 1.27 (#24).
  - The x/ family, compress, grpc/otel, prometheus, amqp091-go and paho, and the modules govulncheck listed as not called (#24–#39; notes 05–08, 10, 13, 14, 16).
  - The full console from `aursu/console` (#33).
- **Image and release (phase 3).**
  - `Dockerfile.release` with the Keycloak SSO smoke test (#41, note 17).
  - The release workflow (#42, note 18).
- **CI repair (phase 1).**
  - Pinned tools, upgrade and resiliency fixes, job timeouts (#2–#23, #29, #32, #37; report 05).

## Vulnerability state at the tag

| Check | Result |
|---|---|
| `govulncheck` v1.10.0, `-show verbose ./...`, Go 1.27.4, 253 modules | **No vulnerabilities found** (none called, imported or required) |
| UI: `yarn npm audit --recursive --environment production --no-deprecations` on `aursu/console` `7832665c91d3` (Node 26.13.1, corepack 0.36.0) | **No audit suggestions** |
| OSV, `github.com/minio/minio`, published since 2025-10-15 | the same 8 advisories as report 03, **none new** |

The 8 OSV advisories against this release:

| CVE | State |
|---|---|
| CVE-2025-62506 | fixed upstream, in the base |
| CVE-2026-33322 | **fixed** (#43) |
| CVE-2026-40344, CVE-2026-41145 | **fixed** (#44) |
| CVE-2026-34204 | open: phase 4 item 3 (SSE metadata through replication headers) |
| CVE-2026-39414 | open: item 4 (S3 Select CSV, unbounded line) |
| CVE-2026-33419 | open: item 5 (LDAP STS enumeration and rate limit) |
| CVE-2026-42600 | open: item 6 (`ReadMultiple`, distributed mode only, needs the root JWT) |

OSV still lists the fixed ones as affecting `github.com/minio/minio`. The
advisories describe upstream, not this fork.

## Check of the published image

Run on media-gw, the image pulled from ghcr. The client sends only the
access key, and every signature is zeros (`e2e.sh`, the same check as in
note 20):

| Request | `aursu.0` test image (master before #44) | this release |
|---|---|---|
| CVE-2026-41145: PutObject with a query-string credential only | 200, object written | 400 InvalidRequest |
| CVE-2026-40344: Snowball extract with a forged `Authorization` | 200, object written | 403 SignatureDoesNotMatch |
| Objects in the bucket afterwards | 2 | none |
| Control: signed upload and read-back with `mc` | ok | ok |

## Next

Phase 4 items 3–6 by the same method: compare the public fixes, cherry-pick
the best one, and then make the next release, `….aursu.2`.
