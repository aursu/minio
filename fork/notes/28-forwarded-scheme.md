# 28 — The client's scheme only from trusted proxies

Date: 2026-11-25.

This is PR 3 of the seven for phase 4 item 5, stacked on note 27 (#52).
It closes the last forwarding header that MinIO read from any sender.

## Why

`GetSourceScheme` read `X-Forwarded-Proto`, `X-Forwarded-Scheme` and the
`proto` parameter of `Forwarded` from any sender, and accepted any value.
It sets the scheme of the `Location` URL that a browser-form upload
(PostObject) and CompleteMultipartUpload return. On the `aursu.1`
release, a client that sends `X-Forwarded-Proto: javascript` gets back
`Location: javascript://…`. The client harms only itself, so the impact
is low. But the rule of note 26 is that a forwarding header counts only
from a trusted proxy, and this was the last header outside it.

## What (PR #53)

1. **`fix(handlers)`: take the client's scheme only from trusted
   proxies.** Only `http` and `https` are accepted.
   - **With `trusted_proxy_header=forwarded`:** the `proto` parameter of
     the element where the client address was found. The proxy the client
     connected to wrote that element, so its `proto` is the scheme the
     client used (RFC 7239, section 5.4).
   - **Otherwise:** the last value of `X-Forwarded-Proto`, then of
     `X-Forwarded-Scheme`. The last value is the one the nearest proxy
     wrote; an earlier one may come from the client.
   - **The address walk now also returns the hop the client was found
     in.** A `Forwarded` element yields both `for` and `proto`; either
     given twice counts as missing.
   - **Removed: the old regular expressions and `proxy_test.go`.** Its
     tests checked that headers were trusted from any sender.
     `TestObjectLocation` now sends its scheme headers from loopback,
     which is trusted by default, and has a case for an untrusted peer.
     **Correction (2026-11-25, merge of #61):** loopback is no longer a
     default that a list replaces; it is always trusted (note 33). The
     test comments say so.
2. **`fix(handlers)`: the forwarder sets `X-Forwarded-Proto`, never
   passes on the client's.** It used to set the header only when the
   request had none. It now sends the scheme the client used: as a
   trusted proxy in front reported it, or as received.

Writing this PR also turned up a defect in #52. The forwarder passed on
the client's own `Forwarded` header. The fix is on the #52 branch, as
commit 3 in note 27, and this branch is rebased onto it.

## Client-visible change (for the release notes)

Behind a TLS-terminating proxy that is not on loopback, `Location` URLs
say `http` until `trusted_proxies` lists the proxy. This is the same
condition as for the client address (note 26).

## Tests (media-gw, golang:1.27.4, `-tags kqueue,dev`)

- **Unit:** 20 scheme cases in `TestGetSourceScheme`, `TestObjectLocation`
  in `cmd`, and two forwarder round trips (a client's scheme replaced;
  a trusted proxy's scheme passed on). Commit 1 was tested alone.
- **Mutations: eight, all caught.**
  - the scheme taken from an untrusted peer;
  - the first value of `X-Forwarded-Proto` taken instead of the last;
  - any scheme accepted;
  - the `proto` of the nearest `Forwarded` element used instead of the
    client's;
  - `proto` given twice accepted;
  - no `X-Forwarded-Scheme` fallback;
  - the forwarder passing the client's value on;
  - the forwarder ignoring a trusted proxy's scheme.

  One mutation did not compile at first and was rewritten.
- **Lint and build:** golangci-lint v2.15.0 reports 0 issues, and
  `go build ./...` passes.
- **Live** (`scratch proto/e2e.sh`): the scheme of the PostObject
  `Location` header, for a client in another container.

  | request | `aursu.1` | branch, default | branch, subnet trusted | same, `forwarded` |
  |---|---|---|---|---|
  | no header | http | http | http | http |
  | `X-Forwarded-Proto: https` | https | http | https | http |
  | `X-Forwarded-Proto: javascript` | **javascript** | http | http | http |
  | `X-Forwarded-Scheme: https` | https | http | https | http |
  | `Forwarded: for=192.0.2.1;proto=https` | https | http | http | https |

  The first version of the check passed headers through `eval` and lost
  their quoting, so `-H` sent nothing. That is why every row first read
  `http`, on the release image too. The release column now shows the
  headers arriving.

## Not in this change

- **`X-Forwarded-Host` and `X-Forwarded-Port`** are set by the forwarder
  only when absent. MinIO itself reads neither, so nothing in this
  deployment depends on them.
- PRs 4–7 are the CVE-2026-33419 fix itself.
