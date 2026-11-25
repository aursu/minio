# 32 — LDAP login: lock out an account that fails from many addresses (CVE-2026-33419, part 4)

Date: 2026-11-25.

This is PR 7, the last of the seven for phase 4 item 5, stacked on
note 31 (#56).

## Why

Notes 30 and 31 limit per address. Many addresses, each staying under
its own limits, can still guess one account's password together. An
IPv6 /48 alone holds 65,536 /64 networks. Alexander asked for a limit of
"100 failures in 15 minutes" per account, configurable.

## What (PR #57)

1. **`feat(ldap)`: lock out an account that fails from many addresses.**
   - **The rule.** One account that fails `login_max_failures_per_account`
     (100) times within `login_account_window` (15m), from any addresses,
     is locked out for every address for `login_account_window`. `0`
     turns this limit off. Both keys are in `identity_ldap`.
   - **This is the one limit that also locks the owner out.** Hence its
     high default: with the other defaults it takes at least ten
     addresses to reach 100 failures in 15 minutes.
   - **A success does not clear the count.** Otherwise the owner logging
     in during an attack would give the guessers a fresh 100.
   - **The answer and the log** are as before: the refusal is the
     wrong-password answer, and the start of the lockout is logged once.
   - **State.** The failure times are kept per account, at most
     `login_max_failures_per_account` of them. They are swept with the
     other states, and the sweep counts all three maps.
   - **Naming.** The per-address check is now `pairLimit`, and
     `accountLimit` is this one.
2. **`test(sts)`: `TestLDAPLoginAccountLimit`** in the LDAP suite. With
   `login_max_failures_per_account=3`, one wrong password for `dillon`
   comes from each of three addresses. The right password for `dillon`
   is then refused from a fourth address, while `svc.algorithm` passes
   from there.

## Client-visible change (for the release notes)

- **100 failed logins of one LDAP account within 15 minutes, from any
  addresses, lock that account out for 15 minutes,** for everyone,
  including its owner.
- **Guessers with enough addresses can therefore keep an account locked.**
  Raise `login_max_failures_per_account`, or set it to 0, where that
  matters more than guessing.

## The limits together (PRs 4–7)

| guesser | limit | defaults |
|---|---|---|
| one address, one account | wait doubling, then lockout of that pair | 1 s ×2 …, 10 failures, 15 min |
| one address, many accounts | lockout of that address | 10 accounts in 15 min, 15 min |
| many addresses, one account | lockout of that account | 100 failures in 15 min, 15 min |
| every case | one answer, one answer time | "user not found or invalid password", 1 s |

All the limits are per server and in memory, as note 30 says. A guesser
with many addresses spread over many accounts is limited only by the
per-pair waits and the answer floor. Password strength and SSO remain
the defence there.

## Alternatives rejected

- **Clear the count on success.** The owner's login during an attack
  would reset it for the guessers.
- **Lock out until an administrator unlocks.** The lockout would last
  indefinitely for a disabled user's owner, and MinIO has no unlock
  command for LDAP users. A window ends on its own.

## Tests (media-gw, golang:1.27.4, `-tags kqueue,dev`)

- **Unit (`limiter_test.go`).**
  - The lockout after 3 failures from 3 addresses; another account is
    not affected, and the lockout ends.
  - The window.
  - A success not clearing the count.
  - The limit alone.
  - The account sweep.

  All of them run under `-race`.
- **LDAP suites with `quay.io/minio/openldap`:** both pass with all four
  new LDAP tests. Two mutations fail the new one:
  - the account lockout never checked;
  - account states keyed by address.
- **Lint and build:** golangci-lint v2.15.0 reports 0 issues, and
  `go build ./...` passes.
- **Live** (scratch `acct/e2e.sh`). OpenLDAP,
  `login_max_failures_per_account=3` on the branch. Clients A–D have
  fixed, different addresses.

  | step | `aursu.1` | this branch |
  |---|---|---|
  | A, B, C: one wrong password each for `dillon` | "LDAP auth failed for DN …" ×3 | the one message ×3 |
  | D: the right password for `dillon` | **credentials** | refused, the same message |
  | D: the right password for `svc.algorithm` | passes the login (no policy attached) | the same |
  | lockouts in the server log | — | 1 |

## Next

All seven PRs of item 5 are open, #51–#57, each stacked on the one
before. Alexander merges them top-down into their base branches. #51
then carries the stack into `master`, and its full CI runs once.
