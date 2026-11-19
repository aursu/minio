# aursu/minio — fork orientation

Fork of `minio/minio` (upstream archived, maintenance mode). Goal: run
**RELEASE.2025-10-15T17-29-55Z** (the last server release) with the **full web
console** of RELEASE.2025-04-22T22-12-26Z, SSO/OIDC login above all, for use as
a test object store.

## Repos

| Local path | GitHub | Role |
|---|---|---|
| `/Users/aursu/workspace/code/minio` | `aursu/minio` (GitHub fork of `minio/minio`) | server; holds all notes |
| `/Users/aursu/workspace/code/console` | `aursu/console` | web console, Go module `github.com/minio/console`; full console restored from `v1.7.6` |
| `/Users/aursu/workspace/code/mds` | `aursu/mds` | UI component library, `v1.0.4` pinned, no changes planned |

Upstream `console` and `mds` no longer exist on GitHub; their original history
is in these repos (see [fork/memory/](fork/memory/)).

## Branches and pull requests

- `master` is the only long-lived branch in every repo.
- Every change reaches `master` through a PR from a short-lived branch,
  deleted after merge. No direct pushes to `master`.
- `aursu/minio` is a GitHub fork: PRs must target `aursu/minio:master`, not
  `minio/minio`.
- Cherry-pick original commits with `-x`; merge PRs with a merge commit or
  rebase, never squash.

All notes, memory and rules for this fork live **in this repo**, under
[fork/](fork/). Read [fork/memory/MEMORY.md](fork/memory/MEMORY.md) at the start
of every session.

## Layout of fork material

| Path | Holds |
|---|---|
| [fork/memory/](fork/memory/) | one fact per file, indexed in `MEMORY.md` |
| [fork/reports/](fork/reports/) | numbered reports: what was found, what was done, which option lost |
| [fork/notes/](fork/notes/) | one development note per code change: what, why, why not the obvious alternative |

Upstream's own `docs/` stays untouched, so it can still be diffed against upstream.

## Rules

- **Reviewable increments.** One unit of change per step, small enough to read
  in one sitting. A mechanical import (vendored source, generated files) goes in
  a commit of its own, with nothing else in it, and says how to verify it.
- **Measure, don't infer.** Every premise is checked against code, a tag, or a
  build before it is acted on. Facts carry an absolute date.
- **Upstream tags are the reference.** Compare against
  `RELEASE.2025-04-22T22-12-26Z` (full console) and
  `RELEASE.2025-10-15T17-29-55Z` (target), never against memory.
- **No secrets** in the repo: no keys, client secrets, tokens. Test IdP
  credentials are throwaway and generated at test time.
- **Language.** Dialogue may be in Russian; everything written into the repo
  (docs, notes, commits, comments) is in English.
- Builds and tests run in Docker; the host has no Go toolchain.
