# 37 — Range reads of compressed SSE objects at a compression index entry

Date: 2026-11-26.

Found by the parallel session during plan 04 step 2.4 (a compression
compatibility check across versions). Present in upstream and in every
aursu release.

## Why

A compressed and encrypted object (SSE-S3 or SSE-KMS, with
`compression allow_encryption=on`) failed some range reads. The server
answered 206 and then sent nothing, so clients reported `unexpected EOF`.

- **The failing reads.** A range that started exactly on an entry of the
  compression index (s2 keeps one per 1 MiB of data), while the
  compressed data before it fitted into the first 64 KiB encryption
  package. Ranges one byte later worked; whole-object reads worked.
- **The data it hits.** Highly compressible data: logs, CSV with repeated
  values, text. 30 MiB of a repeating line compresses to about 4 KiB,
  so every 1 MiB boundary failed. Downloaders that fetch in aligned
  chunks (8 MiB is common) hit exactly these offsets.
- **How it went wrong.** `getCompressedOffsets` turns the range into
  three numbers:
  - `off`, whole encryption packages to skip on disk;
  - `decryptSkip`, bytes to skip inside the first decrypted package;
  - `decOff`, bytes to skip after decompression.

  `NewGetObjectReader` let the s2 reader ignore the stream identifier
  only when `off` or `decOff` was above 0. Here both are 0 and only
  `decryptSkip` is not. So the reader started mid-stream, at a
  compression block, but waited for the identifier that only the start
  of the stream has.

## What (one commit)

**`fix(object)`: ignore the s2 stream identifier after a decrypt skip.**
The condition is now `off > 0 || decOff > 0 || decryptSkip > 0`. The
read starts at the beginning of the compressed stream only when all
three are 0.

It changes only how objects are read. Objects already written are read
correctly after the update; nothing has to be rewritten.

## Public fixes compared (2026-11-26)

pgsty/silo `7c0a99284f31` ("handle indexed ranges within the first
encrypted block") makes the same one-line change. The cause was found
here independently, from a live reproduction. There is no other
sensible fix: the condition must cover the third skip. The test here is
ours. relvo-labs/granary has no change to this code.

## Client-visible change (for the release notes)

Range reads of compressed and encrypted objects no longer fail at 1 MiB
boundaries.

## Tests (media-gw, golang:1.27.4, `-tags kqueue,dev`)

- **`TestAPICompressedEncryptedRange`**
  (`cmd/object-compressed-encrypted-range_test.go`).
  - **The setup.** 10 MiB of a repeating line, over the 8 MiB the
    compression index needs, written through the PUT handler. It runs in
    all eight configurations of the extended object layer test: plain,
    compressed, encrypted, both, each with and without versioning, on
    one drive and on erasure.
  - **The reads.** Ranges of 1000 bytes at 0, 100, 1 MiB, 1 MiB + 1,
    2 MiB, 8 MiB, 9 MiB − 1 and near the end.
  - **On `master`:** the compressed-and-encrypted runs fail at 1, 2 and
    8 MiB with "206, 0 bytes". Every other run and offset passes.
  - **On this branch:** all pass.
- **Related tests pass with `-race`**, as uid 65534: the GET handler
  tests, `TestGetCompressedOffsets`, `TestS2CompressReader`, and every
  test matching Encrypt, Decrypt or Compress.
- **Lint and build:** golangci-lint v2.15.0 reports 0 issues, `go vet` and
  `go build ./...` pass.
- **Live** (scratch `sseczip/repro4.sh`). The parallel session's setup:
  - one container with four drives in one erasure set;
  - bucket default SSE-S3 (`mc encrypt set sse-s3`), compression of
    `.txt`;
  - a 30 MiB repeating text, written by multipart upload and by a single
    PUT;
  - reads with `mc cat --offset N`, whole and cut to 4096 bytes.

  | offset | `aursu.3` | this branch |
  |---|---|---|
  | 0, 1048577, 8388609 | ok | ok |
  | 1048576, 2097152, 8388608, 15728640 | `unexpected EOF`, wrong data | ok |
  | 16777216 (single PUT) | `unexpected EOF` | ok |
  | 16777216 (multipart: start of part 2) | ok | ok |
  | the same object without encryption, every offset | ok | ok |

  An earlier attempt on one drive did not reproduce it. It used `--enc-s3`
  and empty compression extension and MIME lists, with object names
  without `.txt`. The objects were most likely not compressed at all;
  this was not confirmed, because the image has no `grep` to read
  `mc stat`.
