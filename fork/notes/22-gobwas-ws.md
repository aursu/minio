# 22 — github.com/gobwas/ws 1.4.0 → 1.5.0 (GO-2026-6940)

Date: 2026-11-23.

## Why

GO-2026-6940 (GHSA-4xr6-hc3h-f6jq), published 2026-11-23 20:02 UTC:
`ws.ReadFrame` and the `wsutil` message readers allocated a buffer of the
payload length declared in the frame header before any payload arrived; a
peer declaring a huge frame crashes the process out of memory. VulnCheck
turned red on every run after that (first on #45 at 21:15 UTC; #44's run
at 20:20 was before the advisory reached the database).

In MinIO the library carries the internode grid (`internal/grid`):
`grid.Connection.receive`, `grid.readStream`, `grid.Manager.IncomingConn`
and others reach the vulnerable readers. The grid handler checks the
peer's node credentials (`authReq`) before the WebSocket upgrade, so the
attacker is an authenticated peer or a compromised node, not anyone on
the network. Still reachable for govulncheck, and a cluster node should
not be able to crash another one with one frame header.

## What

`go get github.com/gobwas/ws@v1.5.0`, `go mod tidy`. 1.5.0 is the only
release after 1.4.0 and the first with the fix (`6b4858126`: payloads read
through `internal/saferio.ReadN`, at most 32 KiB allocated before the
bytes arrive). Other changes in 1.4.0..1.5.0 (31 commits): requires Go
1.26 (the fork builds with 1.27.4), `github.com/gobwas/pool` replaced by an
internal package (so it leaves `go.mod`), SIMD masking kernels, short-write
errors returned by frame writers, modernised code. No API removed;
`ws.ReadFrame` is deprecated, not gone.

Not taken: setting `wsutil.SetMaxMessageSize`/`SetMaxFrameSize` in the
grid. The fix removes the allocation on a declared length; a limit would
also bound what a peer actually sends, but the grid has no documented
message bound to choose from. A separate change if wanted.

## Tests (media-gw, golang:1.27.4)

- `go build -tags kqueue ./...`, `go vet ./internal/grid/ ./cmd/`: clean.
- `go test -race -tags kqueue,dev ./internal/grid/...`: ok.
- govulncheck v1.10.0 (the CI version): master — GO-2026-6940, "Your code
  is affected by 1 vulnerability"; this branch — "No vulnerabilities
  found".
