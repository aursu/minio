#!/usr/bin/env bash
# Run the server's Go unit tests in Docker, the way the baseline was taken
# (fork/reports/03-baseline.md):
#   - module download as root, into a shared cache volume;
#   - tests as an unprivileged user (uid 65534) without /.dockerenv —
#     as root, 7 permission tests fail falsely; with /.dockerenv,
#     TestCreateServerEndpoints waits forever (fork/memory/server-tests-in-docker.md).
# Flags follow `make test`: -tags kqueue,dev, MINIO_API_REQUESTS_MAX=10000.
#
# Usage: unit-tests.sh [package ...]      default: ./...
# Exit status: that of `go test`.
set -euo pipefail

log_info() { printf '### [INFO] %s\n' "$1"; }
log_err()  { printf '### [ERROR] %s\n' "$1" >&2; }
die()      { log_err "$1"; exit 1; }

readonly UNPRIVILEGED_UID=65534

main() {
    cd -- "${BASH_SOURCE[0]%/*}" || die "failed to cd to script directory"
    local env_file="${TEST_ENV:-../.env}"
    [[ -f "$env_file" ]] || die "missing $env_file (cp .env.example .env)"
    # shellcheck source=/dev/null
    source "$env_file"
    : "${MINIO_REPO:?set MINIO_REPO in $env_file}"
    : "${GO_IMAGE:?set GO_IMAGE in $env_file}"
    : "${GO_MODULE_CACHE_VOLUME:?set GO_MODULE_CACHE_VOLUME in $env_file}"
    : "${GO_BUILD_CACHE_VOLUME:?set GO_BUILD_CACHE_VOLUME in $env_file}"
    : "${TEST_RESULTS_DIR:?set TEST_RESULTS_DIR in $env_file}"

    local packages=("$@")
    [[ ${#packages[@]} -gt 0 ]] || packages=("./...")  # Go pattern: every package

    mkdir -p "$TEST_RESULTS_DIR"
    local log_file
    log_file="$TEST_RESULTS_DIR/unit-$(date +%Y%m%d-%H%M%S).log"

    local container=(docker run --rm
        -v "$MINIO_REPO:/src:ro"
        -w /src
        -e CGO_ENABLED=0
        -e GOFLAGS=-buildvcs=false)

    log_info "downloading modules into volume $GO_MODULE_CACHE_VOLUME"
    "${container[@]}" \
        -v "$GO_MODULE_CACHE_VOLUME:/go/pkg/mod" \
        -v "$GO_BUILD_CACHE_VOLUME:/gocache" \
        "$GO_IMAGE" \
        sh -c "go mod download && chown $UNPRIVILEGED_UID:$UNPRIVILEGED_UID /gocache"

    log_info "go test ${packages[*]} with $GO_IMAGE, log: $log_file"
    local status=0
    # shellcheck disable=SC2016  # $0 and $@ expand inside the container
    "${container[@]}" \
        -v "$GO_MODULE_CACHE_VOLUME:/go/pkg/mod:ro" \
        -v "$GO_BUILD_CACHE_VOLUME:/gocache" \
        -e HOME=/tmp \
        -e GOCACHE=/gocache \
        -e MINIO_API_REQUESTS_MAX=10000 \
        "$GO_IMAGE" \
        sh -c 'rm -f /.dockerenv && exec setpriv --reuid="$0" --regid="$0" --clear-groups "$@"' \
        "$UNPRIVILEGED_UID" \
        go test -count=1 -timeout 60m -tags kqueue,dev "${packages[@]}" \
        >"$log_file" 2>&1 || status=$?

    grep -E '^(ok|FAIL|---)' "$log_file" | grep -vE '^--- (PASS|SKIP)' || true
    if [[ $status -eq 0 ]]; then
        log_info "PASS ($(grep -c '^ok' "$log_file") packages)"
    else
        log_err "FAIL (exit $status), see $log_file"
    fi
    return "$status"
}

main "$@"
