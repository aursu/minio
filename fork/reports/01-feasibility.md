# 01 — Feasibility: full console on RELEASE.2025-10-15T17-29-55Z

Date: 2026-11-19. Status: assessment only, no code changed.

## Question

Bring every console feature removed after RELEASE.2025-04-22T22-12-26Z
(SSO/OIDC above all) into RELEASE.2025-10-15T17-29-55Z.

## Findings

1. **The removal happened outside the server.** The console is the Go module
   `github.com/minio/console`; it went from `v1.7.6` to a stripped v2.0.x
   between the two tags. Details: [console-module-boundary](../memory/console-module-boundary.md).
2. **The server side is intact.** Same console API surface, no removed routes,
   same `madmin-go`. OIDC/STS got fixes (`b44b2a090`, `0638ccc5f`, `d0f50cdd9`)
   and LDAP got fixes (`7a80ec1cc`), all worth keeping.
3. **Scale of the removal:** console v1.7.6 has 795 Go files and 337 UI source
   files; the stripped one has 231 and 129. Account/policy, bucket, config,
   monitoring, IDP/SSO management and SSO login are gone.
4. **What changed after v1.7.6 that is not removal** is small: dependency bumps
   for vulnerabilities (Go: `golang-jwt/jwt/v4` 4.5.2, `x/crypto`, `x/net`,
   `x/oauth2`, `minio/pkg`, `madmin-go`, `minio-go`; npm: `react-router-dom`,
   `react-pdf`, overrides), a download-preview fix, a "Versions mode" tweak,
   branding/licence text.
5. **The upstream console repos are gone, the original history is not.**
   `bexsoft/console` and `bexsoft/mds` carry it; the console commits match the
   Go proxy zips and upstream `go.sum`, the mds commits are the ones the
   original lockfiles pin. Original code only, no community forks. Details:
   [console-source-go-proxy](../memory/console-source-go-proxy.md).

## Conclusion

Feasible, and cheaper than the question suggests. Porting ~420 removed files
forward into the stripped console would be the hard way. The easy way runs in
the other direction: take the **full console v1.7.6** and carry the few
post-v1.7.6 fixes onto it. The server needs only a `go.mod` change.

## Plan (each step is one reviewable change)

Superseded by [02-plan](02-plan.md); kept as first written.

1. Import console v1.7.6 verbatim into the fork. Mechanical; verified by its
   `h1:` hash, not by reading it.
2. Point the server at it (`replace` + `require v1.7.6`), build in Docker.
3. Smoke test in Docker: server + a throwaway OIDC IdP (Keycloak or Dex); SSO
   login in the console; walk the restored admin screens.
4. Carry the Go dependency/security bumps from item 4, one per change.
5. UI: start from the prebuilt bundle; npm bumps need a rebuild, which uses
   original `mds` from the mirror, pinned by commit.
6. Release build (image/tag naming) — decided when step 3 is green.

## Risks

- Admin screens built for the April server may hit behaviour that changed by
  October. Found by step 3, not predictable from the diff (none of the routes
  changed).
- The prebuilt UI carries the npm dependency state of v1.7.6 (Feb 2025).
  Acceptable for a test store; not for an internet-facing one.
- Licence: console and server are AGPLv3; a fork is fine, publishing it means
  publishing the source, which this fork does anyway.

## Decision (2026-11-19)

The original history is pushed to `aursu/console` and `aursu/mds` (`master`
plus original tags, nothing else). The fork works on a branch of
`aursu/console` cut from `v1.7.6`; the server points at it with a `replace`.
This supersedes the earlier option of importing a zip into a `console/`
subdirectory: history makes every forward-ported fix a cherry-pick of an
original commit.
