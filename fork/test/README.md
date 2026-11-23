# Fork test harness

Local test runs in Docker, for pre-checks before a PR and for what CI does not
cover. GitHub Actions remains the PR gate (plan 04, step 0.2).

    cd fork/test && cp .env.example .env     # once
    bin/unit-tests.sh                        # all server packages
    bin/unit-tests.sh ./cmd/ ./internal/...  # selected packages
    DOCKER_CONTEXT=media-gw bin/sso-smoke.sh <image>   # console SSO login

| Script | Runs | Baseline (unchanged tree) |
|---|---|---|
| `bin/unit-tests.sh` | server `go test`, flags as `make test` | all packages pass |
| `bin/sso-smoke.sh <image>` | console login through Keycloak: `policy=consoleAdmin` → admin API 200, `policy=readonly` → 403 | `PASS` (note 17) |

`unit-tests.sh` logs land in `results/` (gitignored); `sso-smoke.sh` prints
to the terminal, with the server's log on failure. Exit status is the test
result.

Why the unit run is unprivileged and without `/.dockerenv`:
[server-tests-in-docker](../memory/server-tests-in-docker.md).
