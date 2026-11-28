# 44 — Prometheus libraries: client_golang 1.25.0, common 0.74.0

Date: 2026-11-26. Plan 04 step 2.4, group D (note 40). Stacked on C2
(note 43).

## What

`go get`, then `go mod tidy -compat=1.21`:

| Module | From | To |
|---|---|---|
| github.com/prometheus/client_golang | v1.23.2 | v1.25.0 |
| github.com/prometheus/client_model | v0.6.2 | v0.6.3 |
| github.com/prometheus/common | v0.67.5 | v0.74.0 |
| github.com/prometheus/procfs | v0.16.1 | v0.23.0 |

Indirect: `github.com/pandatix/go-abnf` v0.5.5 is new (see below);
`go.yaml.in/yaml/v2` leaves `go.mod` because `prometheus/common/model` no
longer imports it outside tests. No code change.

MinIO uses these for every metrics endpoint: `/minio/v2/metrics/*`
(`cmd/metrics-v2.go`, its own registry with the Go and process
collectors), `/minio/metrics/v3` (`cmd/metrics-v3-handler.go`, through
`promhttp.HandlerFor`), and procfs for the process, drive and network statistics
(`cmd`, `internal/disk`, `internal/net`).

## What changes

- **Content negotiation** (`expfmt.Negotiate`, used by every MinIO
  metrics handler). The rewritten negotiation also offers the text
  format version 1.0.0. Prometheus 3 lists it before 0.0.4 in its
  `Accept` header, so a Prometheus 3 scrape now gets
  `text/plain; version=1.0.0; charset=utf-8; escaping=allow-utf-8`
  where it got version 0.0.4. Without `Accept`, with the Prometheus 2
  header, and for protobuf, the answer is unchanged. Measured below: the
  1.0.0 body has the same comment lines and sample names as 0.0.4 for
  MinIO's metrics, and Prometheus scrapes it.
- `promhttp` (v3 endpoint) adds `Vary: Accept` and
  `Vary: Accept-Encoding`, and now supports `name[]` query parameters to
  filter metrics.
- `Gather()` recovers from a panicking collector and returns an error
  instead of crashing the process.
- Name validation always uses the UTF-8 scheme; the default behavior is
  unchanged, and MinIO's names are plain.
- The Go runtime collector's metric set was regenerated for Go 1.25 and
  1.26; the families MinIO exports are the same before and after
  (measured below).
- The minimum Go version of client_golang is 1.26, which the fork's `go`
  line already is.

**The new dependency, pandatix/go-abnf.** `prometheus/common/expfmt` now
parses OpenMetrics 2.0 input with an ABNF engine (go-abnf, about 10,000
lines, no dependencies, its README calls it "built first and foremost as
a testing oracle", not the fastest production parser). It is linked into
the server (231 symbols in the binary) because `expfmt.NewDecoder` is
reachable: MinIO's own code only encodes (`Negotiate`, `NewEncoder`),
but the KES client in minio/kms-go decodes the metrics of a KES server
(`kes.Client.Metrics`, called from `cmd/metrics-v2.go` when the KMS is
KES), with `expfmt.ResponseFormat` of the KES response. The OpenMetrics
2.0 parser runs only if that KES server answers with an OpenMetrics 2.0
content type; the input is the KMS's own response, size-limited by the
client. No advisory names go-abnf.

## Tests (media-gw, golang:1.27.4)

Unit, on this branch:

- `go mod tidy -compat=1.21` a second time: no change. `go build ./...`:
  ok. Cross-compile, all fifteen targets: ok.
- golangci-lint v2.15.0: 0 issues. govulncheck v1.10.0: no
  vulnerabilities. GitHub advisories for every new module version,
  go-abnf included: none.
- `go test -race` for `./internal/...` and for the `cmd` tests matching
  metrics and Prometheus: ok. `go test -tags kqueue,dev ./...` as uid
  65534: 49 packages ok.

Live, the `aursu.2` image and this branch built from source, side by side
(four drives each, `MINIO_PROMETHEUS_AUTH_TYPE=public`, a bucket with one
object written and read):

- **Prometheus v3.16.0** scraping `/minio/v2/metrics/cluster`,
  `/minio/v2/metrics/node` and `/minio/metrics/v3` of both: every target
  `up`, no scrape error. Its `Accept` header, recorded with a small HTTP
  server: `application/openmetrics-text;version=1.0.0;escaping=allow-utf-8;q=0.7,
  application/openmetrics-text;version=0.0.1;q=0.6,
  text/plain;version=1.0.0;escaping=allow-utf-8;q=0.5,
  text/plain;version=0.0.4;q=0.4,*/*;q=0.3`.
- **Content-Type** on all five endpoints (v2 cluster, node, bucket,
  resource; v3) for four `Accept` values: none, the Prometheus 3 header,
  the Prometheus 2 header, protobuf. The only difference: the Prometheus
  3 header gets version 1.0.0 from this branch, 0.0.4 from aursu.2. The
  v3 endpoint adds the two `Vary` headers.
- **Metric families** (names from `# TYPE`) per endpoint, old vs new:
  cluster 66/67, node 87/87, bucket 5/5, resource 83/83, v3 93/94.
  **Series** (name and labels, values and the `server` label removed):
  identical except the version labels (`DEVELOPMENT.GOGET` for a source
  build) and the one extra family on cluster and v3,
  `minio_node_io_read_bytes` and `minio_system_process_io_read_bytes`.
  Both versions emit that metric only when `/proc/self/io` `read_bytes`
  is above zero (`cmd/metrics-v2.go`, `cmd/metrics-v3-system-process.go`).
  The branch binary had read about 97 MB from disk, roughly its own size,
  since it was started from a bind-mounted file; the aursu.2 binary is
  inside an image that had been read before, so its pages were most
  likely cached. Not proven by a swapped run, but the code path is the
  same in both versions and only the counter's value decides it.
- **Format 1.0.0 against 0.0.4** from this branch, same endpoint: comment
  lines and sample names identical on all five endpoints.
