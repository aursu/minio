# 08 — grpc v1.83.2, otel v1.47.0 and what grpc requires (plan 04, before step 2.1)

Date: 2026-11-21. Second of the PRs that raise the server to the console's
versions before step 2.1 (note 07 explains the order).

## What

```
go get google.golang.org/grpc@v1.83.1 go.opentelemetry.io/otel/sdk@v1.45.0
go mod tidy
# after the first two CI runs (GitHub advisories, below):
go get github.com/go-jose/go-jose/v4@v4.1.6
go get google.golang.org/grpc@v1.83.2 go.opentelemetry.io/otel/sdk@v1.47.0
go mod tidy
```

| Module | Before → after | Why it moves |
|---|---|---|
| `google.golang.org/grpc` | 1.72.0 → **1.83.2** | 2 reachable findings, fixed in 1.83.1 (the console's version); 1.83.2 for a GitHub advisory (below) |
| `go.opentelemetry.io/otel` family | 1.35.0 → **1.47.0** | grpc requires 1.44; otel/sdk has 2 findings fixed in 1.45.0; 1.47.0 for a GitHub advisory (below); `otel/log` joins as indirect |
| `github.com/go-jose/go-jose/v4` | 4.1.0 → **4.1.6** | grpc's cascade requires 4.1.4 (closes 1 govulncheck finding); 4.1.6 for three newer GitHub advisories (below) |
| `google.golang.org/protobuf`, `genproto` (3) | 1.36.6 → 1.36.11, 2025-04 → 2026-05 | required by grpc; the console's versions |
| `cloud.google.com/go/storage` | 1.52.0 → 1.56.0 | required by grpc's cascade |
| `google.golang.org/api` | 0.230.0 → 0.264.0 | the same |
| `golang.org/x/oauth2`, `x/time` | 0.29 → 0.36, 0.11 → 0.14 | the same |
| envoy, xds, cel, spiffe, gax-go, GCP otel exporters | minor | indirect, the same |

36 modules change in go.mod.

**The Google Cloud stack comes with grpc, not with prometheus** as first
assumed. grpc 1.83.1's go.mod requires it. 1.83.1 is the first fixed grpc
version, so no smaller grpc bump exists that leaves the GCS SDK where it was.

## Where MinIO meets these modules

MinIO's own code imports none of grpc, otel, protobuf or go-jose:

| Module | Path into the server (`go mod why -m`) | Covered by CI |
|---|---|---|
| grpc | etcd client v3.5.21 (`cmd/iam-etcd-store.go`, `cmd/globals.go`: IAM on etcd, DNS federation); GCS SDK | yes: the IAM jobs with etcd |
| go-jose | `coreos/go-oidc` → OIDC token verification | yes: the IAM jobs with OpenID (Dex) |
| otel | GCS SDK (`cloud.google.com/go/storage`) | no |
| GCS storage, google api | `cmd/warm-backend-gcs.go` (ILM tiering to GCS), `cmd/api-errors.go` (error mapping) | **no** |
| x/time | `internal/bucket/bandwidth/monitor.go` (replication bandwidth limit) | unit tests of `bandwidth` |
| oauth2 | `cmd/utils.go`, an OpenID test helper (`MockOpenIDTestUserInteraction`) | yes |

## grpc behaviour changes, 1.73 → 1.83

From the release notes, only the client side matters: MinIO runs no gRPC
server.

- **1.75:** `round_robin` connects to addresses in random order. The etcd
  client sets `"loadBalancingPolicy": "round_robin"`
  (`internal/resolver/resolver.go`), so its first connections now spread
  over the etcd endpoints instead of starting with the first.
- **1.78:** client target URLs with unbracketed colons in the host are
  rejected, matching Go 1.26+ `net/url`. The server already runs on Go 1.27,
  so `net/url` rejects these anyway.
- **1.82:** the balancer registry is case-sensitive. The etcd client uses
  lowercase `round_robin`; not affected.
- Server-side changes (strict path checking, `grpc-timeout`, unary
  semantics) do not apply.

## GitHub advisories: three steps past the govulncheck fixes

The first push of this PR took go-jose 4.1.4, the version grpc's cascade
requires and the one that closes its govulncheck finding. **Dependency
Review** then failed: it flags every dependency a PR changes when the new
version has a known GitHub advisory, and 4.1.4 had three, published
2026-10-27/28:

| Advisory | Severity | Fixed in |
|---|---|---|
| GHSA-mgcc-cww5-fp2q, DoS when parsing JWKs with RSA keys | high | 4.1.6 |
| GHSA-4w3g-jfj2-cw2q, panic in CBC-HMAC processing | moderate | 4.1.5 |
| GHSA-vvm8-gf78-28g4, panics in JSON parsing | moderate | 4.1.5 |

None of them was in the Go vulnerability database yet, so govulncheck did
not report them. go-jose reaches the server through `coreos/go-oidc`, which
parses the identity provider's JWKs, so the high one is on the OIDC path. A
second commit takes **4.1.6**, the first version with no known GitHub
advisory (4.1.9 is the latest); only go-jose moves.

The second CI run's Dependency Review then flagged grpc 1.83.1, and GitHub's
dependency compare for the PR (`GET
/repos/aursu/minio/dependency-graph/compare/<base>...<head>`, the data the
action uses) also listed otel 1.45.0:

| Advisory | Severity | Affects MinIO? | Fixed in |
|---|---|---|---|
| GHSA-2v4p-qf9q-27wj, xDS servers panic on a request without `:authority` and `Host` | high | no: MinIO runs no gRPC server | grpc 1.83.2 |
| GHSA-q7gw-24fr-5p69, `AttributeValueLengthLimit` bypassed by U+FFFD runes | moderate | otel through the GCS SDK | otel sdk/trace 1.47.0 |

The grpc advisory was published on 2026-09-08, before the first CI run, but
that run's Dependency Review did not report it; why is unknown. Both were
taken anyway: a patch release each, and a clean version.

After that, **every module version this PR changes (36) was checked**
against the GitHub advisory database with `gh api
"/advisories?ecosystem=go&affects=<module>@<version>"`. None has a known
advisory. The query was first validated on the known-bad versions (grpc
1.83.1, otel/sdk 1.45.0, go-jose 4.1.4), which it reports.

Lesson: govulncheck and Dependency Review read different databases. A
version that is clean for govulncheck can still carry a GitHub advisory,
and Dependency Review's own report may be incomplete. Check every changed
module against the GitHub database before pushing.

## Untested here: GCS tiering

As with the HTTP/2 change (note 06), ILM tiering to GCS is not covered by any
test: the GCS SDK moves 1.52 → 1.56 and its otel instrumentation 1.35 →
1.45. Anyone who tiers to GCS should check a transition and a restore.

## Tests

Go 1.27.4, `golang:1.27.4`, on the build host:

| Check | Result |
|---|---|
| `go build ./...`, `go vet ./...` | ok, 0 findings |
| `make verifiers` (lint v2.15.0 + check-gen) | 0 issues; check-gen clean against a committed baseline |
| govulncheck v1.10.0 | 20 → **15** reachable: grpc, otel/sdk, go-jose gone; left amqp091-go 10, prometheus 4, paho 1 |
| `fork/test/bin/unit-tests.sh`, all packages | **46/46 ok** |

The first check-gen run reported clean while git could not run in the
container ("dubious ownership"), so it compared nothing. It was rerun with
`safe.directory` and a real baseline commit.

All checks were run again after each step (go-jose 4.1.6; grpc 1.83.2 and
otel 1.47.0): build and vet ok, lint 0 issues, check-gen clean (git
working), govulncheck 15, unit **46/46 ok**. Both earlier CI runs were green
except Analysis (15 findings, as expected) and Dependency Review (the
advisories above).
