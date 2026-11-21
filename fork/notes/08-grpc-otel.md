# 08 — grpc v1.83.1, otel v1.45.0 and what grpc requires (plan 04, before step 2.1)

Date: 2026-11-21. Second of the PRs that raise the server to the console's
versions before step 2.1 (note 07 explains the order).

## What

```
go get google.golang.org/grpc@v1.83.1 go.opentelemetry.io/otel/sdk@v1.45.0
go mod tidy
```

| Module | Before → after | Why it moves |
|---|---|---|
| `google.golang.org/grpc` | 1.72.0 → **1.83.1** | 2 reachable findings, fixed in 1.83.1; the console's version |
| `go.opentelemetry.io/otel` family | 1.35.0 → **1.45.0** | grpc requires 1.44; otel/sdk has 2 findings fixed in 1.45.0 |
| `github.com/go-jose/go-jose/v4` | 4.1.0 → **4.1.4** | required by grpc's cascade; closes 1 finding |
| `google.golang.org/protobuf`, `genproto` (3) | 1.36.6 → 1.36.11, 2025-04 → 2026-05 | required by grpc; the console's versions |
| `cloud.google.com/go/storage` | 1.52.0 → 1.56.0 | required by grpc's cascade |
| `google.golang.org/api` | 0.230.0 → 0.264.0 | the same |
| `golang.org/x/oauth2`, `x/time` | 0.29 → 0.36, 0.11 → 0.14 | the same |
| envoy, xds, cel, spiffe, gax-go, GCP otel exporters | minor | indirect, the same |

go.mod: 71 lines; go.sum: 178 lines.

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
