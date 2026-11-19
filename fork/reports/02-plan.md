# 02 — Plan: latest server, full console, current dependencies

Date: 2026-11-19. Builds on [01-feasibility](01-feasibility.md).

## Bases

| Repo | Work branch | Cut from | Why |
|---|---|---|---|
| `aursu/minio` | `full-console` | `upstream/master` `7aac2a2c5` | = RELEASE.2025-10-15T17-29-55Z + later upstream fixes, among them `fa18589d1` (PostPolicy did not enforce tag conditions — security), `52eee5a2f` (double response), `18f97e70b` (conditional-put quorum) |
| `aursu/console` | `full-console` | `v1.7.6` `f4a08fc0` | last full console |
| `aursu/mds` | — | `v1.0.4` `027fb7f9` | pinned by console `yarn.lock`; no changes planned |

`master` in every repo stays an exact mirror of upstream.

## Working rules

- One step = one reviewable commit. Mechanical output (`go.sum`, the UI
  bundle in `web-app/build`, 244 tracked files) is committed alone and says
  how to reproduce it.
- Every code change gets a note in `fork/notes/`: what, why, why not the
  obvious alternative.
- Everything builds and runs in Docker. Each phase ends with a report.
- Dependency bumps stay within the major version; a major bump is its own
  decision, listed under residual risk until taken.

## Phase A — baseline (measure, change nothing)

1. Build `upstream/master` unchanged in Docker (`golang:1.24`) — proves the
   build environment.
2. Vulnerability baseline: `govulncheck` on the server built with console
   v1.7.6; `yarn npm audit` on console v1.7.6 `web-app`; GitHub advisories for
   `minio/minio` published after 2025-10-15. → report 03.

## Phase B — full console on the latest server

3. Server `go.mod`: `replace github.com/minio/console => github.com/aursu/console`
   at the `full-console` head; build. Server code is not touched.
4. Smoke harness (compose): server + throwaway Keycloak + `mc`. Pass when:
   SSO login works, the restored admin screens (users, groups, policies,
   buckets, config, IDP, monitoring) load and act, the S3 path is unaffected.

## Phase S — server security patches (own code; added after [03-baseline](03-baseline.md))

Seven advisories against MinIO's own code are open in `minio/minio` and fixed
only in closed AIStor. Each is fixed here by a patch of our own:

1. Write a regression test that reproduces the exploit — it must fail on the
   unchanged tree.
2. Smallest fix that makes it pass; the full unit suite and the integration
   suites stay green.
3. Note in `fork/notes/` citing the advisory, what the fix does, and why not
   the alternative.

Order: CVE-2026-33322 (OIDC — the feature this fork exists for),
CVE-2026-40344 + CVE-2026-41145 (unauthenticated write), CVE-2026-34204,
CVE-2026-39414, CVE-2026-33419 (LDAP), CVE-2026-42600 (distributed only).

## Phase C — original console fixes after v1.7.6

Cherry-picks from `aursu/console` `master`, one per commit:

| Take | Commits |
|---|---|
| fixes made on the full console | `9e0416f` s3.Delete*, `33a7fbb` webhook regex |
| functional fixes (if they apply) | `a2dfcce` preview download, `e479c7b` 5 GiB threshold, `40bee68` bucket search, `88cd0d4` versions "+" |
| dependency/security (in D and E) | `6499469`, `28b0801`, `36d0e78`, `f5ad4de`, `a4281ed`, `ff2b72f`, `6df2db8`, `7e5f4f3` |

Not taken: the removal (`63c6d89`, `71b1b70`, `e2bbf91`, `3dc0fdc`,
`7871f6b`), branding/licence/README, release commits (version and bundle only).

## Phase D — Go dependencies

The binary's module versions are decided by the **server** `go.mod` (MVS
takes the highest requirement), so bumps land there; the console `go.mod`
follows so its own tests run on the same set.

5. Toolchain Go 1.24 → 1.27.4 (current, 2026-11-19), both repos. Fixes
   standard-library CVEs on its own.
6. Security tier: everything `govulncheck` reports, plus upstream's unmerged
   Dependabot branches — `x/crypto` 0.45.0, `go-jose/v4` 4.1.4,
   `nats-server/v2` 2.11.15, `otel/sdk` 1.43.0, `paho.mqtt.golang` 1.5.1,
   `go-ntlmssp` 0.1.1 — grouped by module family.
7. MinIO's own libraries (`minio-go/v7`, `madmin-go/v3`, `pkg/v3`) to their
   latest original releases within the major.
8. The rest, `go get -u` within majors, in groups (`x/*`, cloud SDKs, otel,
   messaging targets, others). Gate after each group: build, `go vet`, IAM /
   STS / OpenID / LDAP package tests, smoke harness.
   Target: `govulncheck` reports nothing reachable.

## Phase E — UI dependencies

9. Reproduce the v1.7.6 bundle with its own toolchain (Node 18, yarn 4.4.0,
   `mds` v1.0.4 from `aursu/mds`) and compare with the tracked bundle — proves
   the pipeline before anything changes.
10. Cherry-pick the original UI dependency commits from Phase C; conflicts
    expected where they were made after the removal.
11. Node to the current LTS, yarn latest 4.x.
12. Direct dependencies within majors; transitive ones via `resolutions`.
    Target: no high/critical in `yarn npm audit`. Rebuild bundle (own commit).

## Phase F — release

13. Multi-stage Dockerfile (UI → Go → minimal runtime); version string marks
    the fork. Image registry and tag naming decided here.

## Testing (every change)

| Level | Suite | When |
|---|---|---|
| unit | server `go test ./...`; console `go test ./api/... ./pkg/...` | every commit; compared with the baseline in report 03 |
| unit, targeted | new regression test per security patch | Phase S |
| integration | console `integration/` (live server + PostgreSQL) | every server or console change |
| integration, SSO | console `sso-integration/` (Dex + OpenLDAP) | console, OIDC, dependency changes |
| integration, IAM | server `make test-iam`, `test-iam-import-with-openid` | IAM/STS/OIDC/LDAP changes |
| UI end-to-end | console `web-app/tests` (TestCafe/Playwright) | UI bundle rebuilds |

A test that fails on the baseline is recorded there and does not block; a test
that passed on the baseline and fails after a change blocks that change.

## Residual risk (accepted unless decided otherwise)

- Server code receives no upstream fixes after 2026-02 (maintenance mode).
  Known advisories are covered by Phase S; flaws found later in AIStor will
  again need our own patches.
- UI majors stay: React 18, react-router 6, styled-components 5, Redux
  Toolkit 1. Each is a rewrite-sized change, not a bump.
- `mds` stays at v1.0.4; its later versions grew with the stripped UI.
