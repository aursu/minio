# 42 — Kafka and NATS notification targets: sarama 1.62.0, nats.go 1.56.0

Date: 2026-11-26. Plan 04 step 2.4, group C1 (note 40). Stacked on the
compress change (note 41), which these versions need.

## What

`go get`, then `go mod tidy -compat=1.21`:

| Module | From | To | Use |
|---|---|---|---|
| github.com/IBM/sarama | v1.45.1 | v1.62.0 | Kafka target (`internal/event/target/kafka.go`) |
| github.com/nats-io/nats.go | v1.41.2 | v1.56.0 | NATS target, core and JetStream (`nats.go`) |
| github.com/nats-io/nats-server/v2 | v2.11.1 | v2.15.3 | tests only (embedded server) |

Indirect: nats-io/jwt 2.8.3, nkeys 0.4.17, google/go-tpm 0.9.8 (from
nats-server), antithesis-sdk-go 0.9.0 (a no-op build of the testing SDK,
from nats-server); eapache/go-xerial-snappy and eapache/queue leave
(sarama no longer needs them). No code change.

The old nats-server v2.11.1 had 20 GitHub advisories. It is only linked
into tests, so govulncheck did not report them, but they were on the
dependency graph.

## What changes for MinIO

**sarama.** About 26,000 changed non-test lines from 1.45.1 to 1.62.0, most of
it admin, consumer and protocol work that the Kafka target does not use
(it is a `SyncProducer` with `RequiredAcks = 1`, two retries, 5 s
timeouts, optional SASL, TLS and compression). What it does use:

- **The default protocol version moved from 2.1.0 to 2.8.0.** MinIO sets
  `config.Version` only when `MINIO_NOTIFY_KAFKA_VERSION` is given, so
  without it the producer now speaks 2.8.0. Checked live against a
  Kafka 2.7 broker without a version set (below): delivery is the same.
- Producer fixes in 1.62.0: partial batches are flushed on `Close`, the
  batch in flight keeps flushing on retry, a recovery no longer uses up a
  retry, and metadata is refreshed before a retry only when stale.
- When a broker is down, the logged error now reads `EOF` where it used
  to read "client has run out of available brokers"; delivery from the
  queue directory afterwards is the same.

**nats.go.** v1.56.0 has a "breaking changes" section about TLS:

- "TLS settings now require TLS": fields set directly on `Options` now
  imply `Secure()`. MinIO configures TLS only through `nats.Secure`,
  `nats.RootCAs`, `nats.ClientCert` and `nats.TLSHandshakeFirst`, and all
  four already set `Secure = true` in v1.41.2 (compared in the source), so
  nothing changes for MinIO.
- With TLS, servers discovered from the cluster now come only from the
  INFO received inside TLS. MinIO gives one address. Against a TLS NATS
  cluster older than nats-server 2.12, fail-over to the other nodes is
  available only after the server sends a later INFO.
- `ErrSecureConnWanted` is wrapped: MinIO does not compare with it.

The other releases (1.42–1.55) fix reconnect, drain and JetStream
consumer races, leaks and stalls; the minimum Go version is 1.26, which
the fork's `go` line already is.

## Tests (media-gw, golang:1.27.4)

Unit, on this branch:

- `go mod tidy -compat=1.21` a second time: no change. `go build ./...`:
  ok.
- Cross-compile, all fifteen targets of `buildscripts/cross-compile.sh`:
  ok.
- golangci-lint v2.15.0: 0 issues. govulncheck v1.10.0: no
  vulnerabilities. GitHub advisories for each new module version,
  indirect ones included: none.
- `go test -race` for `internal/event/...` (the NATS tests run against the
  embedded nats-server 2.15.3) and `internal/store/...`: ok.
- `go test -tags kqueue,dev ./...` as uid 65534: 49 packages ok.

Live, the `aursu.2` image (sarama 1.45.1, nats.go 1.41.2) against this
branch built from source, the same brokers for both. Five targets, each
with a queue directory: Kafka 4.4.0 (`apache/kafka`, KRaft); Kafka 2.7
(`confluentinc/cp-kafka:6.1.15` with ZooKeeper) once without a version
and once with `2.7.0`; NATS 2.15.3 core publish; NATS 2.15.3 JetStream.
Per version: 20 PUTs, every broker stopped, 1 PUT, brokers started, 30 s,
5 PUTs; then the events each broker holds (Kafka topic end offsets, a
core NATS subscriber's count, the JetStream stream's message count).

| Target | aursu.2 | this branch |
|---|---|---|
| Kafka 4.4, no version | 26/26 | 26/26 |
| Kafka 2.7, no version (sarama default) | 26/26 | 26/26 |
| Kafka 2.7, version 2.7.0 | 26/26 | 26/26 |
| NATS JetStream | 26/26 | 26/26 |
| NATS core | 25/26 | 25/26 |

Core NATS delivers at most once: the queued event is published right
after MinIO reconnects, and a subscriber that has not resubscribed yet
misses it. Both versions lose the same one event.

NATS over TLS (TLS required on the server, `MINIO_NOTIFY_NATS_TLS=on`, a
CA file, the server addressed by IP): 10 PUTs, server restarted, 6 more.
Three runs: aursu.2 16, 16, 16; this branch 15, 16, 16. The single miss is
the same core NATS race as above, not a TLS failure: the connection and
the reconnect after the restart worked in every run.
