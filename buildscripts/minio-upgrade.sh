#!/bin/bash

trap 'cleanup $LINENO' ERR

# shellcheck disable=SC2120
cleanup() {
	MINIO_VERSION=dev /tmp/gopath/bin/docker-compose \
		-f "buildscripts/upgrade-tests/compose.yml" \
		down || true

	MINIO_VERSION=dev /tmp/gopath/bin/docker-compose \
		-f "buildscripts/upgrade-tests/compose.yml" \
		rm || true

	for volume in $(docker volume ls -q | grep upgrade); do
		docker volume rm ${volume} || true
	done

	docker volume prune -f
	docker system prune -f || true
	docker volume prune -f || true
	docker volume rm $(docker volume ls -q -f dangling=true) || true
}

verify_checksum_after_heal() {
	local sum1
	sum1=$(curl -s "$2" | sha256sum)
	mc admin heal --json -r "$1" >/dev/null # test after healing
	local sum1_heal
	sum1_heal=$(curl -s "$2" | sha256sum)

	if [ "${sum1_heal}" != "${sum1}" ]; then
		echo "mismatch expected ${sum1_heal}, got ${sum1}"
		exit 1
	fi
}

# add_alias returns once one node answers; behind nginx the others may still
# be starting (502, InternalError, "not initialized"), and each such error
# body carries a new RequestId, so comparing checksums then fails at random.
# Wait until the anonymous read the check relies on succeeds 8 times in a
# row - twice round the 4 nodes nginx balances over - for up to 120s.
# Each change of the HTTP status is printed with its second, so a passing
# run shows how long the cluster took. On failure the last answer and the
# cluster's state are printed: without them a timeout explains nothing.
wait_for_anonymous_read() {
	local streak=0 status="" last_status="" body
	body="$(mktemp)"
	for second in $(seq 1 120); do
		status=$(curl -s -o "${body}" -w '%{http_code}' "$1" || true)
		if [ "${status}" != "${last_status}" ]; then
			echo "anonymous read: HTTP ${status} at ${second}s"
			last_status="${status}"
		fi
		if [ "${status}" = 200 ]; then
			streak=$((streak + 1))
			if [ "$streak" -ge 8 ]; then
				echo "anonymous read ready after ${second}s"
				rm -f "${body}"
				return 0
			fi
		else
			streak=0
		fi
		sleep 1
	done
	echo "anonymous read of $1 did not succeed 8 times in a row within 120s"
	echo "last answer, HTTP ${status}:"
	head -c 2048 "${body}"
	echo
	rm -f "${body}"
	print_cluster_state
	exit 1
}

# What the nodes and nginx were doing when a check failed.
print_cluster_state() {
	local service
	echo "=== docker-compose ps"
	MINIO_VERSION=dev /tmp/gopath/bin/docker-compose \
		-f "buildscripts/upgrade-tests/compose.yml" ps -a || true
	echo "=== mc admin info minio"
	timeout 30 mc admin info minio || true
	for service in minio1 minio2 minio3 minio4 nginx; do
		echo "=== logs of ${service}, last 200 lines"
		MINIO_VERSION=dev /tmp/gopath/bin/docker-compose \
			-f "buildscripts/upgrade-tests/compose.yml" \
			logs --no-color --tail 200 "${service}" || true
	done
}

verify_checksum_mc() {
	local expected
	expected=$(mc cat "$1" | sha256sum)
	local got
	got=$(mc cat "$2" | sha256sum)

	if [ "${expected}" != "${got}" ]; then
		echo "mismatch - expected ${expected}, got ${got}"
		exit 1
	fi
	echo "matches - ${expected}, got ${got}"
}

add_alias() {
	for i in $(seq 1 4); do
		echo "... attempting to add alias $i"
		until (mc alias set minio http://127.0.0.1:9000 minioadmin minioadmin); do
			echo "...waiting... for 5secs" && sleep 5
		done
	done

	echo "Sleeping for nginx"
	sleep 20
}

# The release the upgrade starts from. CI runs a matrix over versions in use
# (upgrade-ci-cd.yaml); set both together - the commit is the tag's target.
if [ -n "${OLD_VERSION:-}${OLD_COMMIT:-}" ] && { [ -z "${OLD_VERSION:-}" ] || [ -z "${OLD_COMMIT:-}" ]; }; then
	echo "set OLD_VERSION and OLD_COMMIT together"
	exit 1
fi
OLD_VERSION="${OLD_VERSION:-RELEASE.2019-12-19T22-52-26Z}"
OLD_COMMIT="${OLD_COMMIT:-586614c73f902085465d07a1f854ca6c1efb9845}"

# The old images are gone from Docker Hub and Quay: build the release from
# source and wrap it with the repo Dockerfile, tagged as compose.yml expects.
build_old_release_image() {
	local context
	context="$(mktemp -d)"
	"${BASH_SOURCE[0]%/*}/install-minio-release.sh" "${OLD_VERSION}" "${OLD_COMMIT}" "${context}/minio" || exit 1
	cp -r dockerscripts "${context}/"
	docker build -q -t "minio/minio:${OLD_VERSION}" -f Dockerfile "${context}" || exit 1
	rm -rf "${context}"
}

__init__() {
	sudo apt install curl -y
	export GOPATH=/tmp/gopath
	export PATH=${PATH}:${GOPATH}/bin

	"${BASH_SOURCE[0]%/*}/install-mc.sh" "${GOPATH}/bin/mc" || exit 1

	## this is needed because github actions don't have
	## docker-compose on all runners
	COMPOSE_VERSION=v2.35.1
	mkdir -p /tmp/gopath/bin/
	wget -O /tmp/gopath/bin/docker-compose https://github.com/docker/compose/releases/download/${COMPOSE_VERSION}/docker-compose-linux-x86_64
	chmod +x /tmp/gopath/bin/docker-compose

	cleanup

	TAG=minio/minio:dev make docker
	build_old_release_image

	MINIO_VERSION=${OLD_VERSION} docker-compose \
		-f "buildscripts/upgrade-tests/compose.yml" \
		up -d --build

	add_alias

	mc mb minio/minio-test/
	mc cp ./minio minio/minio-test/to-read/
	mc cp /etc/hosts minio/minio-test/to-read/hosts
	mc anonymous set download minio/minio-test

	verify_checksum_mc ./minio minio/minio-test/to-read/minio

	curl -s http://127.0.0.1:9000/minio-test/to-read/hosts | sha256sum

	MINIO_VERSION=dev /tmp/gopath/bin/docker-compose -f "buildscripts/upgrade-tests/compose.yml" stop
}

main() {
	MINIO_VERSION=dev /tmp/gopath/bin/docker-compose -f "buildscripts/upgrade-tests/compose.yml" up -d --build

	add_alias
	wait_for_anonymous_read http://127.0.0.1:9000/minio-test/to-read/hosts

	verify_checksum_after_heal minio/minio-test http://127.0.0.1:9000/minio-test/to-read/hosts

	verify_checksum_mc ./minio minio/minio-test/to-read/minio

	verify_checksum_mc /etc/hosts minio/minio-test/to-read/hosts

	cleanup
}

(__init__ "$@" && main "$@")
