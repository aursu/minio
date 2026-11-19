# 04 — Plan: final build first, security fixes afterwards

Date: 2026-11-19. Supersedes [02-plan](02-plan.md); facts from
[03-baseline](03-baseline.md).

## Decisions taken (2026-11-19)

- One long-lived branch per repo: `master`. No mirror branch — upstream
  history stays reachable through `master`'s own history and the tags.
- Every change reaches `master` through a pull request from a short-lived
  branch, deleted after merge.
- Fixes for the open server advisories are **our own code** — no original
  fix exists (MinIO ships them only in closed AIStor).
- Order: the final build (full console, current dependencies, image) first;
  the security fixes afterwards, on top of it.

## Pull-request rules

- One PR = one reviewable unit. Mechanical output (UI bundle, `go.sum`,
  a restored tree) gets a PR or commit of its own and states how to verify it.
- PR description: what, why, why not the obvious alternative, tests run with
  their results against the baseline.
- `aursu/minio` is a GitHub fork: a PR must target **`aursu/minio:master`**,
  never `minio/minio`.
- Original commits are cherry-picked with `-x`, so each names its source.
  Merge with a merge commit (or rebase) — never squash: squashing loses
  which original commit a change came from.
- Development notes for every repo live in this repo, `fork/notes/`.

## Phase 0 — tooling (aursu/minio)

0.1 Test harness in `fork/test/`: the unit recipe from report 03, console
    integration, SSO integration (Dex + OpenLDAP: `quay.io/minio/dex`,
    `quay.io/minio/openldap` still published, 2025-05-14). One command each,
    exit status = pass/fail. Run on the unchanged tree: must match the
    baseline.

## Phase 1 — full console (aursu/console)

1.1 **Restore.** One commit on top of `master` whose tree is exactly
    `v1.7.6` (`git read-tree`). Verify: `git diff v1.7.6 HEAD` is empty.
    History is kept, so every later original commit stays cherry-pickable.
    Tests: console Go unit tests.
1.2 **Original Go-side fixes:** `9e0416f` (s3.Delete*), `33a7fbb` (webhook
    regex). Tests: unit.
1.3 **UI pipeline.** `mds` dependency URL → `github.com/aursu/mds.git#v1.0.4`
    (same commit `027fb7f9`, lockfile unchanged otherwise). Rebuild the bundle
    with the original toolchain (Node 18, yarn 4.4.0) and compare with the
    committed one — proves the pipeline before anything changes.
1.4 **Original UI fixes and dependency commits:** `a2dfcce`, `e479c7b`,
    `40bee68`, `88cd0d4` where they apply; `36d0e78`, `f5ad4de`, `a4281ed`,
    `ff2b72f`, `6df2db8`, `7e5f4f3` (react-router 6.30.1, `jspdf`/
    `react-component-export-image` removal, and the rest).
1.5 **UI dependencies.** Node → current LTS, yarn → latest 4.x, direct
    dependencies within majors, transitive ones via `resolutions`. Target: no
    high/critical in the shipped closure. Bundle rebuild = its own commit.
    Tests: unit + UI end-to-end suites.
1.6 **Console Go side.** Go 1.27.4, modules aligned with the server.

## Phase 2 — server (aursu/minio)

2.1 Point at the full console: `replace github.com/minio/console =>
    github.com/aursu/console <commit>`. Server code untouched.
    Tests: unit, console integration, **SSO integration**.
2.2 Toolchain Go 1.24 → 1.27.4. Clears the ~33 standard-library findings.
2.3 Security tier: every module `govulncheck` reports (x/crypto, x/net,
    x/text, amqp091-go, prometheus, grpc, otel/sdk, go-jose, paho, compress),
    grouped by family, one PR per group. Target: 0 reachable.
2.4 Remaining modules within majors, in groups.

Gate for every PR: unit suite as in the baseline; integration suites for
anything touching IAM, STS, OIDC, LDAP, console, HTTP or dependencies.

## Phase 3 — final build

3.1 `Dockerfile` for the fork: builder `golang:1.27`, version stamped through
    `buildscripts/gen-ldflags.go`, minimal runtime base, non-root user.
3.2 Release: tag in `aursu/minio`, image published, release report with the
    `govulncheck`, UI audit and OSV state.

Open, decided at 3.1: runtime base image, registry (e.g. `ghcr.io/aursu`),
version string format (`mc admin info` parses `RELEASE.<time>`), whether
upstream CI runs in the fork or a workflow of our own builds the image.

## Phase 4 — security fixes (own code)

One PR per advisory, test first: a regression test that reproduces the
exploit and fails on `master`; the smallest fix that makes it pass; full
suites green; note in `fork/notes/`. Release after each, or after the group.

| Order | CVE | Area |
|---|---|---|
| 1 | CVE-2026-33322 | OIDC JWT algorithm confusion (the SSO path) |
| 2 | CVE-2026-40344, CVE-2026-41145 | unsigned-trailer uploads written without signature |
| 3 | CVE-2026-34204 | SSE metadata injection via replication headers |
| 4 | CVE-2026-39414 | S3 Select CSV unbounded line → OOM |
| 5 | CVE-2026-33419 | LDAP STS enumeration + rate limit |
| 6 | CVE-2026-42600 | `ReadMultiple` path traversal (distributed only) |

Before every release: re-query OSV for `github.com/minio/minio` — new
AIStor-only advisories keep appearing.

## Residual risk

- UI majors stay (React 18, react-router 6, styled-components 5, Redux
  Toolkit 1): each is a rewrite, not a bump.
- `mds` stays at v1.0.4.
- Flaws in MinIO's own code found later need our own patches again.
