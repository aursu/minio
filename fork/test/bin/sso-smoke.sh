#!/usr/bin/env bash
# Console SSO smoke test of a server image against Keycloak (plan 04, 3.1;
# fork/notes/17-release-image.md).
#
# Starts Keycloak (dev mode) and the image on a private Docker network,
# then walks the console's OpenID flow the way a browser does:
#   GET /api/v1/login -> Keycloak login form -> /oauth_callback code+state
#   -> POST /api/v1/login/oauth2/auth -> session cookie -> admin API calls.
# Keycloak puts a fixed claim `policy` into the tokens (MinIO's claim
# name). Two logins:
#   policy=consoleAdmin  admin/info, users, groups, policies -> 200
#   policy=readonly      the same calls                       -> 403
# The second is the control: the rights come from the IdP's claim.
#
# Usage: sso-smoke.sh <image>        e.g. local/minio:test
# Exit status: 0 when both logins behave as above.
# Runs where `docker` points (DOCKER_CONTEXT=media-gw for the home server);
# containers and network are named sso-smoke-* and removed at exit.
set -euo pipefail

log_info() { printf '### [INFO] %s\n' "$1"; }
log_err() { printf '### [ERROR] %s\n' "$1" >&2; }
die() {
	log_err "$1"
	exit 1
}

readonly NETWORK="sso-smoke"
readonly KEYCLOAK_CONTAINER="sso-smoke-keycloak"
readonly MINIO_CONTAINER="sso-smoke-minio"
readonly REALM="minio"
readonly CLIENT_ID="minio"
readonly CLIENT_SECRET="sso-smoke-client-secret"
readonly SSO_USER="ssouser"
readonly SSO_PASSWORD="sso-smoke-pass-123"
readonly ADMIN_APIS="admin/info users groups policies"

cleanup() {
	local status=$?
	if [[ $status -ne 0 ]]; then
		log_err "failed (exit $status); server state and log:"
		docker inspect "$MINIO_CONTAINER" \
			--format '{{.State.Status}} exit={{.State.ExitCode}} oom={{.State.OOMKilled}}' 2>&1 || true
		docker logs "$MINIO_CONTAINER" 2>&1 | tail -n 40 || true
	fi
	docker rm -f "$MINIO_CONTAINER" "$KEYCLOAK_CONTAINER" >/dev/null 2>&1 || true
	docker network rm "$NETWORK" >/dev/null 2>&1 || true
}

kcadm() { docker exec "$KEYCLOAK_CONTAINER" /opt/keycloak/bin/kcadm.sh "$@"; }

start_keycloak() {
	docker run -d --name "$KEYCLOAK_CONTAINER" --network "$NETWORK" --network-alias keycloak \
		-e KC_BOOTSTRAP_ADMIN_USERNAME=admin -e KC_BOOTSTRAP_ADMIN_PASSWORD=admin \
		"$KEYCLOAK_IMAGE" start-dev --hostname=http://keycloak:8080 >/dev/null
	local second
	for second in $(seq 1 120); do
		if kcadm config credentials --server http://localhost:8080 --realm master \
			--user admin --password admin >/dev/null 2>&1; then
			log_info "keycloak up after ${second}s"
			return 0
		fi
		sleep 1
	done
	docker logs "$KEYCLOAK_CONTAINER" 2>&1 | tail -n 40
	die "keycloak not up after 120s"
}

# Prints the id of the claim mapper, which switch_policy changes.
configure_realm() {
	kcadm create realms -s realm="$REALM" -s enabled=true >/dev/null
	local client_uuid
	client_uuid="$(kcadm create clients -r "$REALM" -i -s clientId="$CLIENT_ID" -s enabled=true \
		-s publicClient=false -s secret="$CLIENT_SECRET" -s standardFlowEnabled=true \
		-s directAccessGrantsEnabled=false -s 'redirectUris=["http://minio:9001/oauth_callback"]')"
	# The profile fields are set so that Keycloak asks for nothing at login.
	kcadm create users -r "$REALM" -s username="$SSO_USER" -s enabled=true -s emailVerified=true \
		-s email="$SSO_USER@example.test" -s firstName=Sso -s lastName=User >/dev/null
	kcadm set-password -r "$REALM" --username "$SSO_USER" --new-password "$SSO_PASSWORD"
	local mapper_id
	mapper_id="$(kcadm create "clients/$client_uuid/protocol-mappers/models" -r "$REALM" -i \
		-s name=policy -s protocol=openid-connect -s protocolMapper=oidc-hardcoded-claim-mapper \
		-s 'config."claim.name"=policy' -s 'config."claim.value"=consoleAdmin' \
		-s 'config."jsonType.label"=String' -s 'config."id.token.claim"=true' \
		-s 'config."access.token.claim"=true' -s 'config."userinfo.token.claim"=true')"
	printf '%s %s\n' "$client_uuid" "$mapper_id"
}

switch_policy() {
	local client_uuid="$1" mapper_id="$2" policy="$3"
	kcadm update "clients/$client_uuid/protocol-mappers/models/$mapper_id" -r "$REALM" \
		-s "config.\"claim.value\"=$policy"
}

start_minio() {
	local image="$1"
	docker run -d --name "$MINIO_CONTAINER" --network "$NETWORK" --network-alias minio \
		-e MINIO_ROOT_USER=smokeadmin -e MINIO_ROOT_PASSWORD=smoke-secret-123 \
		-e MINIO_IDENTITY_OPENID_CONFIG_URL="http://keycloak:8080/realms/$REALM/.well-known/openid-configuration" \
		-e MINIO_IDENTITY_OPENID_CLIENT_ID="$CLIENT_ID" \
		-e MINIO_IDENTITY_OPENID_CLIENT_SECRET="$CLIENT_SECRET" \
		-e MINIO_IDENTITY_OPENID_CLAIM_NAME=policy \
		-e MINIO_IDENTITY_OPENID_SCOPES=openid,profile,email \
		-e MINIO_IDENTITY_OPENID_DISPLAY_NAME=Keycloak \
		-e MINIO_BROWSER_REDIRECT_URL=http://minio:9001 \
		"$image" server /data --console-address :9001 >/dev/null
}

# The browser's part, in a container on the same network. Prints one
# "GET /api/v1/<api>: <status>" line per admin API.
browser_login() {
	docker run --rm -i --network "$NETWORK" \
		-e SSO_USER="$SSO_USER" -e SSO_PASSWORD="$SSO_PASSWORD" -e ADMIN_APIS="$ADMIN_APIS" \
		--entrypoint bash "$GO_IMAGE" -s <<'CLIENT'
set -euo pipefail
cd /tmp
ready=""
for second in $(seq 1 120); do
	if curl -fs -o /dev/null http://minio:9000/minio/health/ready && curl -fs -o /dev/null http://minio:9001/; then
		ready=1
		echo "server ready after ${second}s"
		break
	fi
	sleep 1
done
[[ -n $ready ]] || { echo "server not ready after 120s"; exit 1; }

login_json="$(curl -fsS http://minio:9001/api/v1/login)"
idp_url="$(grep -o '"redirect":"[^"]*"' <<<"$login_json" | head -n 1 |
	sed 's/^"redirect":"//; s/"$//; s/\\u0026/\&/g')"
[[ $idp_url == http://keycloak:8080/* ]] || { echo "no IdP redirect in: $login_json"; exit 1; }
form="$(curl -fsS -c idp.jar -b idp.jar "$idp_url")"
action="$(grep -o 'action="[^"]*"' <<<"$form" | head -n 1 | sed 's/^action="//; s/"$//; s/&amp;/\&/g')"
callback="$(curl -sS -c idp.jar -b idp.jar -o /dev/null -w '%{redirect_url}' \
	--data-urlencode "username=$SSO_USER" --data-urlencode "password=$SSO_PASSWORD" "$action")"
[[ $callback == http://minio:9001/oauth_callback\?* ]] || { echo "unexpected callback: $callback"; exit 1; }
query="${callback#*\?}"
param() { tr '&' '\n' <<<"$query" | sed -n "s/^$1=//p"; }
code="$(param code)"
state="$(param state)"
auth_status="$(curl -sS -c console.jar -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' \
	-d "{\"code\":\"$code\",\"state\":\"$state\"}" http://minio:9001/api/v1/login/oauth2/auth)"
[[ $auth_status == 204 ]] || { echo "oauth2/auth answered $auth_status"; exit 1; }
session="$(curl -fsS -b console.jar http://minio:9001/api/v1/session)"
grep -q '"status":"ok"' <<<"$session" || { echo "session not ok: $session"; exit 1; }
echo "login ok, session ok"
for api in $ADMIN_APIS; do
	echo "GET /api/v1/$api: $(curl -sS -b console.jar -o /dev/null -w '%{http_code}' "http://minio:9001/api/v1/$api")"
done
CLIENT
}

# Logs in and checks that every admin API answered the expected status.
check_login() {
	local policy="$1" expected="$2"
	log_info "login with policy=$policy, admin APIs must answer $expected"
	local output
	output="$(browser_login)" || {
		printf '%s\n' "$output"
		die "login with policy=$policy failed"
	}
	printf '%s\n' "$output"
	local api answered
	for api in $ADMIN_APIS; do
		answered="$(sed -n "s|^GET /api/v1/$api: ||p" <<<"$output")"
		[[ $answered == "$expected" ]] || die "policy=$policy: /api/v1/$api answered '$answered', expected $expected"
	done
}

main() {
	[[ $# -eq 1 ]] || die "usage: sso-smoke.sh <image>"
	local image="$1"
	cd -- "${BASH_SOURCE[0]%/*}" || die "failed to cd to script directory"
	local env_file="${TEST_ENV:-../.env}"
	[[ -f $env_file ]] || die "missing $env_file (cp .env.example .env)"
	# shellcheck source=/dev/null
	source "$env_file"
	: "${GO_IMAGE:?set GO_IMAGE in $env_file}"
	: "${KEYCLOAK_IMAGE:?set KEYCLOAK_IMAGE in $env_file}"

	trap cleanup EXIT
	cleanup
	docker network create "$NETWORK" >/dev/null
	log_info "keycloak $KEYCLOAK_IMAGE"
	start_keycloak
	local client_uuid mapper_id
	read -r client_uuid mapper_id < <(configure_realm)
	[[ -n $mapper_id ]] || die "realm setup failed"
	log_info "server $image"
	start_minio "$image"

	check_login consoleAdmin 200
	switch_policy "$client_uuid" "$mapper_id" readonly
	check_login readonly 403
	log_info "PASS"
}

main "$@"
