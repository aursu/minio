# 47 — tinylib/msgp 1.4.0 → 1.6.4, zeebo/xxh3 1.0.2 → 1.1.0

Date: 2026-11-26. Plan 04 step 2.4, part of group B (note 40). Stacked
on E (note 46).

## Why now, and why on its own

Group F1 wants minio-go 7.4.1. Every minio-go from 7.0.98 requires msgp
1.6.1 or later, and every one from 7.1.0 requires xxh3 1.1.0. Both are
direct dependencies of the server and belong to group B, because the
on-disk format and node-to-node messages depend on them. So they get
their own PR and their own checks, below F1. The alternative was minio-go
7.0.97, the last release that needs neither. Recommended, accepted
(Alexander, 2026-11-26): this PR first, then F1 on 7.4.1.
reedsolomon and pgzip stay in the rest of group B.

## Where MinIO uses these libraries

| Library | Use | Data |
|---|---|---|
| msgp | every `_gen.go`: `xl.meta` (`xlMetaV2*`), `FileInfo`, data usage cache, bucket metadata, replication state, metacache entries; storage REST and grid payloads between nodes | on disk; in transit between nodes |
| xxh3 | `hashDeterministicString`/`Bytes` in `cmd/xl-storage-format-utils.go` (the version signature in `xl.meta` headers, compared across drives); the checksum of every grid message (`internal/grid/connection.go`); lifecycle and replication worker sharding; the TLS certificate fingerprint in `cmd/admin-handlers.go` | on disk; in transit between nodes |

A change in either one's output would show as drives that disagree on a
version, or as grid messages rejected between nodes of different versions
during a rolling upgrade.

## What changes

- `go.mod`: msgp v1.6.4, xxh3 v1.1.0. Nothing else moves.
- msgp is in go.mod's `tool` block, so `go generate` (and with it
  `make check-gen` in CI) uses the new generator. Regeneration changes one
  file, `cmd/bucket-replication-utils_gen.go`: for `PurgeTargets`, the
  generator now calls the methods of `VersionPurgeStatusType` through
  `(*replication.VersionPurgeStatusType)(&v)`. `cmd`'s
  `VersionPurgeStatusType` is an alias of that type
  (`cmd/bucket-replication-utils.go`), so the methods and the bytes are
  the same.
- msgp runtime: new API (generic `ReadArray`/`ReadMap`, `AppendMapSorted`,
  `BinaryAppender` and `TextAppender` support, `Number.Coerce*`), and
  reader limits (`SetMaxElements`, `SetMaxStringLength`) that are off
  unless set; MinIO sets none. The recursion limit for `Skip` and
  interface decoding is 100000 in both versions.
- xxh3: the generic accumulator is rewritten, the AVX2 and AVX-512
  assembly is rescheduled, AVX-512 gets a block function for the
  streaming `Hasher`, and arm64 gets a NEON path. The algorithm is the
  same; only the code that computes it changed.

## Tests (media-gw, golang:1.27.4)

Byte identity, base (`deps/tier-cloud-sdks`) against this branch:

- xxh3: `Hash`, `HashString`, `HashSeed`, `Hash128`, `Hash128Seed` and a
  streaming `Hasher` (7-byte writes), over every length 0–4096 and lengths
  up to 5 MiB, at offsets 0, 1 and 7. Identical for 1.0.2 and 1.1.0 on
  amd64 (AVX2, on a Ryzen 7 5700G), on 386 (the generic code) and on
  linux/arm64 under qemu (NEON in 1.1.0, generic in 1.0.2). The upstream
  1.1.0 test suite, which compares its SSE2, AVX2 and generic paths, passes.
  **Not run on AVX-512 hardware**: neither media-gw nor the workstation
  has it.
- msgp: a temporary test in `cmd` (not committed) encodes 200 randomly
  filled instances each of 14 generated types (`FileInfo`,
  `FileInfoVersions`, `RawFileInfo`, `xlMetaV2Object`,
  `xlMetaV2DeleteMarker`, `xlMetaV2Version`, `xlMetaV2VersionHeader`,
  `ReplicationState`, `BucketMetadata`, `dataUsageCache`, `DiskInfo`,
  `MetadataHandlerParams`, `lastDayTierStats`,
  `BucketReplicationResyncStatus`) with `MarshalMsg` and `EncodeMsg`,
  decodes them with `UnmarshalMsg` and `DecodeMsg`, and loads every
  `xl.meta` in `cmd/testdata` (23 files, one with 855 versions):
  `AppendTo`, the version signatures, the `FileInfo` of every version; and
  the metacache sample, read and written back. Map order is random in Go,
  so outputs are compared in a canonical form (sorted keys, sorted string
  sets). Base and branch, two runs each: identical.

Unit, on this branch:

- `go mod tidy -compat=1.21` a second time: no change. `go generate`: only
  the file above. `go build ./...`: ok. Cross-compile, all fifteen
  targets: ok.
- golangci-lint v2.15.0: 0 issues. govulncheck v1.10.0: no
  vulnerabilities. GitHub advisories for msgp v1.6.4 and xxh3 v1.1.0:
  none.
- `go test -race` for `./internal/...` (`internal/grid` included) and for
  the `cmd` tests matching xl, meta, metacache, format, heal, usage,
  replication, grid, signature and version: ok, except
  `TestReplicationResync`, which panics in that subset on the base too
  (it passes alone and in the full run; it depends on what ran before it).
  Without it the subset passes on both. `go test -tags kqueue,dev ./...`
  as uid 65534: 49 packages ok.

Live, a distributed deployment of four nodes with two drives each (one
erasure set of eight), the `aursu.4` image (msgp 1.4.0, xxh3 1.0.2) and
this branch built from source. Each set written is a versioned bucket
(three inline versions, two 12 MiB multipart versions, a delete marker)
and 100 objects of up to 96 KiB plus one of 40 MiB; every version's
sha256 is kept. Damage: one set's objects removed from one drive, then
`mc admin heal -r`.

| Phase | Nodes | Checks | Result |
|---|---|---|---|
| 1 | all aursu.4 | write set 1; read through n2 | 107 of 107 versions ok |
| 2 | n1, n2 aursu.4; n3, n4 this branch | read through n1 and n3; write set 2 through n3; damage a drive on n1 and on n2; heal through n3; read through n4 | 214 of 214 ok; listings identical on both versions; healed (208 `xl.meta` on each of the 8 drives) |
| 3 | all this branch | read; write set 3; damage drives on n4 and n3; heal through n2 | 321 of 321 ok; healed (312 on each drive) |
| 4 | all aursu.4 (rollback) | read through n1 and n4; damage a drive on n2; heal through n1 | 321 of 321 ok; listings unchanged; healed (312 on each drive) |

The logs hold only what restarts produce (connection refused, no write
quorum while peers start) and, in the mixed phase, MinIO's notice that
the peers run different binaries. No grid checksum error, no corrupt or
mismatched metadata.
