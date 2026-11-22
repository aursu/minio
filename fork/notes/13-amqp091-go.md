# 13 — amqp091-go v1.14.0 (plan 04, step 2.3)

Date: 2026-11-22.

## What

```
go get github.com/rabbitmq/amqp091-go@v1.14.0
go mod tidy
```

| Module | Before → after | Why |
|---|---|---|
| `github.com/rabbitmq/amqp091-go` | 1.10.0 → **1.14.0** | 10 reachable govulncheck findings, all fixed in 1.14.0 |

Nothing else moves: amqp091-go 1.14.0 requires only `go.uber.org/goleak`
(tests), already in the graph. The findings are reached from MinIO's AMQP
notification target (`internal/event/target/amqp.go`: `Dial`,
`Channel.Confirm`, `Channel.Publish`, …) and its config parsing
(`internal/config/notify`: `ParseURI`, `URI.String`).

## What changed in the library that MinIO could notice

The changelog for 1.11–1.14 is mostly hardening: frame-size limits before
and after negotiation, string length checks, TLS 1.2 minimum when the TLS
config comes from the URI, SASL credentials zeroed after the handshake,
data races fixed. Two changes alter behaviour, and neither applies to how
MinIO uses the library:

- **Automatic connection, channel and topology recovery (1.12, 1.13).**
  Off unless `Config.Recovery` is set; `amqp091.Dial`, which MinIO calls,
  leaves it nil. MinIO's own reconnect (re-dial when the connection is
  closed, `store.ErrNotConnected` on a refused connection) stays the only
  one.
- **Notifications no longer block the reader (1.13, #349).** In 1.10 a
  publisher confirmation was sent to each `NotifyPublish` listener with a
  blocking send. In 1.14 `notifyAll` waits at most 5 s (`notifyTimeout`)
  per listener and then drops the confirmation. MinIO opens a channel per
  event, registers a listener with a buffer of 1, publishes once and reads
  `<-confirms`: the one confirmation always fits the empty buffer at once,
  so nothing can be dropped. A caller that published several messages
  before reading confirmations could lose some; MinIO does not.

## GitHub advisories

`affects=github.com/rabbitmq/amqp091-go@1.10.0` returns 11 advisories (the
check's positive control); `@1.14.0` returns none.

## Tests

On the build host, Go 1.27.4:

| Check | Result |
|---|---|
| `go build ./...`, `go vet ./...` | ok |
| `make verifiers` (golangci-lint v2.15.0, check-gen) | 0 issues, clean after check-gen |
| govulncheck v1.10.0 | reachable **11 → 1** (paho, the next PR); imported but not called 5 → 4 |
| unit suite (`fork/test`) | PASS, 46 packages |

Against a real broker, RabbitMQ 4.1.4, master (amqp091-go 1.10.0) and this
branch built from source. Target `PRIMARY` with a durable fanout exchange,
delivery mode 2 and **publisher confirms on**; a queue bound to the
exchange is read through the management API.

| Step | master | this branch |
|---|---|---|
| 20 PUTs, events in the queue | 20 | 20 |
| broker stopped, 1 PUT | PUT ok; log: `not connected to target server/service` once | the same |
| broker started again, 5 PUTs | 5 events, keys `after1`…`after5` | the same |

So delivery with confirms and MinIO's reconnect after a broker restart
behave the same on both versions.
