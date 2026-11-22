# 05 — CI as the PR gate: what broke, what was fixed, what is flaky

Plan 04, step 0.2. Interim report, state on 2026-11-19 (PRs #3–#16).

## Summary

The first CI run in the fork (PR #3) failed nearly every job, and none of the
failures came from the change under review. The upstream CI had decayed in
three ways since development stopped:

1. **Unpinned tools moved on** (typos, golangci-lint, `ubuntu-latest`).
2. **Upstream distribution channels were withdrawn.** `dl.min.io` returns
   HTTP 410, `minio/minio` is gone from Docker Hub and Quay, and `minio/mc` is
   archived.
3. **Upstream's last Dockerfile change** targets an internal release pipeline.

Each cause was fixed in its own PR. What remains red is either expected
(vulncheck until Go is upgraded), flaky upstream tests, or the self-hosted
`mint` job.

## Causes and fixes

| Job(s) | Cause | Fix |
|---|---|---|
| shfmt | our script used spaces | #5 |
| Spell check | `crate-ci/typos@master` moved to a new dictionary | #6: pinned to v1.50.2 by commit; 2 typos fixed, 2 names allowed |
| lint | `install.sh` from `master`: v2.12+ checksums also list an SBOM, so its grep fails | #7: golangci-lint v2.11.4; 5 QF1012 rewrites |
| healing / verify | `ubuntu-latest` = 26.04, whose uutils `shred` rejects `-`, giving a 0-byte test file; `mc od` then divides by zero | #8: runners pinned to `ubuntu-24.04` |
| everything using `mc` | `dl.min.io` 410; `mc` built from archived `master` | #9, #10, #12: `install-mc.sh` builds `aursu/mc` at `RELEASE.2025-08-13T08-35-41Z` (and the 2021 release for `setup_3site`) |
| dependency-review | Dependency graph was off | repo setting, enabled 2026-11-19 |
| resiliency | `FROM minio/minio:latest` gone; Dockerfile copied pipeline artefacts | #11: upstream Dockerfile before `05e569960`, base `ghcr.io/aursu/minio/minio:RELEASE.2025-03-12…` by digest |
| upgrade tests, healing rewrite | old releases unobtainable | #13: `install-minio-release.sh` builds 2019, 2020 and 2024 from source |
| test-pbac | regression of #10: `mc` no longer on `PATH` | #14 |
| test-upgrade | race: checksums compared while nodes still start | #16 |

**A hidden false green.** Before #13, `rewrite-old-new.sh` stored the 410 page
as the "old binary". `verify-healing-with-rewrite` was green without ever
starting the old server.

## Flaky tests (upstream), not server defects

- **Decom (`test-decom`), `bucket2` listing mismatch.** It failed on #10 and
  passed on #11 with the same server code. The bucket has an ILM transition with
  `transition-days 0`, so the listing changes asynchronously.
- **Resiliency, the healing tests** (`test_resiliency_healing_missing_xl_metas`,
  `_truncated_parts`, `_induced_bitrot`). Usually only the "before healing"
  result is off, while "after healing" is green. It is off in one of two ways:
  - the count is higher than the test induced (5 or 4 missing `xl.meta` where
    the test removed 1; 6 for 2; 3 corrupted parts for 2);
  - the count is right, but the color is one step milder than the test
    expects (green instead of yellow, yellow instead of red).

  #29 on 2026-11-22 (a change to `minio-upgrade.sh` and docs only, nothing
  the resiliency job runs) showed both, and two shapes not seen before, each
  in iteration 1 of its test:
  - `missing_xl_metas`: before yellow for 1 missing (want green), and
    **after yellow with 0 missing** (want green);
  - `truncated_parts`: **grey, 0 missing and 0 corrupted, before and
    after** (want 1 corrupted before). The test saw no object state at all.

  The rerun of the same commit passed every test, 4 iterations each.

  Seen on:
  - #11;
  - two PRs on 2026-11-19, one changing only documentation and one only CI
    (Go 1.24.13, `missing_xl_metas`, all four iterations);
  - #25 on 2026-11-21: `missing_xl_metas` on the first attempt;
    `truncated_parts` and `induced_bitrot`, iterations 2–4, on the rerun of
    the same commit, which had passed them on the first attempt.

  Locally, inside Docker-in-Docker, the tests ran **4 of 4 fully green**, 9/9
  tests each, on both this tree and `RELEASE.2025-04-22T22-12-26Z`, with
  identical test scripts. The likely cause is background healing racing the
  test's steps on a loaded runner.

  **Rule:** a "before healing" count lower than induced, or an "after
  healing" that is not green, is not this flake by default. Look at it on
  its own: does the same shape come back on a rerun or on another PR?

  **Both #29 shapes came back** on #32 (2026-11-22), a change to the
  resiliency setup's wait only, whose setup needed 2 attempts, so nothing
  after it ran differently. Iteration 4 of each test:
  - `missing_xl_metas`: before red 3 missing (want red **4**), after
    **yellow with 0 missing**;
  - `truncated_parts`: **0 missing and 0 corrupted before and after**,
    red this time instead of grey (want 4 corrupted before).

  Iterations 1–3 of `missing_xl_metas` had the usual shapes (5 missing for
  1; green for yellow; yellow for red); the other seven tests passed. So
  these two shapes are part of this flake: both were seen on PRs that
  change no server code. The rule stands for every other shape, and for
  these two on a PR that does change the server: rerun, and suspect the
  change only if the shape survives the rerun.
- **Resiliency, the initial setup: `retry: timeout while running: mc alias
  set`** after ten `502 Bad Gateway`. Explained and fixed, note 11. nginx
  in front of the cluster keeps `max_fails=1`, `fail_timeout=10s`; when its
  first request comes before the nodes listen, it locks all four upstreams
  out for 10 s, and the setup gave `mc alias set` exactly 10 s. On the
  build host 2 of 5 starts hit the lockout on master and on #31 alike. Seen
  on #31 on 2026-11-22.
- **Replication (`test-replication`, in "MinIO advanced tests"), a server
  that never starts.** #29 on 2026-11-22: in
  `docs/bucket/replication/setup_2site_existing_replication.sh` all four
  `minio server` processes were started; `./mc ready sitea` then got
  `connection refused` on 127.0.0.1:9001 for 46 minutes, until the run was
  cancelled. Only two `minio` processes were left at that point, so at least
  the 9001 server died on start. Why is unknown: the script sends each
  server's output to `/tmp/sitea_1.log` etc., which CI never prints, and `mc
  ready` has no timeout, so a dead server turns into a hang instead of a
  failure. The step normally takes 9 minutes; the rerun of the same commit
  took 9 and passed. Making the script print those logs and bound the wait
  would turn the next occurrence into an explained failure, as note 09 did
  for the upgrade test.
- **Upgrade, `test-upgrade`.** The upgrade itself works. In a local
  reproduction (repo `compose.yml`, 4 nodes + nginx), switching from 2019 to
  this tree: about 13 s of 502/InternalError, then the correct content for 2
  minutes. The check simply ran too early; fixed in #16.

  **Observed since, not explained:** the wait added in #16 itself timed out
  ("anonymous read … did not succeed 8 times in a row within 120s") on
  "Upgrade from RELEASE.2019-12-19", #28 head `b7f363831`: CI attempts 1
  and 2, then passed on attempt 3. A local reproduction ran that head 8
  times (7 passed) and the previous head (grpc 1.83.1, otel 1.45) 5 times
  (5 passed). The only difference was grpc 1.83.2 and otel 1.47, and
  nothing on the anonymous S3 read path uses them. No failure kept the
  HTTP answer or the node logs; since note 09 the test prints both, so
  the next failure can be read instead of guessed at. On #29, the first CI
  run with it, the cluster answered 200 from the first second and was
  ready after 8 s.

None of these is fixed by retrying in CI. They are documented here, so a red run
of these jobs is read against this list before anyone suspects the change under
review.

## Expected red

- **vulncheck (Analysis).** Go standard library vulnerabilities; resolved by
  the Go upgrade in plan step 2.2.
- **mint.** Needs a self-hosted runner named `mint`, which the fork does not
  have, so its runs stayed queued forever. **Disabled 2026-11-19** through the
  API (`gh workflow disable mint.yml`), and its queued runs were cancelled.
  Re-enable with `gh workflow enable mint.yml --repo aursu/minio`.

## Lessons

- **Upstream test scripts are written for disposable runners.**
  `resiliency-tests.sh` removes every container and prunes images and volumes;
  `minio-upgrade.sh` prunes too. Locally they run only inside Docker-in-Docker
  ([resiliency-tests-destructive-cleanup](../memory/resiliency-tests-destructive-cleanup.md)).
- **`curl` without `-f`, and scripts without `set -e`, turn a missing
  dependency into a test that passes.** When a job goes green after a fix, check
  that it ran what it claims to.
- **An upstream green run is no baseline** when that run itself used substitutes
  (a broken `make docker` under `set +e`, the 410 page as a binary).

## Next

1. Merge #16 and confirm a full CI run on master's state.
2. ~~Disable `mint`~~: done 2026-11-19.
3. Open question: an upgrade check from `RELEASE.2025-03-12` (the version
   actually deployed) to this tree. It needs the user's answer on whether data
   on that version will move to the fork.
4. Then Phase 1 (`aursu/console` restore) per plan 04.
