# 24 — CVE-2026-39414: S3 Select reads a newline-free line into memory (plan 04, phase 4 item 4)

Date: 2026-11-24.

## Why

High; GHSA-h749-fxx7-pwpg. The defect has been present since
RELEASE.2018-08-18.

S3 Select splits its input into 128 KiB blocks and completes each block
up to the next newline with `bufio.Reader.ReadBytes('\n')`. That call has
no bound. With an input that contains no newline, the whole rest of the
object is read into a single allocation, and the server is killed for
running out of memory.

A GZIP object is enough to trigger it. A 3 MB object inflates to 3 GiB.
Any user with `s3:PutObject` and `s3:GetObject` can run the attack.

The advisory names the CSV reader. The JSON Lines readers have the same
code:

- `json.PReader`, used on CPUs without AVX2/CLMUL;
- `simdj.Reader`, used on all other CPUs, which includes media-gw and most
  servers. `simdjson-go`'s `ParseNDStream` also completes its block with
  an unbounded `ReadBytes`.

The advisory's only fix is in AIStor, which is closed source and not used
here.

## Public fixes compared (2026-11-24)

| Source | CSV | JSON Lines, PReader | JSON Lines, simdjson path | Error code in the stream | Tests |
|---|---|---|---|---|---|
| pgsty/silo `3252d5b7f349` + `fd69c89d05d3` (2026-04, 2026-06) | 1 MiB per line, `ReadSlice` loop | same | **removes simdjson for JSON Lines**: every CPU uses PReader | `OverMaxRecordSize`, and every `SelectError` keeps its code. That exposed a writer-ordering race, fixed later in `3598c4305dfd` + `035aa6c201fb` | JSON Lines oversized record, error-code preservation |
| jarvit/minio-image `0001` (2026-10) | 1 MiB, counted only from the block end | **not covered** | **not covered** | every `SelectError` keeps its code | CSV reader: unbounded input, header, long valid lines; one GZIP case through Evaluate |
| relvo-labs/granary `a6f9814e5625` + `4dd1885b754f` + `3196f58fb2c7` (2026-11-21) | 1 MiB per **whole** line, counted across the block boundary; on overflow the block is dropped, so no truncated record is emitted | same | **kept**: a `lineLimitReader` wraps the input before `ParseNDStream`, and the third-party parser is not modified | unchanged: `InternalError` carrying the OverMaxRecordSize message | +1034 lines: boundary cases at the limit and at limit+1, CRLF, custom delimiters, quoted newlines, header line, GZIP through Evaluate for CSV, PReader and simdjson |

**Taken: granary's three commits** (recommended, accepted). Reasons:

- It is the only fix that bounds all three readers without giving up
  simdjson. pgsty pays for its coverage of the simdjson path with the
  performance of every JSON Lines query.
- Its tests show that valid input is unchanged up to the limit.
- It applies to our tree without conflicts.

## What

`git cherry-pick -x` of the three commits, author kept. The only
adaptations are in the three new test files:

- `github.com/relvo-labs/granary/internal/…` becomes
  `github.com/minio/minio/internal/…`;
- gofmt reorders the imports of `simdj/reader_linelimit_test.go` after
  that rename.

Behaviour after the change:

- **Over the limit.** An input line longer than 1 MiB (the S3 Select
  `maxCharsPerRecord`) ends the query with the OverMaxRecordSize message.
  A header line over the limit fails in `Open` with
  `OverMaxRecordSize`/400. Memory per split is bounded by the block, the
  limit and one bufio buffer.
- **JSON DOCUMENT mode** keeps its existing 10 MiB document cap.

## Tests (media-gw, golang:1.27.4, `-tags kqueue,dev`)

- **New tests**, 18 functions, on master with only the test files added:
  - 26 results fail, every over-limit case for CSV, PReader and the
    Evaluate GZIP cases;
  - `simdj` does not build, because its tests need `newLineLimitReader`;
  - the 24 at-limit and valid-input results pass, which shows that the
    tests do not reject what master accepts.
- **On the branch:** all 77 results pass. `go test -race ./internal/s3select/...`
  passes for all six packages, `go vet` is clean and `gofmt -l` lists
  nothing.
- **Live check** (scratch script, not committed). The server runs with a
  2 GiB memory limit, and the queries run as a user with only
  `s3:PutObject` and `s3:GetObject`:

  | | release `aursu.1` | this branch |
  |---|---|---|
  | CSV `sum`, JSON Lines, a 1,000,000-byte CSV line | correct results | correct results |
  | CSV, 3 MB GZIP → 3 GiB without a newline | **OOM-killed** | error with the OverMaxRecordSize message; `memory.peak` unchanged (332 MiB); server running |
  | JSON Lines, same (simdjson path) | **OOM-killed** | same; `memory.peak` unchanged (397 MiB); server running |

- **A client defect seen along the way.** `mc` RELEASE.2025-08-13 inside
  the image once panicked and once hung after printing all records of a
  small JSON Lines query. Raw `curl` against both images completed the
  stream (End event) 20 times out of 20, with no server panics. The
  defect is in the client's SelectResults parser, which pgsty describes
  as a double close in minio-go (`475236c79c77`). It is not a server
  issue.

## Not in this change

- **Error code in the event stream.** A mid-stream reader error still
  reaches the client as `InternalError` with the OverMaxRecordSize
  message. jarvit and pgsty forward the `SelectError` code instead. That
  changes the code for every mid-stream Select error, CSV and JSON parsing
  errors included. pgsty needed a follow-up fix for the order of records
  and the error message (`3598c4305dfd`). It is outside the advisory and
  can be a separate change: pgsty `3252d5b7f349`'s `select.go` hunk with
  `3598c4305dfd`.
