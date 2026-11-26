# 35 — CVE-2026-42600: the storage REST ReadMultiple call is removed (plan 04, phase 4 item 6)

Date: 2026-11-26.

## Why

Medium (CVSS 4.0: 6.9); GHSA-xh8f-g2qw-gcm7. Present since
RELEASE.2022-07-24T01-54-52Z, in distributed deployments only.

The internode storage REST API had a `ReadMultiple` call,
`POST /minio/storage/<drive>/v63/rmpl`. Its msgpack body names a bucket, a
prefix and file names, and `xlStorage.ReadMultiple` joined them to the
drive path with `pathJoin`. `pathJoin` resolves `..` and confines nothing,
and the request validity middleware checks only the URL and the form, not
the body. So `../` in the bucket read any file the server process can
read, and returned it in the response. In a container running as root
that is the whole filesystem.

The caller needs the internode token: an HS512 JWT signed with the root
secret key. Every node holds it, and so does anyone with the root
credentials. A compromised node, or a leaked root key, becomes a read of
host files: TLS keys, KMS key material, `/etc/shadow`. Unlike the data,
these do not change when the root credentials are rotated.

**Nothing in MinIO calls `ReadMultiple`.** The last caller,
`readMultipleFiles` in `cmd/erasure-common.go` (used to complete multipart
uploads), was removed upstream on 2024-08-12 (`2e0fd2cba`, #20227). Only
the route, the handler, the REST client method, the `StorageAPI` method
and its wrappers were left: an attack surface with no use.
**Correction (2026-11-26):** this said the last caller went on 2023-10-24
(`c60f54e5b`, #18312), as does the commit message of #64. That commit
removed an earlier caller, the multipart listing; `git log -S` lists every
commit that changes the count, and the last one was taken for the first.
The parallel session found the right one. The conclusions stand.

## What (one commit)

**`fix(storage)`: remove the ReadMultiple storage REST call.**

- **Removed:** the route and the handler (`cmd/storage-rest-server.go`),
  the REST client method, the `StorageAPI` method and its
  implementations (`xlStorage`, `xlStorageDiskIDCheck`, the test
  `naughtyDisk`), the `ReadMultipleReq`/`ReadMultipleResp` types and
  their generated msgp code, and the `ReadMultiple` storage metric.
- **Removed with it:** the HTTP stream helpers that only this handler
  used: `httpStreamResponse`, `streamHTTPResponse`, `waitForHTTPStream`
  and `poolBuf8k`. golangci-lint reports them as unused once the handler
  is gone.
- **Regenerated:** `storage-datatypes_gen.go` and its test with the
  module's `go tool msgp` (the diff only removes code, so the generator
  matches the one that made the file), and `storagemetric_string.go` with
  `stringer` from `golang.org/x/tools` v0.52.0.
- **Regression test** `TestStorageRESTReadMultipleRemoved`
  (`cmd/storage-rest-readmultiple_test.go`). It builds the msgpack body by
  hand, so it does not depend on the removed types, and sends it through
  the authenticated storage REST client to a real server, with the bucket
  pointing at a temporary directory beside the drive. On `master` it
  fails: `bucket "../002" read a file outside the drive`. On this branch
  the call is refused.

**The storage metric numbers shift.** Every metric after `ReadMultiple`
moves down by one. The numbers are array indexes inside one process; the
metrics leave the process by name (`storageMetric(i).String()` in
`DiskMetrics`), so nothing between nodes or in Prometheus depends on them.
The `ReadMultiple` metric itself disappears from the drive metrics; it
was always zero.

**No storage REST version bump.** The version is compared between nodes
to refuse a mix of releases. Removing a call that no release since
2024-08 sends does not need that.

## Public fixes compared (2026-11-26)

Read as a checklist (memory `minio-fork-working-method`); none of their
code is taken.

- **AIStor** (closed source, from the advisory text): removed the call,
  its types, the client wrapper and the route.
- **pgsty/silo:** removed it as well; `38366f654325` then dropped the
  stream helpers it left unused. Same scope as this change.
- **relvo-labs/granary:** kept the call and confined its paths to the
  volume (`314627fd6054`). A later commit (#82, `6dcdcfb57e86`) confines
  the volume and path arguments of **all** grid storage RPCs, see "Not in
  this change".
- **jarvit/minio-image:** no commit touches `cmd/storage-rest-server.go`.

## Alternatives rejected

- **Validate the paths**, as `StatInfoFile` and `ReadVersion` do through
  `getVolDir`, or as granary does. It keeps code that nothing calls, and
  `getVolDir` itself only rejects `""`, `.` and `..`, so it would not have
  stopped `bucket/../..` either.
- **Keep the metric constant** so the numbers do not shift: the numbers
  never leave the process, so there is nothing to keep stable.

## Client-visible change (for the release notes)

- None for S3 clients. A node of an older release that sent `rmpl` would
  get an error, but no release since 2024-08 sends it, and MinIO does not
  run mixed releases.
- The drive metrics no longer list a `ReadMultiple` entry.

## Tests (media-gw, golang:1.27.4, `-tags kqueue,dev`)

- **Unit**, as uid 65534 with `-race`: the regression test, the storage
  REST client tests, the `xlStorage` and disk ID check tests, the msgp
  round trips and the multipart object tests pass. On `master` only the
  regression test fails, as above. `TestXLStorageVerifyFile` panics in
  `internal/ringbuffer` in this container on `master` too (O_DIRECT on
  overlayfs), so it was skipped here; CI runs it.
- **Lint and build:** golangci-lint v2.15.0 reports 0 issues, `go vet` and
  `go build ./...` pass.
- **Live** (scratch `cve42600/e2e.sh` and `cve42600/poc`). Two nodes with
  two drives each. A client on the same network mints the internode token
  from the root credentials and asks node 2 for bucket `../tmp`, file
  `outside`, which holds a line written only to node 2's `/tmp`:

  | image | answer |
  |---|---|
  | `master` | HTTP 200, the response carries "outside the drive root" |
  | this branch | HTTP 426, no such call (the answer MinIO gives to an unknown storage REST path) |

## Not in this change

- **The other storage RPCs.** `getVolDir` rejects only `""`, `.` and
  `..`, and the grid (websocket) storage RPCs bypass the request validity
  middleware. So a volume such as `bucket/../..` or a path with `../`
  reaches the filesystem outside the drive through other calls as well,
  for reads, writes, renames and deletes. Same attacker as this CVE (the
  internode token), no advisory of its own. granary #82 is the checklist
  for it. It is a separate item.
