# 34 — Trusted proxy values without case, strict hop parsing

Date: 2026-11-25.

This is a review follow-up to note 26 (#51), stacked on note 33.

## What

1. **`fix(handlers)`: read trusted proxy values without case, parse hops
   strictly.**
   - **Case.** `trusted_proxy_header` is read without case and without
     surrounding spaces, and so is the word `none` in
     `trusted_proxies`. Header names are case-insensitive (RFC 9110), and
     `X-Forwarded-For` copied from a proxy config means the same header.
     Before, it failed the config lookup. The header is still stored in
     canonical form.
   - **Hops.** A hop in a forwarding header is an address only when it
     has the right form:
     - brackets come in a pair, and only around IPv6;
     - a port is a decimal number up to 65535, or an obfuscated port of
       RFC 7239 (`node-port = port / obfport`, `_` followed by letters,
       digits, `.`, `_`, `-`).

     Before, `[2001:db8::1` and `2001:db8::1]` lost their bracket and
     were read as `2001:db8::1`. `net.SplitHostPort` also accepted
     `192.0.2.1:abc` and `[192.0.2.1]:80`. Such hops now stop the walk,
     like any other hop that is not an address. Since the address itself
     was always checked, this is about strictness, not security.

## Alternatives rejected

- **Keep lowercase only,** like `list_quorum` and the other enumerated
  api values. Those are MinIO's own words. A header name is not, and
  people copy it from proxy configs in its usual capitalization.
- **Reject obfuscated ports:** RFC 7239 allows them in `Forwarded`, and
  a proxy that hides its ports would then make every hop it writes a
  non-address.

## Tests (media-gw, golang:1.27.4, `-tags kqueue,dev`)

- **Unit.**
  - `TestParseTrustedProxiesCase`: `X-Forwarded-For`, `X-REAL-IP`,
    `Forwarded` and ` x-real-ip `; `NONE`, `None` and ` nOnE `.
  - `TestParseNode`: 30 forms, valid and invalid.
  - Everything passes with `-race`.
- **Mutations, all caught:** port not checked; bracket pair not required;
  brackets around IPv4 allowed; no obfuscated port; port above 65535;
  header case-sensitive; `none` case-sensitive; header not canonical.
- **Lint and build:** golangci-lint v2.15.0 reports 0 issues; `go build
  ./...` passes.
