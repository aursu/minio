# 26 — Client address only from trusted proxies

Date: 2026-11-24.

This is the first of seven PRs for phase 4 item 5 (CVE-2026-33419, LDAP
user enumeration and unlimited password guessing). The rate limits that
fix that CVE count failures per client address. That address has to be
trustworthy first, and it was not.

## Why

`handlers.GetSourceIP` took the client address from `X-Forwarded-For`,
`X-Real-IP` or `Forwarded`, in that order. It read them from any sender,
and it took the **leftmost** entry, which is the one the client writes
itself. A proxy that appends its peer does not help.

That address is used by:

- **the `aws:SourceIp` policy condition** (`cmd/bucket-policy.go`). A
  bucket that allows access only from one network can be opened by any
  client with one header. Checked live on the `aursu.1` release image, in
  the table below;
- **the audit log and the `Host` field of bucket notifications**, which
  recorded the forged address;
- **the login rate limits** of the next PRs, which would be bypassed the
  same way.

The hidden switch `_MINIO_API_XFF_HEADER=off` turned off `X-Forwarded-For`
only; `X-Real-IP` and `Forwarded` were still read.

The fix is ours. The upstream code at the fork point trusts the headers
from anyone, and the advisory of CVE-2026-33419 does not mention the
client address.

## What (PR #51)

1. **`fix(handlers)`: take the client address only from trusted
   proxies.**
   - **The rule.** The client address is the connection peer. Only when
     the peer is a trusted proxy is the configured header read: from the
     right, past every trusted proxy, to the first address that is not
     one. A hop that is not an IP address (`unknown`, a hostname, an
     empty entry) stops the walk at the last trusted proxy.
   - **`api trusted_proxies`** (`MINIO_API_TRUSTED_PROXIES`): IP
     addresses and CIDR prefixes. The default, `127.0.0.1/32,::1/128`,
     trusts loopback only. The embedded console and the SFTP and FTP
     servers call the S3 API over loopback and pass the address of their
     own client in `X-Forwarded-For`, so this default keeps their client
     addresses. `none` trusts no proxy, and so does an empty value of
     the config key. **Correction (2026-11-25):** this said "an empty value
     trusts no proxy" for the environment variable too. MinIO reads an
     empty environment variable as unset, so `MINIO_API_TRUSTED_PROXIES=`
     kept the loopback default; the live SFTP check of PR 2 showed it.
     `none` was added for that.
   - **`api trusted_proxy_header`** (`MINIO_API_TRUSTED_PROXY_HEADER`):
     `x-forwarded-for` (default) or `x-real-ip`. It is configured, not
     guessed. A proxy rewrites only its own header, and any other header
     still carries what the client sent.
   - **X-Real-IP is one address**, the last field if several are sent. A
     trusted address in it is the client, not a proxy to walk past.
   - **IPv4-mapped IPv6 addresses** are compared and reported as IPv4, and
     zones are dropped.
   - **Invalid values** fail the config lookup, as for other keys. The
     server applies the config even then; it then trusts no proxy, so
     the failure is closed. The config is merged with the defaults before
     the lookup, so a key that was never set has its default, and an
     empty value means what it says.
   - `_MINIO_API_XFF_HEADER` is removed, and `docs/config/README.md`
     documents the keys and the proxy setup.
2. **`fix(handlers)`: the forwarder sets `X-Real-IP`, never passes on
   the client's.** Requests forwarded to a federated deployment or a
   site replication peer went out with the client's own `X-Real-IP` if
   it had one. A receiving deployment that trusts the forwarder with
   `x-real-ip` would believe it. `X-Real-IP` is now the client address
   as the forwarding deployment resolved it. `X-Forwarded-For` needs
   nothing, because `httputil.ReverseProxy` appends the peer to it.
3. **`test(handlers)`**: X-Real-IP is one address, not a list. It was
   added after a mutation survived, see Tests.

## Client-visible change (for the release notes)

- **Behind a proxy that is not on loopback** (another host or container,
  a load balancer), the client address becomes the proxy's address until
  `trusted_proxies` lists it. This affects `aws:SourceIp` conditions, the
  audit log and notifications. Set `trusted_proxies` to the proxy
  addresses, and make the proxy append its peer to `X-Forwarded-For`
  (nginx: `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;`).
- **A federated deployment or a site replication peer** that forwards
  requests is a proxy as well, and must be listed to keep the client
  address.
- **`_MINIO_API_XFF_HEADER` no longer exists.**
- **The `Forwarded` header is not read** until PR 2 (note 27). Before,
  it was read only when neither of the other two headers was present.

## Alternatives rejected

- **Keep reading the headers when `trusted_proxies` is unset**, with a
  warning. That is the vulnerable behavior, and every rate limit built
  on it would be bypassable.
- **Trust private ranges (RFC 1918) by default.** In a container
  network, every other container is in a private range, including a
  compromised one.
- **Read whichever header is present.** A proxy that appends to
  `X-Forwarded-For` leaves a client's `X-Real-IP` untouched, so the
  header has to be named.
- **The leftmost untrusted entry**, as some libraries do. Everything
  left of the nearest untrusted entry was written by that untrusted
  sender.

## Tests (media-gw, golang:1.27.4, `-tags kqueue,dev`)

- **Unit.**
  - `internal/handlers`: config parsing, 21 resolution cases (chains,
    several header fields, ports, IPv6, mapped IPv4, non-address hops, no
    peer), the default list and the forwarder. The forwarder test is a
    real round trip through `httptest`.
  - `internal/config/api` (no tests there before): the default, a list,
    the empty value, environment overrides and invalid values.
  - Each commit was tested alone; vet and golangci-lint v2.15.0 are
    clean, and `go build ./...` passes.
- **Mutations, all caught:** an untrusted peer's header read; the walk
  not stopping at the first untrusted hop; walking from the left; a
  non-address hop skipped; no unmapping; the forwarder keeping the
  client's `X-Real-IP`; a lookup error falling back to the default; an
  empty value read as the default. X-Real-IP fields read as a list
  survived at first, so commit 3 adds the test that catches it.
- **Live** (`scratch trusted/e2e.sh`). A bucket allows anonymous
  `GetObject` only from `203.0.113.7` (`aws:SourceIp`). A client in
  another container on 172.29.77.0/24 forges that address. "loopback" is
  a client in the server's network namespace, as the embedded console is.

  | request | `aursu.1` | branch, default | branch, list = client subnet | same, `x-real-ip` |
  |---|---|---|---|---|
  | no header | 403 | 403 | 403 | 403 |
  | `X-Forwarded-For: 203.0.113.7` | **200** | 403 | 200 | 403 |
  | `X-Real-IP: 203.0.113.7` | **200** | 403 | 403 | 200 |
  | `Forwarded: for=203.0.113.7` | **200** | 403 | 403 | 403 |
  | `X-Forwarded-For: 203.0.113.7, 198.51.100.9` | **200** | 403 | 403 | 403 |
  | loopback, `X-Forwarded-For: 203.0.113.7` | — | 200 | 403 | 403 |

  The fourth column shows a trusted proxy that appended the real client
  (198.51.100.9) after a forged entry: the forged entry is ignored.

## Not in this change (the next PRs of item 5)

- **PR 2:** the `Forwarded` header (RFC 7239), as
  `trusted_proxy_header=forwarded`. The SFTP and FTP servers will set it
  as well.
- **PR 3:** the scheme (`X-Forwarded-Proto`, `X-Forwarded-Scheme`,
  `proto=`) is still read from any sender. It only sets the scheme of
  the `Location` URL in responses.
- **PRs 4–7:** CVE-2026-33419: one LDAP failure answer, then the login
  limits per account and address, per address, and per account, with
  every number configurable.
