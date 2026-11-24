# 21 — Snowball extract: data race in the per-entry permission check

Date: 2026-11-23. Found by `make test-race` on #44 (phase 4 item 2).

## Why

`PutObjectExtractHandler` hands `putObjectTar` to `untar`, which writes
entries up to 128 KiB (`xioutil.MediumBlock`) from up to 16 parallel
goroutines. Each entry ran

    isPutActionAllowed(ctx, getRequestAuthType(r), bucket, object, r, …)

on the one shared request:

- `getRequestAuthType` assigns `r.Form` (`url.ParseQuery` on every call),
  and `getConditionValues` inside `isPutActionAllowed` calls it again;
  meanwhile other entries read `r.Form` (`getReqAccessKeyV4`,
  `getSessionToken`, the version ID);
- `isPutActionAllowed` writes `Cred`, `Owner`, `Region` of the request's
  shared log info;
- the result was assigned to the handler's `s3Err`, also written for an
  object-lock refusal;
- a denied entry wrote the error response from its goroutine; `untar`
  then returned the error and the handler wrote a second one. `untar`
  can return before its goroutines finish (`return asyncErr` without
  `wg.Wait`), so the goroutines' writes and the handler's could overlap.

The per-entry check came with upstream `60f842315` (2025-02, "Quick patch
for Snowball AutoExtract"), on top of the concurrent extraction of
`bd3dfad8b` (2022). No upstream test extracted several small entries, so
`-race` never saw it. granary's `TestUnsignedTrailerExtractLarge` (#44) is
the first; on master a header-signed extract of 16 entries shows the same
races.

## Public fixes (2026-11-23)

- relvo-labs/granary: none; its master still calls `getRequestAuthType(r)`
  per entry.
- pgsty/silo `ff44527a3ce7` ("isolate Snowball replication trust per
  entry", +151 test lines under `-race`) and `ab3ae99ca30c`: credentials
  resolved once, each entry checked against its own clone of the request
  through a new `isPutActionAllowedWithCred`, the first API error kept.
  Not cherry-pickable: it is built on pgsty's replication-trust layer
  (`replication-trust.go`, `cloneRequestWithoutReplicationHeaders`, a
  `requestTags` argument), which this tree does not have.

So our own change, the smallest that removes the shared writes.

## What

`cmd/object-handlers.go`, `PutObjectExtractHandler` only:

- `entryAllowed(object, action)` runs `isPutActionAllowed` under
  `entryMu` with the handler's `rAuthType` (computed once at the top) and
  keeps the first denial; both per-entry checks (PutObject, and
  ReplicateObject for replica uploads) use it;
- the first object-lock refusal is kept the same way;
- entries no longer write to `w`; after `untar` the handler reads both
  under `entryMu` and writes the one response, with the same choice of
  error as before (entry error if set, else `untar`'s error);
- with `ignore-errors`, `untar` skips a denied entry and goes on; the
  request is still answered with the denial, as before (the entry's own
  response used to win);
- the version ID is read from `r.Form` once, before extraction.

Object writes stay parallel; only the in-memory policy check is
serialized.

Rejected: a clone of the request per entry, as pgsty does — the shared
log info (`logger.GetReqInfo(ctx)`) would still be written concurrently,
so it needs the lock or a split of `isPutActionAllowed` anyway.
Making `getRequestAuthType` stop assigning `r.Form` — it runs on every
request; a change there is far wider than this bug.

## Tests (media-gw, golang:1.27.4, `-tags kqueue,dev`)

`cmd/object-handlers-extract-race_test.go`,
`TestPutObjectExtractParallelEntries`, on single-drive and erasure:
16 small entries signed (all written, 200); 16 + 16 entries anonymously
with a bucket policy that allows `bucket/*` and denies `bucket/no/*`
(403 `AccessDenied`, one error document, nothing under `no/`).

| Tree | `-count=5` | `-race -count=5` |
|---|---|---|
| master + the test | pass | **fail**: 4 runs, 21 race reports |
| this branch | pass | pass, 0 races |

#44's tests on top of this branch, `-race -count=3`: all pass, 0 races
(the combination applies without conflicts). `gofmt`, `go vet ./cmd/`
clean.

## Not in this change

`untar` returning before its goroutines finish: entries may still be
written after the response for an early error. Upstream behaviour, not a
data race once entries stop touching `w` and the shared state.
