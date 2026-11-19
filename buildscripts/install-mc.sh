#!/usr/bin/env bash
# Build mc (the MinIO Client) at a pinned release, for the test scripts.
#
# dl.min.io no longer serves community binaries (HTTP 410) and minio/mc is
# archived, so every script gets mc from here: the fork aursu/mc, pinned by
# commit to the last community release, built the way that release was built.
#
# Usage: install-mc.sh <mc-destination> [functional-tests-destination]
#   The optional second path receives functional-tests.sh from the same
#   commit, so the test script always matches the binary.
#
# A test that needs an older client on purpose sets MC_RELEASE_TAG and
# MC_COMMIT together, e.g. RELEASE.2021-03-12T03-36-59Z / 501b7914...
set -euo pipefail

readonly DEFAULT_RELEASE_TAG="RELEASE.2025-08-13T08-35-41Z"
readonly DEFAULT_COMMIT="7394ce0dd2a80935aded936b09fa12cbb3cb8096"

MC_GIT_URL="${MC_GIT_URL:-https://github.com/aursu/mc}"
MC_RELEASE_TAG="${MC_RELEASE_TAG:-}"
MC_COMMIT="${MC_COMMIT:-}"

SOURCE_DIR=""
cleanup() { [[ -z $SOURCE_DIR ]] || rm -rf "$SOURCE_DIR"; }
trap cleanup EXIT

die() {
	printf 'install-mc: %s\n' "$1" >&2
	exit 1
}

# Creates the parent directory, as callers install into e.g. $GOPATH/bin.
absolute_path() {
	local parent
	mkdir -p -- "$(dirname -- "$1")"
	parent="$(cd -- "$(dirname -- "$1")" && pwd)" || die "cannot enter $(dirname -- "$1")"
	printf '%s/%s\n' "$parent" "$(basename -- "$1")"
}

main() {
	[[ $# -ge 1 && $# -le 2 ]] || die "usage: install-mc.sh <mc-destination> [functional-tests-destination]"
	local mc_destination functional_tests_destination=""
	mc_destination="$(absolute_path "$1")"
	[[ $# -lt 2 ]] || functional_tests_destination="$(absolute_path "$2")"

	# A tag without its commit (or the reverse) would stamp a wrong version.
	if [[ -z $MC_RELEASE_TAG && -z $MC_COMMIT ]]; then
		MC_RELEASE_TAG="$DEFAULT_RELEASE_TAG"
		MC_COMMIT="$DEFAULT_COMMIT"
	elif [[ -z $MC_RELEASE_TAG || -z $MC_COMMIT ]]; then
		die "set MC_RELEASE_TAG and MC_COMMIT together"
	fi

	SOURCE_DIR="$(mktemp -d)"
	git -C "$SOURCE_DIR" init --quiet
	git -C "$SOURCE_DIR" fetch --quiet --depth 1 "$MC_GIT_URL" "$MC_COMMIT" ||
		die "cannot fetch $MC_COMMIT from $MC_GIT_URL"
	git -C "$SOURCE_DIR" checkout --quiet FETCH_HEAD

	# The variables mc's release build sets (buildscripts/gen-ldflags.go),
	# set here: older releases' gen-ldflags stamps the build time instead.
	# -X on a variable a release lacks (CopyrightYear in 2021) is ignored.
	local release_time="${MC_RELEASE_TAG#RELEASE.}"
	# 2025-08-13T08-35-41Z -> 2025-08-13T08:35:41Z (RFC 3339, as releases set it)
	local version
	version="${release_time%%T*}T$(tr - : <<<"${release_time#*T}")"
	local ldflags="-s -w"
	ldflags+=" -X github.com/minio/mc/cmd.Version=$version"
	ldflags+=" -X github.com/minio/mc/cmd.ReleaseTag=$MC_RELEASE_TAG"
	ldflags+=" -X github.com/minio/mc/cmd.CommitID=$MC_COMMIT"
	ldflags+=" -X github.com/minio/mc/cmd.ShortCommitID=${MC_COMMIT:0:12}"
	ldflags+=" -X github.com/minio/mc/cmd.CopyrightYear=${release_time:0:4}"
	(cd "$SOURCE_DIR" && CGO_ENABLED=0 go build -trimpath -tags kqueue -ldflags "$ldflags" -o "$mc_destination")

	if [[ -n $functional_tests_destination ]]; then
		cp "$SOURCE_DIR/functional-tests.sh" "$functional_tests_destination"
	fi

	printf 'install-mc: %s\n' "$("$mc_destination" --version | head -n 1)"
}

main "$@"
