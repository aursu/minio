# aursu/minio — fork orientation

Fork of `minio/minio` (upstream archived, maintenance mode). Goal: run
**RELEASE.2025-10-15T17-29-55Z** (the last server release) with the **full web
console** of RELEASE.2025-04-22T22-12-26Z, SSO/OIDC login above all, for use as
a test object store.

## Repos

| Local path | GitHub | Work branch | Role |
|---|---|---|---|
| `/Users/aursu/workspace/code/minio` | `aursu/minio` | `full-console` | server; holds all notes |
| `/Users/aursu/workspace/code/console` | `aursu/console` | `full-console` (from `v1.7.6`) | web console, Go module `github.com/minio/console` |
| `/Users/aursu/workspace/code/mds` | `aursu/mds` | — (`v1.0.4` pinned) | UI component library |

`master` in each repo mirrors upstream (the original history) and is never
committed to. Upstream `console` and `mds` no longer exist on GitHub.

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
