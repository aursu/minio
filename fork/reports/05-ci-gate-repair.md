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
  `_truncated_parts`, `_induced_bitrot`). Only the "before healing" result is
  ever off, while "after healing" is always green. It is off in one of two ways:
  - the count is higher than the test induced (5 or 4 missing `xl.meta` where
    the test removed 1);
  - the count is right, but the color is one step milder than the test
    expects (green instead of yellow, yellow instead of red).

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
  test's steps on a loaded runner. A red "after healing", or a "before healing"
  count lower than induced, is not this flake.
- **Upgrade, `test-upgrade`.** The upgrade itself works. In a local
  reproduction (repo `compose.yml`, 4 nodes + nginx), switching from 2019 to
  this tree: about 13 s of 502/InternalError, then the correct content for 2
  minutes. The check simply ran too early; fixed in #16.

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
