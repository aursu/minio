# 25 — S3 Select line limit: one implementation, error codes kept (follow-up to note 24)

Date: 2026-11-24.

## Why

Reviewing #49 (note 24, CVE-2026-39414) left four weak points:

1. **Duplication.** The bounded read loop existed twice, in the CSV reader
   and in `json.PReader`. The OverMaxRecordSize constructor existed three
   times, in csv, json and simdj. The 1 MiB constant had three names. The
   `countingByteReader` test helper existed three times.
2. **`io.Reader` contract.** The simdjson wrapper returned `(0, err)` and
   dropped the bytes of the read in which a line passed the limit.
3. **Test weight.** About 1,000 lines of tests checked the same
   boundaries at three levels.
4. **Error code.** Every mid-stream error reached the client as
   `InternalError` carrying the error's text.

Working on point 4 turned up two more defects, both upstream and older
than #49:

- **Records lost before an error.** `Evaluate` dropped the records still
  in its output queue, up to 99, before sending the error event.
- **Read error lost in PReader.** A `json.PReader` worker overwrote the
  splitter's error with the decoder's error, which is nil for valid JSON.
  A read error on the last block was therefore lost, and the query ended
  as if the object were complete. One such error is a GZIP checksum
  mismatch.

## What (PR #50)

1. **`refactor(s3select)`** (ours). New package
   `internal/s3select/linelimit`:
   - `MaxLineSize`;
   - `ErrLineTooLong`, a sentinel that implements `s3select.SelectError`.
     The readers return it unwrapped, because `cmd` matches the error
     with `err.(s3select.SelectError)`;
   - `CompleteLine`, the bounded loop. It checks the limit **before**
     appending, so a rejected split never grows past the limit;
   - `Reader`, for simdjson. In the read where a line passes the limit,
     it returns the input up to exactly the limit together with the
     error.

   The CSV, PReader and simdj readers use the package, and their copies
   are gone. The boundary, consumption and wrapper tests move into
   `linelimit`, once. The reader packages keep only the checks that they
   are wired to it: header, body, every record delimiter, valid input
   and DOCUMENT mode. The comments citing granary's issue #79 are
   removed. Net −173 lines.
2. **pgsty `3598c4305dfd`** (cherry-pick, author kept). The message
   writer stages a record still in its queue before it writes the error
   event. Its `select` between the record channel and the error channel
   picks at random, so that record used to be dropped.
3. **pgsty `035aa6c201fb`** (cherry-pick). Tests for the writer ordering.
   They need `testResponseBody` from pgsty `475236c79c77`; we add only
   that helper, because the rest of that commit changes a test helper we
   do not have. The commit message says so.
4. **`fix(s3select)`** (ours, from the `Evaluate` and PReader parts of
   pgsty `3252d5b7f349` and `fd69c89d05d3`):
   - `Evaluate` sends the queued records first, then the error. An S3
     Select error keeps its own code and message; anything else stays
     `InternalError`.
   - If sending the queued records fails, the original error is still
     reported. Sending can fail in three ways:
     - an output record over the limit, which `sendRecord` reports
       itself; the second report is a no-op, because `FinishWithError`
       on a finished writer does nothing;
     - a closed writer, which means the client is gone;
     - a marshal error. That error is dropped, and the client gets the
       original error instead.

     pgsty stops at that point instead, so after a marshal failure the
     client gets no error at all.
   - `json.PReader` keeps the splitter's error unless parsing failed, and
     reports a parse failure as `JSONParsingError`, as simdj and the CSV
     reader already do.
   - pgsty's other change in those commits, dropping simdjson for JSON
     Lines, is not taken.
5. **`test(s3select/linelimit)`**: throughput benchmarks for `Reader` and
   `CompleteLine`.

## Client-visible change (for the release notes)

This is not a pure refactor. Commits 2 and 4 change what S3 Select clients
see, and the release notes of the next release must say so.

- **Error codes.** Errors in the middle of a Select stream used to
  arrive as `InternalError`. They now carry their own S3 code:
  `OverMaxRecordSize` for an input line over 1 MiB (CVE-2026-39414),
  and `JSONParsingError` for invalid JSON Lines. Errors that are not S3
  Select errors stay `InternalError`, for example a failed `CAST`.
- **Records before an error.** Records produced before an error are now
  delivered before the error event. Up to 99 come from the output queue
  of `Evaluate` (commit 4), and one from the queue of the message writer
  (commit 2).
- **Corrupt compressed JSON Lines input.** A read error on the last
  block, such as a GZIP checksum mismatch, now fails the query. Before,
  the result was returned as complete.

The refactor (commit 1) does not change behaviour. PRs are merged without
squash, so each of commits 2 and 4 can be reverted alone. Commit 4 holds
the error codes, the queued records and the PReader fix together.

## Tests (media-gw, golang:1.27.4, `-tags kqueue,dev`)

- **All suites.** `go test -race ./internal/s3select/...` passes for all
  seven packages that have tests; gofmt and vet are clean.
- **Mutations of `linelimit`, all caught:**
  - limit check off by one: `TestCompleteLineBoundary` fails;
  - newline counted as part of the line: `TestCompleteLineBoundary`
    fails;
  - `Reader` returning the whole read: `TestReader` and
    `TestReaderNoNewlineBounded` fail.
- **`select_errors_test.go` without commit 4:**
  - Select error codes come back as `InternalError`;
  - a corrupt GZIP trailer reads as complete through PReader. CSV and
    simdjson report it.
- **Without the queue flush alone:** 16300 records arrive instead of
  16385.
- **GZIP regression tests of #49:** they now check the
  `OverMaxRecordSize` code instead of the message text. Both JSON Lines
  readers are covered by `forEachJSONLinesReader`.

## Performance

Benchmarks were run on media-gw, three versions interleaved, 6 runs
each: before #49 (`10a331777`), #49 (`d1ab36d90`) and this branch.

| benchmark | before #49 | #49 | #50 |
|---|---|---|---|
| `SelectAll_1M` (CSV through Evaluate) | 69.4 ms | 70.4 ms | 70.3 ms |
| `AggregateCount_1M` | 29.9 ms | 30.1 ms | 30.3 ms |
| `ReaderHuge/99689K` (CSV reader) | 55.0 ms | 54.6 ms | 54.9 ms |
| `PReader` (12 inputs) | within ±2% of each other | | |
| simdjson JSON Lines reader, 64 MiB (temporary benchmark) | 282–287 MB/s | — | 286–293 MB/s |

- **No difference** in time, B/op or allocs/op beyond the 1–2% spread.
- **The line limit is not a measurable cost.** `linelimit.Reader` scans
  about 10.5 GB/s and `CompleteLine` splits about 21 GB/s. Against that,
  the CSV reader runs at about 1.8 GB/s and the simdjson reader at about
  0.3 GB/s.
- **There was nothing to optimise for speed.** The gains are the bounded
  memory (#49) and the smaller code (#50).

## Live check (scratch scripts, not committed)

- **Gzip bomb** (`cve39414/e2e.sh`). The image of this branch runs with
  `--memory 2g`, against a 3 MB GZIP object that inflates to 3 GiB
  without a newline:
  - CSV and JSON Lines fail with **`OverMaxRecordSize`** (#49:
    `InternalError`);
  - `memory.peak` stays flat, at 338 and 343 MiB;
  - the server keeps running;
  - the controls give correct results.
- **Stream completion** (`jsonl-stream.sh`, raw `curl`): a JSON Lines
  stream completes 20 times out of 20, with no server panic. Commit 4
  changes how the stream ends, so this was checked again.
- **The `mc` client** (RELEASE.2025-08-13) still panics sometimes on a
  small JSON Lines query. That is its own SelectResults parser, see
  note 24.

## Not in this change

- **The per-package `s3Error` types** in csv, json, simdj and s3select
  stay. They are upstream's layout, and the line-limit error no longer
  needs them.
- **Errors from SQL evaluation** (a failed `CAST`, for example) are plain
  errors and still reach the client as `InternalError`. AWS returns
  specific codes such as `CastFailed`. That is a separate change in the
  `sql` package.
- **Records in the block of an over-limit line.** On overflow the
  complete records in the same 128 KiB block are still dropped (granary's
  design, note 24). The query ends with an error either way.
