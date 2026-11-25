# 31 — LDAP login: lock out an address that fails on many accounts (CVE-2026-33419, part 3)

Date: 2026-11-25.

This is PR 6 of the seven for phase 4 item 5, stacked on note 30 (#55).

## Why

The limits of note 30 count per account and address. Password spraying
tries one common password on many users, a few tries per user, so it
never reaches `login_max_failures` on any one account. On `aursu.1`,
and with #55 alone, it is not limited.

Alexander's design (2026-11-24): "if more than 10 sessions accumulate,
we go home". Applied to one address, that is the rule here.

## What (PR #56)

1. **`feat(ldap)`: lock out an address that fails on many accounts.**
   - **The rule.** An address that fails on `login_max_accounts_per_address`
     (10) different accounts within `login_address_lockout` (15m) is
     locked out for every account for `login_address_lockout`. `0` turns
     this limit off. Both keys are in `identity_ldap`.
   - **The window.** A failed account counts against its address for
     `login_address_lockout`.
   - **A success takes only its own account off the address.** The
     accounts that failed before still count, so a guesser cannot clear
     them by logging in with an account of its own.
   - **The answer and the log** are as in note 30. The refusal is the
     wrong-password answer, and the start of the lockout is logged once.
     `failed()` now returns a description of the lockouts it started, for
     that log line, instead of a boolean.
   - **IPv6 counts per /64.**
   - **Each limit works with the other turned off.** Address states are
     swept together with the account states.
2. **`test(sts)`: `TestLDAPLoginAddressLimit`** in the LDAP suite. With
   `login_max_accounts_per_address=2`, one wrong password for `dillon`
   and one for `svc.algorithm` from one address are followed by the
   right password for `dillon`, refused there. From another address it
   passes.

## A defect of #55 found here

The new sweep test showed that `attempt` added a state to the map and
then swept, before the state's wait was set. A sweep at that moment
dropped the new state as expired, and with it the reservation. The
same order in `failed` would have dropped a new address state. The fix
is on #55 (`fix(ldap): sweep before adding a login state, not after`),
with a correction in note 30. This branch is rebased onto it, and
`failed` sweeps before adding as well.

## Client-visible change (for the release notes)

- **One address failing on 10 different LDAP accounts within 15 minutes
  is locked out for 15 minutes,** for every account, with the usual
  failure message.
- **Many users behind one NAT address share this limit.** Raise it, or
  set it to 0, if that is a problem.

## Alternatives rejected

- **A global limit** (all addresses together, Alexander's first idea).
  One guesser would lock everyone out. With trustworthy addresses
  (#51–#53) the limit can be per address.
- **Counting unknown users against the address.** There is no password
  to guess for them; the 1 s answer floor of note 29 already slows
  trying names.

## Tests (media-gw, golang:1.27.4, `-tags kqueue,dev`)

- **Unit (`limiter_test.go`).**
  - The lockout after 3 accounts with the limit at 3. The same account
    twice counts once, and another address does not count. The IPv6 /64
    is covered, and the lockout ends.
  - The window.
  - A success removes only its own account.
  - Each limit with the other turned off.
  - The address sweep.

  All of them run under `-race`.
- **LDAP suites with `quay.io/minio/openldap`:** both pass with the new
  test. Two mutations fail it:
  - the address lockout never checked: "right password from the
    locked-out address was not refused alike";
  - address states keyed by account: the same message.
- **Lint and build:** golangci-lint v2.15.0 reports 0 issues, and
  `go build ./...` passes.
- **Live** (scratch `addr/e2e.sh`). OpenLDAP;
  `login_max_accounts_per_address=2` and `login_backoff=0s` on the branch.
  Clients A and B have fixed, different addresses.

  | step | `aursu.1` | this branch |
  |---|---|---|
  | A: one password on `dillon` and `svc.algorithm` | two "LDAP auth failed for DN …" | the one message, twice |
  | A: then the right password for `dillon` | **credentials** | refused, the same message |
  | B: the right password for `dillon` | credentials | credentials |
  | lockouts in the server log | — | 1 |

## Not in this change

- **One account from many addresses** (a botnet) is PR 7.
