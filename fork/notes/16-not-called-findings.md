# 16 — the four not-called govulncheck findings (plan 04, step 2.4, group 1)

Date: 2026-11-23.

## Why

After step 2.3 `govulncheck` reports 0 reachable findings, but still lists
four modules the server imports or requires without calling the vulnerable
code. Each is a known-bad version shipped in the binary; the next call site
added, here or in a dependency, makes it reachable.

| Finding | Module, before | What | Server's path to it |
|---|---|---|---|
| GO-2026-4514 (CVE-2026-32285) | `github.com/buger/jsonparser` 1.1.1 | `Delete` on malformed input: denial of service | `internal/jwt/parser.go` uses `Get`, `ObjectEach`, `ParseString`, `ParseInt`; not `Delete` |
| GO-2026-4503 (CVE-2026-26958) | `filippo.io/edwards25519` 1.1.0 | `Point.MultiScalarMult`: invalid result | through `go-sql-driver/mysql` (MySQL notification target); not called |
| GO-2026-5543 (CVE-2026-32952) | `github.com/Azure/go-ntlmssp` 2022 pseudo-version | `ProcessChallenge` panics on a malformed challenge | through `go-ldap/ldap/v3`; the server never binds with NTLM |
| GO-2026-6107 (GHSA-6vch-q96h-7gc3) | `go.etcd.io/etcd/client/pkg/v3` 3.5.21 | TLS listener without a handshake timeout | through `etcd/client/v3` (`logutil` only); the server opens no etcd listener |

## What

Each module goes to the **latest patch release of the minor it is on**,
which is also at or above the fixed version:

```
go get github.com/buger/jsonparser@v1.1.2 filippo.io/edwards25519@v1.1.1 \
  github.com/Azure/go-ntlmssp@v0.1.1 go.etcd.io/etcd/api/v3@v3.5.36 \
  go.etcd.io/etcd/client/v3@v3.5.36 go.etcd.io/etcd/client/pkg/v3@v3.5.36
go mod tidy
```

| Module | Before → after |
|---|---|
| `github.com/buger/jsonparser` | 1.1.1 → **1.1.2** |
| `filippo.io/edwards25519` | 1.1.0 → **1.1.1** |
| `github.com/Azure/go-ntlmssp` | `v0.0.0-20221128193559-754e69321358` → **0.1.1** |
| `go.etcd.io/etcd/api/v3`, `client/v3`, `client/pkg/v3` | 3.5.21 → **3.5.36** (moved together, as etcd releases them) |
| `google.golang.org/protobuf` | 1.36.11 → 1.36.12 (required by etcd 3.5.36) |
| `google.golang.org/genproto/googleapis/{api,rpc}` | `20260526…` → `20260803…` (the same) |

## Why not the latest within the major

- **jsonparser 1.6.1.** 1.2.0 to 1.6.1 were released within four days in
  July 2026, after five years without a release. Its only user here is the
  JWT parser, the token path for every STS and console session. A move of
  that size gets a PR of its own with the diff read, not a ride in a
  security group.
- **etcd 3.7.4.** Two minors up; etcd is the IAM backend in the etcd
  deployments. It is a 2.4 group of its own, with its changelog read and
  tested against the etcd server version CI uses.
- **edwards25519 1.2.0** adds API next to 1.1.1 (same day). Nothing here
  needs it.

## What changes in behaviour

- **jsonparser 1.1.2** is the 2021 master plus the fix, not just the fix.
  For the calls the JWT parser makes:
  - `ParseInt` now detects overflow correctly. 1.1.1 checked `b < v` after
    `10*v + digit`, which misses wraps: on 2,000,000 random 18–25-digit
    numbers it returned a wrong value without an error for 55,524 of them
    (e.g. `41927999538077221282` → `5034511390658118050`); 1.1.2 for none,
    and both accepted every value that fits. So an `exp`, `iat` or `nbf`
    beyond int64 could come back as an unrelated number; 1.1.2 returns
    `OverflowIntegerError` and the token is rejected as malformed.
  - `ParseString`/`ParseInt` return a new `NullValueError` for `null`. The
    parser checks `dataType` first (`exp: Expected number`), so `null`
    never reaches them; no change.
  - `findKeyStart` skips leading whitespace; it serves `Delete`/`Set`,
    which the server does not call.
- **edwards25519 1.1.1** changes `extra.go` only (`MultiScalarMult`).
- **go-ntlmssp 0.1.1** reworks message parsing; reached only through
  go-ldap's NTLM bind, which the server does not use.
- **etcd 3.5.22 → 3.5.36**, `clientv3` entries: the resolver uses
  `State.Endpoint.Addresses` (3.5.22), no grpc `Metadata` field (3.5.27),
  the endpoint printed in the unary interceptor (3.5.28), a race on
  `leaseCache.entries` fixed (3.5.33). The rest are server, raft, etcdctl
  and dependency bumps. 3.5.36 was tagged on 2026-11-18; its changelog on
  `main` still says "TBC" but lists server and raft fixes only.

## GitHub advisories

`gh api "/advisories?ecosystem=go&affects=<module>@<version>"` for each new
version: none for jsonparser 1.1.2, edwards25519 1.1.1, go-ntlmssp 0.1.1,
etcd `api`/`client`/`client/pkg` 3.5.36 and protobuf 1.36.12. Positive
controls: jsonparser 1.1.1 → GHSA-6g7g-w4f8-9c9x, edwards25519 1.1.0 →
GHSA-fw7p-63qq-7hpr. etcd `client/pkg/v3` 3.5.21 returns nothing either:
GitHub's database does not map GHSA-6vch-q96h-7gc3 to that module yet, so
for etcd the Go vulnerability database (GO-2026-6107, fixed in 3.5.33) is
the source.

## Tests

On the build host, Go 1.27.4:

| Check | Result |
|---|---|
| `go build ./...`, `go vet ./...` | ok |
| `make verifiers` (golangci-lint 2.15.0, check-gen) | 0 issues, tree clean |
| govulncheck v1.10.0 | reachable **0**; imported/required but not called **4 → 0** |
| unit suite (`fork/test`) | PASS, 46 packages |
| jsonparser `ParseInt`, 2,000,000 random numbers against `math/big` | 1.1.1: 55,524 wrong values accepted; 1.1.2: 0 |

etcd as IAM backend and LDAP are covered by CI's IAM matrix (`etcd=` and
`ldap=` entries).
