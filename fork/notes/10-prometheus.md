# 10 — prometheus v0.311.3 and what it requires (plan 04, before step 2.1)

Date: 2026-11-22. Third of the PRs that raise the server to the console's
versions before step 2.1 (note 07 explains the order).

## What

```
go get github.com/prometheus/prometheus@v0.311.3 github.com/go-openapi/swag@v0.29.2
go mod tidy
```

| Module | Before → after | Why it moves |
|---|---|---|
| `github.com/prometheus/prometheus` | 0.303.0 → **0.311.3** | 4 reachable findings, fixed in 0.311.3 (the console's version) |
| `prometheus/client_golang`, `prometheus/common` | 1.22.0 → 1.23.2, 0.63.0 → 0.67.5 | required by prometheus 0.311.3 |
| go-openapi: `strfmt`, `validate`, `loads`, `spec`, `analysis`, `errors`, `jsonpointer`, `jsonreference` | 0.23 → 0.26.1, 0.24 → 0.25.1, 0.22 → 0.23.2, … | required by prometheus 0.311.3 (strfmt 0.26.1) |
| `github.com/go-openapi/swag` | 0.23.1 → **0.29.2**, split into 12 submodules | see below |
| Azure `azcore`, `azidentity`, MSAL | 1.18 → 1.21, 1.9 → 1.13.1, 1.4.2 → 1.6.0 | prometheus's cascade |
| `miekg/dns`, `coreos/go-systemd/v22`, `go.uber.org/zap` | 1.1.65 → 1.1.72, 22.5 → 22.6, 1.27.0 → 1.27.1 | the same |
| `google.golang.org/api`, `x/time`, `otelhttp`, `gax-go`, `genproto` | minor | the same |
| removed | `mongo-driver`, `govalidator`, `easyjson`, `mitchellh/mapstructure`, `oklog/ulid` v1 | strfmt 0.26 dropped them |
| added | `go-viper/mapstructure/v2` 2.5.0, `oklog/ulid/v2`, `go.yaml.in/yaml/v2,v3`, `grafana/regexp` | their replacements |

## swag 0.29.2, not the 0.25.4 that `go get` picks

`go get prometheus@v0.311.3` alone takes swag 0.25.4, and `go mod tidy` then
fails:

```
github.com/go-openapi/swag/loading tested by
github.com/go-openapi/swag/loading.test imports
github.com/go-openapi/testify/enable/yaml/v2 imports
github.com/go-openapi/testify/v2/assert/yaml: module github.com/go-openapi/testify/v2@latest found (v2.8.1), but does not contain package
```

This is the blocker the console hit on its step 1.6 (console note 16).
swag 0.29.2 moves `testify/enable/yaml/v2` out into its own module, so
tidy succeeds with no `replace`. A `replace` was rejected in the console for
a reason that applies here too: it does not reach the modules that depend
on this one.

The server needs none of the console's `swag.*` → `swag/conv` rewrite: its
only go-openapi call is `loads.Embedded` in `cmd/common-main.go` (the
console's swagger spec), and it calls no `swag.*` function.

## A default-constructed `TextParser` now panics

prometheus/common **0.66.0** made a zero-value `expfmt.TextParser` invalid:
parsing with it panics ("Invalid name validation scheme requested:
unset"); callers must use `expfmt.NewTextParser`. The compiler does not
catch this. Two modules in the graph still use the zero value:

- `prometheus/prom2json` v1.4.2, `ParseReader`;
- `minio/madmin-go/v3` v3.0.109, `ParsePrometheusResults`.

Neither runs in the server:

- MinIO's own code and the console call neither, nor `prom2json` at all.
- In the built binary, `prom2json.ParseReader` is not linked.
  `ParsePrometheusResults` is, and `-ldflags=-dumpdep` shows the only path
  to it: `mc/cmd.init` → `adminPrometheusMetricsCmd` → `mainSupportMetrics`
  → `printPrometheusMetricsV3` → `prometheusMetricsReader.JSON`. That is
  mc's own `mc admin prometheus metrics` command, linked because the
  console imports `mc/cmd`. mc's command tree is run only by `mc/cmd.Main`,
  which nothing in the server references; the server's CLI
  (`minio/cmd.Main`) runs MinIO's commands only.
- Nothing else in the binary references `TextParser.TextToMetricFamilies`.

The aursu/console build already runs common 0.67.5 next to madmin-go
(3.0.91 there) and mc, so the combination is not new to the fork. If a later change calls madmin's metrics
client from the server or the console, it panics: the fix then is a
madmin-go that uses `NewTextParser`.

## The metrics endpoints are unchanged

Changes to client_golang and common touch how metrics are encoded, so both
servers were built, master and this branch, started (single node, 4
drives, `MINIO_PROMETHEUS_AUTH_TYPE=public`) and scraped:
`/minio/v2/metrics/{cluster,node,bucket,resource}` and
`/minio/metrics/v3`, with the default `Accept`, OpenMetrics and protobuf.

| Endpoint | Families (`# TYPE`) master / branch | HELP text, series label sets |
|---|---|---|
| v2 cluster | 42 / 42, no difference | identical |
| v2 node | 50 / 50, no difference | identical |
| v3 | 57 / 57, no difference | identical |
| v2 bucket, resource | empty on both (no buckets; first collection not yet run) | — |

Content types match too: both builds answer OpenMetrics requests with text
0.0.4, and every content type, text and protobuf, carries
`escaping=underscores`.

## Where else MinIO meets these modules

| Module | Where | Covered by CI |
|---|---|---|
| client_golang, common | metrics v2/v3 handlers, `cmd/http-stats.go` | yes (the comparison above, metrics in the integration jobs) |
| Azure `azcore`, `azidentity` | `cmd/warm-backend-azure.go` (ILM tiering to Azure), `cmd/api-errors.go` | **no** |
| `miekg/dns` | `internal/config/dns` (bucket DNS federation, CoreDNS + etcd), `cmd/common-main.go` | partly: the etcd IAM jobs, not DNS federation |
| `coreos/go-systemd` | `cmd/server-main.go`, `cmd/signals.go` (sd_notify) | no |
| `zap` | `internal/config/etcd` (etcd client logger) | yes: the etcd IAM jobs |
| `x/time` | `internal/bucket/bandwidth/monitor.go` | unit tests of `bandwidth` |

As with GCS in note 08, **ILM tiering to Azure is untested here**: anyone
who tiers to Azure should check a transition and a restore.

## GitHub advisories

Every module version this PR adds or changes (46) was checked against the
GitHub advisory database before the push (`gh api
"/advisories?ecosystem=go&affects=<module>@<version>"`, validated on grpc
1.83.1, which it reports). One match: **GHSA-3m87-5598-2v4f** for prometheus
0.311.3. It is a **withdrawn** advisory (CVE-2019-3826, an XSS in the
Prometheus server's web UI before 2.7.1): it "does not apply to the
Prometheus golang package", and the module's v0.311.3 falls into `< 2.7.1`
only because module versions and server versions are numbered differently.
All other modules: no advisory.

## Tests

Go 1.27.4, `golang:1.27.4`, on the build host:

| Check | Result |
|---|---|
| `go build ./...`, `go vet ./...` | ok, 0 findings |
| `make verifiers` (lint v2.15.0 + check-gen) | 0 issues; check-gen clean, git working in the container |
| govulncheck v1.10.0 | 15 → **11** reachable: prometheus gone; left amqp091-go 10, paho 1 |
| `fork/test/bin/unit-tests.sh`, all packages | **46/46 ok** |
| metrics endpoints, master vs branch | identical (above) |
| linked symbols, `-ldflags=-dumpdep` | as above |
