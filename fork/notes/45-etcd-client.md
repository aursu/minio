# 45 — etcd client 3.5.36 → 3.6.17

Date: 2026-11-26. Plan 04 step 2.4, group D2 (note 40). Stacked on D
(note 44).

## What

`go get go.etcd.io/etcd/api/v3@v3.6.17 go.etcd.io/etcd/client/v3@v3.6.17`,
then `go mod tidy -compat=1.21`:

| Module | From | To |
|---|---|---|
| go.etcd.io/etcd/api/v3 | v3.5.36 | v3.6.17 |
| go.etcd.io/etcd/client/v3 | v3.5.36 | v3.6.17 |
| go.etcd.io/etcd/client/pkg/v3 (indirect) | v3.5.36 | v3.6.17 |
| github.com/grpc-ecosystem/grpc-gateway/v2 (indirect) | v2.28.0 | v2.29.0 |

No code change. MinIO uses the client for IAM stored in etcd
(`cmd/iam-etcd-store.go`, `MINIO_ETCD_ENDPOINTS`), federation DNS
(`internal/config/dns/etcd_dns.go`) and the config in
`internal/config/etcd`: `clientv3.New`; `Get` with prefixes, `Put`,
`Delete`, a prefix `Watch` for IAM changes, and `Put` with a lease
(`Grant`, `WithLease`) for credentials that expire, such as STS.

## Why 3.6, not 3.7

The latest release is v3.7.4, and the first attempt took it. Every v3.7
release from v3.7.0-beta.0 on requires coreos/go-systemd v22.7.0 (through
`client/pkg/v3`), and v22.7.0 does not build for netbsd/amd64 (note 40:
`unix.CLOCK_MONOTONIC` is not defined for netbsd in any
golang.org/x/sys). With 3.7.4 the netbsd cross-compile failed the same
way as on #66's first CI run. Every v3.6 release, v3.6.17 included,
requires go-systemd v22.5.0, so the fork's v22.6.0 stays.

The etcd project maintains the current minor and the two before it, so
3.5, 3.6 and 3.7 are all supported now; 3.5 drops out when 3.8 is
released. No GitHub advisory names 3.5.36, 3.6.17 or 3.7.4. Recommended,
accepted (Alexander, 2026-11-26): 3.6.17 now; 3.7 once go-systemd or
x/sys builds for netbsd again.

The 3.6 changelog has almost nothing for `clientv3`: the `Metadata` field
of `naming/endpoints.Endpoint` is deprecated (MinIO does not use it). The
rest is server, etcdctl and gRPC-gateway work.

## Tests (media-gw, golang:1.27.4)

Unit, on this branch:

- `go mod tidy -compat=1.21` a second time: no change. `go build ./...`:
  ok. Cross-compile, all fifteen targets: ok.
- golangci-lint v2.15.0: 0 issues. govulncheck v1.10.0: no
  vulnerabilities.
- `go test -race` for `./internal/...` (`internal/config/etcd` included):
  ok. `go test -tags kqueue,dev ./...` as uid 65534: 49 packages ok. The
  etcd-backed IAM tests in `cmd` need a running etcd and are covered by
  the CI's IAM integration job (its etcd service is `quay.io/coreos/etcd:v3.5.1`,
  so CI also checks this client against an older 3.5 server) and the live
  run below.
- GitHub advisories for every new module version: none.

Live, IAM in etcd: the `aursu.4` image (etcd client 3.5.36) and this
branch built from source (3.6.17) take turns on one etcd server; one run
for each server version, etcd 3.5.36, 3.6.17 and 3.7.4 (`quay.io/coreos/etcd`).

1. aursu.4 creates a bucket with an object, a custom policy `p-read`
   (get and list on that bucket), `user1` with `readwrite`, `user2` in
   group `g1`, which has `p-read`, a service account `svc1key` of
   `user1`, and STS credentials of `user1` (AssumeRole).
2. This branch reads all of it, checks access with each identity
   (including aursu.4's STS credentials), then etcd is restarted, and
   the branch adds `user3` (`readonly`, also in `g1`), a service account
   `svc3key` and its own STS credentials.
3. aursu.4 again: reads everything, including what the branch wrote, and
   checks access with every identity, the branch's STS credentials and
   `svc3key` included.

Result, the same for all three etcd versions:

| Check | Phase 1 | Phase 2 | Phase 3 |
|---|---|---|---|
| users, policies, group members and policy, service accounts | as written | identical to phase 1, then with `user3`, `svc3key` | identical to phase 2 |
| keys under `config/iam/` in etcd | 9 | 9, then 13 | 13 |
| `user1`, `svc1key`: list, get, put | ok | ok | ok |
| `user2` (group policy): list and get ok, put denied | as expected | as expected | as expected |
| STS from aursu.4 | ok | ok | ok |
| STS from this branch | — | ok | ok |
| writes after the etcd restart | — | ok | — |

No error in either server's log.
