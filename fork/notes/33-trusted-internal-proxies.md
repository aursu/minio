# 33 — Loopback and the cluster nodes are always trusted proxies

Date: 2026-11-25.

This is a review follow-up to note 26 (#51), stacked on note 27 (#52).
Alexander decided to fix it in code, not only in the documentation.

## Why

1. **A list replaced loopback.** `trusted_proxies` defaulted to
   `127.0.0.1/32,::1/128`. A deployment that listed its load balancer
   replaced that default. The embedded console and the SFTP and FTP
   servers call the S3 API over loopback, so they then lost the client
   address, unless the administrator knew to add loopback to the list.
2. **The cluster nodes were not trusted.** `proxyRequest`
   (`cmd/handler-utils.go`) forwards a request through
   `handlers.Forwarder` to another node of a distributed deployment.
   - **Callers:** pool status and decommission
     (`proxyRequestByNodeIndex`, `cmd/admin-handlers-pools.go`), heal
     status by client token (`cmd/admin-handlers.go`), and batch job
     cancel (`cmd/batch-handlers.go`).
   - **The effect.** The node that receives the request checks the
     policy again, and sees the forwarding node as the client. So an
     `aws:SourceIp` condition on such an admin action failed on a
     forwarded request, and the receiving node's audit log showed the
     node address. Before #51 a forwarding header was believed from
     anyone, so this used to work.
   - **Listings are not forwarded.** The review said `ListObjects` is
     forwarded, but in this tree it is not. `proxyRequestByToken` lives
     in `cmd/bucket-listobjects-handlers.go`, yet only the heal and batch
     handlers call it.

## What

1. **`fix(handlers)`: always trust loopback and the cluster nodes.**
   - **`trusts`.** Loopback (`127.0.0.0/8`, `::1`, also IPv4-mapped) and
     the cluster nodes are trusted proxies whatever the list says. The
     list names the external proxies only, and its default is now empty.
     `none` and an empty list name no proxy; loopback and the nodes
     stay trusted.
   - **`handlers.SetClusterNodes`** keeps the node addresses in an atomic
     set.
   - **`cmd/trusted-cluster-nodes.go`.** It takes the hosts of
     `globalProxyEndpoints` (URL endpoints, so distributed deployments
     only). They are resolved through `globalDNSCache` at startup and
     every minute, because a node that comes back may have a new
     address. A host that does not resolve keeps its last addresses; an
     IP literal is used as it is.
   - **Docs.** `docs/config/README.md` and the key help text describe
     this.
2. **`test(api)`: check the listed proxies, and clear every api variable
   read.**
   - The #58 tests used loopback as the proxy that an invalid setting
     must not trust. Loopback is always trusted now, so they use a listed
     `10.0.0.0/8` instead.
   - From the review of #60: `clearAPIEnv` also clears the legacy
     `_MINIO_OBJECT_MAX_VERSIONS` and the two trusted proxy variables, so
     a variable set in the shell cannot change these tests.

## What trusting them means

- **Loopback.** Any process on the host can write the header. Such a
  process can read the data anyway.
- **A node.** A node can claim any client address. It holds the root
  credentials anyway.
- **A multi-homed node.** If a node's requests to another node leave from
  an address its name does not resolve to, it is not recognized. The
  receiving node then reports the node address as the client, as before
  this change.

## Alternatives rejected

- **Documentation only** (the reviewer's suggestion): it leaves the trap
  in place for every deployment behind a proxy.
- **Loopback in the default list, only the nodes added implicitly:** two
  rules for one kind of thing, and the first trap stays.
- **Authenticate forwarded requests with the internode JWT in a header,
  instead of by address.** It would survive address changes. But that
  token is root-equivalent (storage REST accepts it), and request headers
  reach the receiving node's audit log and trace. Every path would have
  to strip it.
- **Trust the nodes' subnet:** it would trust any host on it.
- **Resolve the node names per request:** any client could trigger DNS
  lookups.

## Client-visible change (for the release notes)

- `api trusted_proxies` defaults to empty, and lists external proxies
  only. Loopback and the nodes of the deployment are always trusted. An
  explicit `127.0.0.1/32,::1/128` from the old default still works.
- In a distributed deployment, admin requests forwarded between nodes
  keep the client address: `aws:SourceIp` and the audit log see the
  client.

## Tests (media-gw, golang:1.27.4, `-tags kqueue,dev`)

- **Unit.**
  - `TestAlwaysTrustedProxies`: loopback, IPv6, mapped and other
    loopback addresses, node addresses with an empty and a non-empty
    list; a former node is no longer trusted.
  - `TestResolveClusterNodes`: lookups, IP literals, a node on a new
    address, a failed lookup keeping its addresses, a new node.
  - The `internal/config/api` tests reworked as above.
  - Everything passes with `-race`. The api tests also pass with
    `MINIO_API_REQUESTS_MAX=10000`, `_MINIO_OBJECT_MAX_VERSIONS=7`,
    `MINIO_API_TRUSTED_PROXIES=bogus`, `MINIO_API_TRUSTED_PROXY_HEADER`
    and `MINIO_API_CORS_ALLOW_ORIGIN` set.
- **Mutations, all caught:**
  - loopback not trusted;
  - nodes not trusted;
  - node addresses not unmapped;
  - a failed lookup forgetting a node;
  - an IP literal dropped.

  The mutation `err == nil || true` on the lookup survived. It is
  equivalent to the original, because a failed lookup returns no
  addresses and the last ones are kept.
- **Lint and build:** golangci-lint v2.15.0 reports 0 issues; `go build
  ./...` passes.
- **Live** (scratch `nodes/e2e.sh`). Two nodes, `n1` and `n2`, each with
  two drives. User `canceler` may run `admin:CancelBatchJob` only from
  the client's address (`aws:SourceIp`). A batch job id ends with the
  index of its node, so `replicate-e2e:1` sent to `n1` is forwarded to
  `n2`.

  | request | #52 | this branch |
  |---|---|---|
  | to `n2`, job of `n2` (not forwarded) | allowed | allowed |
  | to `n1`, job of `n2` (`n1` → `n2`) | **Access Denied** | allowed |

## Not in this change

- Trusted proxy values without case and strict hop parsing: note 34.
