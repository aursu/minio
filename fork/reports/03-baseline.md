# 03 — Baseline: build, vulnerabilities, tests

Date: 2026-11-19. Phase A of [02-plan](02-plan.md). Nothing in any repo was
changed; variants were built in scratch copies.

## A1 — build

`full-console` (= `upstream/master` + docs) builds unchanged in `golang:1.24`
(Go 1.24.13), `CGO_ENABLED=0`, `-tags kqueue -trimpath`: 2 min 42 s, binary
reports `go1.24.13 linux/amd64`.

The same tree with `github.com/minio/console` downgraded to `v1.7.6`
(`go get …@v1.7.6 && go mod tidy`) **also builds unchanged** — Phase B step 3
is confirmed before it is made.

## A2 — vulnerabilities

### Go dependencies (`govulncheck` v1.10.0, analysed under Go 1.24.13)

| Variant | Reachable (called) | Imported only | Required only |
|---|---|---|---|
| as shipped (console v2.0.4) | 71, from 9 modules + stdlib | 16 | 22 |
| with console v1.7.6 | 71, identical list | 16 | 22 |

The full console adds no reachable Go vulnerability. All 71 have a fixed
version:

| Source | Count | Fixed by |
|---|---|---|
| standard library (`net/http`, `crypto/tls`, `crypto/x509`, `html/template`, `net/url`, `encoding/*`, `archive/tar`, `os`) | ~33 | Go ≥ 1.26.11 — toolchain bump |
| `golang.org/x/crypto` v0.37.0 | 13 | v0.56.0 |
| `rabbitmq/amqp091-go` v1.10.0 | 11 | v1.14.0 |
| `golang.org/x/net` v0.39.0 | 8 | v0.60.0 |
| `prometheus/prometheus` v0.303.0 | 4 | v0.311.3 |
| `google.golang.org/grpc` v1.72.0 | 2 | v1.83.1 |
| `otel/sdk` v1.35.0 | 2 | v1.45.0 |
| `x/text`, `go-jose/v4`, `paho.mqtt.golang`, `klauspost/compress` | 1 each | x/text v0.39.0, go-jose v4.1.4, paho v1.5.1, compress v1.20.2 |

`govulncheck` does not report the main module itself — see the next table.

### MinIO server code (OSV + GitHub advisories, after 2025-10-15)

| CVE | Sev | What | In our base? |
|---|---|---|---|
| CVE-2025-62506 | high | session-policy bypass in service accounts / STS | **fixed** (`c1a49490` is in the tree) |
| CVE-2026-33322 | critical | JWT algorithm confusion in **OIDC** auth: whoever knows the `ClientSecret` forges tokens, gets any policy | open |
| CVE-2026-33419 | critical | LDAP STS: user enumeration + no rate limit | open |
| CVE-2026-40344 | high | unsigned-trailer uploads: object write without signature (Extract/Snowball path, signature check) | open |
| CVE-2026-41145 | high | unsigned-trailer uploads: query-string credential signature bypass | open |
| CVE-2026-34204 | high | SSE metadata injection through `X-Minio-Replication-*` headers on plain PutObject | open |
| CVE-2026-39414 | high | S3 Select CSV: unbounded line read → OOM | open |
| CVE-2026-42600 | medium | path traversal in `ReadMultiple` storage-REST (distributed mode only, needs root JWT) | open |

The advisories list upstream `master` `7aac2a2c5` — our base — as affected.
Fixes exist only in the closed MinIO AIStor; `minio/minio` will not receive
them. Each advisory describes the flaw and, for some, the fix (CSV: cap line
scan at 128 KiB; ReadMultiple: handler removed). Fixing them here means writing
our own patches.

### Console UI (`yarn npm audit`, v1.7.6 lockfile, Node 18, yarn 4.4.0)

278 advisories: 9 critical, 105 high, 133 moderate, 31 low. Walking the
lockfile from `package.json` `dependencies` leaves 4 critical / 39 high — an
over-approximation, because this Create React App project lists
`react-scripts` itself under `dependencies` (that drags in `tar`, `glob`,
`minimatch`, `brace-expansion`). What reaches the browser bundle:

| Package | Issue | Note |
|---|---|---|
| `@remix-run/router` 1.18.0 (`react-router-dom` 6.25.1) | XSS via open redirect | upstream later took 6.30.1 |
| `jspdf` 2.5.2, `canvg` 3.0.10 (via `react-component-export-image`) | HTML/PDF injection, ReDoS, prototype pollution | upstream later dropped the package and pinned `jspdf` ^3 |
| `lodash` 4.17.21 | `_.template` code injection | only if `_.template` is used — check |
| `decode-uri-component` (via `query-string`) | DoS | |

The remainder is build/test tooling (`webpack-dev-server`, `testcafe`,
`playwright`, `node-gyp`, `knip`), not shipped.

## A3 — tests

| Suite | Result |
|---|---|
| console v1.7.6 Go unit tests (`./api/... ./pkg/...`) | **pass**, 9 packages with tests |
| server unit tests, all packages except `cmd` | **pass**, 45 packages |
| server `cmd` package (280 s) | **pass** — only with the recipe below |

Two traps, both in the test environment, neither a code defect
([server-tests-in-docker](../memory/server-tests-in-docker.md)):

1. As root, 7 storage tests fail (`TestReadDirFail`, `TestXLStorage*`): they
   make paths unreadable and expect "access denied"; root reads anyway.
2. With `/.dockerenv` present, `TestCreateServerEndpoints` hangs forever:
   `UpdateIsLocal` (`cmd/endpoint.go`) treats a host that resolves to
   localhost inside a container as "not resolved yet" and keeps waiting.

Recipe (the baseline every later change is compared with):

    docker run --rm -v <repo>:/src:ro -v minio-gomod:/go/pkg/mod:ro -w /src golang:1.24 \
      sh -c 'rm -f /.dockerenv; setpriv --reuid=65534 --regid=65534 --clear-groups \
        env HOME=/tmp GOCACHE=/tmp/gocache CGO_ENABLED=0 GOFLAGS=-buildvcs=false PATH=$PATH \
        go test -count=1 -timeout 60m -tags kqueue ./...'

Test suites available for later phases, all original:

- console `integration/` — Go suite against live servers (`make test-integration`);
- console `sso-integration/` — Dex + OpenLDAP + server, walks the SSO login
  flow (`make test-sso-integration`); images `quay.io/minio/dex`,
  `quay.io/minio/openldap`, Dex config in the repo;
- console `web-app/tests` — 36 TestCafe/Playwright UI suites (permissions-*);
- server `make test-iam` (external IDP), `test-iam-import-with-openid`,
  `test-iam-ldap-upgrade-import`.
