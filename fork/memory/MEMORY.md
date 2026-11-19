# Fork memory index

One line per fact file. Update the file, not just this line.

- [Console is a separate module](console-module-boundary.md) — the whole web UI is `github.com/minio/console`; the server uses 4 of its packages, same in both releases.
- [Original console source](console-source-go-proxy.md) — upstream `console`/`mds` repos are gone (2026-11-19); **original code only**; history in `bexsoft/*` mirrors, proven against proxy zips + upstream `go.sum`; no community forks.
