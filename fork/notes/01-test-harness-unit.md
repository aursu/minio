# 01 — Test harness: server unit tests

Plan 04, step 0.1 (first part). Date: 2026-11-19.

## What

`fork/test/bin/unit-tests.sh` runs the server's Go unit tests in Docker with
the recipe that produced the clean baseline in report 03. Configuration comes
from `fork/test/.env` (template `.env.example`); logs go to `fork/test/results/`.

## Why

Every later change is judged against that baseline. A one-liner in a report is
easy to run differently by accident (as root, or with `/.dockerenv` present) —
and either mistake produces failures or a hang that look like regressions. One
script, one way to run.

## How it works

1. Module download as root into the shared cache volume, and the build-cache
   volume handed to uid 65534. Tests then mount the module cache read-only.
2. Tests as uid 65534 after `rm -f /.dockerenv`, through `setpriv` — the
   `golang` image has no `sudo`, and the file must go before dropping root.
3. Flags as `make test` (`-tags kqueue,dev`, `MINIO_API_REQUESTS_MAX=10000`),
   so local runs and CI (`go-lint.yml` runs `make test`) test the same thing.
   The baseline in report 03 used `-tags kqueue`; this run re-establishes it
   with the `make test` flags.

## Why not the obvious alternatives

- **`make test` in a container:** it first runs `verifiers` (lint, gofmt,
  install tools) and builds the binary; slower, needs network for tool
  installs, and still runs as root.
- **`docker run --user 65534`:** the user could not delete `/.dockerenv`, and
  `TestCreateServerEndpoints` would hang.
- **Fresh build cache per run:** correct but adds minutes to every run; a named
  volume owned by 65534 keeps it.
