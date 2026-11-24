# 20 — CVE-2026-41145, CVE-2026-40344: unsigned-trailer uploads without signature (plan 04, phase 4 item 2)

Date: 2026-11-23.

## Why

Both high (8.8), both from upstream `76913a9fd` (2023-05, "Signed trailers
for signature v4"). `STREAMING-UNSIGNED-PAYLOAD-TRAILER` means the
*payload* is unsigned; the request itself must still be signed. Two paths
wrote objects with only an access key ID known:

- **CVE-2026-41145** (GHSA-hv4r-mvr4-25vw). `PutObjectHandler` and
  `PutObjectPartHandler` verified the signature only when an
  `Authorization` header was present, while `isPutActionAllowed` took the
  identity from either that header or the `X-Amz-Credential` query
  parameter. A PUT with the credential in the query and no signature at
  all was written as that key's owner.
- **CVE-2026-40344** (GHSA-9c4q-hq6p-c237). `PutObjectExtractHandler`
  (Snowball auto-extract) had no case for the unsigned-trailer auth type:
  any signature passed and the tar was extracted into the bucket.

The advisories' only fix is in AIStor (closed source, not used).

## Public fixes compared (2026-11-23)

| Source | Query credentials | Malformed `Authorization` | Date / skew | Extract | Tests |
|---|---|---|---|---|---|
| pgsty/silo `f444b6f37e44` + `efb6e5b00b03` (2026-04-16) | presigned rejected in `newUnsignedV4ChunkedReader` | first version verified only a header with the SigV4 prefix; fixed in `296c9710b164` (2026-11-10) | added in `14ecb222e714` (2026-11-11) | signature verified | +381 lines |
| jarvit/minio-image `0002` + `0003` (2026-10-29) | query signature verified too, presigned unsigned-trailer uploads stay allowed | not covered | not covered | signature verified | +275 lines |
| relvo-labs/granary `ea5efbdf53b6` + `2bd3cbd9f88b` + `a02d3dd81517` (2026-11-14) | any presigned/V2 query parameter makes the request `authTypeUnknown` at classification | `authTypeUnknown`, never anonymous | in the auth middleware, as for the other SigV4 types | signature verified, and the trailer checksum is read before 200 | +476 lines, production router, single-drive and erasure |

**Taken: granary's three commits** (recommended, accepted). Its first
commit already has what pgsty added seven months later in two follow-ups,
and it closes the hole at the point where the request's auth type is
decided, so no handler can take the identity from the query again. The
other two commits are its own follow-ups: a test assertion simplified,
and a sticky terminal error in `s3UnsignedChunkedReader` that the extract
drain needs (granary#47: archives over 4 KiB were written, then answered
with an error). jarvit keeps presigned unsigned-trailer uploads working,
which the AWS SDKs do not send (their presigned PUTs use
`UNSIGNED-PAYLOAD`); pgsty and granary reject them.

Rejected: our own fix (the rule is a public one where it exists);
pgsty's pair alone (the two holes its own later commits closed).

## What

`git cherry-pick -x` of the three commits, authors kept. Code
byte-identical to granary's. In `cmd/unsigned-trailer-auth_test.go`
(first commit) the imports `github.com/relvo-labs/granary/internal/…`
are `github.com/minio/minio/internal/…`; the files' Granary copyright
headers stay. The "#44" in the commit subjects is relvo-labs/granary#44,
not a PR here.

Behaviour changes, for a client of this server:

- an unsigned-trailer request carrying any presigned or V2 query
  parameter → 400 `InvalidRequest`;
- an unsigned-trailer request with an `Authorization` header that is not
  SigV4 → rejected, no longer treated as anonymous;
- signed unsigned-trailer requests obey the 15-minute date skew;
- policy conditions: `s3:signatureversion` = `AWS4-HMAC-SHA256` and
  `s3:authType` = `REST-HEADER` for signed trailer uploads of both kinds
  (they were empty);
- Snowball extract with an unsigned trailer now works when signed (on
  master it failed with "invalid tar header": the aws-chunked body was not
  decoded) and fails on a bad trailer checksum. Extraction stays
  non-transactional: entries written before a late checksum error remain.

## Tests (media-gw, golang:1.27.4, `-tags kqueue,dev`)

- **Test first.** granary's five tests on `master` plus the two test
  files: all five fail, 123 of 202 subtests. Among them: PutObject and
  UploadPart with `query-credential` → 200 and the object/part written;
  Extract `raw-tar-wrong-signature` and `raw-tar-query` → 200 and the
  entry written; `expired-date` → 200.
- **With the fix:** 202/202 pass; `gofmt`, `go vet ./cmd/` clean.
- **Live**, the release image on a single node, a `curl` client that
  knows only the root access key (a throwaway script, output in the PR).
The router tests above run in CI with every `cmd` test run, so the
regression stays guarded:

| Image | PUT, `X-Amz-Credential` only | Extract, zero signature | Signed `mc cp` |
|---|---|---|---|
| master (`local/minio-release-test:aursu.0`) | **200, object written** | **200, entry written** | ok |
| this branch | 400 `InvalidRequest`, nothing written | 403 `SignatureDoesNotMatch`, nothing written | ok |

## Not in this change

Further granary fixes in the same code, not part of these advisories:
`4f54ac7e3ecb` (extract answered 200 before the signed-streaming final
chunk and trailer signature were read), `7c9a50215f19` and
`09a8e7a2ac29` (streaming readers accept a truncated body as a short last
block), `2b69e2cd73c1` (`x-amz-*` headers not in `SignedHeaders` are
accepted; adapted from pgsty `1233254309b1`). Candidates for a later
change.
