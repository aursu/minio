# 06 — x/crypto, x/net, x/text and the x/ family; go line 1.26.0 (plan 04, step 2.3, x/ group)

Date: 2026-11-21. Split from the first version of #24 (note 05, "Revised"),
which took this bump as a side effect of a newer x/tools. Here it is a
change of its own.

## What

```
go get golang.org/x/crypto@latest golang.org/x/net@latest golang.org/x/text@latest
go mod tidy
```

| Module | Before → after | Reachable findings closed |
|---|---|---|
| `golang.org/x/crypto` | 0.37.0 → **0.59.0** | 12 (fixed in 0.56.0) |
| `golang.org/x/net` | 0.39.0 → **0.63.0** | 7 (fixed in 0.60.0) |
| `golang.org/x/text` | 0.24.0 → **0.44.0** | 1 (fixed in 0.39.0) |
| `golang.org/x/sys`, `sync`, `term` | 0.32 → 0.50, 0.13 → 0.25, 0.31 → 0.48 | — |
| `golang.org/x/mod`, `tools` (indirect) | 0.24 → 0.42, 0.32 → 0.52 | — |
| **`go` line** | 1.24.0 → **1.26.0** | — |

The `go` line is required by x/net 0.63. Step 2.1 needed it anyway: the
full console's `go` line is 1.26.0. x/tools moves only as a requirement of
x/net; stringer is still installed separately at v0.53.0 (note 05).

A second commit fixes one lint finding that appears with the new `go`
line (below).

## Why latest

The first fixed versions (x/crypto 0.56, x/net 0.60, x/text 0.39) would be
the minimal bump. "Latest" was taken because:

- the console took x/net 0.60.0, x/crypto 0.57.0 and x/text 0.42.0
  (aursu/console #27, its note 15). At 2.1, version selection takes the
  higher of the server's and the console's versions, so the server's own
  choice decides;
- every fixed version needs the `go` 1.26 line anyway (x/net ≥ 0.60);
- **x/net must not be one of the in-between versions.** x/net 0.54 compiles
  its own HTTP/2 server only with the `http2legacy` tag under Go 1.27
  (`//go:build !(go1.27 && !http2legacy)`), and grpc 1.72 uses
  `http2.TrailerPrefix` from that file, so 0.54 does not build here. 0.63
  keeps the constant in an unconstrained file.

## Risk: HTTP/2 under Go 1.27

From x/net 0.6x on, under Go 1.27, `http2.Server` and `http2.Transport`
are thin wrappers around the standard library's own HTTP/2
(`http2/server_wrap.go`, `transport_wrap.go`, build tag
`go1.27 && !http2legacy`). The old implementation is built only with
`-tags http2legacy`. What that means here, checked in the code:

| User of `golang.org/x/net/http2` | What it uses | Affected? |
|---|---|---|
| MinIO's own code | nothing (a commented-out snippet in `internal/http/transports.go`); the S3 server and clients use `net/http` | **no** |
| grpc 1.72 (`internal/transport`), the etcd client: IAM on etcd, DNS federation | the frame layer only: `Framer`, frame types, error codes, settings, plus `TrailerPrefix` | **no**: those files carry no build constraint; same code as before |
| `cloud.google.com/go/auth/httptransport`, `google.golang.org/api/transport/http` (GCS) | `http2.ConfigureTransports`, then `ReadIdleTimeout = 31s` | **yes**: the HTTP/2 client is now the standard library's |
| `azure-sdk-for-go/sdk/azcore/runtime` (Azure) | `http2.ConfigureTransports`, then `ReadIdleTimeout = 10s`, `PingTimeout = 5s` | **yes**, the same |

In MinIO, the Google and Azure SDKs serve **ILM tiering to GCS and Azure
remote tiers** (`cmd/warm-backend-gcs.go`, `cmd/warm-backend-azure.go`).
Their keep-alive settings are carried over: `transport_wrap.go` maps
`ReadIdleTimeout` to the standard library's `HTTP2Config.SendPingTimeout`,
and `PingTimeout` to `PingTimeout`.

**No test covers that path.** The unit tests and the CI workflows do not
tier to GCS or Azure; I found none. The chance of breakage is low: the
standard library's HTTP/2 is the maintained successor, and the settings
carry over. But it is untested here. Anyone who tiers to GCS or Azure
should check a transition and a restore after this change. Everything else
that uses HTTP/2 is either unaffected or covered by CI.

## govet printf with the new go line

With the `go` line at 1.26.0, govet's printf check reports one message in
`internal/grid/grid_test.go` (`TestSingleRoundtripGenerics`):
`t.Errorf("want nil, got %q", resp)`, where `resp` is a `*testResponse`
struct. It is now `%+v`. The same message elsewhere in the file formats a
string or an `*MSS` (`map[string]string`), which `%q` handles; those stay.
Our own change; upstream has no newer commit.

## Not in this change

The other 21 findings of step 2.3, in 7 modules: amqp091-go 10,
prometheus 4, grpc 2, otel/sdk 2, paho, go-jose, compress. One PR per
group, as planned.

## Tests

Go 1.27.4, `golang:1.27.4`:

| Check | Result |
|---|---|
| `go build ./...`, `go vet ./...` | ok |
| `make verifiers` (lint v2.15.0 + check-gen) | 0 issues; `go generate` + tidy leave the tree clean |
| govulncheck v1.10.0 | **41 → 21** reachable; x/crypto, x/net, x/text: 0 |
| `fork/test/bin/unit-tests.sh`, all packages | **46/46 packages ok** |
