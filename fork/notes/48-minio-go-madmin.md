# 48 — minio-go 7.0.91 → 7.4.1, madmin-go 3.0.109 → 3.0.110

Date: 2026-11-27. Plan 04 step 2.4, group F1 (note 40). Stacked on B2
(note 47), which carries the msgp and xxh3 versions minio-go 7.4.1
requires.

## What

`go get github.com/minio/minio-go/v7@v7.4.1 github.com/minio/madmin-go/v3@v3.0.110`,
then `go mod tidy -compat=1.21`:

| Module | From | To | Why it moves |
|---|---|---|---|
| github.com/minio/minio-go/v7 | v7.0.91 | v7.4.1 | the change |
| github.com/minio/madmin-go/v3 | v3.0.109 | v3.0.110 | the change |
| github.com/minio/crc64nvme | v1.0.1 | v1.1.1 | minio-go |
| github.com/klauspost/crc32 | — | v1.3.0 | minio-go (AVX-512 CRC32) |
| gopkg.in/ini.v1 | v1.67.1 | v1.67.3 | minio-go; `github.com/go-ini/ini` leaves (same package, renamed import path) |
| charmbracelet/x/ansi, x/term; lucasb-eyer/go-colorful; mattn/go-colorable, go-isatty, go-runewidth; clipperhouse/displaywidth, uax29 (new) | | | see below |

minio-go 7.4.1 lists golangci-lint in its go.mod `tool` block, so its
requirements include the linter's whole dependency set. Minimal version
selection raises the terminal libraries in that set (charmbracelet and
friends) for the server too, where console → mc → bubbletea links them
(15 packages in `go list -deps`). The server never draws a terminal UI;
the cross-compile below covers their platform code.

One code change, `cmd/warm-backend-minio.go`, below.

madmin-go 3.0.110 rewrites `ReplDiagInfo` (replication health
diagnostics) and adds fields to `ObjectSummary`. The server uses neither;
nothing else changes.

## What changes for the server

The server uses minio-go as a client (bucket, batch and site replication,
remote tiers, the FTP and SFTP gateways) and its `set`, `tags`, `s3utils`,
`credentials`, `encrypt`, `signer`, `policy` and `replication` packages
internally. `cmd/common-main.go` sets `minio.MaxRetry = 1`, so the
library's retry changes do not reach the server. The requests below were
recorded against a stub S3 endpoint with the server's client options and
call shapes, on both library versions.

- **MinIO tier, objects of one part (up to 128 MiB): the checksum was
  lost; fixed here.** Since v7.0.95 `PutObject` adds
  `X-Amz-Checksum-Algorithm` to the metadata before it chooses between a
  single PUT and multipart; for a single PUT, `putObjectDo` then takes
  that key for a caller-supplied checksum and adds no CRC trailer. 7.0.91
  sent `STREAMING-UNSIGNED-PAYLOAD-TRAILER` with an
  `x-amz-checksum-crc32c` trailer; 7.4.1 sends `UNSIGNED-PAYLOAD`, the
  algorithm header and no value. The tier then neither verifies the
  upload nor stores a CRC32C (measured below). The tier backend now sets
  `Checksum: minio.ChecksumCRC32C`, which takes the other branch in
  `putObjectDo`: the trailer is back, plus the algorithm header, which
  MinIO accepts. Multipart uploads carried a CRC32C per part before and
  after and are unchanged. The FTP and SFTP gateways also set
  `TrailingHeaders`, but upload with an unknown size, which is always
  multipart.
- **Streaming-signed PUTs over http** (replication, batch replication,
  the S3 tier, federation copy) now send `Content-Encoding: aws-chunked`
  (prepended to the object's own encoding) and sign `Content-Type`. A
  MinIO target strips `aws-chunked` before storing
  (`cmd/handler-utils.go`); measured below, replicas keep `gzip`. A proxy
  in front of a target that rewrites `Content-Type` would now break the
  signature. HTTPS targets are unaffected.
- **A cancelled multipart upload is aborted** (7.4.1): the abort runs
  detached from the cancelled context. Reaches the server only through
  multipart tier uploads (over 128 MiB) and FTP/SFTP.
- Benign, by reading the code and the recorded requests: retain-until
  dates now formatted in UTC (the server's already are); `StatObject`
  still returns version ID and delete-marker flag with its error;
  end-to-end checksum verification on `GetObject` and parallel GETs are
  opt-in and the server sets neither; `s3utils` accepts and rejects the
  same bucket names (only error texts changed); `set.StringSet` is now
  `Set[string]` with msgp methods, but no `_gen.go` type or gob payload
  of the server contains one, and its JSON is the same; `tags` only gains
  JSON methods; `encrypt` uses `encoding/json` instead of goccy for the
  KMS context; the replication config XML types are unchanged; the
  `credentials` providers the server uses (static) are unchanged; RDMA
  code is behind the `rdma` build tag and not built.

A trap for future code: `PutObjectOptions.validate` now rejects
`x-amz-checksum-*` metadata keys on a client without `TrailingHeaders`.
Replication passes such keys (`getCRCMeta`) on a client without trailing
headers; it works because replication uses `Core.PutObject`, which skips
the validation. Moving it to `Client.PutObject` would fail every object
stored with a checksum.

## Tests (media-gw, golang:1.27.4)

Unit, on this branch:

- `go mod tidy -compat=1.21` a second time: no change. `go generate`: no
  change. `go build ./...`: ok. Cross-compile, all fifteen targets: ok.
- golangci-lint v2.15.0: 0 issues. govulncheck v1.10.0: no
  vulnerabilities. GitHub advisories for every new module version
  (indirect included): none.
- `go test -race` for `./internal/...` and for the `cmd` tests matching
  replication, batch, tier, warm backend, transition, site replication,
  target, tag, policy, lifecycle, encryption, SSE, copy, multipart, post
  policy and bucket (without `TestReplicationResync`, note 47): ok.
  `go test -tags kqueue,dev ./...` as uid 65534: 49 packages ok.

Live, two single-node deployments (four drives each), `sa` and `sb`,
each running the `aursu.4` image's binary or this branch's, in all four
combinations; SSE-S3 through a static KMS key. Requests are signed by a
small SigV4 client, so headers are compared as the server returns them.

- **Bucket replication** `sa` → `sb`, versioned bucket and object-lock
  bucket: three versions with content type, `Content-Encoding: gzip`,
  cache control, disposition, user metadata (one UTF-8 value) and tags;
  a 40 MiB multipart object; objects uploaded with CRC64NVME, SHA256 and
  (multipart) CRC32C and CRC64NVME checksums; SSE-S3 single and
  multipart; a delete marker; a permanently deleted version; governance
  retention and legal hold. Then tags changed and removed, retention
  extended, legal holds set and cleared. Every combination: all versions
  `COMPLETED`, version listings (IDs, delete markers) identical, every
  version's body identical, and the same three header differences
  between source and replica as aursu.4 → aursu.4 (below).
- **Tiering** on `sa` to `sb`, MinIO and S3 tier types, ILM transition:
  1 KiB and 12 MiB objects transition, read whole and in ranges, restore
  and read again; after `sa` is restarted on the other binary, read
  again. All ok in every combination.
- **Batch replication** `sa/rep/plain/` → `sb/batch`: all versions,
  bodies and headers identical in every combination.
- **Site replication** aursu.4 ↔ branch, both ways round: a bucket made
  on each site, a user made on one and a policy made on the other and
  attached there; buckets, users and policies in sync on both
  (`mc admin replicate status`: 2/2, 6/6, 1/1), objects replicated
  with identical headers and bodies, the user reads through the policy
  from the other site.
- **MinIO tier checksum**, a 1 KiB, a 12 MiB and a 200 MiB object, what
  the tier stores:

| Source → tier | 1 KiB | 12 MiB | 200 MiB |
|---|---|---|---|
| aursu.4 → aursu.4 | CRC32C, full object | CRC32C, full object | CRC32C, composite |
| this branch without the fix → aursu.4 | none | none | CRC32C, composite |
| this branch → aursu.4 | CRC32C, full object | CRC32C, full object | CRC32C, composite |
| this branch → this branch | CRC32C, full object | CRC32C, full object | CRC32C, composite |

Source/replica differences that aursu.4 → aursu.4 shows as well, and so
are not from this change: the 40 MiB multipart object is replicated as a
single PUT (plain ETag on the replica); replicas of objects uploaded
without a checksum carry a CRC64NVME one; and in the object-lock bucket,
the object whose retention was extended and legal hold set ends with the
legal hold but without retention mode and date on the replica. The last
one is not investigated here.
