---
name: gh-cli
description: GitHub CLI for this fork runs in Docker (ghcr.io/aursu/rockylinux:10.2.20260525.0-gh); its fine-grained token in ~/.config/gh is dedicated to aursu/minio, console and mds; --repo aursu/minio is mandatory.
metadata:
  type: reference
---

The workstation has no `gh`. Use the image
`ghcr.io/aursu/rockylinux:10.2.20260525.0-gh` (it also has `git`). It was
available from 2026-11-19.

**Token.** It is a fine-grained PAT, entered once with `gh auth login` and
kept in `~/.config/gh/hosts.yml` on the host. It never goes into an image, a
repo or a log. **It is dedicated to this project:** `aursu/minio`,
`aursu/console` and `aursu/mds`. Its permissions are Contents R, Pull requests
RW, Actions RW and Commit statuses R (Actions RW verified 2026-11-19: a
cancel on a completed run returns 409, not 403). `git push` stays on the host over SSH.
Never print the token: `gh auth status` masks it.

Check that it works:

```sh
docker run --rm --user "$(id -u):$(id -g)" -e HOME=/tmp \
  -e GH_CONFIG_DIR=/gh -v "$HOME/.config/gh:/gh" \
  ghcr.io/aursu/rockylinux:10.2.20260525.0-gh gh auth status
```

Day-to-day use. Run as the host uid, because the uid has no home in the image
and `HOME=/tmp` covers that. The repo is mounted read-only.

```sh
G() { docker run --rm -i --user "$(id -u):$(id -g)" -e HOME=/tmp \
  -e GH_CONFIG_DIR=/gh -e GH_NO_UPDATE_NOTIFIER=1 -e GH_PROMPT_DISABLED=1 \
  -v "$HOME/.config/gh:/gh" -v "$PWD:/w:ro" -w /w \
  ghcr.io/aursu/rockylinux:10.2.20260525.0-gh gh "$@"; }

G pr create --repo aursu/minio --base master --head <branch> --title "<t>" --body-file - <<'EOF'
<body>
EOF
G pr checks <n> --repo aursu/minio
G run view <run-id> --repo aursu/minio --log-failed
```

**`--repo aursu/minio` is mandatory.** The fork's parent is `minio/minio`, so
without it `gh` targets upstream (see the PR rules in `CLAUDE.md`).
