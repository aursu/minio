---
name: server-tests-in-docker
description: MinIO server unit tests in Docker must run as non-root and without /.dockerenv, or 7 tests fail and TestCreateServerEndpoints hangs forever.
metadata:
  type: gotcha
---

Found 2026-11-19 while taking the test baseline (`fork/reports/03-baseline.md`).

- **Root:** `TestReadDirFail` and six `TestXLStorage*` tests chmod paths and
  expect "access denied"; root reads them anyway → 7 false failures.
- **`/.dockerenv`:** `IsDocker()` (`cmd/update.go`) turns on "orchestrated"
  mode; `UpdateIsLocal` (`cmd/endpoint.go`) then waits for a host that
  resolves to localhost to "resolve to a valid IP" — forever.
  `TestCreateServerEndpoints` hangs at ~1% CPU until the `go test` timeout.
  Diagnose a hang with `kill -QUIT <test pid>` inside the container: it dumps
  the stacks and `go test` moves on.

Run as uid 65534 after `rm -f /.dockerenv`, with `GOCACHE`/`HOME` in `/tmp`
and the module-cache volume read-only. The full command is in report 03.
With it, every package of the unchanged tree passes.
