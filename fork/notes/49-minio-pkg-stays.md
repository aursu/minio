# 49 — minio/pkg stays at v3.1.3

Date: 2026-11-27. Plan 04 step 2.4, group F2 (note 40). Decision: no
upgrade. Recommended, accepted (Alexander, 2026-11-27).

## Why not the latest

Every minio/pkg release after v3.3.0 changes who is allowed to do what,
or breaks LDAPS, and none of it is fixed in v3.11.0, the latest:

| Defect | First release | v3.11.0 |
|---|---|---|
| merging policies drops statements that differ only in `NotResource` | v3.3.1 | present |
| LDAPS ignores the server's TLS settings | v3.3.3 | present |
| built-in `readonly` denies `admin:CreateUser` | v3.3.4–v3.3.7 | present |
| a wildcard admin action pattern ignores `Resource` | v3.6.1 | present |

1. **Merging drops `NotResource` statements (from v3.3.1).**
   `dropDuplicateStatementsMany` identifies statements by a hash
   (`Statement.hash`) over effect, actions, not-actions, resources and
   conditions, but not `NotResource`. Two statements that differ only
   there count as duplicates, and one is dropped. The server merges a
   user's own and group policies with `MergePolicies`
   (`cmd/iam-store.go`). Example: one policy denies `s3:GetObject` with
   `NotResource arn:aws:s3:::x/*`, another the same with `y/*`; merged,
   v3.1.3 keeps three statements and denies `GetObject` on `x/o`; v3.3.1
   and later keep two and allow it.
2. **LDAPS without the server's TLS settings (from v3.3.3).** `connect`
   in `ldap/ldap.go` changed from `ldap.DialTLS("tcp", addr, l.TLS)` to
   `ldap.DialURL("ldaps://" + addr)` with no TLS option; go-ldap then
   dials with a nil `tls.Config`. The CAs in `certs/CAs`,
   `tls_skip_verify`, the configured server name and cipher suites are
   ignored, and only the system trust store is used. With a private CA,
   LDAP logins, `AssumeRoleWithLDAPIdentity` and LDAP config validation
   fail.
3. **`readonly` denies `admin:CreateUser` (between v3.3.4 and v3.3.7).**
   Built-in policies are loaded on every start unless overridden. A user
   with `readonly` can no longer change their own password (the
   self-service path checks deny statements only), and a user holding
   `readonly` with any `admin:CreateUser` grant can no longer create
   users. Probably intended upstream, but a change for existing users.
4. **Wildcard admin pattern ignores `Resource` (from v3.6.1).**
   `AdminAction.IsValid` became a wildcard match, so a statement whose
   action pattern matches any `admin:` name (`*Policy`, `*Get*`, `*List*`,
   `*Create*` …) counts as an admin statement, and the resource check is
   skipped for every action in it. Example:
   `{"Effect":"Allow","Action":["*Policy"],"Resource":["arn:aws:s3:::mybucket"]}`
   allows, in v3.1.3, only bucket-policy actions on `mybucket`; in
   v3.6.1 and later it also allows `s3:PutBucketPolicy` on every bucket
   and `admin:CreatePolicy` and `admin:AttachUserOrGroupPolicy`, enough
   to attach `consoleAdmin` to oneself. Stored policies are loaded
   without validation, so existing ones widen at once. (`*Get*` on
   `public/*` read any object in v3.6.1; v3.10.1 re-checks resources for
   bucket-scoped admin actions, which closes that case but not the
   first.)

Also: v3.9.0 and later require coreos/go-systemd v22.7.0, which does not
build for netbsd/amd64 (note 45); v3.7.0–v3.8.1 do not build with the
fork; v3.11.0 changes the type of the action constants and breaks the
embedded console (`github.com/aursu/console`).

## Why not v3.3.0

v3.3.0 is the last release without the four defects. It already
replaces how a policy is evaluated (an index of statements by action,
and hash-based removal of duplicate statements), which would need its
own review and IAM tests, and in return the server gets only new action
names that no handler checks. Not worth it now.

go-ldap and go-oidc, which later minio/pkg releases raise, are updated
on their own in group G.

## How it was checked

The table above comes from one program run against each release (all
from v3.1.4 to v3.6.1, then v3.10.1 and v3.11.0) in `golang:1.27.4` on
media-gw, plus reading `ldap/ldap.go` and go-ldap's `DialURL`:

```go
p := parse(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["*Policy"],"Resource":["arn:aws:s3:::mybucket"]}]}`)
allowed(p, "s3:PutBucketPolicy", "other", "")     // v3.1.3 false, v3.6.1 true
allowed(p, "admin:AttachUserOrGroupPolicy", "", "") // v3.1.3 false, v3.6.1 true

a := parse(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:*"],"Resource":["arn:aws:s3:::*"]},{"Effect":"Deny","Action":["s3:GetObject"],"NotResource":["arn:aws:s3:::x/*"]}]}`)
b := parse(`{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Action":["s3:GetObject"],"NotResource":["arn:aws:s3:::y/*"]}]}`)
m := policy.MergePolicies(a, b)
len(m.Statements)                    // v3.1.3 3, v3.3.1+ 2
allowed(m, "s3:GetObject", "x", "o") // v3.1.3 false, v3.3.1+ true
```

where `parse` is `policy.ParseConfig` and `allowed` is `IsAllowed` with
`Args{AccountName, Action, BucketName, ObjectName}`.

Before any later move of minio/pkg: rerun these cases, and run an audit
of stored policies (`mc admin cluster iam export`) for action patterns
without a service prefix, `NotResource` statements and users with
`readonly`.
