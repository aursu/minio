# 07 — github.com/klauspost/compress v1.18.0 → v1.20.2 (plan 04, before step 2.1)

Date: 2026-11-21.

## Why now, and why alone

Step 2.1 points the server at the full console (`replace
github.com/minio/console => github.com/aursu/console`). A trial of that
replace moved about 90 requirement lines in the server's go.mod: the
console's go.mod requires newer versions of shared libraries (grpc,
prometheus, compress, go-openapi and more), and Go's version selection
raises the server to them. Several of those libraries are on the server's
runtime path.

So the server is first raised to the console's versions in group PRs of
its own, each with its own CI and note:

1. compress (this note);
2. grpc, protobuf, genproto, otel;
3. prometheus, which brings the GCS, Azure, Google API, oauth2 and dns
   updates with it.

After them, 2.1 is the replace plus the go-openapi stack only.

## What

```
go get github.com/klauspost/compress@v1.20.2
go mod tidy
```

Only compress moves: go.mod 1 line, go.sum 2 lines.

| | |
|---|---|
| Closes | GO-2026-6865 (GHSA-fpmh-frq8-7rj8): panic on crafted two-block frames in `fse`/`huff0`, which zstd uses |
| Reached from | the zstd encoder in the admin server-update path (`cmd.downloadBinary`); govulncheck shows one call path per symbol, so there may be others |
| Also fixed on the way | GO-2026-5841, OOB read in `s2.NewDict` (fixed in 1.18.7); imported, not called |

## Why 1.20.2 and not latest

1.20.2 is the first fixed version and the one the console requires, so 2.1
does not move compress again. 1.20.3 and 1.20.4 are mostly decoder speed
work (new arm64 and pure-Go zstd/huff0 paths); 1.20.5 was released on
2026-11-20, one day before this change. They can follow in step 2.4.

## What MinIO uses from compress

| Package | Use in MinIO | Change between 1.18.0 and 1.20.2 that matters |
|---|---|---|
| `s2` | object compression on disk (`object-api-utils.go`), metacache listing streams, auto-extract of uploaded tar archives (`untar.go`), S3 Select input, the event queue store | arm64 assembly encoders (1.20.0); bounds and length checks. The stream format is unchanged: data written with 1.18 reads back with 1.20 |
| `zstd` | the server-update download (`update.go`), the data usage cache, the peer REST server, `untar.go`, S3 Select input | arm64 decoder assembly (1.19.0); the fixed panic |
| `gzhttp` | wraps the **admin** and **KMS** routers (`cmd/admin-router.go`, `cmd/kms-router.go`) | **zstd negotiation, on by default** (below) |
| `gzip` | the gzhttp level constant in both routers, log rotation, the response recorder | standard-library backports |
| `zip` | admin export/import of IAM and bucket metadata, diagnostics bundles (`notification.go`) | standard-library backports |

## Behaviour change: zstd on the admin and KMS APIs

Since 1.18.4, the `gzhttp` wrapper also offers zstd, and since then it
**enables zstd by default and prefers it over gzip** when a client accepts
both (`zstdEnabled: true`, `preferZstd: true` in `gzhttp/compress.go`).
MinIO creates both wrappers with `gzhttp.NewWrapper(gzhttp.MinSize(1000),
gzhttp.CompressionLevel(gzip.BestSpeed))`, so it gets the new default.

Who sees it:

- **`mc`, the console and other madmin-go clients: nobody.** madmin-go's
  transport sets `DisableCompression: true` (`transport.go`), so it sends no
  `Accept-Encoding` and gets uncompressed responses, as before.
- **A client that advertises zstd** (for example `curl --compressed` built
  with zstd) now gets zstd instead of gzip. It asked for zstd, so it can
  decode it.

Not done: turning zstd off with `gzhttp.EnableZstd(false)` in both routers.
That would keep the wire behaviour byte-for-byte as before, at the cost of
two lines of server code in a dependency PR. It stays available if a client
turns out to mishandle it.

## Old and new data read each other

A rolling upgrade has old and new nodes reading each other's compressed
data. A throwaway program, built once against v1.18.0 and once against
v1.20.2, wrote s2 and zstd data with one version and read it back with the
other:

- s2 streams with the writer options MinIO uses: default, better
  compression (amd64), padding (encrypted objects), and the metacache block
  size and concurrency;
- ranged reads through the s2 index for inputs above `compMinIndexSize`
  (8 MiB), started mid-stream with `ReaderIgnoreStreamIdentifier` and
  `Skip`, as `cmd/object-api-utils.go` does;
- zstd `EncodeAll` / `DecodeAll`;
- random and text inputs of 0 B to 11 MiB.

On linux/amd64 all four directions pass (1.18→1.18, 1.18→1.20, 1.20→1.18,
1.20→1.20), 68 checks each. The encoders of the two versions also produce
identical output on amd64 for this sample.

**Not checked: arm64.** 1.20.0 added arm64 assembly s2 encoders, so arm64 is
where the encoder changed. The test host has no arm64 emulation. MinIO on
arm64 uses the default s2 options.

## Tests

Go 1.27.4, `golang:1.27.4`:

| Check | Result |
|---|---|
| `go build ./...`, `go vet ./...` | ok, 0 findings |
| `make verifiers` (lint v2.15.0 + check-gen) | 0 issues; check-gen clean |
| govulncheck v1.10.0 | 21 → **20** reachable; compress gone, the other 6 modules unchanged |
| `fork/test/bin/unit-tests.sh`, all packages | **46/46 ok** |

The first unit run reported one failure, `TestServerConfigFile` in `cmd`,
with `open testdata/config/1.yaml: operation not permitted`. That came from
the test host, not from this change. MinIO opens files with `O_NOATIME`
(`internal/ioutil/read_file_noatime_supported.go`), and Linux refuses
`O_NOATIME` with `EPERM` unless the caller owns the file. The tree was owned
by another uid than the unprivileged test user (65534). With the tree owned
by the test user, `cmd` passed (177 s). Docker Desktop and GitHub runners do
not show this: there the test user owns the files.
