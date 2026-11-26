# 07 — Release RELEASE.2026-11-26T08-33-33Z.aursu.2

Date: 2026-11-26. Plan 04 phase 4, items 3–6.

| | |
|---|---|
| Tag | `RELEASE.2026-11-26T08-33-33Z.aursu.2` (annotated) on `master` `aacdfa209` (#55's merge) |
| Image | `ghcr.io/aursu/minio:RELEASE.2026-11-26T08-33-33Z.aursu.2`, also `:latest` |
| Digest | `sha256:600c339316ce0c127704fea2393a837942b16f149068cd8468b94a37329be185` (both tags) |
| Package | public: the manifest is served to an anonymous token |
| `minio --version` | `RELEASE.2026-11-26T08-33-33Z.aursu.2 (commit-id=aacdfa209fcd…)`, `go1.27.4 linux/amd64` |
| Workflow | "Release image" run `45784219881`: build, version stamp, console login through Keycloak, publish; all green, 5 min |
| Since `aursu.1` | #47–#62 and #64 (#63 was a CI control, closed), 90 commits |
| Dependencies | `go.mod` and `go.sum` unchanged since `aursu.1`; console still `aursu/console` `7832665c91d3` |

## What is in it

- **Security fixes (phase 4 items 3–6).**
  - CVE-2026-34204: SSE metadata through replication headers is accepted only from authorized replication. Fixed by #48, granary `be6063e8819f` (note 23).
  - CVE-2026-39414: S3 Select CSV and JSON Lines read a line of at most 1 MiB instead of the whole object. Fixed by #49 and #50 (notes 24, 25).
  - CVE-2026-33419: LDAP login. #54 makes a failed login give one answer and take at least 1 s. #55–#57 limit logins per account and address, per address, and per account (notes 29–32).
  - CVE-2026-42600: the storage REST `ReadMultiple` call is removed. Fixed by #64 (note 35).
- **Our own fix: the client address.** It is now taken from a forwarding header only behind a trusted proxy. Before, one forged header was enough to pass an `aws:SourceIp` condition, which the LDAP limits also depend on.
  - #51: the fix itself.
  - #52: the `Forwarded` header.
  - #53: the scheme.
  - #58, #59, #61, #62: review follow-ups. Loopback and the cluster nodes are always trusted.
  - Notes 26–28, 33, 34.
- **Tests.** #60 isolates the api config tests from the environment.

## Changes that clients and operators see

**Behind a proxy**
- The client address comes from a forwarding header only when the connection comes from a trusted proxy. It is used by `aws:SourceIp`, the audit log, notifications and the LDAP login limits.
  - Loopback and the deployment's own nodes are always trusted.
  - Every other proxy must be listed in `api trusted_proxies` (`MINIO_API_TRUSTED_PROXIES`): a load balancer, a federated deployment, a site replication peer.
  - Until it is listed, the client address is the proxy's.
- The header is configured, not guessed. `api trusted_proxy_header` takes `x-forwarded-for` (the default), `x-real-ip` or `forwarded` (RFC 7239). The proxy must append its peer to the header (nginx: `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;`).
- `Location` URLs say `https` behind a TLS-terminating proxy only when that proxy is trusted.
- `_MINIO_API_XFF_HEADER` is removed.

**LDAP logins (STS, console, SFTP, FTP)**
- A failed login always answers `LDAP authentication failed: user not found or invalid password`. It takes at least 1 s (`identity_ldap login_failure_delay`). Unusual causes go to the server log.
- With LDAP enabled, some built-in logins take about 1 s longer, because LDAP is tried first:
  - root or built-in-user logins through the console;
  - SFTP logins without `=svc`.
- Failed logins are limited:
  - per account and address, with a growing wait;
  - per address: one address failing on 10 accounts within 15 minutes is locked out for 15 minutes;
  - per account: 100 failures of one account from any addresses lock it out for 15 minutes.
- During a wait or a lockout even the right password is refused. Every number is configurable, and 0 turns a limit off.
- Behind a proxy that is not listed, all clients share the proxy's address, and so its limits.

**S3 Select**
- An input line over 1 MiB fails with `OverMaxRecordSize`.
- Errors in the middle of a stream carry their own S3 code instead of `InternalError`.
- Records produced before an error are delivered before the error event.
- A corrupt compressed JSON Lines input fails the query instead of returning a truncated result.

**Metrics**
- The drive metrics no longer list `ReadMultiple`.

## Vulnerability state at the tag

| Check | Result |
|---|---|
| `govulncheck` v1.10.0, `-show verbose ./...`, Go 1.27.4, on `b0a67e614` (#64; #55 changes no dependency) | **No vulnerabilities found** |
| UI: `yarn npm audit --recursive --environment production --no-deprecations` on `aursu/console` `7832665c91d3` (Node 26.13.1, corepack 0.36.0) | **No audit suggestions** |
| OSV, `github.com/minio/minio`, published since 2025-10-15 | the same 8 CVEs as at `aursu.1`, **none new** |

| CVE | State |
|---|---|
| CVE-2025-62506 | fixed upstream, in the base |
| CVE-2026-33322 | fixed in `aursu.1` (#43) |
| CVE-2026-40344, CVE-2026-41145 | fixed in `aursu.1` (#44) |
| CVE-2026-34204 | **fixed** (#48) |
| CVE-2026-39414 | **fixed** (#49, #50) |
| CVE-2026-33419 | **fixed** (#54–#57) |
| CVE-2026-42600 | **fixed** (#64) |

OSV still lists all of them as affecting `github.com/minio/minio`. The advisories describe upstream, not this fork.

## Check of the published image

All checks ran on media-gw with the image pulled from ghcr.

**CVE-2026-42600** (scratch `cve42600/e2e.sh`). Two nodes. A client mints the internode token from the root credentials and asks node 2 for `../tmp/outside`:

| image | answer |
|---|---|
| `aursu.1` | HTTP 200, the response carries the file |
| `aursu.2` | HTTP 426, no such call |

**Client address** (scratch `trusted/e2e.sh`). A bucket allows anonymous `GetObject` only from `203.0.113.7` (`aws:SourceIp`). A client in another container forges that address:

| request | default | list = client subnet | same, `x-real-ip` |
|---|---|---|---|
| no header | 403 | 403 | 403 |
| `X-Forwarded-For: 203.0.113.7` | 403 | 200 | 403 |
| `X-Real-IP: 203.0.113.7` | 403 | 403 | 200 |
| `Forwarded: for=203.0.113.7` | 403 | 403 | 403 |
| `X-Forwarded-For: 203.0.113.7, 198.51.100.9` | 403 | 403 | 403 |
| loopback, no header | 403 | 403 | 403 |
| loopback, `X-Forwarded-For: 203.0.113.7` | 200 | 200 | 403 |

The `aursu.1` column of note 26 answered 200 to all four forged headers.

**LDAP login limits** (scratch `limit/e2e.sh`, `login_max_failures=3`, `login_backoff=0s`, `login_lockout=10m`, OpenLDAP test server). Client A sends 20 wrong passwords, then the right one. Client B, from another address, sends the right one:

| | `aursu.1` | `aursu.2` |
|---|---|---|
| A, 20 wrong passwords | `LDAP server error: LDAP auth failed for DN uid=dillon,…: Invalid Credentials` | `LDAP authentication failed: user not found or invalid password` |
| A, then the right password | credentials | refused, the same message |
| B, the right password | credentials | credentials |
| lockouts in the server log | 0 | 1 |

## CI on the way

- **#54: Resiliency failed four attempts of one CI run.**
  - The failures were in the healing checks.
  - A fresh run passed 22/22, and the same tree passed 8 of 9 runs on media-gw. `master` passed 3 of 5 there, so the flake hits `master` too.
  - We could not find out why that run failed every time.
- **#55: `TestStorageRESTClientListDir` failed once.** The rerun passed, and 100 runs on media-gw passed.

Both are recorded in the fork's CI notes.

## Next

- **Deferred (Alexander, 2026-11-26: not now): confine the volume and path arguments of every storage RPC.**
  - `getVolDir` rejects only `""`, `.` and `..`, and the grid storage RPCs bypass the request validity middleware.
  - So a holder of the internode token can reach files outside the drive through other calls as well. This is the same attacker as CVE-2026-42600.
  - There is no advisory of its own. Checklist: relvo-labs/granary #82 (note 35).
- **Offered earlier, not done:**
  - console: send `X-Real-IP` and `Forwarded`, not only `X-Forwarded-For`;
  - limits on SFTP/FTP service-account secrets;
  - `X-Forwarded-Host` and `X-Forwarded-Port`;
  - Dependabot alerts.
