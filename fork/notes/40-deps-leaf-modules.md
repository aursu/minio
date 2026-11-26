# 40 — plan 04 step 2.4, group A: sixteen leaf modules

Date: 2026-11-26.

## Why

Step 2.4 of `fork/reports/04-plan-final-build.md` moves the remaining
modules to their latest release within the same major version. #39
cleared the four modules `govulncheck` listed as imported but not called
(end of 2.3). The rest had not been surveyed.

Survey on master `cf1c74d3e` with `go list -m -u all` in `golang:1.27.4`:
140 requirements in `go.mod` have a newer release within their major
version, 56 of them direct. They are split into groups by risk, one PR
per group, stacked:

| Group | Modules | Check |
|---|---|---|
| A (this note) | small leaf libraries | unit tests, race, lint |
| C | notification targets: sarama, nats, amqp091, paho, redigo, mysql, lib/pq | a real broker, master vs branch |
| D, D2 | prometheus; etcd 3.5 → 3.7 | metrics endpoints; IAM on etcd |
| E | Azure, GCS and Google API SDKs (tiering) | tiering against emulators |
| F1–F3 | minio-go and madmin-go; minio/pkg (policy engine); sio and kms-go (encryption) | per group, data written by `aursu.1` must still read |
| B | reedsolomon, compress, pgzip, highwayhash, xxh3, msgp | on-disk format, both directions |
| G | go-oidc, go-ldap, aead mtls, pkg/sftp, goftp | after phase 4 item 5 |
| H | indirect modules left over | `go mod tidy` |

B was to wait for phase 4 item 6, because the msgp bump regenerates
`cmd/storage-datatypes_gen.go`, which item 6 changed, and G for item 5,
which owned the LDAP, SFTP and FTP code. Both items were merged while
this group was being prepared (#55–#57, #64), so neither waits any more.
The groups go into the release after `aursu.2`.

## What

`go get` of sixteen modules, then `go mod tidy -compat=1.21` (the
command `make check-gen` runs):

| Module | From | To |
|---|---|---|
| github.com/beevik/ntp | v1.4.3 | v1.6.0 |
| github.com/cheggaaa/pb | v1.0.29 | v1.0.30 |
| github.com/coreos/go-systemd/v22 | v22.6.0 | v22.7.0 |
| github.com/cosnicolaou/pbzip2 | v1.0.5 | v1.0.6 |
| github.com/dustin/go-humanize | v1.0.1 | v1.1.0 |
| github.com/fatih/color | v1.18.0 | v1.19.0 |
| github.com/klauspost/cpuid/v2 | v2.2.10 | v2.4.2 |
| github.com/lithammer/shortuuid/v4 | v4.2.0 | v4.3.0 |
| github.com/miekg/dns | v1.1.72 | v1.1.73 |
| github.com/philhofer/fwd | v1.2.0 | v1.2.1 |
| github.com/pierrec/lz4/v4 | v4.1.22 | v4.1.34 |
| github.com/pkg/xattr | v0.4.10 | v0.4.12 |
| go.uber.org/atomic | v1.11.0 | v1.12.0 |
| go.uber.org/zap | v1.27.1 | v1.28.0 |
| golang.org/x/oauth2 | v0.36.0 | v0.38.0 |
| golang.org/x/time | v0.15.0 | v0.16.0 |

No indirect requirement moved. No module raises the `go` line above the
fork's 1.26.0. No code change was needed.

The three largest diffs were read (non-test sources):

- **pierrec/lz4 v4.1.34** (about 4,000 changed lines). Decodes untrusted
  input in MinIO: snowball upload extraction (`cmd/untar.go`) and S3
  Select over LZ4 objects (`internal/s3select`). Maintained by the
  original authors. Releases v4.1.23–v4.1.29 are retracted (`Reader.WriteTo`
  dropped data after a partial read, fixed in v4.1.30); v4.1.22, the
  version we had, predates that range, and v4.1.34 has the fix. It also
  adds dictionaries, new amd64 and arm64 decoders and xxHash32 kernels.
- **klauspost/cpuid v2.4.2** (about 1,600 lines). Feature detection used
  by `internal/init` and the simdjson check in `internal/s3select`. New
  in 2.3–2.4: CPU topology, RISC-V detection, more x86 feature flags.
- **beevik/ntp v1.6.0** (about 1,700 lines). Used only for
  `ntp.Time(MINIO_NTP_SERVER)` in `internal/bucket/object/lock`, when that
  variable is set. 1.5–1.6 add an experimental NTPv5 mode (off unless
  asked for), kernel receive timestamps on Linux and macOS, and
  dependency updates. `ntp.Time` keeps its NTPv4 behavior.

The other thirteen diffs are small (8 to 480 lines).

## Held back, not taken

Recommended, accepted (Alexander, 2026-11-26):

- **github.com/buger/jsonparser stays at v1.1.2** (latest v1.6.1). It
  parses the claims and the `alg` header of every internal JWT
  (`internal/jwt/parser.go`). v1.2–v1.6 came out within four days in July
  2026: about 2,700 changed lines, mostly a faster parser (word-at-a-time
  string scans, fast skipping) and new API. None of it is a security fix,
  and v1.1.2 has no advisory. A rewritten parser on the token path is a
  risk with nothing to gain.
- **github.com/minio/mux stays at v1.9.2** (latest v1.10.1). The router
  of the whole S3 API. v1.10 is a speed rewrite of route matching:
  per-request memoization of regexp results, literal templates compared
  as strings without the regexp, and one combined context value for
  route, variables and router. Its comments say it was tuned for the
  AIStor route table. No security fix either. A change in which route
  matches changes which handler, and so which permission check, a request
  reaches.

Also left out of this group: `github.com/go-openapi/loads` (v0.23.2 →
v0.25.4). The go-openapi modules move together with the console's
(console note 16), so it goes with the next console alignment, not here.

## Tests (media-gw, golang:1.27.4)

Run on master `5324cbecb` plus this change. The branch was then rebased
onto master `08ae8dd59` (items 5 and 6, no change to `go.mod` or
`go.sum`); there tidy and build were repeated, and this PR's CI runs the
full suites.

- `go mod tidy -compat=1.21` a second time: `go.mod` and `go.sum`
  unchanged. `go build ./...`: ok.
- `go test -race -tags kqueue,dev` on every package outside `cmd` that
  imports a bumped module (`internal/bucket/object/lock`,
  `internal/bucket/lifecycle`, `internal/config/storageclass`,
  `internal/http`, `internal/ioutil`, `internal/jwt`, `internal/s3select`,
  `internal/color`, `internal/config/dns`, `internal/bucket/bandwidth`,
  `internal/config/etcd`, `internal/init`, `internal/dsync`,
  `internal/grid`): ok, no race.
- `go test -tags kqueue,dev ./...` as uid 65534 with
  `MINIO_API_REQUESTS_MAX=10000` (the `make test` setup): 49 packages ok,
  none failed, `cmd` included.
- golangci-lint v2.15.0 (`--build-tags kqueue`, the repository config):
  0 issues.
- govulncheck v1.10.0: no vulnerabilities.
- GitHub advisories (`/advisories?ecosystem=go&affects=<module>@<version>`)
  for each of the sixteen new versions: none. The query was checked on
  `github.com/buger/jsonparser@1.1.1`, which returns GHSA-6g7g-w4f8-9c9x.

The first run of these checks had jsonparser v1.6.1 and mux v1.10.1 in as
well; it passed too. They are held back for the reasons above, not
because a test failed.
