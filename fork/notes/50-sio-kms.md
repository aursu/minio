# 50 — minio/sio 0.4.1 → 0.4.3, kms-go/kms → v0.6.0

Date: 2026-11-27. Plan 04 step 2.4, group F3 (note 40). Stacked on F1
(note 48; note 49 for F2).

## What

`go get github.com/minio/sio@v0.4.3 github.com/minio/kms-go/kms@v0.6.0`,
then `go mod tidy -compat=1.21`:

| Module | From | To | Latest |
|---|---|---|---|
| github.com/minio/sio | v0.4.1 | v0.4.3 | v0.5.1 |
| github.com/minio/kms-go/kms | v0.5.1-0.20250225090116-4e64ce8d0f35 | v0.6.0 | v0.7.0 |

Nothing else moves; github.com/minio/kms-go/kes stays at v0.3.1, the
latest, and aead.dev/mtls at v0.2.1. No code change.

sio is DARE, the format of every SSE object, of IAM and config data
encrypted at rest, of bucket metadata secrets (remote target
credentials) and of sealed object keys. kms-go/kms is the client of the
MinIO (AIStor) KMS server, used only with `MINIO_KMS_SERVER`; KES and
the static key (`MINIO_KMS_SECRET_KEY`) do not use it.

Recommended, accepted (Alexander, 2026-11-27): sio v0.4.3, not v0.5.1;
kms-go/kms upgraded with build, unit tests and code review only, since a
live test would need an AIStor KMS server. v0.7.0 was the choice; it
fails the cross-compile (below), so v0.6.0 it is.

## sio

- v0.4.1 → v0.4.3: the package buffer pool returns buffers through a
  `sync.OnceFunc`, so a buffer cannot be recycled twice; sealing after
  the final package returns an error instead of panicking; encryption
  and decryption errors are wrapped with context. The DARE format and the
  reader and writer logic are unchanged.
- **The default cipher on arm64 changes.** `supportsAES` was
  `cpu.X86.HasAES && cpu.X86.HasPCLMULQDQ || s390x`, always false on
  arm64, so objects were encrypted with ChaCha20-Poly1305 there. Now
  arm64 with the AES and PMULL extensions gets AES-256-GCM. Most of the
  server's encrypt calls (`cmd/encryption-v1.go`, `internal/crypto/sse.go`,
  bucket metadata, sealed keys) set no `CipherSuites` and take this
  default. The cipher is recorded in every package header and both
  versions read both ciphers, so existing objects and mixed versions are
  unaffected; amd64 is unchanged.
- Why not v0.5.1: its reader, writer and DARE code are those of v0.4.3;
  it renames the constant `AES_256_GCM` to `AES_GCM` (same value) and
  accepts 128-bit keys, which MinIO never uses. It would need the rename
  in `internal/crypto/crypto.go`, `internal/config/crypto.go` and
  `cmd/admin-handlers.go` for nothing.

## kms-go/kms

- v0.6.0: the client calls the server makes (`Version`, `ListKeys`,
  `CreateKey`, `GenerateKey`, `Decrypt`), their request and response
  types, the config type and the error values are byte-identical to the
  current pseudo-version. `NewClient` now accepts a nil TLS config
  (the server always passes one). The rest is log-filter and HSM API the
  server does not use.
- **Why not v0.7.0:** it requires aead.dev/mtls v0.4.0, which does not
  build for 32-bit targets: `key.go` compares an `int` (an RSA exponent)
  with `math.MaxUint32`, an overflow on linux/arm, linux/386 and
  linux/mips. kms-go/kms v0.6.0 requires mtls v0.2.1. (Checked before
  that: v0.7.0 with mtls v0.4.0 keeps the identity of existing `k1:` and
  `k2:` API keys and of the client certificate built from them, so the
  move is safe once mtls builds everywhere.)

## Tests (media-gw, golang:1.27.4)

Unit, on this branch:

- `go mod tidy -compat=1.21` a second time: no change. `go build ./...`:
  ok. Cross-compile, all fifteen targets: ok.
- golangci-lint v2.15.0: 0 issues. govulncheck v1.10.0: no
  vulnerabilities. GitHub advisories for sio v0.4.3 and kms-go/kms
  v0.6.0: none.
- `go test -race` for `./internal/...` (`internal/crypto`, `internal/kms`
  included) and for the `cmd` tests matching encrypt, decrypt, SSE, KMS,
  crypto, compress, seal, ETag, bucket, multipart, copy, config and IAM
  (without `TestReplicationResync`, note 47): ok.
  `go test -tags kqueue,dev ./...` as uid 65534: 49 packages ok.
- **DARE across versions:** sio v0.4.1 and v0.4.3 each encrypt 13 sizes
  (0 bytes to 5 MiB, around the 64 KiB package size) with AES-256-GCM,
  with ChaCha20-Poly1305 and with the default, in DARE 1.0 and 2.0; each
  version decrypts the other's 78 ciphertexts: all identical to the
  plaintext. `EncryptedSize` matches the output.

Live: one deployment (four drives) runs, on the same drives, the
`aursu.4` image, then this branch, then `aursu.4` again; static KMS key,
compression of encrypted objects on (`.txt`). Each phase writes or
reads: SSE-S3 objects of 0 B to 40 MiB, single and multipart, in a
bucket with default encryption and per object; compressible SSE objects
of 100 B to 30 MiB; a user, a policy attachment and a service account
(IAM is encrypted at rest); a replication target on a second deployment
(its credentials are encrypted in the bucket metadata). Every object is
read whole and in ranges (first byte, across the first package
boundary, at 5 MiB, the last 1000 bytes); the service account reads an
object; replication must reach `COMPLETED`.

| Phase | Server | Result |
|---|---|---|
| 1 | aursu.4 writes set 1 | 16 of 16 objects ok; IAM ok; replicated |
| 2 | this branch: reads set 1, writes set 2 | 32 of 32 ok; both sets' users and service accounts work; replicated |
| 3 | aursu.4 reads both | 32 of 32 ok; IAM ok; replicated |

Phase 2 was run with this branch merged with the current master. Without
it, the ranged read at 5 MiB of a compressed SSE object fails on the
branch (the response ends before the body), and so it does with F1's
binary (sio v0.4.1): that is the defect fixed on master by note 37
(#71), which the stack D…F3 does not contain yet. aursu.4 has the fix
and read every object in phases 1 and 3.
