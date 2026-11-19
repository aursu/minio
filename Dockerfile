# Base for local and CI images (make docker, resiliency tests). minio/minio
# is gone from Docker Hub; this is the newest community image we hold
# (fork-owned ghcr copy), pinned by digest. ./minio replaces its binary.
FROM ghcr.io/aursu/minio/minio:RELEASE.2025-03-12T18-04-18Z@sha256:fe0667890b65bca50312a905304bf3bfca898a0ac67e929a2ff83c310c40f4d6

RUN chmod -R 777 /usr/bin

COPY ./minio /usr/bin/minio
COPY dockerscripts/docker-entrypoint.sh /usr/bin/docker-entrypoint.sh

ENTRYPOINT ["/usr/bin/docker-entrypoint.sh"]

VOLUME ["/data"]

CMD ["minio"]
