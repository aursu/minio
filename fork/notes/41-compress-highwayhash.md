# 41 — klauspost/compress 1.20.2 → 1.20.5, minio/highwayhash 1.0.3 → 1.0.4

Date: 2026-11-26. Plan 04 step 2.4, the first part of group B (note 40).

## Why now, and why on its own

Group C1 (Kafka and NATS notification targets) wants sarama 1.62.0 and
nats-server 2.15.3. Both require klauspost/compress 1.20.4, and every
recent nats-server requires minio/highwayhash 1.0.4. nats-server is only
used by tests, but minimal version selection applies to the whole
module, so the server binary would get both. compress is the library
MinIO uses for its on-disk object compression, so it gets its own PR and
its own checks, below C1 in the stack. It goes to the latest patch,
1.20.5. The alternative was to stay on sarama 1.61.1 and nats-server
2.15.1, which keep compress at 1.20.2, and leave compress to the rest of
group B. Recommended, accepted (Alexander, 2026-11-26): this PR first,
then C1 on the latest versions.

## Where MinIO uses these libraries

| Use | Package | Data |
|---|---|---|
| Object compression (`MINIO_COMPRESSION_ENABLE`), `cmd/object-api-utils.go` | `s2`; on amd64 with `WriterBetterCompression` (assembly encoder), with padding for encrypted objects | on disk; for parts over 8 MiB the index is stored via `RemoveIndexHeaders`, read back via `RestoreIndexHeaders`, `Index.Load`, `Index.Find` for range reads |
| Event queue store (`queue_dir`), `internal/store` | `s2` | on disk until delivered |
| Data usage cache, `cmd/data-usage-cache.go` | `zstd` | on disk, rewritten |
| Listing cache, `cmd/metacache-stream.go` | `s2` | on disk, temporary |
| Snowball auto-extract (`cmd/untar.go`) and S3 Select over compressed input (`internal/s3select`) | `zstd` and `s2` directly; `.gz` through klauspost/pgzip, which decodes with this module's DEFLATE package; `.lz4` is pierrec/lz4, not this module | untrusted input |
| IAM and bucket metadata export and import, cluster inspect, `fmt-gen` | `zip` (writers and readers) | admin input and output |
| Binary update between nodes, `cmd/update.go`, `cmd/peer-rest-server.go` | `zstd` | in transit |
| Admin and KMS API responses (`gzhttp`, `MinSize(1000)`, `BestSpeed`); call home and log rotation (`gzip` writer); the trace response recorder (`gzip` reader) | `gzhttp`, `gzip` | in transit |
| Bitrot hash of every shard (`cmd/bitrot.go`); also a log entry hash (`internal/logger`) and the Elasticsearch target's document IDs | highwayhash | on disk; in transit |

Not this module: the `x-minio-extract` zip listing (`cmd/s3-zip-handlers.go`)
uses minio/zipindex.

## What changes

compress: three releases (2026-10-12, 11-09, 11-20), about 60 pull
requests.

- Speed work, mostly in zstd and huff0: arm64 assembly, faster pure-Go
  decoders, aligned loops, cheaper table setup. Not only arm64: the s2
  amd64 assembly encoder (`s2/encodeblock_amd64.s`, about 600 changed
  lines) was regenerated, and `s2/writer.go` and `s2/reader.go` changed.
  MinIO compresses objects on amd64 with exactly that encoder, so the
  release image (amd64) runs the new code on every compressed write.
- Hardening against malformed input: zstd `DecodeAll` no longer
  preallocates more than the input can decode to; s2 verifies the CRC of
  skipped chunks, keeps `ErrCorrupt` on oversized or truncated chunks,
  checks stream bounds and no longer reads past the source when
  converting LZ4; zip takes the standard library's Zip64 fixes and honors
  `zipinsecurepath`; the DEFLATE and gzip packages fix checkpoints, short writes and EOF
  handling.
- `gzhttp`: reads `Accept-Encoding` across several header lines, no
  longer compresses a 206 without `Content-Range`, keeps
  `Vary: Accept-Encoding` when the handler sets `Vary`, flushes and
  hijacks through `http.ResponseController`, keeps the status chosen by
  buffered writes. A new `AcceptWildcards` option lets `*` select gzip or
  zstd; it is off by default and MinIO does not set it, so `*` still gets
  an uncompressed answer.
- s2 index: `RemoveIndexHeaders`, which MinIO calls when it stores an
  index, now rejects a short index chunk; `WriterAddIndex` no longer
  writes an index for an empty stream (MinIO builds one only for more
  than 8 MiB); `Index.LoadStream` (not used by MinIO) checks bounds.
  `RestoreIndexHeaders`, `Index.Load` and `Index.Find`, the read side
  MinIO uses for range reads, are byte-identical in both versions.
- zstd now documents that encoder output is stable only within one
  version; the format stays compatible.

No advisory names 1.20.2 or 1.20.5.

highwayhash 1.0.4: the hashing code is unchanged. The `digest` and
`digest64` types are exported as `Digest` and `Digest64`, with
constructors that return them, and a comment typo is fixed.

## Tests (media-gw, golang:1.27.4)

Unit, on this branch (base `deps/leaf-modules`):

- `go mod tidy -compat=1.21` a second time: no change. `go build ./...`:
  ok. golangci-lint v2.15.0: 0 issues. govulncheck v1.10.0: no
  vulnerabilities.
- `go test -tags kqueue,dev ./...` as uid 65534 (the `make test` setup):
  49 packages ok, none failed.
- `go test -race` for `internal/s3select`, `internal/store`,
  `internal/http` and `internal/logger`: ok. For `cmd`, the tests whose
  names match compression, S2, index, range, untar, snowball, metacache,
  data usage and highwayhash: ok, on this branch and on its base alike.
- `TestAllBitrotAlgorithms` is not in that race run. Run on its own, it
  panics (`integer divide by zero` in `internal/ringbuffer`) on the base as
  well as on this branch: `newStreamingBitrotWriter` takes its buffer from
  `globalBytePoolCap`, which only erasure pool setup fills, so a test run
  without one gets an empty ring buffer. In the full package run an
  earlier test sets the pool up and it passes (above). A test isolation
  defect that predates this change; CI runs the whole package with
  `-race`.

Live, cross-version: the `aursu.2` image (compress 1.20.2, highwayhash
1.0.3) and this branch built from source (`go version -m` shows compress
1.20.5, highwayhash 1.0.4), run one after the other on the same four
drives (one erasure set). Configuration: compression on for `.txt`,
`MINIO_COMPRESSION_ALLOW_ENCRYPTION=on`, SSE-S3 with a static KMS key, a
webhook target with a queue directory.

- Objects: 0 B, 1 KiB, 100 KiB, 7 MiB (no index), 9 MiB in one PUT
  (index), 40 MiB multipart (16 + 16 + 8 MiB parts: two indexed), 30 MiB
  of one repeated line, 20 MiB random (incompressible), and a `.bin` file
  that is not compressed. Each is read whole (sha256), from 11 offsets
  (4 KiB each, including the 8 and 16 MiB boundaries) and its last 1000
  bytes: 80 checks per set. On disk (one drive), the 30 MiB object takes
  1435 bytes in the plain bucket and 2111 in the SSE-S3 one, so the
  encrypted objects are compressed too.
- Snowball auto-extract of one tar (1 MiB and 9 MiB `.txt`, 2 MiB `.bin`,
  a file in a subdirectory) compressed four ways: `.gz` (pgzip, so this
  module's DEFLATE package), `.zst`, `.s2` (made by `s2c` v1.20.2), `.lz4`. 16
  checks per version.
- IAM export (two users with policies) and bucket metadata export (tags,
  a lifecycle rule, SSE configuration), both zip files, then cleared and
  imported by the other version.

| Phase | Server | Check | Result |
|---|---|---|---|
| 1 | aursu.2 | writes set 1 into a plain and an SSE-S3 bucket; 10 events queued while the webhook is down | written |
| 1 | aursu.2 | reads set 1: plain, SSE-S3 | 80/80; 78/80, see below |
| 1 | aursu.2 | snowball extract | 16/16 |
| 1 | aursu.2 | IAM and bucket metadata export | two zip files |
| 2 | branch | reads set 1: plain, SSE-S3 | 80/80; 78/80, see below |
| 2 | branch | delivers the queue written by aursu.2 | 10/10 events |
| 2 | branch | reads what aursu.2 extracted | 16/16 |
| 2 | branch | snowball extract | 16/16 |
| 2 | branch | imports aursu.2's export; exports | users, policies, tags, rule restored |
| 2 | branch | writes set 2, reads it back (plain) | 80/80 |
| 3 | aursu.2 | reads set 2 (written by the branch): plain, SSE-S3 | 80/80; 78/80, see below |
| 3 | aursu.2 | reads set 1 again: plain, SSE-S3 | 80/80; 78/80, see below |
| 3 | aursu.2 | reads what the branch extracted | 16/16 |
| 3 | aursu.2 | imports the branch's export | users, policies, tags, rule restored |

Admin API responses (`GET /minio/admin/v3/info`, `Accept-Encoding:
gzip`): the same status and headers from both versions,
`Content-Encoding: gzip` and `Vary: Accept-Encoding`. No server logged an
error other than the webhook being down in phase 1.

## A failure that is not this change's

The two failing reads are the same in every direction, including
aursu.2 reading its own objects in phase 1: the 30 MiB object
of one repeated line, in the SSE-S3 bucket, read from offset 8388608 and
from offset 15728640, ends with `unexpected EOF`. A second run had each
version write and read its own objects: aursu.2 fails on its own objects
exactly as the branch does on its own, for a single-PUT and for a
multipart object; plain-bucket copies of the same data read correctly,
and so do the other offsets of the encrypted object (0, 1, 65535,
1048583, 4194311, 9437183, 16777216, 25165831, the last 100 and the
last 1000 bytes). So the fault is
in released code, independent of the compress version: range reads of
encrypted, highly compressed objects at some offsets. Whole-object reads
work. It is outside step 2.4 and is recorded for a separate
investigation.
