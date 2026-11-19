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
set -euo pipefail

MC_GIT_URL="${MC_GIT_URL:-https://github.com/aursu/mc}"
readonly MC_RELEASE_TAG="RELEASE.2025-08-13T08-35-41Z"
readonly MC_COMMIT="7394ce0dd2a80935aded936b09fa12cbb3cb8096"

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

	SOURCE_DIR="$(mktemp -d)"
	git -C "$SOURCE_DIR" init --quiet
	git -C "$SOURCE_DIR" fetch --quiet --depth 1 "$MC_GIT_URL" "$MC_COMMIT" ||
		die "cannot fetch $MC_COMMIT from $MC_GIT_URL"
	git -C "$SOURCE_DIR" checkout --quiet FETCH_HEAD

	# Same flags as mc's own release build: version, release tag, commit id.
	local ldflags
	ldflags="$(cd "$SOURCE_DIR" && MC_RELEASE=RELEASE go run buildscripts/gen-ldflags.go "${MC_RELEASE_TAG#RELEASE.}")"
	(cd "$SOURCE_DIR" && CGO_ENABLED=0 go build -trimpath -tags kqueue -ldflags "$ldflags" -o "$mc_destination")

	if [[ -n $functional_tests_destination ]]; then
		cp "$SOURCE_DIR/functional-tests.sh" "$functional_tests_destination"
	fi

	printf 'install-mc: %s\n' "$("$mc_destination" --version | head -n 1)"
}

main "$@"
