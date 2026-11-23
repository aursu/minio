# 18 — the release image workflow (plan 04, step 3.2)

Date: 2026-11-23.

## Why

Plan 04 decided that the image is built by a workflow of our own in
aursu/minio and published to `ghcr.io/aursu/minio`. Upstream's release
pipeline is not in the repository; its Dockerfiles downloaded from
dl.min.io (note 17).

## What

`.github/workflows/release-image.yml`, one job on `ubuntu-24.04`:

| Step | Does |
|---|---|
| Release name | for a tag: checks it is `RELEASE.<YYYY-MM-DDTHH-MM-SSZ>.aursu.<N>` (plan 04, "Version string"); otherwise a development build |
| Build | `Dockerfile.release`, linux/amd64, `RELEASE` build argument, loaded into the runner's Docker; BuildKit cache in the GitHub Actions cache |
| Version stamp | for a tag: `minio --version` must print exactly `minio version <tag> (commit-id=<tagged commit>)` |
| Console login through Keycloak | `fork/test/bin/sso-smoke.sh` (note 17) with `TEST_ENV=../.env.example`: `consoleAdmin` → admin API 200, `readonly` → 403 |
| Log in, Publish | for a tag only: the checked image is tagged `:<tag>` and `:latest` and pushed with `GITHUB_TOKEN` |

Triggers:

- push of a tag matching `RELEASE.*.aursu.*` — the release;
- a pull request into master that touches the image's inputs
  (`Dockerfile.release*`, the workflow, `gen-ldflags.go`, `install-mc.sh`,
  `dockerscripts/`, the SSO test) — build and checks, no push;
- `workflow_dispatch` — build and checks of a branch, no push.

A release is made by pushing an annotated tag on a master commit, e.g.

    git tag -a RELEASE.2026-11-24T00-00-00Z.aursu.1 -m "…" <master commit>
    git push origin RELEASE.2026-11-24T00-00-00Z.aursu.1

The tag's time is the release time stamped into the binary; it is chosen
when tagging, not read from the clock.

## Decisions and alternatives

- **The pushed image is the checked image.** `Publish` runs `docker tag`
  and `docker push` on the image the checks ran against. A second
  `build-push-action` with `push: true` was the first draft: even with
  every layer from the cache, it produces a new image whose config was
  not checked.
- **`:latest` is pushed with every release**, as the community image was
  published (plan 04, "Image layout": open choices follow the
  full-console community image).
- **Actions are pinned by commit SHA** (version in a comment), unlike the
  other workflows' major tags: this job has `packages: write`. Versions
  as of 2026-11-23: checkout v7.0.1, setup-buildx v4.4.1, build-push
  v7.4.0, login v4.8.0.
- **The SSO test runs in the release job**, so a tag whose image cannot
  log in through an IdP is never published. It adds about a minute.
- **Pull-request runs are limited by paths**, so the 30-minute job does
  not run on unrelated changes; the server's own suites still run there.
- Not in this step, open for later: a GitHub Release object and the
  release report (plan 04, 3.2: govulncheck, UI audit and OSV state; to
  be written per release in `fork/reports/`), SBOM/provenance
  attestations, image signing. arm64 is not built (decided 2026-11-23).

## Tests

- `actionlint` 1.7.12 (with its shellcheck pass): no findings.
- The tag check, run in bash against
  `RELEASE.2026-11-24T00-00-00Z.aursu.1` and `….aursu.12` (accepted) and
  `…Z-aursu.1`, `…T00:00:00Z…`, `….aursu.`, `….aursu.1x` (rejected).
- The build, version and SSO steps run on this pull request itself (its
  paths match). The tag path (version check, push) runs first on the
  first release; its result goes into that release's report.
- To check after the first push: the package's visibility on GitHub
  (packages created from a workflow may start private).
