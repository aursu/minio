# 36 — Storage RPC volumes and paths confined to the drive (plan 04, phase 4 item 7)

Date: 2026-11-26.

Follow-up of note 35 (CVE-2026-42600): the same flaw in every other
storage call. No advisory of its own.

## Why

Nodes reach each other's drives through the storage API: a volume (a
bucket, or a metadata volume such as `.minio.sys/tmp`) and a path inside
it. `xlStorage` joined both to the drive path with `pathJoin`, which
resolves `..` and confines nothing.

- **The grid calls** (the internode websocket) take them from a msgpack
  body. 18 grid handlers are registered: 17 `storage*RPC` plus
  `WalkDir`. All but `DiskInfo` take a volume, a path or a bucket
  (`NSScanner`).
- **The HTTP storage calls** take the volume and path from the query.
  The request validity middleware checks those: `setAuthMiddleware` fills
  `r.Form` from the query before it runs. But several handlers take paths
  from the msgpack body, which nothing checks:
  - `ReadParts` and `DeleteBulk` (their paths);
  - `DeleteVersions` (the names);
  - `VerifyFile` (`FileInfo.DataDir`).

  `StatInfoFile` and `CleanAbandonedData` read no body, so a
  form-encoded POST body reaches `r.Form` only through `ParseForm` in
  `IsValid`, after the middleware.
- **The only check**, `getVolDir`, refused a volume that was exactly
  `""`, `.` or `..`. `bucket/../..` passed, and so did `/`, which
  `pathJoin` turns into the drive root. Paths were not checked at all.

The caller needs the internode token, which every node and anyone with the
root credentials holds (note 35). With it, on `master`, the tests below
read a file outside the drive, wrote new files and created directories
there, moved a file out, and deleted it; the live check deleted a file
beside the drives through `DeleteBulk`, middleware and all. In a container running as root
that is the host filesystem: a leaked root key or one compromised node
becomes a write to any node's host, not only access to the data.

## What (one commit)

**`fix(storage)`: confine storage RPC volumes and paths to the drive.**

- **One rule: no `.` or `..` segment**, through `hasBadPathComponent`.
  This is the check the request validity middleware applies to S3 URLs and
  query values, and `isValidObjectName` applies to object names. So no
  bucket, object or prefix a client can name contains such a segment, and
  the rule refuses nothing a node legitimately sends.
- **Volumes:** `getVolDir` refuses them with `errVolumeAccessDenied`.
  It also refuses a volume of separators only (`/`, `///`, `\`), which
  names the drive root itself: every bucket, `format.json` and
  `.minio.sys` at once. Every `xlStorage` method that takes a volume,
  including `origvolume`, `srcVolume` and `dstVolume`, goes through it;
  `WalkDir` too. A leading `/` before a real volume (`/bucket`) stays
  allowed: `pathJoin` keeps it below the drive.
- **Paths:** `checkPathArgs` refuses them with `errFileAccessDenied`.
  Every method that joins a caller's path calls it first, with the
  secondary paths from the same request:
  - `FileInfo.DataDir` in `RenameData`, `CheckParts`, `VerifyFile` and
    `DeleteVersion`;
  - `DeleteOptions.OldDataDir` in `DeleteVersion`;
  - `skipParent` in `RenamePart`;
  - every name in `DeleteVersions`, `DeleteBulk` and `ReadParts`;
  - `WalkDirOptions.BaseDir` in `WalkDir`.

  `WriteMetadata` needs no `DataDir` check: `xlMeta.AddVersion` parses it
  as a UUID.
- **`NSScanner`** refuses a bucket name with such a segment: the scanner
  joins it to the drive path.
- Both errors are already mapped across the storage REST client, so a
  remote caller sees the same error as a local one.

## Alternatives rejected

- **Check in the grid and HTTP handlers**, as granary also does (#82,
  `hasBadBodyPath`), instead or as well. `xlStorage` is the one place every
  route reaches, local calls included, and a handler added later is
  covered without anyone remembering to. "The same as HTTP" is no
  argument for it either: the HTTP handlers that read paths from the body
  have no early check.
- **Resolve the path and check that it stays below the drive.** It also
  needs the symlink question answered (MinIO follows them inside drives),
  and it would still let `bucket/../other-bucket` through, which no
  client can name either.
- **A new storage REST version**, to refuse old peers: nothing a release
  sends changes, so there is nothing to refuse.

## Public fixes compared (2026-11-26)

A checklist, no code taken (memory `minio-fork-working-method`):

- **relvo-labs/granary #82** (`6dcdcfb57e86`): the same rule in
  `getVolDir` and a `checkPathArgs` in `xlStorage`, plus early checks in
  the grid handlers. Its list of secondary paths (`DataDir`, `OldDataDir`,
  `skipParent`, `BaseDir`, the scanner's bucket) matches the one above,
  which was made from our tree.
- **pgsty/silo** `ca7baa670d06`: a guard wrapped around the remotely
  exposed `StorageAPI`, checking every path-bearing method and nested
  metadata field, rejecting traversal and volume-root aliases, plus a
  raw-volume check in `getVolDir` for peer S3 calls. The review of #67
  found the root alias here independently. Our `getVolDir` covers those calls too (peer bucket operations
  end in `MakeVol`, `StatVol` and `DeleteVol`). It also validates erasure
  geometry and part sizes in `FileInfo`; see "Not in this change".

## Client-visible change (for the release notes)

None. A storage call with a `.` or `..` segment in a volume or path now
fails; no client can name one, and no release sends one.

## Tests (media-gw, golang:1.27.4, `-tags kqueue,dev`)

- **`TestStorageRESTPathConfinement`** (`cmd/xl-storage-path-confinement_test.go`).
  - **The setup.** Two in-process servers on a real grid. A directory
    beside the drive holds a file and a copied `xl.meta`.
  - **The calls.** Through the storage REST client a node uses:
    `ReadAll`, `ReadFileStream`, `StatInfoFile`, `ListDir`, `ReadXL`,
    `ReadVersion`, `WriteAll`, `WriteMetadata`, `CreateFile`,
    `AppendFile`, `RenameFile` (both directions), `Delete`,
    `DeleteVersion`. Each one is sent once with the volume and once with
    the path pointing there.
  - **The checks.** Every call must fail, and nothing beside the drive
    may change.
  - **On `master`:** 19 of the 28 calls succeed. The file is read
    through `ReadAll` and `ReadFileStream`, then moved away or deleted,
    and 5 files are created beside the drive.
  - **No middleware in this test.** `newStorageRESTHTTPServerClient`
    registers the handlers without `globalMiddlewares`. So the HTTP calls
    with the volume and path in the query (`CreateFile`, `AppendFile`,
    `ReadFileStream`, `StatInfoFile`) succeed here on `master`, while a
    real server's middleware refuses them. The grid calls and the body
    paths are unaffected; the live check covers the real router.
  - **Correction (2026-11-26, review of #67):** the first version of this
    note said the HTTP storage calls were not protected, citing
    `CreateFile` and `AppendFile` in this test. That was the missing
    middleware; only the body paths bypass it.
- **`TestXLStoragePathArguments`:** the secondary paths above, `MakeVol`,
  `StatVol`, the scanner's bucket, and the drive-root volumes `/`, `///`
  and `\` get the access-denied errors. Names with dots that are not `.`
  or `..` segments (`.hidden`, `a/.b/c..`, `..d/e`) and `.minio.sys/tmp`
  still work. On `master` all 17 fail; `MakeVol` there creates a
  directory outside the drive.
- **Mutations: 13, all caught.** For each, one check was removed:
  - `getVolDir`, its `..` check and its drive-root check;
  - `checkPathArgs` itself;
  - the checks of `ListDir`, `Delete`, `RenameFile`, `ReadParts`,
    `DeleteVersions`, `NSScanner` and `WalkDir`;
  - the secondary paths `OldDataDir`, `CheckParts`' `DataDir` and
    `skipParent`.
- **The whole `cmd` and `internal` test suites** as uid 65534 with
  `-race`: see the PR (`TestXLStorageVerifyFile` skipped; it panics in
  this container on `master` too, note 35).
- **Lint and build:** golangci-lint v2.15.0 reports 0 issues, `go vet` and
  `go build ./...` pass.
- **Live** (scratch `item7/e2e.sh` and `item7/poc`): two nodes, each with
  its drives on one tmpfs at `/mnt`. A client mints the internode token
  from the root credentials and sends `DeleteBulk` to node 2 through its
  real router and middlewares. The volume goes in the query, the paths
  in the body.

  | request | `master` | this branch |
  |---|---|---|
  | volume `.minio.sys`, path `../../victim` | `/mnt/victim` deleted | `file access denied`, still there |
  | volume `/`, path `rootplant` | `/mnt/data1/rootplant` deleted | `volume access denied`, still there |

  With `/tmp/victim` as the target (another filesystem), `master` reached
  it but failed with `invalid cross-device link`. A delete moves the file
  into the drive's trash, so on one filesystem the delete succeeds.
  The grid calls were not driven live: their client needs a full node.
  The unit test runs them over a real grid between two servers.

## Not in this change

- **Malformed `FileInfo` from a peer** (erasure geometry, part sizes),
  which pgsty also validates: that is a crash or a corrupt write by a
  holder of the internode token, not an escape from the drive. Not
  looked at here.
- **Deleting a whole volume through `Delete` with an empty path.** It
  stays inside the drive, and the same caller may call `DeleteVol`.
