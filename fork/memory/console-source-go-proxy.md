---
name: console-source-go-proxy
description: Upstream console/mds repos are deleted; original history survives in bexsoft mirrors, proven commit-for-commit against the Go proxy and upstream go.sum. Original code only — no community forks.
metadata:
  type: gotcha
---

As of 2026-11-19 `github.com/minio/console`, `github.com/minio/object-browser`
and `github.com/minio/mds` (the React component library of the UI) all return
"Repository not found".

**Rule: original MinIO code only.** Community forks that continue development
are not used.

## Where the original history survives

`github.com/bexsoft/console` and `github.com/bexsoft/mds` — mirrors kept by
Benjamin Perez, a former MinIO console developer. Pushed to `aursu/console`
and `aursu/mds` 2026-11-19 (`master` + original tags only). Verified 2026-11-19:

| Object | Proof it is original |
|---|---|
| console `v1.7.6` = commit `f4a08fc0af8f` | proxy `@latest` Origin names this commit for `minio/console` `refs/tags/v1.7.6`; `git archive` equals the proxy zip (0 differing files); zip `h1:E0jq9n…` equals upstream `go.sum` at `RELEASE.2025-04-22T22-12-26Z` |
| console `master` = `2017f33b26e1` (v2.0.4) | the commit `RELEASE.2025-10-15T17-29-55Z` pins; `git archive` equals the proxy zip; zip `h1:jOW1gg…` equals upstream `go.sum` |
| mds `027fb7f9834e` (v1.0.4) | pinned by commit in original console v1.7.6 `web-app/yarn.lock`; ancestor of mirror `master` |
| mds `400914d72cb3` | pinned by commit in original console v2.0.4 `yarn.lock`; ancestor of mirror `master` |

A git commit id is a hash of the whole tree, so a commit pinned by an original
lockfile is original content whoever hosts it.

Other former-MinIO-developer repos checked 2026-11-19, nothing taken but one tag:

- `Alevsk/console` (Lenin Alevski): `master` at 2024-10-09, an ancestor of
  ours; its extra commits are 2022 feature branches. Nothing newer.
- `dvaldivia/mds` (Daniel Valdivia): carries the original tag
  `v1.0.4 → 027fb7f9` that `bexsoft/mds` lacks — **pushed to `aursu/mds`**, so
  the console's `mds.git#v1.0.4` resolves unchanged. Its other extras are a
  2024 v2.4/v3.x line and personal branches no console version pins; six
  lightweight tags (`v0.0.3`, `v2.0.0`–`v2.0.3`, `v2.2.4`) point elsewhere than
  the annotated originals — not used.

## Fallback

The Go proxy never deletes module zips (Go + React source + prebuilt bundle,
no history):

    curl -sSL -o console-v1.7.6.zip \
      https://proxy.golang.org/github.com/minio/console/@v/v1.7.6.zip

Related: [[console-module-boundary]].
