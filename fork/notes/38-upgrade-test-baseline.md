# 38 — Upgrade test starts from 2025-04-22, not 2019

Date: 2026-11-27.

## What

The "Upgrade old version tests" workflow (`upgrade-ci-cd.yaml`) used to
start from three releases: 2019-12-19, 2024-10-13 and 2025-03-12. The
2019 entry is replaced by `RELEASE.2025-04-22T22-12-26Z` (commit
`0d7408fc`). That is the last community release with the full console,
the one users of the fork move from. `buildscripts/minio-upgrade.sh`
defaults to the same release, so a local `make test-upgrade` runs what
CI runs.

Corrections are marked in note 04 and reports 08 and 09.

## Why

The 2019 job failed about once in four CI runs. Reads through nginx
alternated between 200 and 403 for the whole 120 s wait. The cause was
measured on media-gw on 2026-11-26 and 2026-11-27:

- **Setup.** Four nodes that are never recreated, with static IPs and
  peers in `/etc/hosts`. MinIO runs under a restart loop. The binary is
  replaced on every node, then all four get SIGTERM at once.
- **Upgrade from 2019.** In 4 of 14 runs one node answered
  `403 AccessDenied` for the whole 300 s window. Through that node
  `mc anonymous get` said `private`; through the other three it said
  `download`.
- **Restarts and upgrades between current releases.** In 12 runs
  (aursu.4 → aursu.4, aursu.2 → aursu.4), every node served the object
  within 17 s.

**The mechanism.** A 2019 bucket keeps its policy in the legacy file
`.minio.sys/buckets/<bucket>/policy.json`. `loadBucketMetadataParse`
(`cmd/bucket-metadata.go`) handles such a bucket in three steps, with no
lock:

1. It reads `.metadata.bin`.
2. If that file is missing, it reads the legacy files.
3. `convertLegacyConfigs` saves `.metadata.bin` and deletes the legacy
   files.

The race:

- Node B misses `.metadata.bin`.
- Node A saves `.metadata.bin` and deletes `policy.json`.
- B finds no legacy files and caches the bucket with no policy.

B keeps that cache until `refreshBucketsMetadataLoop` runs, every 15 min.
The stored metadata stays correct, because B has nothing to convert and
so does not save.

**Not fixed.** Only a direct upgrade from a release older than 2020-05
reaches this code; every later release writes `.metadata.bin`. Alexander
(2026-11-27): an upgrade from a 2019 or 2020 release to the current one
is not a real path. A test of that path is therefore noise, and it hid
the real question the job asks: whether a release users run upgrades
cleanly.

The diagnosis of 2026-11-26 was wrong. It said that nodes stay "not
initialized" for minutes. MinIO answers that with 503, not 403, and the
minutes-long stalls came from recreating the containers: IP swaps and
Docker DNS.

## Alternatives rejected

- **Fix the race** with a cluster lock around read → migrate → save. It
  is real code in the startup path, for an upgrade nobody makes.
- **Keep the 2019 job and rerun it on failure.** CI would stay red for
  no reason about once in four runs, and each failure would cost a
  rerun and a look at the log.
- **Raise the 120 s wait.** The stale cache lasts up to 15 min, so the
  job would have to wait that long.

## Tests

- **Before the PR,** on media-gw: `make test-upgrade` from
  `RELEASE.2025-04-22T22-12-26Z`, with the old release built by
  `install-minio-release.sh`.
- **In CI:** the PR runs the matrix 2024-10-13, 2025-03-12 and
  2025-04-22.
