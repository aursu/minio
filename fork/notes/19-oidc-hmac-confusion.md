# 19 — CVE-2026-33322: OIDC JWT algorithm confusion (plan 04, phase 4 item 1)

Date: 2026-11-23.

## Why

GHSA-5cx5-wh4m-82fh, critical. `PopulatePublicKey`
(`internal/config/identity/openid/jwt.go`) registered the OIDC client
secret as an HMAC verification key under `kid = <client ID>`, and
`Validate` accepted HS256/HS384/HS512. Anyone who knows the client secret
can sign a token with any claims (`sub`, `aud = <client ID>`,
`policy = consoleAdmin`) and exchange it through STS
`AssumeRoleWithWebIdentity` for credentials with that policy; the IdP and
its keys are never involved. The retry after a JWKS refresh also parsed
with the package-level `jwtgo.ParseWithClaims`, which has no algorithm
allow-list. The behaviour came from upstream `7be65f66b` (2022-11, "support
HS256 series of JWT signature for OpenID connect"); `TestJWTHMACType`
asserted it.

The advisory's only fix is in AIStor (closed source, not used). minio/minio
is archived and never changed `jwt.go` after 2024-09.

## Public fixes compared (2026-11-23)

All of them do the same three things: the client secret is no longer a
verification key, HS* is removed from the allowed methods, the retry uses
the restricted parser. None keeps HMAC as an option.

| Source | Beyond the three | Tests |
|---|---|---|
| pgsty/silo `d24f449e0891` (2026-04-15; in silo releases since 2026-04-17; imprezahost/impreza-s3-server carries the same change) | key map typed `crypto.PublicKey` — which is `any`, so no actual restriction | +214 lines: HS256 rejected, RS256 accepted, refresh/retry |
| jarvit/minio-image patch `0005-CVE-2026-33322-oidc-reject-hmac-id-token.patch` (2026-10-29) | key type must match the algorithm family | +137 lines |
| relvo-labs/granary `f54f82e1bf69` (2026-11-21) | key type must match the algorithm family (`verificationKeyFor`) | +291 lines: HS256/384/512 with the client secret, `alg=none`, the RSA public key (PEM, DER, modulus) as an HMAC secret, RS/ES family mismatches, a key not in the JWKS, valid RS256/RS512/RS3256/ES256 — each on the preloaded and the refresh path |

**Taken: granary's commit** (recommended, accepted). The fix is the same
in all three, so pgsty's longer track record weighs less than test
coverage; granary's tests cover the most attack shapes. Its key-family
check is defence in depth — golang-jwt already rejects a mismatched key
type (see the baseline below) — and it handles the fork's SHA3 methods,
which are `*SigningMethodRSA`/`*SigningMethodECDSA`
(`ecdsa-sha3_contrib.go`, `rsa-sha3_contrib.go`).

Rejected: our own fix (identical in substance to the three; the rule is
original code where it exists); making HMAC opt-in (whoever holds the
secret can still forge, by HS256's design); accepting HS* only when the
provider's discovery document lists it (Keycloak lists HS256 too).

## What

`git cherry-pick -x f54f82e1bf69` from relvo-labs/granary, author kept.
`jwt.go` and `jwt_test.go` are byte-identical to granary's change. In
`jwt_alg_test.go`: the `arn` import is `github.com/minio/minio/internal/arn`
(granary renamed its module), in gofmt order, and the comment's "issue
#70" names `relvo-labs/granary#70`.

Behaviour change: an IdP that signs `id_token`s with HS* is no longer
accepted. Keycloak, Dex, Entra ID and Google sign with RS256 by default.

`fork/test/bin/sso-smoke.sh` gains an STS check (second commit): a
Keycloak `id_token` (password grant; the test client now allows direct
access grants) must be exchanged for credentials — the control — and an
HS256 token signed with the client secret, `kid = client ID`,
`policy = consoleAdmin` must be refused. The release workflow runs this
script, so the regression is guarded at every release.

## Tests (media-gw, golang:1.27.4)

- **Test first.** granary's tests on master, without the fix:
  `TestValidateRejectsSymmetricAndMismatchedAlgorithms` fails in exactly
  the six client-secret cases (HS256/384/512 × preloaded/refresh) — "forged
  token was accepted; claims=…sub:attacker-chosen-identity" — and the
  inverted `TestJWTHMACType` fails. `alg=none`, the RSA-key-as-HMAC-secret
  variants, family mismatches and unknown keys already fail on master
  (golang-jwt's own checks); valid RS/ES tokens pass.
- **With the fix:** the `openid` package, 56 tests and subtests, all
  pass; `go vet`, `gofmt`, `go build ./...` clean; golangci-lint v2.15.0
  with the repo config on the package: 0 issues.
- **End to end**, `sso-smoke.sh` with Keycloak 26.8.2:

| Image | Console logins | STS, Keycloak id_token | STS, forged HS256 token |
|---|---|---|---|
| master (`local/minio-release-test:aursu.0`) | 200 / 403 | 200, credentials | **200, credentials** — exploit reproduced, script fails |
| this branch | 200 / 403 | 200, credentials | 400 `InvalidParameterValue`, script `PASS` |

## Not in this change

All OpenID providers share one key map, and `Validate` checks neither the
issuer nor which provider a key came from: a key from one provider's JWKS
is accepted for another provider's role ARN (reported in
relvo-labs/granary#75). Needs a separate change.
