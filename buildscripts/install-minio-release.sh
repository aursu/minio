#!/usr/bin/env bash
# Build an old MinIO server release from source, for the upgrade tests.
#
# dl.min.io no longer serves community binaries (HTTP 410) and the
# minio/minio images are gone from Docker Hub and Quay, so a test that starts
# from an old release builds it here: the fork aursu/minio at the release's
# commit, with the version flags the release build sets.
#
# Usage: install-minio-release.sh <release-tag> <commit> <destination>
#   e.g. install-minio-release.sh RELEASE.2024-03-26T22-10-45Z 8bce123b... ./minio.old
#   The commit is the tag's target; both are pinned at the call site.
set -euo pipefail

MINIO_GIT_URL="${MINIO_GIT_URL:-https://github.com/aursu/minio}"

SOURCE_DIR=""
cleanup() { [[ -z $SOURCE_DIR ]] || rm -rf "$SOURCE_DIR"; }
trap cleanup EXIT

die() {
	printf 'install-minio-release: %s\n' "$1" >&2
	exit 1
}

# Creates the parent directory if needed.
absolute_path() {
	local parent
	mkdir -p -- "$(dirname -- "$1")"
	parent="$(cd -- "$(dirname -- "$1")" && pwd)" || die "cannot enter $(dirname -- "$1")"
	printf '%s/%s\n' "$parent" "$(basename -- "$1")"
}

main() {
	[[ $# -eq 3 ]] || die "usage: install-minio-release.sh <release-tag> <commit> <destination>"
	local release_tag="$1" commit="$2" destination
	[[ $release_tag =~ ^RELEASE\.[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}-[0-9]{2}-[0-9]{2}Z$ ]] ||
		die "not a release tag: $release_tag"
	[[ $commit =~ ^[0-9a-f]{40}$ ]] || die "not a full commit id: $commit"
	destination="$(absolute_path "$3")"

	SOURCE_DIR="$(mktemp -d)"
	git -C "$SOURCE_DIR" init --quiet
	git -C "$SOURCE_DIR" fetch --quiet --depth 1 "$MINIO_GIT_URL" "$commit" ||
		die "cannot fetch $commit from $MINIO_GIT_URL"
	git -C "$SOURCE_DIR" checkout --quiet FETCH_HEAD

	# The variables minio's release build sets (buildscripts/gen-ldflags.go).
	# -X on a variable a release lacks (CopyrightYear in 2019) is ignored.
	local release_time="${release_tag#RELEASE.}"
	# 2024-03-26T22-10-45Z -> 2024-03-26T22:10:45Z (RFC 3339, as releases set it)
	local version
	version="${release_time%%T*}T$(tr - : <<<"${release_time#*T}")"
	local ldflags="-s -w"
	ldflags+=" -X github.com/minio/minio/cmd.Version=$version"
	ldflags+=" -X github.com/minio/minio/cmd.ReleaseTag=$release_tag"
	ldflags+=" -X github.com/minio/minio/cmd.CommitID=$commit"
	ldflags+=" -X github.com/minio/minio/cmd.ShortCommitID=${commit:0:12}"
	ldflags+=" -X github.com/minio/minio/cmd.CopyrightYear=${release_time:0:4}"
	(cd "$SOURCE_DIR" && CGO_ENABLED=0 go build -trimpath -tags kqueue -ldflags "$ldflags" -o "$destination")

	printf 'install-minio-release: built %s (%s) at %s\n' "$release_tag" "${commit:0:12}" "$destination"
}

main "$@"
