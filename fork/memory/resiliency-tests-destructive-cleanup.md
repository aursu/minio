---
name: resiliency-tests-destructive-cleanup
description: docs/resiliency/resiliency-tests.sh removes every running container and prunes all images and volumes on the Docker daemon it talks to; run it only on a CI runner or inside Docker-in-Docker.
metadata:
  type: gotcha
---

`docs/resiliency/resiliency-tests.sh` (`make test-resiliency`) was written for
throwaway CI runners. Its `cleanup` runs `docker rm -f` on **every** container
in `docker ps -q`, not only its own compose project. `cleanup_and_prune` adds
`docker system prune --volumes --force` and `docker image prune --all --force`.

Against a workstation's daemon it would delete unrelated containers, the Go
module and build cache volumes, and every unused image.

**What happened (2026-11-19).** It was run locally with the host's Docker
socket mounted. The first container in `docker ps -q` was the runner
container itself, so the script killed itself before the prune. Nothing else
was lost, but only because of that ordering.

**How to run it locally:** inside `docker:29-dind` (`--privileged`), with the
script exec'd in that container against the inner daemon. Use a fresh dind
container per run, so no image survives between server builds.

Other scripts that prune the daemon (grep `docker ps -q|prune`, 2026-11-19):
`buildscripts/minio-upgrade.sh` (`make test-upgrade`: `docker volume prune -f`,
`docker system prune -f`, and it removes every volume whose name contains
`upgrade`), `.github/workflows/run-mint.sh`, `multipart/migrate.sh` and
`mint.yml`. Run these only in CI or Docker-in-Docker as well.
Related: [[server-tests-in-docker]].
