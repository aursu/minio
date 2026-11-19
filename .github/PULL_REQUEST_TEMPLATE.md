<!--
Target: aursu/minio:master. This repo is a GitHub fork; make sure the base
repository is NOT minio/minio.
-->

## What

<!-- The change in one or two sentences. One PR = one reviewable unit. -->

## Why

<!-- The problem it solves; link the plan step or advisory (fork/reports/…). -->

## Why not the obvious alternative

## Source

- [ ] original upstream code (cherry-picked with `-x`, or a pin to an original commit)
- [ ] our own code (fork-only change)
- [ ] mechanical output (generated files, bundle, `go.sum`) — how to reproduce:

## Tests

<!-- What ran and the result, compared with the baseline (fork/reports/03-baseline.md).
     For a security fix: the regression test that fails without the change. -->

## Checklist

- [ ] Development note in `fork/notes/` (code changes)
- [ ] No secrets in the diff
- [ ] Merge with a merge commit or rebase — not squash
