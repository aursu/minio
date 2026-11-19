---
name: minio-images-removed
description: MinIO's public server and mc images are gone from Docker Hub and Quay; the Dex/OpenLDAP test images still exist and are copied to ghcr.io/aursu before they vanish too.
metadata:
  type: gotcha
---

Checked 2026-11-19:

| Image | State |
|---|---|
| `minio/minio`, `minio/mc` (Docker Hub) | repository 404 |
| `quay.io/minio/minio`, `quay.io/minio/mc` | not pullable |
| `quay.io/minio/dex:latest` | pullable — `sha256:8a30a48b…`, amd64, built 2024-05-15 |
| `quay.io/minio/openldap:latest` | pullable — `sha256:f29419ae…`, amd64, built 2025-05-13 |

Anything that pulls a MinIO image — upstream workflows, the console's
`Makefile` test targets (`minio/mc`, `quay.io/minio/minio:latest`) — fails or
will fail. Our server image is built from this repo; `mc` from
`github.com/minio/mc` source. Dex and OpenLDAP are copied by digest to
`ghcr.io/aursu/` (plan 04, step 0.0) and tests point there.

Related: [[console-source-go-proxy]].
