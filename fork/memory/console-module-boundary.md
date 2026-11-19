---
name: console-module-boundary
description: The web console is the Go module github.com/minio/console; the server only embeds it, and the server↔console interface did not change between the two releases.
metadata:
  type: project
---

The web UI is not in this repo. It is the Go module `github.com/minio/console`
(later renamed "object-browser"), which embeds the React build and is imported
by the server in `cmd/api-router.go`, `cmd/common-main.go`, `cmd/globals.go`:
packages `api`, `api/operations`, `pkg/auth/idp/oauth2`, `pkg/certs`.

| Server tag | console version |
|---|---|
| RELEASE.2025-04-22T22-12-26Z | `v1.7.6` (full console) |
| RELEASE.2025-10-15T17-29-55Z | `v1.7.7-0.20250905210349-2017f33b26e1` (= object-browser v2.0.4, stripped) |

The stripping happened in the console module (its v2.0.0, server commit
`8cad40a48`, 2025-05-09), not in the server. Verified 2026-11-19:

- the server's import set and call sites are identical in both tags;
- no admin, S3 or STS route was removed between the tags;
- the server still builds an OpenID provider config for the console
  (`buildOpenIDConsoleConfig` in `cmd/common-main.go`) — the stripped console
  ignores it;
- `madmin-go/v3` is `v3.0.109` in both tags, so console v1.7.6 already built
  against today's admin client in the April release.

Related: [[console-source-go-proxy]].
