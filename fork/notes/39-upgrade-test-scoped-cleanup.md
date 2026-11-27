# 39 — Upgrade test cleans up only its own resources

Date: 2026-11-27.

## What

`cleanup()` in `buildscripts/minio-upgrade.sh` is now a single
`docker-compose -f buildscripts/upgrade-tests/compose.yml down --volumes
--remove-orphans`. That command removes:

- the test's containers;
- its network;
- its twelve named data volumes;
- the anonymous `/data` volumes that the `Dockerfile` `VOLUME` creates.

The cleanup now leaves everything else on the daemon alone.

## Why

The old cleanup, inherited from upstream, ran these commands against the
whole Docker daemon:

- `docker volume rm` on every volume whose name contains `upgrade`;
- `docker volume prune -f` (twice);
- `docker system prune -f`;
- `docker volume rm $(docker volume ls -q -f dangling=true)`, which
  removes **every** volume no container uses, named ones included.

On a CI runner this does no harm, because the runner is thrown away
afterwards. On a shared host it is destructive.

On 2026-11-27 one `make test-upgrade` run on the shared build host
(note 38) deleted:

- another job's named Go module and build cache volumes;
- 17 GB of BuildKit cache;
- the unused compose networks of nine other projects.

`cleanup()` runs three times in every run: before the test, after it,
and from the `ERR` trap.

## How

`down --volumes` removes the volumes that the compose file declares and
the anonymous volumes of the project's containers. `--remove-orphans`
removes containers from an earlier run whose services were renamed. Both
the old-release and the dev-release starts use the same compose file
with no `-p`, so they share one project, `upgrade-tests`.

## Alternatives rejected

- **Keep the prune lines and guard them with `CI=true`.** Two cleanup
  modes means two code paths to keep correct. The scoped cleanup is
  enough for CI as well: the runner starts empty and is discarded
  afterwards.
- **Also remove the images the test builds** (`minio/minio:dev`,
  `minio/minio:<old release>`). The next run overwrites those tags, and a
  developer may want them to look into a failure. They are left in
  place.

## Tests

- **On media-gw,** `make test-upgrade` runs with a snapshot of the
  daemon's containers, volumes and networks taken before and after. The
  test must pass. After the run the snapshot may differ only by the two
  image tags.
- **In CI,** the upgrade matrix runs when the stack reaches `master`.
