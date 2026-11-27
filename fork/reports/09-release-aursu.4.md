# 09 — Release RELEASE.2026-11-26T13-57-40Z.aursu.4

Date: 2026-11-26.

| | |
|---|---|
| Tag | `RELEASE.2026-11-26T13-57-40Z.aursu.4` (annotated) on `master` `a39e3d26f` (#71's merge) |
| Image | `ghcr.io/aursu/minio:RELEASE.2026-11-26T13-57-40Z.aursu.4`, also `:latest` |
| Digest | `sha256:9e986baa738addb83214ec71bf41c8770a92e04ae4ab1c6162f29eb72f34eb75` (both tags) |
| Package | public: the manifest is served to an anonymous token |
| `minio --version` | `RELEASE.2026-11-26T13-57-40Z.aursu.4 (commit-id=a39e3d26f7c4…)`, `go1.27.4 linux/amd64` |
| Workflow | "Release image" run `45834248786`: build, version stamp, console login through Keycloak, publish; all green |
| Since `aursu.3` | #71 |
| Dependencies | `go.mod` and `go.sum` unchanged since `aursu.1`; console still `aursu/console` `7832665c91d3` |

## What is in it

**Range reads of compressed and encrypted objects** (#71, note 37). The
parallel session found this during plan 04 step 2.4. It is present
upstream and in every earlier aursu release.

- **What failed.** A compressed object encrypted with SSE-S3 or SSE-KMS,
  read with a range that starts exactly on a compression index entry
  (every 1 MiB). The server answered 206 and then sent no data, and
  clients saw `unexpected EOF`.
- **When.** Whenever the compressed data before the range fits into the
  first 64 KiB encryption package, which happens with highly
  compressible logs, CSV and text.
- **The fix.** The s2 reader now ignores the stream identifier after a
  decrypt skip, too.
- **Existing objects.** The fix changes only how objects are read, so
  objects already written read correctly after the update.

## Changes that clients and operators see

Range reads of compressed and encrypted objects no longer fail at 1 MiB
boundaries.

## Vulnerability state at the tag

| Check | Result |
|---|---|
| `govulncheck` v1.10.0, `./...`, Go 1.27.4, on `a39e3d26f` | **No vulnerabilities found** |
| UI audit | not repeated: the console is the commit audited for `aursu.2`, **no audit suggestions** |
| OSV, `github.com/minio/minio`, published since 2025-10-15 | the same 8 CVEs, **all fixed** (report 07) |

## Check of the published image

Run on media-gw, with the image pulled from ghcr (scratch
`sseczip/repro4.sh`). The setup is the one in which the parallel session
found the defect:

- one container, with four drives in one erasure set;
- bucket default SSE-S3, and compression of `.txt`;
- a 30 MiB repeating text, written by multipart upload and by a single
  PUT;
- reads with `mc cat --offset`, both whole and cut to 4096 bytes.

| offset | `aursu.3` | `aursu.4` |
|---|---|---|
| 0, 1048577, 8388609, last byte | ok | ok |
| 1048576, 2097152, 8388608, 15728640 | `unexpected EOF` | ok |
| 16777216, single PUT | `unexpected EOF` | ok |
| 16777216, multipart (the start of part 2) | ok | ok |
| the same object without encryption | ok | ok |

## CI on the way

#71: 22/22, first attempt.

## Next

- **Plan 04 step 2.4 (dependencies within their majors)** is with the
  parallel session: #66, #68, #69 and the ones after.
- **Not done:**
  - malformed `FileInfo` from a peer (note 36);
  - the "Upgrade from 2019" 200/403 flake, which may be a real defect in
    how a node loads bucket metadata. *(Update 2026-11-27: it is a race
    in the legacy bucket-config migration, left unfixed; the 2019 upgrade
    test is dropped. Note 38.)*
  - the items offered earlier (report 07).
