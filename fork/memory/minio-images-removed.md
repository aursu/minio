---
name: minio-images-removed
description: MinIO's public server and mc images are gone from Docker Hub and Quay; use Alexander's copies in ghcr.io/aursu/minio/* (public) — last official server, mc, and the Dex/OpenLDAP test images.
metadata:
  type: reference
---

Upstream state, checked 2026-11-19: `minio/minio`, `minio/mc` on Docker Hub
return 404; `quay.io/minio/minio`, `quay.io/minio/mc` are not pullable.
Anything that pulls them — upstream workflows, the console's `Makefile` test
targets — must be pointed at the copies below.

Copies in Alexander's registry, all public, amd64, verified 2026-11-19:

| Image | Digest | Notes |
|---|---|---|
| `ghcr.io/aursu/minio/minio:RELEASE.2025-03-12T18-04-18Z` | `sha256:fe066789…` | last official server image MinIO published; embeds console **v1.7.6** — the reference for how the restored console behaves |
| `ghcr.io/aursu/minio/mc:RELEASE.2025-08-13T08-35-41Z` | `sha256:bdfae21c…` | last official `mc` image; use it for tests instead of building `mc` |
| `ghcr.io/aursu/minio/dex:latest` | list `sha256:d166f4d4…` → `sha256:2faa1449…` | identical to `quay.io/minio/dex:latest` (original manifest inside the list) |
| `ghcr.io/aursu/minio/openldap:latest` | list `sha256:072da886…` → `sha256:45afc9fa…` | identical to `quay.io/minio/openldap:latest` |

Copied with `docker buildx imagetools create`, which wraps the original
single manifest in a manifest list: compare the inner digest, not the top one.

Related: [[aistor-not-usable]], [[console-source-go-proxy]].
