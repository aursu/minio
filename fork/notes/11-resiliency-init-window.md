# 11 — the resiliency setup outlasts nginx's 10-second lockout

Date: 2026-11-22. A test-script change only; no server code moves.

## Why

On #31 (prometheus 0.311.3) "Resiliency" failed before its first test. The
initial setup, `docs/resiliency/resiliency-initial-script.sh`, got ten
`502 Bad Gateway` answers from nginx to `mc alias set` and gave up:

```
11:01:27.18  sleep to wait for MinIO Server to be ready prior mc commands
11:01:27.22  mc: <ERROR> Unable to initialize new alias … 502 Bad Gateway
             … 9 more, one a second …
11:01:37.43  retry: timeout while running: mc alias set
```

The same start was timed on the build host, master against the #31 branch,
5 rounds each, probing `/minio/health/cluster` through nginx every 0.25 s
from the moment `docker compose up -d` returned:

| | fast start | slow start |
|---|---|---|
| master | 3 of 5: 200 after 2.6–3.6 s, 503 before | 2 of 5: 200 after 10.9 and 11.2 s, **502 before** |
| #31 | 3 of 5: 200 after 2.3–3.6 s, 503 before | 2 of 5: 200 after 10.4 and 10.7 s, **502 before** |

Both builds have the same two modes, at the same rate. So this is not a
regression of #31; it is a race in the test.

The cause is in `docs/resiliency/nginx.conf`: the `minio` upstream lists its
four servers with nginx's defaults, `max_fails=1` and `fail_timeout=10s`.
When the first request arrives before the nodes listen on 9000, each
upstream fails once, nginx marks all four unavailable for 10 seconds and
answers 502 to everything in that time. The 503s of the fast mode are the
nodes themselves, already listening but not yet at quorum. The setup script
allowed `mc alias set` 10 attempts, 1 second apart: the length of the
lockout. On #31 the tenth attempt came less than a second before it ended.

## What

`TIMEOUT=10` → `TIMEOUT=60` in `resiliency-initial-script.sh`, with a
comment there on why. Nothing else in the script or the job changes.

## Not done

- **`max_fails=0` in `nginx.conf`.** It would remove the lockout, but the
  nginx configuration is part of what the resiliency tests exercise: the
  later tests stop nodes behind this same upstream. Waiting longer changes
  only how patient the setup is, not the system under test.
- **No wait on `/minio/health/cluster` before `mc alias set`.** `mc ready`
  right after the alias already waits for the cluster; the alias step only
  needs nginx to forward at all.
- **`./mc ready` keeps no timeout.** The job's own timeout bounds it, and it
  has not hung.
- **Not run: `docs/resiliency/resiliency-tests.sh`.** Its `cleanup` removes
  every container on the host and prunes all images and volumes (report 05,
  lessons). The test below calls only the setup script, against its own
  compose project.

## Tests

On the build host (Docker 29.9.3, compose v2.35.1), the minio binary and the
compose image built from this branch, `mc` from
`buildscripts/install-mc.sh`. Each round starts a fresh cluster with `docker
compose up -d` and runs the setup script at once, as `resiliency-tests.sh`
does; rounds alternate the old script (master) and the new one.

| Script | Rounds | Result | `mc alias set` failures | Setup time |
|---|---|---|---|---|
| old, `TIMEOUT=10` | 6 | **6 failed**, `retry: timeout while running: mc alias set` | 10 each | 10–11 s |
| new, `TIMEOUT=60` | 6 | **6 passed**, alias added, bucket created, data copied | 10 or 11 | 11–12 s |

All 123 failures, in both scripts, were nginx's `502 Bad Gateway`. With the
setup script as the cluster's first client, this host hit the lockout in
every round, where the 0.25 s health probe above hit it in 2 of 5: how
often it happens depends on when the first request lands, which is why CI
runners see it only now and then. Each time, the 11th attempt succeeded,
which is the lockout ending, not the nodes coming up late.

shfmt v3.10.0 with the CI's `runner / shfmt` option, `-s -d`, on the
changed script: no output, rc 0.
