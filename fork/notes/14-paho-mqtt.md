# 14 — paho.mqtt.golang v1.5.1 (plan 04, step 2.3)

Date: 2026-11-22.

## What

```
go get github.com/eclipse/paho.mqtt.golang@v1.5.1
go mod tidy
```

| Module | Before → after | Why |
|---|---|---|
| `github.com/eclipse/paho.mqtt.golang` | 1.5.0 → **1.5.1** | 1 reachable govulncheck finding (GO-2025-4173, GHSA-32fw-gq77-f2f2), fixed in 1.5.1 |

Nothing else moves: paho 1.5.1 requires `x/net` 0.44, `x/sync` 0.17 and
`gorilla/websocket` 1.5.3, all below what the server already has.

The finding: `packets.encodeBytes` wrote a 16-bit length, so a string
longer than 65,535 bytes produced a wrong length followed by extra data that
the broker would parse as later fields. 1.5.1 truncates instead. The path
is MinIO's MQTT notification target (`internal/event/target/mqtt.go`,
`Connect` and `Publish`); the strings encoded that way are the topic,
client id, user name and password, all from MinIO's own configuration. The
event payload does not go through `encodeBytes`.

## What else changed in 1.5.1, against MinIO's use

The whole diff from 1.5.0 was read (`client.go`, `net.go`, `netconn.go`,
`options.go`, `packets/packets.go`, `router.go`, new `connnotf.go`).

- **Connection notifications** (`OnConnectionNotification`): a new,
  optional callback; nil unless set. MinIO does not set it.
- **Router and ACK handling** (`router.go`, `net.go`): acknowledgements for
  *incoming* publishes after the connection is gone are now dropped
  instead of racing a closed channel. MinIO only publishes; it subscribes
  to nothing.
- **Proxy selection for `tcp://` and `mqtt://` brokers** (`netconn.go`).
  This one can change behaviour. 1.5.0 used a proxy only when the
  lower-case `all_proxy` variable was set, and then ignored the client's
  dialer (and its connect timeout). 1.5.1 always calls
  `proxy.FromEnvironmentUsing(dialer)`, which reads `ALL_PROXY` and
  `all_proxy`, honours `NO_PROXY`, and keeps the dialer. A deployment that
  sets only the upper-case `ALL_PROXY` will now reach its MQTT broker
  through that proxy, unless the broker is listed in `NO_PROXY`. Without
  any of these variables nothing changes.

## GitHub advisories

`affects=github.com/eclipse/paho.mqtt.golang@1.5.0` returns
GHSA-32fw-gq77-f2f2 (the check's positive control); `@1.5.1` returns none.

## Tests

On the build host, Go 1.27.4:

| Check | Result |
|---|---|
| `go build ./...`, `go vet ./...` | ok |
| `make verifiers` (golangci-lint v2.15.0, check-gen) | 0 issues, clean after check-gen |
| govulncheck v1.10.0 | reachable 11 → 10: the paho finding is gone; the 10 left are amqp091-go, fixed by note 13's PR |
| unit suite (`fork/test`) | PASS, 46 packages |

Against a real broker, Mosquitto 2.0.22, master (paho 1.5.0) and this branch
built from source. Target `PRIMARY`, broker `tcp://127.0.0.1:1883`, topic
`minio`, QoS 1; a `mosquitto_sub` at QoS 1 counts the events.

| Step | master | this branch |
|---|---|---|
| 20 PUTs, events received | 20 distinct | 20 distinct |
| broker stopped, 1 PUT | PUT ok; log: `not connected to target server/service` once | the same |
| broker started again, 5 PUTs | `after1`…`after5` | the same |

No proxy variables were set in the test, so the proxy change above is
covered by reading, not by a run.
