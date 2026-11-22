# 09 — the upgrade test prints why it failed

Date: 2026-11-22. A CI change only; no server code moves.

## Why

`buildscripts/minio-upgrade.sh` waits, after switching the cluster to the
new build, until an anonymous read of `minio-test/to-read/hosts` succeeds 8
times in a row (#16). On #28 that wait timed out on "Upgrade from
RELEASE.2019-12-19" in two CI attempts and once in eight local runs (report
05, flaky tests). Each failure left one line:

```
anonymous read of http://127.0.0.1:9000/minio-test/to-read/hosts did not succeed 8 times in a row within 120s
```

It does not say what the cluster answered (502 from nginx, 503 from a node
still starting, 403 if the anonymous policy were lost, a connection
refused), nor what the nodes logged. So the failure could not be tied to the
change under review or cleared of it: the decision to merge #28 rested on
pass counts alone.

## What

`wait_for_anonymous_read`:

- records each answer with `curl -w '%{http_code}'` instead of `curl -f`;
- prints a line each time the status changes, with its second, e.g.
  `anonymous read: HTTP 502 at 1s`, `anonymous read: HTTP 200 at 14s`;
- ends a passing run with `anonymous read ready after <N>s`, so passing runs
  show how close they came to the 120 s limit;
- on timeout prints the last status and up to 2 KiB of its body, then calls
  `print_cluster_state`.

`print_cluster_state` (new) prints `docker-compose ps -a`, `mc admin info
minio` (capped at 30 s) and the last 200 log lines of `minio1`…`minio4` and
`nginx`. Every command in it ends with `|| true`: a diagnostic must not
replace the failure it is reporting.

`curl` without `-f` returns 0 on an HTTP error, but a refused connection still
returns non-zero. `|| true` keeps the assignment's status at 0 either way, so
the loop never depends on whether an `ERR` trap or `set -e` is in effect, and
`-w` still prints `000` for the refused connection.

## Not done

- **The 120 s limit and the streak of 8 are unchanged.** Raising them could
  hide a real regression in startup time; first the diagnostics, then a
  decision on numbers.
- **No retry in the script.** A retry would turn the observation into a
  silent pass; CI reruns stay a visible, manual step.
- **Only this wait reports the state.** The checksum checks after it compare
  content the wait has just proven readable; they have not failed since #16.
- shellcheck v0.10.0 reports two old findings in `cleanup` (unquoted
  `${volume}` and `$(docker volume ls …)`); they are upstream lines, left
  alone.

## Tests

On the build host:

| Check | Result |
|---|---|
| shellcheck v0.10.0 | nothing in the new lines (two old findings, above) |
| the function against nginx, stubs for `mc` and `docker-compose`, path `/` | `HTTP 200 at 1s`, `ready after 8s`, rc 0 |
| the same, path `/missing` | 404 body printed, then `ps`, `mc admin info` and the five services' logs in order; rc 1 |
| `make test-upgrade` from 2019, full run, 2 times | both passed (270 s, 95 s); each printed `HTTP 200 at 1s` and `ready after 8s`, so on this host the cluster already answered when `add_alias`'s 20 s sleep ended |
