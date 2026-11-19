---
name: server-cves-aistor-only
description: MinIO server CVEs published after 2025-10 are fixed only in closed AIStor; the community tree (our base) stays vulnerable unless the fork patches them itself.
metadata:
  type: gotcha
---

As of 2026-11-19, seven advisories against MinIO's own server code list
upstream `master` `7aac2a2c5` as affected and name only MinIO AIStor releases
as fixed: CVE-2026-33322 (OIDC JWT algorithm confusion, critical),
CVE-2026-33419 (LDAP brute-force, critical), CVE-2026-40344 and
CVE-2026-41145 (unauthenticated write via unsigned-trailer uploads),
CVE-2026-34204 (SSE metadata injection), CVE-2026-39414 (S3 Select CSV OOM),
CVE-2026-42600 (ReadMultiple path traversal, distributed only).

`govulncheck` does **not** report these: it skips the main module. Check
OSV for `github.com/minio/minio` (Go ecosystem) and the GitHub advisories
directly:

    curl -s -X POST https://api.osv.dev/v1/query \
      -d '{"package":{"name":"github.com/minio/minio","ecosystem":"Go"}}'

There is no original fix to cherry-pick. Each patch here is our own code, with
a regression test that reproduces the exploit first. Re-run the OSV query
before every release: new AIStor-only advisories keep appearing.

Details and status: `fork/reports/03-baseline.md`, plan phase S.
Related: [[console-module-boundary]].
