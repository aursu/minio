# 15 — every CI job has a time limit; the root lockdown test stops waiting

Date: 2026-11-23. A CI change only; no server code moves.

## Why

On #36 (paho 1.5.1) "Root lockdown tests" ran for 2 h 18 min and ended only
because a push to the branch cancelled it. Its successful runs take 4–5
minutes. The log shows where it stopped, in `buildscripts/disable-root.sh`:

```
22:55:37  + ./mc ready sitea
22:55:42  The cluster 'sitea' is not ready          … every 5 s, 10 minutes
23:05:43  The cluster 'sitea' is unreachable: … 127.0.0.1:9001: connect: connection refused
                                                    … every 5 s until 01:05
```

Two things let it run that long:

- `mc ready` polls until the cluster answers and has no limit of its own.
- No job in these workflows set `timeout-minutes`, so GitHub's own limit
  applies: 6 hours.

Why site A did not come up is not known. Its two servers write to
`/tmp/sitea_1.log` and `/tmp/sitea_2.log`, and nothing printed them. The
next run on the same branch passed (4 min 21 s).

## What

**A limit on every job.** Each job gets `timeout-minutes` of about twice the
longest successful run in the last 100 runs of this repo, rounded up, and
never under 10:

| Workflow | Job | Longest run | Limit |
|---|---|---|---|
| MinIO advanced tests | Advanced Tests | 45.8 min | 90 |
| Linters and Tests | Go | 32.4 | 60 |
| IAM integration | the five-way matrix | 30.0 | 60 |
| IAM integration | the two import jobs | 6.5 | 15 |
| Crosscompile | Build Tests | 22.8 | 45 |
| Healing Functional Tests | Go | 18.9 | 40 |
| Upgrade old version tests | Upgrade from … | 10.1 | 25 |
| Functional Tests | Go - healing | 8.5 | 20 |
| Resiliency Functional Tests | Go | 8.5 | 20 |
| Root lockdown tests | Go | 5.4 | 15 |
| VulnCheck, Spelling, shfmt, Helm, Dependency Review | | ≤ 1.2 | 10 |
| Lock Threads, Issue Workflow (not run on PRs) | | – | 10 |

Mint Tests already had 120 and keeps it. A matrix job's limit applies to
each entry, not to the matrix as a whole.

**A bounded wait in the root lockdown test.** `disable-root.sh` waits through
`wait_ready <alias> <log files>`: `mc ready` under `timeout 120`; when that
runs out it prints the last 100 lines of each server log and fails the job.
In the passing runs of #33–#36 every wait ended within 10 seconds.

## Not changed

- **The other scripts that call `mc ready`** (about 30 calls under
  `docs/site-replication/`, `docs/distributed/` and others) keep the
  unbounded call. The job limits now cap them; each gets the same helper
  when it is the one that hangs and its logs are needed.
- **The first two waits' logs.** The four servers of the first phase start
  with `2>&1 >/tmp/log$i.txt`, which sends stderr to the job output and
  only stdout to the files. Their logs already reach the job output, so
  the files may be empty; the redirection is upstream's and stays.
- **A self-hosted runner.** The waits on #36 came from this hang, not from
  queueing: every job started within a second of being created.

## Tests

On the build host, `golang:1.27.4`, this branch. The container's
`/.dockerenv` is removed first: with it MinIO takes itself to be in a
container and refuses endpoints that resolve to localhost, which a CI runner
never sees.

| Case | Result |
|---|---|
| `make test-root-disable`, as CI runs it | pass, rc 0, 37 s; all six waits ready |
| site A's second node moved to `127.0.0.1:9099`, where nothing listens | rc 1 after 138 s: `cluster 'sitea' not ready within 120s`, then both site A logs (`drive not found, will be retried` …) |
| the first run, before `/.dockerenv` was removed | rc 1 after 120 s at the first wait, instead of polling on; the `/tmp/log*.txt` files empty, as described above, and the servers' errors in the job output |
| `shfmt -s -d buildscripts/disable-root.sh` (shfmt 3.12.0) | clean |
| actionlint 1.7.7, all workflows, master against this branch | the same two findings on both (`lock-threads@v3` too old, runner label `mint` unknown); none added |
