# 12 — the server embeds the full console (plan 04, step 2.1)

Date: 2026-11-22.

## What

```
go mod edit -replace github.com/minio/console=github.com/aursu/console@7832665c91d3
go mod tidy
```

`go mod tidy` turns the commit into its pseudo-version:

```
replace github.com/minio/console => github.com/aursu/console v1.7.7-0.20261120232317-7832665c91d3
```

`7832665c9` is aursu/console master after its #28: upstream's full console
v1.7.6, the UI and Go work of plan 04 phase 1 (#1–#28), Go 1.27.4. Before
this change the server embedded upstream's simplified console
`v1.7.7-0.20250905210349-2017f33b26e1`, the one with the admin pages
removed.

Server code is untouched. go.mod gains the replace line; in go.sum the two
lines (tree and go.mod hash) of `minio/console` give way to the two of
`aursu/console`. **No other module moves.** That is what
the three PRs before this one were for (#27 compress, #28 grpc and otel,
#31 prometheus and the go-openapi stack, notes 07, 08, 10): they raised
every module the console shares with the server to at least the console's
version, so Go's version selection has nothing left to raise here, and the
review of this PR is the console itself, not a dependency cascade.

## Why a replace, and why by commit

- The console's go.mod declares `module github.com/minio/console`, and the
  server imports that path. A replace keeps every import as it is; renaming
  the module would touch both repositories for no gain.
- The require line keeps upstream's pseudo-version. The replace ignores
  it, and changing it to a fake `minio/console` version would claim a
  release that does not exist.
- A commit, not a branch: go.sum pins the content, and the next console
  change reaches the server only through a PR here that moves the line.
- aursu/console is public, so proxy.golang.org serves it; no `GOPRIVATE`,
  no credentials in CI.

## GitHub advisories

`github.com/minio/console` has two: GHSA-4999-659w-mq36 (`< 0.12.3`) and
GHSA-jv3f-7m33-qp65 (`< 0.28.0`). The embedded console is 1.7.x, so neither
applies. `github.com/aursu/console` has none. No other module changed, so
nothing else to check.

## The console's CI

The console's "Integration Tests with Latest Distributed MinIO" and "SSO
Integration Test" build `aursu/minio` master with the console under test.
They appended a second `replace` for the console to go.mod, which after this
change fails with `conflicting replacements`. aursu/console #29 makes them
use `go mod edit -replace` instead (its note 17). **#29 should merge before
this PR**, or the console's CI breaks until it does.

#29's run is also this change's integration and SSO gate: it builds
`aursu/minio` master `29316c0c5` with console `7832665c9` plus a CI-only
change, which is the same server binary this branch builds.

## Tests

On the build host, Go 1.27.4:

| Check | Result |
|---|---|
| `go build ./...`, `go vet ./...` | ok |
| `make verifiers` (golangci-lint v2.15.0, check-gen) | 0 issues, tree clean after check-gen |
| govulncheck v1.10.0 | 11 reachable, **unchanged** from master: amqp091-go (10) and paho (1), plan step 2.3 |
| unit suite (`fork/test`) | PASS, 46 packages |

The console the server now serves, master against this branch: one-drive
server, `minioadmin` login through the embedded console's API on :9001,
then its admin endpoints:

| `GET /api/v1/…` | master (simplified console) | this branch (full console) |
|---|---|---|
| `login` (POST) | 204 | 204 |
| `users` | 404 | **200** |
| `groups` | 404 | **200** |
| `policies` | 404 | **200** |
| `admin/info` | 501 | **200** |
| `configs` | 404 | **200** |

Integration and SSO: aursu/console #29's CI (above).
