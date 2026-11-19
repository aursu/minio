# Fork memory index

One line per fact file. Update the file, not just this line.

- [Console is a separate module](console-module-boundary.md) — the whole web UI is `github.com/minio/console`; the server uses 4 of its packages, same in both releases.
- [Server CVEs fixed only in AIStor](server-cves-aistor-only.md) — 7 open advisories on our base (OIDC JWT, LDAP, unsigned-trailer writes…); `govulncheck` skips the main module — query OSV; fixes are our own code (2026-11-19).
- [Server tests in Docker](server-tests-in-docker.md) — run as non-root and `rm /.dockerenv`, or 7 false failures and a forever hang in `TestCreateServerEndpoints` (2026-11-19).
- [Original console source](console-source-go-proxy.md) — upstream `console`/`mds` repos are gone (2026-11-19); **original code only**; history in `bexsoft/*` mirrors, proven against proxy zips + upstream `go.sum`; no community forks.
