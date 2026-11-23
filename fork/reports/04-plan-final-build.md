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

0.0 **Secure the test images.** MinIO has already removed `minio/minio` and
    `minio/mc` from Docker Hub (404) and Quay (not pullable), checked
    2026-11-19. `quay.io/minio/dex` (amd64, built 2024-05-15) and
    `quay.io/minio/openldap` (amd64, built 2025-05-13, seeded test directory)
    are still there and both the console SSO test and upstream
    `iam-integrations.yaml` depend on them. **Done 2026-11-19** (Alexander):
    `ghcr.io/aursu/minio/{dex,openldap}:latest`, verified identical to the
    Quay originals; plus copies of the last official server
    (`RELEASE.2025-03-12T18-04-18Z`, embeds console v1.7.6) and `mc`
    (`RELEASE.2025-08-13T08-35-41Z`). All public. Tests and CI point there —
    see [minio-images-removed](../memory/minio-images-removed.md).
    AIStor images are not used anywhere (proprietary evaluation licence, no
    reverse engineering): [aistor-not-usable](../memory/aistor-not-usable.md).
0.1 Test harness in `fork/test/`: the unit recipe from report 03, console
    integration, SSO integration (Dex + OpenLDAP from 0.0). One command each,
    exit status = pass/fail. Run on the unchanged tree: must match the
    baseline. Used for local pre-checks and for what CI does not cover.
0.2 **GitHub Actions = the PR gate.** Upstream's workflows already run on
    `pull_request` into `master`, on GitHub-hosted `ubuntu-latest` runners
    (free for a public repo):

    | Workflow | Runs |
    |---|---|
    | `go-lint.yml` | `make test` (all unit tests), `make test-race` |
    | `go.yml` | `make verify`, `make test-timeout` |
    | `iam-integrations.yaml` | `make test-iam` with OpenLDAP, etcd, Dex (OIDC); site replication LDAP/OIDC; IAM import with OpenID |
    | `replication.yaml`, `go-healing.yml`, `go-resiliency.yml`, `upgrade-ci-cd.yaml`, `root-disable.yml` | replication, ILM, decom, healing, resiliency, upgrade |
    | `vulncheck.yml` | `govulncheck` (also on push) |

    - Actions must be enabled once in the fork's Actions tab (GitHub disables
      workflows in forks; no run so far, 2026-11-19).
    - `mint.yml` needs a self-hosted `mint` runner we do not have: disable it
      in the Actions UI. `lock.yml`/`issues.yaml` are upstream housekeeping:
      disable likewise. No file changes.
    - `vulncheck.yml` installs `govulncheck@latest`, which needs Go ≥ 1.26:
      it fails until 2.2. Expected; recorded, not fixed separately.
    - The Go version matrix (`1.24.x`) moves with the toolchain in 2.2.
    - The first PR (this plan, docs only) gives the CI baseline of the
      unchanged code; failures there are recorded and do not block.

    The console's own CI (`.github/workflows/jobs.yaml`: Go tests,
    integration, 8 UI permission suites) came back with the restore in 1.1,
    but its end-to-end jobs were red. Repaired 2026-11-20 in
    aursu/console #10 and #11 (its `fork/notes/05-ci-gate-repair.md`):
    - the test server is `ghcr.io/aursu/minio/minio` pinned by digest, or
      built from `aursu/minio` with the console under test embedded;
    - `mc` is copied from the server container;
    - TestCafe 3.7.7 (3.5.0 hangs on Firefox 160);
    - Playwright comes from the lockfile.

    Results: 83 permission tests, 86 integration, 2 SSO, 6 playwright, all
    green. The only reds are the vulnerability checks that 1.5 and 1.6
    address.

## Phase 1 — full console (aursu/console)

1.1 **Restore.** One commit on top of `master` whose tree is exactly
    `v1.7.6` (`git read-tree`). Verify: `git diff v1.7.6 HEAD` is empty.
    History is kept, so every later original commit stays cherry-pickable.
    Tests: console Go unit tests. **Done 2026-11-19** (aursu/console #1).
1.2 **Original UI fixes** (corrected 2026-11-19: both are web-app changes,
    not Go): `9e0416f` (s3.Delete* permissions; also bumps `jspdf` to 3.0 and
    with it `dompurify` to 3.2), `33a7fbb` (webhook regex). **Done after 1.3**:
    without a working `mds` the UI cannot be installed, tested or rebuilt, and
    a fix that is not in the committed bundle never reaches the server.
    Tests: upstream's UI checks (warnings, prettier, dead code) + `yarn test`;
    bundle rebuild in the same PR. **Done 2026-11-19** (aursu/console #5).
1.3 **UI pipeline.** `mds` dependency URL → `github.com/aursu/mds.git#v1.0.4`
    (same commit `027fb7f9`, lockfile unchanged otherwise). Rebuild the bundle
    with the original toolchain (Node 18, yarn 4.4.0) and compare with the
    committed one — proves the pipeline before anything changes.
    **Done 2026-11-19** (aursu/console #4).
1.4 **Original UI fixes and dependency commits.** Done 2026-11-20
    (aursu/console #7, #9, #12). Each of the 31 upstream commits after
    v1.7.6 has a recorded decision in the console's
    `fork/notes/03-upstream-commits-after-v1.7.6.md`.
    - Taken: `e479c7b`, `a2dfcce`, `88cd0d4` (#7); the Dependabot bumps
      `ff2b72f`, `6df2db8` (#9). The formidable bump needed our own Jest
      setup commit, because upstream had deleted its UI tests.
    - Not taken: `40bee68`. It fixes the simplified console's sidebar
      filter, which the full console does not have.
    - Superseded by 1.5: the dependency snapshots `36d0e78`, `f5ad4de`,
      `a4281ed`, `7e5f4f3`. Each conflicts in `yarn.lock`.
    - Moved to 1.6: the Go commits `6499469` (jwt) and `28b0801`.

    **Corrected 2026-11-20:** this step used to list `7e5f4f3` as removing
    `jspdf`/`react-component-export-image`. It does not; it only bumps
    dependencies. Upstream removed that package in `63c6d89`, the
    simplified console, together with the dashboard that uses it.
1.5 **UI dependencies.** Node → current LTS, yarn → latest 4.x, direct
    dependencies within majors, transitive ones via `resolutions`. Target: no
    high/critical in the shipped closure. Bundle rebuild = its own commit.
    Tests: unit + UI end-to-end suites. It also covers:
    - **`react-component-export-image`**. It ships a prebuilt jsPDF 1.5.3,
      with 15 advisories, in the bundle. Only its PNG export of dashboard
      widgets is used, and that path is unreachable for jsPDF's advisories.
      Replacing it with a maintained library or dropping the button is
      **our own decision**, since upstream only removed it along with the
      dashboard.
    - `http-proxy-middleware` → 2.0.10 (CVE-2026-55602, dev server only).
    - `testcafe` in `package.json` (3.6.2) aligned with CI's 3.7.7, and the
      CI's unpinned root `yarn add minio` moved to the lockfile.

    **Done 2026-11-20** (aursu/console #13–#22). Production audit
    (`yarn npm audit --environment production`): **99 → 0**; it was
    4 critical and 39 high before. "React Code Has No Known Vulnerable Deps"
    is green.
    - #13: `react-component-export-image` replaced by `html2canvas`
      directly (our recommendation, accepted). jsPDF leaves the bundle.
    - #14: react-router 6.30.6, lodash 4.18.1; form-data and qs refreshed.
    - #15: our own validation of the post-login `redirect-path`, an open
      redirect that react-router 6 did not fully guard.
    - #16: transitive dependencies; tar 7 through node-pre-gyp 2 and
      node-gyp 11.
    - #17: react-pdf 9.2.1 (canvas 3, N-API), so Node 18 could be left.
    - #18: **Node 26.13.1** (active LTS, the owner's choice over 24),
      **yarn 4.18.1** with its supply-chain defaults kept, corepack pinned
      in CI.
    - #19: Playwright 1.65.1 (1.49 hung installing browsers on Node 26).
    - #20: test tools pinned (minio JS 8.0.7; TestCafe 3.7.7 in CI and
      `package.json`).
    - #21: **react-router 7.18.6**, the owner's choice over an audit
      exception. Four relative navigation targets were made absolute after
      v7's splat-path change looped the login redirect.
    - #22: `http-proxy-middleware` 2.0.12, dev server only.

    Not done, for later: recharts 3 and superagent 10. The audit reports
    both majors as deprecation notices, not advisories. react-router 8
    needs React 19.
1.6 **Console Go side.** Go 1.27.4, modules aligned with the server,
    including upstream's `6499469` (jwt) and `28b0801` (Go vulnerabilities).

    **Done 2026-11-21** (aursu/console #23–#28; its `fork/notes/13`–`16`).
    `govulncheck` reachable findings: **65 → 0**; "Analysis" is green.
    - #23: toolchain **Go 1.27.4**, golangci-lint v2.15.0 with the v1
      lint scope kept, CI takes Go from `go.mod`, govulncheck runs with
      the toolchain's own `go` first on PATH (65 → 13).
    - #24: `actions/setup-go` v6, so jobs install the `toolchain` line
      instead of downloading it at run time.
    - #25: TestCafe `--browser-init-timeout` against the Firefox
      connection flake.
    - #26: upstream `6499469` (jwt/v4 4.5.2).
    - #27: `golang.org/x/net` 0.60.0 and the x/ family; the console's
      **`go` line is now 1.26.0**. Upstream `28b0801` (x/net 0.38) is
      superseded: 0.38 is still vulnerable.
    - #28: prometheus 0.311.3, grpc 1.83.1, compress 1.20.2. prometheus
      pulls the go-openapi stack (strfmt 0.26.1, validate 0.25.1, …);
      **swag 0.29.2** was taken so that no `replace` is needed. swag 0.25.4
      needs one (`go-openapi/testify/v2 => v2.0.2`, as in prometheus's
      own go.mod), and a `replace` does not reach modules that require the
      console. 96 deprecated `swag.*` calls moved to `swag/conv`,
      `netutils`, `cmdutils`.

    CI on #28: 30/30 green; Integration 86, SSO 2, permission suites 83,
    Subpath 1, playwright 6, React 41.

## Phase 2 — server (aursu/minio)

**Order changed 2026-11-21: 2.2 comes before or together with 2.1.** The
console's `go` line is 1.26.0 since aursu/console #27, so a module that
requires it needs `go >= 1.26`; the server is at `go 1.24.0`. The console's
CI jobs that embed it raise the line inside the job, which is why they pass.

2.1 Point at the full console: `replace github.com/minio/console =>
    github.com/aursu/console <commit>`. Server code untouched.
    Tests: unit, console integration, **SSO integration**.
    Already shown to build: since aursu/console #11, the console's
    Integration and SSO jobs build `aursu/minio` master with
    `replace github.com/minio/console => <console tree>`, and both pass.
    Expect the go-openapi stack to move with it (strfmt 0.26.1,
    swag 0.29.2); no `replace` beyond the console's own is needed.
    **Done 2026-11-22** (#33, note 12): `replace … => github.com/aursu/console
    v1.7.7-0.20261120232317-7832665c91d3`. The go-openapi stack had already
    moved with prometheus (#31), so no other module moved. The embedded
    console's admin API (`users`, `groups`, `policies`, `configs`,
    `admin/info`) answers 200 where the simplified one gave 404/501.
    aursu/console #29 went first: its Integration and SSO jobs now
    overwrite this replace with `go mod edit` instead of appending a
    conflicting second one.
2.2 Toolchain Go 1.24 → 1.27.4. Clears the ~33 standard-library findings.
    golangci-lint must move with it: the server's v2.11.4 is built with Go
    1.26 and refuses code that targets Go 1.27; v2.15.0 (Go 1.27.0) is the
    first that works. In the console that move added staticcheck's ST/QF
    checks; pinning `checks: [SA*, S1*]` kept the v1 scope (console
    note 13).
    **Done 2026-11-21** (#24, note 05).
2.3 Security tier: every module `govulncheck` reports (x/crypto, x/net,
    x/text, amqp091-go, prometheus, grpc, otel/sdk, go-jose, paho, compress),
    grouped by family, one PR per group. Target: 0 reachable.
    **Done 2026-11-23**: x/ family (#25, note 06), compress (#27, note
    07), grpc and otel (#28, note 08), prometheus (#31, note 10),
    amqp091-go 1.14.0 (#35, note 13), paho 1.5.1 (#36, note 14).
    `govulncheck` reachable findings: **0**; "VulnCheck" on master went green
    on `722f1fc2f` (#36's merge), the first time in this fork.
    Left, imported or required but not called: go-ntlmssp (fixed in
    0.1.1), jsonparser (1.1.2), edwards25519 (1.1.1), etcd `client/pkg/v3`
    (3.5.33). They go into 2.4.
2.4 Remaining modules within majors, in groups. First the four modules
    `govulncheck` still lists as not called (end of 2.3).

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

- UI majors stay (React 18, styled-components 5, Redux Toolkit 1): each
  is a rewrite, not a bump. (Corrected 2026-11-21: react-router was moved
  to 7 in 1.5, aursu/console #21; this line still said 6.)
- `mds` stays at v1.0.4.
- Flaws in MinIO's own code found later need our own patches again.
