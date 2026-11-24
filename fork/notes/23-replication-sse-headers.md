# 23 — CVE-2026-34204: SSE metadata injected through replication headers (plan 04, phase 4 item 3)

Date: 2026-11-24.

## Why

High; GHSA-3rh2-v3gr-35p9. Introduced by upstream `468a9fae8` (2024-03,
"Enable replication of SSE-C objects").

`extractMetadataFromMime` turned the `X-Minio-Replication-*` headers into
internal object metadata for every request:

- the sealed key, seal algorithm and IV;
- `Encrypted-Multipart`, `Actual-Object-Size` and the SSE-C CRC.

It did not check that the request was replication. Any caller with
`s3:PutObject` could therefore store an object that the server treats as
SSE-encrypted with a key that does not exist. From then on GetObject and
HeadObject refuse the object for everyone, so this is a targeted denial
of service.

`completeMultipartOpts` had the same problem. It took `Actual-Object-Size`
from any request with `X-Minio-Source-Replication-Request`, and the SSE-C
CRC from any request at all.

The advisory's only fix is in AIStor, which is closed source and not
used here.

## Public fixes compared (2026-11-24)

The columns are the paths that write metadata.

| Source | PutObject | NewMultipart | CompleteMultipart (`Actual-Object-Size`, SSE-C CRC) | CopyObject REPLACE | POST policy, Extract | Tests |
|---|---|---|---|---|---|---|
| pgsty/silo `56fa63bfd155` (2026-04-15) | mapped only after the `ReplicateObject` check | adds the `ReplicateObject` check for REPLICA status | **not covered** | mapped for authorized REPLICA copies; strips replication headers from the opts request | not mapped; changes the Extract handler that #45 rewrote, so it conflicts | handler-utils and object-handlers units |
| jarvit/minio-image `0004` (2026-10-29) | mapped only from headers of a request with the source flag; any request that calls itself replication needs `ReplicateObject` | same | **not covered** | permission check only | not mapped | one test: forged PUT, forged replica 403, a legitimate replica |
| relvo-labs/granary `be6063e8819f` (2026-11-21) | mapped only for REPLICA status **and** the source flag, after the existing `ReplicateObject` check | mapped only for an authorized REPLICA request | **only for an authorized replication request** | never mapped | never mapped (the headers leave `supportedHeaders`) | +412 lines: every path, single-drive and erasure, plus authorized replicator and root, which must keep the metadata |

pgsty later changed this code again on top of its own replication-trust
layer: `938603458d7a` "harden CORS and replication request trust" and
`4fcdf37ce656` "preserve normalized replica metadata". Neither applies
here without that layer.

**Taken: granary `be6063e8819f`** (recommended, accepted). Reasons:

- It is the only fix that also closes CompleteMultipart.
- Its tests prove that the real replication path keeps its SSE metadata.
- It applies to our tree: the only conflict was the import block (granary
  renamed its module).

Copy never maps the headers. That costs nothing, because server-side
metadata replication (`replicateAll`, branch `rAction != replicateAll`) goes
through CopyObject with `getCopyObjMetadata`, which skips the reserved
keys and sends no replication SSE headers.

## What

`git cherry-pick -x be6063e8819f`, author kept. The only adaptations are
in the import paths:

- `github.com/relvo-labs/granary/internal/…` becomes
  `github.com/minio/minio/internal/…`, in `handler-utils.go` and in the
  test file;
- gofmt reorders the test file's imports after that rename.

Behaviour after the change:

- **Ignored.** A non-replication request that carries
  `X-Minio-Replication-*` SSE headers is still stored, but the headers
  are dropped. The answer stays 200 and the object stays readable.
- **SSE metadata on a replica.** Only a REPLICA PutObject or
  NewMultipartUpload that carries `X-Minio-Source-Replication-Request`
  and passes `s3:ReplicateObject` gets the SSE metadata.
- **CompleteMultipartUpload.** `Actual-Object-Size` and the SSE-C CRC are
  taken only from an authorized replication request.

## Tests (media-gw, golang:1.27.4, `-tags kqueue,dev`)

- **Granary's tests in `cmd/replication-sse-headers_test.go`**, 5 tests
  with 26 subtests across single-drive and erasure:
  - on master with only the test file added, 5/5 tests fail and 16
    subtests fail, every unauthorized case. The 10 authorized subtests
    pass, which shows that the existing mapping keeps working;
  - on the branch, all 31 test and subtest results pass, under `-race`
    too.
- **Wider tests:** `go test -race -run
  'SSEReplication|Replicat|ExtractMetadata|Multipart|CopyObject|PostPolicy|PutObject'
  ./cmd/` passes. `go build ./...` and `go vet ./cmd/` are clean, and
  `gofmt -l` lists nothing.
- **Live check** (scratch script, not committed). Two servers, `curl
  --aws-sigv4` as a user with only `s3:PutObject` and `s3:GetObject`, and
  random sealed key and IV:

  | | release `aursu.1` | this branch |
  |---|---|---|
  | forged PUT, without and with the source flag | 200, 200 | 200, 200 |
  | read back as root | both unreadable: "stored using a form of Server Side Encryption" | both read back |
  | bucket replication, 6-byte and 20 MiB multipart object | COMPLETED → REPLICA, md5 equal | COMPLETED → REPLICA, md5 equal |

## Not in this change

- **REPLICA status on NewMultipartUpload is still not checked.** A caller
  without `s3:ReplicateObject` can still start a multipart upload with
  `X-Amz-Bucket-Replication-Status: REPLICA`. With this fix the SSE
  headers are ignored, but the object is still marked as a replica.
  PutObject checks this; the multipart path never did, and that is
  pre-existing upstream. pgsty's and jarvit's fixes add the check. It is
  outside this advisory (status, not encryption metadata), so it can be a
  separate change.
- SSE-C replication end to end is not covered by the live check, because
  SSE-C needs TLS between the sites. The unit tests cover the authorized
  mapping.
