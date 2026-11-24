# 27 — The Forwarded header (RFC 7239) as a trusted proxy header

Date: 2026-11-25.

This is PR 2 of the seven for phase 4 item 5, stacked on note 26 (#51).
Alexander asked for the standard header next to the de-facto ones.

## What

1. **`feat(handlers)`: `trusted_proxy_header=forwarded`.**
   - **The address.** It comes from the `for` parameter of each
     `Forwarded` element. The right-to-left walk past trusted proxies is
     the same as for `X-Forwarded-For`. Several header fields form one
     list.
   - **Parsing (RFC 7239, section 4).** Elements are split at `,` and
     parameters at `;`, but only outside quoted-strings. Quoted-pairs
     (`\x`) are unescaped, and the parameter name is matched without
     case. A quoted IPv6 address in brackets, with or without a port, is
     accepted, and so is IPv4 with a port.
   - **An element that is not an address stops the walk** at the last
     trusted proxy. That covers an element with no `for`, an element with
     `for` twice, `unknown`, and obfuscated identifiers (`_hidden`).
   - **Only `for` is used.** `by`, `host` and `proto` are ignored here;
     the scheme is PR 3.
   - **`SetForwardingHeaders`** sets all three headers to one address.
2. **`fix(sftp,ftp)`: the SFTP and FTP servers pass their client address
   in every forwarding header.** They call the S3 API over loopback and
   set only `X-Forwarded-For`. With `trusted_proxy_header` set to
   `x-real-ip` (possible since #51) or `forwarded`, the S3 API saw
   127.0.0.1 as the client. `aws:SourceIp` conditions then failed for
   SFTP and FTP users, and the audit log lost their address. The live
   check below shows it on the #51 image.

3. **`fix(handlers)`: the forwarder appends a `Forwarded` element.**
   Requests forwarded to a federated deployment or a site replication
   peer went out with the client's own `Forwarded` header untouched. A
   receiving deployment that trusts the forwarder with `forwarded` would
   take the client's last element as the client. The forwarder now adds
   `for=<peer>;proto=<scheme it received>`, as RFC 7239 asks of a proxy.
   The test resolves the forwarded request on the receiving side.
   Found while writing PR 3.

## A defect of #51 found here

The live SFTP check showed that `MINIO_API_TRUSTED_PROXIES=` did not
trust "no proxy". It kept the loopback default, because MinIO reads an
empty environment variable as unset. The fix is on the #51 branch
(`fix(handlers): "none" trusts no proxy, also from the environment`),
with a correction marked in note 26. This branch is rebased onto it.

## Alternatives rejected

- **A regular expression for `for=`**, as upstream had for the old
  header reading. It cannot respect quoted-strings, so a `,` or `;`
  inside a quoted value shifts the elements. A test covers each case.
- **Taking the header that is present.** The header stays configured;
  see note 26.

## Tests (media-gw, golang:1.27.4, `-tags kqueue,dev`)

- **Unit.**
  - 19 `Forwarded` cases in `TestGetSourceIP`, plus one where it is not the chosen header: the RFC 7239 section 7
    examples, quoting, separators inside quoted-strings, quoted-pairs,
    non-address elements and duplicate `for`.
  - `TestSetForwardingHeaders` checks that a loopback client passes its
    address whichever header is trusted. It also checks the exact
    `Forwarded` value, bracketed and quoted for IPv6 (RFC 7239,
    section 6).
- **Mutations: ten, all caught.** At first four survived, and the tests
  that catch them were added:
  - splitting at every comma;
  - splitting at every semicolon;
  - an escaped quote ending a quoted-string;
  - IPv6 without brackets in the setter.

  One mutation did not compile at first and was rewritten.
- **Lint and build:** golangci-lint v2.15.0 reports 0 issues, and
  `go build ./...` passes.
- **Live** (`scratch fwd/e2e.sh`). An SFTP user may `GetObject` only from
  the client subnet (`aws:SourceIp`); the anonymous policy is the one of
  note 26.

  | check | #51 image | this branch |
  |---|---|---|
  | header `forwarded`, client subnet trusted: `Forwarded: for=203.0.113.7` | 403 (invalid value, no proxy trusted) | 200 |
  | same: `Forwarded: for=203.0.113.7, for=198.51.100.9` | 403 | 403 |
  | same: `Forwarded: for="[2001:db8::7]", for=203.0.113.7` | 403 | 200 |
  | same: `X-Forwarded-For: 203.0.113.7` | 403 | 403 |
  | SFTP get, header `x-forwarded-for` | downloaded | downloaded |
  | SFTP get, header `x-real-ip` | **refused** | downloaded |
  | SFTP get, header `forwarded` | refused (invalid value) | downloaded |
  | SFTP get, `MINIO_API_TRUSTED_PROXIES=none` | — | refused (client is 127.0.0.1) |

## Not in this change

- **The embedded console** passes its client only in `X-Forwarded-For`
  (console `api/tls.go`). With `x-real-ip` or `forwarded`, console logins
  are recorded as 127.0.0.1. Fixing it is a console PR plus a dependency
  bump here; it is offered separately.
- **The scheme** (`proto=`, `X-Forwarded-Proto`) is PR 3.
