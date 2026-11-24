# 29 — LDAP login: one answer, one response time (CVE-2026-33419, part 1)

Date: 2026-11-25.

This is PR 4 of the seven for phase 4 item 5, stacked on note 28 (#53).
It is the first half of the CVE fix: the server no longer tells which
users exist. PRs 5–7 add the limits on password guessing.

## Why

`POST /?Action=AssumeRoleWithLDAPIdentity` needs no authentication
(GHSA-jv87-32hw-hh99, critical). Its answer to a failed login differed
by cause, as on the `aursu.1` release:

- **an unknown user:** `LDAP server error: Unable to find user DN: User
  DN not found for: nosuchuser`;
- **a known user with a wrong password:** `LDAP server error: LDAP auth
  failed for DN uid=dillon,ou=people,ou=swengg,dc=min,dc=io: LDAP Result
  Code 49 "Invalid Credentials"`.

Anyone could list the users, with their DNs. The unknown user was also
answered without the password bind, so a slower LDAP server would show
the difference in time as well. The SFTP and FTP logins call the same
`Bind`.

The third-party patches (granary, pgsty, jarvit) were read as a
checklist of affected code paths, following the rule of 2026-11-24.
jarvit's names the SFTP and FTP paths. The code here is ours.

## What (PR #54)

1. **`fix(ldap)`: one answer and one response time.**
   - **`Bind` returns a `*LoginError` for every failure that depends on
     the user:** an unknown, ambiguous or unsearchable user, or a refused
     password bind. Its message is always `ErrLoginFailed`, "LDAP
     authentication failed: user not found or invalid password", and
     `errors.Is(err, ErrLoginFailed)` holds.
   - **The text keeps "not found".** The console tries its built-in
     login (root, access keys) only when the LDAP error contains "not
     found" (console `api/client.go`). Without those words, root could
     not log in to the console while LDAP is enabled.
   - **A failed login takes at least `identity_ldap
     login_failure_delay`** (`MINIO_IDENTITY_LDAP_LOGIN_FAILURE_DELAY`,
     default `1s`), which hides the missing password bind. It must be
     above the slowest normal lookup and bind. A negative or invalid
     value fails the config.
   - **The cause stays in the error, for the server only.** A cause that
     is not an unknown user or a wrong password (an ambiguous user, a
     bind refused for another reason, a lost connection) is logged as an
     `authN` warning, because an administrator should see it. Ordinary
     failures are not logged, so that password guessing does not flood
     the log.
   - **Failures of the LDAP server itself still say "LDAP server
     error".** These are the connection and the lookup bind; they do not
     depend on the user.
   - **The same applies to STS, SFTP and FTP.** `docs/sts/ldap.md`
     documents the key and the answer.
2. **`test(sts)`: `TestLDAPLoginFailure`** in the LDAP suite. An unknown
   user and a wrong password must give the same message, with "not
   found" and without the user, and both must take at least the delay,
   within 300 ms of each other.

## Client-visible change (for the release notes)

- A failed LDAP login (STS, SFTP, FTP) always answers `LDAP
  authentication failed: user not found or invalid password`. It no
  longer says why or names the DN. Administrators find unusual causes
  in the server log.
- A failed LDAP login takes 1 s by default (`login_failure_delay`).
- **Logins that try LDAP first pay the delay when they fall back.** The
  console tries LDAP before its built-in login. SFTP, for a user name
  without `=ldap` or `=svc`, tries LDAP before the built-in users. With
  LDAP enabled, a root or built-in-user login through either path takes
  about 1 s longer. `=svc` on SFTP skips LDAP and the delay.

## Alternatives rejected

- **A decoy password bind for unknown users.** An LDAP server can answer
  a bind to a missing DN faster than a password check, so the time would
  still differ. A floor does not depend on the server.
- **A message without "not found".** The console's root fallback depends
  on those words. Changing the console instead would mean a console PR
  and a dependency bump for no gain, since the message names no cause
  either way.
- **Logging every failed login.** An attacker would fill the log; the
  audit log already records each STS request.

## Tests (media-gw, golang:1.27.4, `-tags kqueue,dev`)

- **Unit:** `TestLoginError` checks the message, `errors.Is`, and
  `Ordinary` for the four causes.
- **LDAP suites, run as CI runs them** (scratch `ldap/run-ldap.sh`,
  `quay.io/minio/openldap`). `TestIAMWithLDAPServerSuite` and
  `TestIAMWithLDAPNonNormalizedBaseDNConfigServerSuite` pass on all
  four server types.
  - **The new test on the code without the fix** fails with exactly the
    two messages quoted above.
  - **Without the delay** (mutation), an unknown user is answered in
    about 1.1 ms, and the test fails.
- **Lint and build:** golangci-lint v2.15.0 reports 0 issues, and
  `go build ./...` passes.
- **Live** (scratch `ldap/e2e.sh`): OpenLDAP, a MinIO image, raw STS
  calls, 5 tries each, then console logins.

  | check | `aursu.1` | this branch |
  |---|---|---|
  | STS, unknown user | "User DN not found for: nosuchuser", 1.2–1.4 ms | one message, 1.001–1.003 s |
  | STS, wrong password | "LDAP auth failed for DN uid=dillon,…", 1.3 ms | the same message, 1.001–1.002 s |
  | console, root (built-in login) | 204 | 204 |
  | console, LDAP user | 204 | 204 |
  | console, LDAP user, wrong password | 401 | 401 |

  Against this local OpenLDAP the timing difference on `aursu.1` is small
  (about 0.1 ms). The message alone gave the users away.

## Not in this change

- **Password guessing is still unlimited.** PRs 5–7 add the limits per
  account and client address, per address and per account. They will
  return the same message.
- **Service-account secrets on SFTP and FTP** are checked without a
  limit too. That is outside this CVE, and will be offered separately.
