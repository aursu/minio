---
name: aistor-not-usable
description: AIStor images (quay.io/minio/aistor/*) are under a proprietary evaluation licence that forbids reverse engineering; keep them out of the fork, its tests and CI, and derive security fixes only from public advisory text.
metadata:
  type: convention
---

`quay.io/minio/aistor/minio` and `quay.io/minio/aistor/mc` are public to pull
(e.g. `RELEASE.2026-09-19T17-05-25Z`), source `github.com/miniohq/aistor`
(private). Their licence (`/licenses/LICENSE` in the image, "MinIO Software
License", read 2026-11-19):

- without a paid Enterprise Agreement: "one instance … for non-production,
  internal evaluation purposes only";
- "You may not reverse engineer, decompile or disassemble … any portion of
  the Software."

Rules for this fork, decided 2026-11-19:

- No AIStor binary in the fork's build, tests or CI — the original AGPL `mc`
  copy covers testing ([[minio-images-removed]]).
- Security fixes (plan 04, phase 4) are written from the public advisory text
  and our own reading of the AGPL source — never from AIStor binaries. This
  keeps the fork's code clean AGPL and its provenance defensible.
