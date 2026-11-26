# 43 — AMQP, MQTT, Redis, MySQL and PostgreSQL notification targets

Date: 2026-11-26. Plan 04 step 2.4, group C2 (note 40). Stacked on C1
(note 42).

## What

`go get`, then `go mod tidy -compat=1.21`:

| Module | From | To | Target |
|---|---|---|---|
| github.com/rabbitmq/amqp091-go | v1.14.0 | v1.17.0 | AMQP (`internal/event/target/amqp.go`) |
| github.com/eclipse/paho.mqtt.golang | v1.5.1 | v1.5.2 | MQTT (`mqtt.go`) |
| github.com/gomodule/redigo | v1.9.2 | v1.9.3 | Redis (`redis.go`) |
| github.com/go-sql-driver/mysql | v1.9.2 | v1.10.2 | MySQL (`mysql.go`) |
| github.com/lib/pq | v1.10.9 | v1.12.3 | PostgreSQL (`postgresql.go`) |

Indirect: filippo.io/edwards25519 v1.1.1 → v1.2.0 (from the MySQL
driver). No code change.

## Why, besides staying current

**go-sql-driver/mysql v1.10.2 fixes GHSA-9qjq-77h9-895w** (medium): a
truncated initial handshake packet, including the greeting sent before
TLS starts, made the driver panic, so anyone who can answer as the
MySQL server (a spoofed address, a man in the middle before TLS) can
crash the client process. Affected: every version up to v1.10.1, so ours.
It also fixes GHSA-4v5x-9mc8-gr98 (high), which needs
`LOAD DATA LOCAL INFILE`; the MySQL target does not use it. Both are
advisories of the driver's repository, published 2026-11-17. As of
2026-11-26 neither is in the GitHub global advisory database nor in
OSV (`/advisories?affects=…@v1.9.2` and the OSV query return nothing), so
govulncheck and Dependabot do not report them yet.

## What changes for the targets

- **amqp091-go 1.15–1.17**: SASL credentials kept across `Reconnect()`,
  the URI `connection_timeout` honored during recovery, the
  pre-negotiation frame size enforced on reconnect, the TLS config built
  before dialing, sockets closed on dial errors. `QosConfig.PrefetchSize`
  and `QosConfig.Global` are deprecated; the AMQP target only publishes
  and sets no QoS.
- **paho 1.5.2**: a reconnect race when resending stored packets is
  fixed, and a QoS 0 publish dropped while disconnected now completes its
  token. `Client` gains `DeleteRoute` (MinIO uses the client from
  `mqtt.NewClient` and does not implement the interface). A
  `ConnectTimeout` of 0 now disables the timeout; MinIO does not call
  `SetConnectTimeout`, so the library default (30 s) applies.
- **redigo 1.9.3**: `valkey://` URLs accepted, the server path in a URL
  honored, simple string keys in `ScanStruct`. The Redis target dials an
  address, not a URL.
- **mysql 1.10**: the advisories above; `interpolateParams` with unsafe
  multibyte charsets is now rejected by charset too, not only by
  collation. The DSN MinIO builds from host, port and credentials does
  not set `interpolateParams`; a DSN given by the administrator
  (`dsn_string`) could, and is then checked the stricter way. MariaDB
  metadata skipping.
- **lib/pq 1.11–1.12**: `sslmode=prefer` and `allow`, `sslnegotiation`,
  `ssl_min/max_protocol_version` are now supported; `sslmode=verify-ca`
  no longer checks the host name when connecting by DNS name (it now does
  what libpq does); `~/.postgresql/root.crt` becomes the default
  `sslrootcert` when it exists (no such file in the release image); the
  `datestyle` startup parameter is sent. The default `sslmode` stays
  `require`; 1.12.0 announces that a later release may change it to
  `prefer`.

## Tests (media-gw, golang:1.27.4)

Unit, on this branch:

- `go mod tidy -compat=1.21` a second time: no change. `go build ./...`:
  ok. Cross-compile, all fifteen targets: ok.
- golangci-lint v2.15.0: 0 issues. govulncheck v1.10.0: no
  vulnerabilities. GitHub advisories for every new module version: none.
- `go test -race` for `internal/event/...` and `internal/store/...`: ok.
  `go test -tags kqueue,dev ./...` as uid 65534: 49 packages ok.

Live, the `aursu.2` image against this branch built from source, the
same brokers for both, each target with a queue directory: RabbitMQ
4.4.1 (durable fanout exchange, delivery mode 2, publisher confirms on, a
bound durable queue counted through the management API), Mosquitto
2.0.23 (QoS 1, a subscriber with a persistent session, broker
persistence on), Redis 8.8.4 (`access` format, list length), MySQL 8.4.13
and PostgreSQL 18.7 (`access` format, row count; PostgreSQL with
`sslmode=disable`). Per version: 20 PUTs, every broker stopped, 1 PUT,
brokers started, 5 PUTs.

| Target | aursu.2 | this branch |
|---|---|---|
| AMQP | 26/26 | 26/26 |
| MQTT | 26/26 | 26/26 |
| Redis | 26/26 | 26/26 |
| MySQL | 26/26 | 26/26 |
| PostgreSQL | 26/26 | 26/26 |

Both versions logged the same errors while the brokers were down ("not
connected to target server/service", and `driver: bad connection` once
for PostgreSQL), and delivered every queued event after the restart.

Not tested live: TLS to any of these brokers, and the MySQL handshake
crash itself (it needs a server that sends a truncated greeting).
