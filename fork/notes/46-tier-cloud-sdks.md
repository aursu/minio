# 46 — Azure and Google Cloud SDKs for tiering

Date: 2026-11-26. Plan 04 step 2.4, group E (note 40). Stacked on D2
(note 45).

## What

`go get`, then `go mod tidy -compat=1.21`:

| Module | From | To | Latest |
|---|---|---|---|
| github.com/Azure/azure-sdk-for-go/sdk/azcore | v1.21.0 | v1.24.0 | v1.24.0 |
| github.com/Azure/azure-sdk-for-go/sdk/azidentity | v1.13.1 | v1.15.0 | v1.15.0 |
| github.com/Azure/azure-sdk-for-go/sdk/storage/azblob | v1.6.1 | **v1.8.0** | v1.8.3 |
| cloud.google.com/go/storage | v1.56.0 | **v1.70.0** | v1.71.1 |
| google.golang.org/api | v0.272.0 | **v0.310.0** | v0.317.0 |

They follow: google.golang.org/grpc v1.83.2 → v1.84.0, the
cloud.google.com/go/{auth,iam,monitoring,compute/metadata} family,
gax-go, s2a-go, the Google OpenTelemetry exporters and otel contrib
instrumentations, genproto, felixge/httpsnoop, spiffe/go-spiffe and the
Azure internal and MSAL modules. No code change.

MinIO uses these for remote tiers only: `cmd/warm-backend-azure.go`
(`azblob` with a shared key or a service principal through `azidentity`)
and `cmd/warm-backend-gcs.go` (`storage.NewClient` with a credentials
file).

## Why not the latest three

Recommended, accepted (Alexander, 2026-11-26): the latest versions that
add nothing unrelated to the server binary.

- **azblob v1.8.1 and later require apache/arrow-go/v18** (for an
  optional Arrow format of `List Blobs` responses), and with it
  google/flatbuffers, golang.org/x/exp and zeebo/xxh3 v1.1.0. With
  v1.8.3, 21 arrow packages were linked into the server. MinIO's Azure
  tier puts, gets and deletes blobs and never asks for Arrow. zeebo/xxh3
  is a direct dependency of the server (`cmd/xl-storage-format-utils.go`,
  `internal/grid`), so its version belongs with the on-disk format group
  (B), not to a tier SDK. v1.8.0 needs none of it. 1.8.1 also changed the
  default upload and download concurrency to depend on the CPU count;
  that change is not in 1.8.0.
- **cloud.google.com/go/storage v1.71.x and google.golang.org/api
  v0.311.0 and later require grpc v1.85.0, and grpc v1.85.0 requires
  envoyproxy/go-control-plane/envoy at a pseudo-version**
  (`v1.39.1-0.20260819172001-e6e3fd93e4be`, a commit, not a release), which
  is linked into the server through grpc's xDS packages. storage v1.70.0
  with api v0.310.0 needs grpc v1.84.0, and envoy stays at the released
  v1.37.0.

Each move to the latest version can be made once the reason is gone:
an azblob that does not pull arrow, and a grpc on a released envoy.

## What changes

- azblob 1.7–1.8.0: newer service versions (up to 2026-06-06); from 1.8.0,
  `Expect: 100-continue` is sent for one minute after a 429, 500 or 503.
- azcore 1.22–1.24: retry logging redacts request URLs; the default HTTP
  client comes from `sdk/internal`.
- GCS storage 1.57–1.70 and the Google API client: new features and
  dependency updates; the tier's calls (writer, reader, delete, attrs)
  are unchanged.

## Tests (media-gw, golang:1.27.4)

Unit, on this branch:

- `go mod tidy -compat=1.21` a second time: no change. `go build ./...`:
  ok. Cross-compile, all fifteen targets: ok.
- golangci-lint v2.15.0: 0 issues. govulncheck v1.10.0: no
  vulnerabilities. GitHub advisories for every new module version (grpc
  v1.84.0 included): none.
- `go test -race` for `./internal/...` and for the `cmd` tests matching
  tier, warm backend, transition, lifecycle, Azure and GCS: ok.
  `go test -tags kqueue,dev ./...` as uid 65534: 49 packages ok.
- No arrow package in `go list -deps` of the server; grpc v1.84.0,
  envoy v1.37.0, zeebo/xxh3 v1.0.2.

Live, tiering, the `aursu.4` image and this branch built from source,
taking turns on one deployment (four drives). Remote tiers: Azure on
Azurite 3.38.0 (an account of its own, shared key) and GCS on
fake-gcs-server 1.56.1 (reached through `STORAGE_EMULATOR_HOST`, which
the GCS client honors; a generated service-account file). An ILM rule
with a past transition date moves `az/` and `gcs/` objects to the tier
right after the PUT. Objects: 1 KiB and 12 MiB (multipart). Reads: whole
(sha256), from offset 5 MiB (4 KiB) and the last 1000 bytes.

| Phase | Server | Checks | Result |
|---|---|---|---|
| 1 | aursu.4 | adds both tiers and the rule; puts, transitions, reads and restores its own objects | all ok, transition within 1 s |
| 2 | this branch | reads and restores aursu.4's tiered objects; puts, transitions, reads and restores its own | all ok |
| 3 | aursu.4 | reads and restores the branch's tiered objects | all ok |

The tier configuration written by aursu.4 was read by the branch and back.
Azurite held 2 blobs after phase 1 and 4 after phase 2 (the tier's probe
object is removed after the check).
