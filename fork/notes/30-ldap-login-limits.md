# 30 — LDAP login limits per account and client address (CVE-2026-33419, part 2)

Date: 2026-11-25.

This is PR 5 of the seven for phase 4 item 5, stacked on note 29 (#54).
Note 29 stopped the enumeration of users. This PR limits password
guessing against one account from one client address. PRs 6 and 7 add
limits for one address against many accounts, and for one account from
many addresses.

## Why

`AssumeRoleWithLDAPIdentity` needs no authentication, and nothing
limited how often a password could be tried. `requests_max` limits
concurrency only. On the `aursu.1` release, 20 wrong passwords in a row
and then the right one all reach the LDAP server, and the last one
returns credentials. SFTP and FTP logins are the same.

## Design (agreed with Alexander, 2026-11-24/25)

Alexander asked for the Linux login model: the wait doubles after every
failure, and after N failures the account is blocked. He also asked
that every number be configurable.

- **The key is the account and the client address.** The account is its
  normalized DN. The address must be trustworthy, which is why
  #51–#53 came first.
  - A session token would not help as a key: a client decides whether
    to send it back, and the console and the SDKs keep no cookies.
  - A key by account alone would let anyone lock any user out. With the
    address in the key, a guesser locks out only their own address.
- **The waits** (`identity_ldap`, all configurable):
  - after a failure, the next try waits `login_backoff` (1s);
  - the wait grows by `login_backoff_factor` (2) with every further
    failure in a row: 1 s, 2 s, 4 s … 256 s. A factor of 1 keeps a static
    period;
  - `login_max_failures` (10) failures in a row lock the account out for
    that address for `login_lockout` (15m). `0` turns the limits off;
  - a success clears the count. Failures are forgotten `login_lockout`
    after the last wait ended.
- **An attempt reserves the wait at once,** as if it had failed.
  Otherwise a thousand parallel requests would all pass the check before
  the first failure was counted.
- **Only an invalid-credentials answer counts as a failure.** Only known
  accounts are counted, after the lookup: an unknown user has no
  password to guess, and keeps no state.
- **A refused login gets the wrong-password answer** of note 29, after
  the same delay. A distinct answer would reveal that the account
  exists, because only existing accounts are ever locked. The start of a
  lockout is logged once as an `authN` warning, with the DN and the
  address. Refusals are not logged.
- **Addresses.**
  - For STS the address is resolved through `api trusted_proxies`; for
    SFTP and FTP it is the connection peer.
  - IPv6 counts per /64, the smallest network a client is usually given,
    so a client cannot start afresh by changing its address within its
    own network.

With the defaults, one address gets at most 10 tries per account in
about 24 minutes: 511 s of waits, then a 15-minute lockout. That is
about 600 a day, where it was unlimited before.

## What (PR #55)

1. **`feat(ldap)`: login limits per account and client address.**
   - `limiter.go` holds an in-memory map from key to state, under one
     mutex.
   - The time source can be swapped, for tests.
   - Expired states are swept whenever the map has doubled since the
     last sweep, so memory follows the number of live states.
   - `Bind(username, password, clientAddr)` checks the limits after the
     lookup and records the result of the password bind. The three
     callers pass the address.
   - The four keys are in `config.go`, `help.go` and `docs/sts/ldap.md`,
     with a "Login limits" section in the docs.
2. **`test(sts)`: `TestLDAPLoginLimits`** in the LDAP suite. It sets
   `login_max_failures=3`. Three wrong passwords from one address are
   followed by the right password, refused with the wrong-password
   answer. The right password from another address passes. The test
   sends `X-Forwarded-For` from loopback, which is trusted by default.

## Client-visible change (for the release notes)

- **LDAP logins are now limited per account and client address,** with
  the defaults above.
- **During a wait or a lockout, even the right password is refused,**
  with the usual `LDAP authentication failed: user not found or invalid
  password`.
- **Behind a proxy, list it in `api trusted_proxies`.** Otherwise every
  client has the proxy's address, and one guesser locks an account out
  for everyone.

## Alternatives rejected

- **Answer a lockout with 429 or "too many attempts".** That would tell
  that the account exists, since unknown users are never locked out.
- **Keep the state in the cluster** (shared through the peers). That
  would be harder and slower on every login. Each server counts on its
  own, so a cluster of N servers allows N times the tries. The note and
  the docs say so.
- **Sleep through the wait instead of refusing.** The waits grow to
  minutes. A sleeping request holds a goroutine and the client's
  connection, and outlives proxy timeouts.

## Tests (media-gw, golang:1.27.4, `-tags kqueue,dev`)

- **Unit, with a fake clock (`limiter_test.go`, 9 tests):**
  - the whole backoff and lockout sequence (1 s … 256 s, the lockout,
    and the count restarting after it);
  - a static period;
  - the cap at `login_lockout`;
  - concurrent attempts refused by the reservation, including the
    reserved lockout;
  - a success forgetting the failures;
  - the keys: another address or account is not affected, mapped IPv4
    is IPv4, IPv6 counts per /64;
  - forgetting after `login_lockout`;
  - the limits turned off, and a nil limiter;
  - the sweep.

  `TestLoginError` gains the refusal (ordinary) and the lockout start
  (not ordinary, so logged). All of it runs under `-race`.
- **LDAP suites with `quay.io/minio/openldap`:** both pass on all four
  server types, with `TestLDAPLoginLimits`. Two mutations of the limiter
  fail the integration test:
  - never refusing: "right password during the lockout was not refused
    alike";
  - a key without the address: "right password from another address was
    refused".
- **Lint and build:** golangci-lint v2.15.0 reports 0 issues, after an
  ineffectual assignment in a test was fixed. `go build ./...` passes.
- **Live** (scratch `limit/e2e.sh`). OpenLDAP; `login_max_failures=3`,
  `login_backoff=0s`, `login_lockout=10m` on the branch. Clients A and B
  run in containers with fixed, different addresses.

  | step | `aursu.1` | this branch |
  |---|---|---|
  | A: 20 wrong passwords | 20 × "LDAP auth failed for DN uid=dillon,…" | 20 × the one message |
  | A: then the right password | **credentials** | refused, the same message |
  | B: the right password | credentials | credentials |
  | lockouts in the server log | — | 1 |

  The first run gave B the address A had just released, because Docker
  reuses a freed address. The clients now have fixed addresses.

## Not in this change

- **One address against many accounts** (password spraying) is PR 6.
- **One account from many addresses** (a botnet) is PR 7.
- **Sharing the state across a cluster** is not planned; see above.
- **Service-account secrets on SFTP and FTP** are still unlimited. They
  are outside this CVE, and will be offered separately.
