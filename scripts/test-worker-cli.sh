#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
cli_image="pagewright-cli-test:$(date +%s)-$$"
set --
case "${PAGEWRIGHT_WORKER_APPARMOR_PROFILE:-}" in
  '') ;;
  pagewright-worker) set -- --security-opt apparmor=pagewright-worker ;;
  *) echo 'Unsupported worker AppArmor profile' >&2; exit 1 ;;
esac
docker build --target cli-acceptance -t "$cli_image" pagewright/worker
docker run --rm --user 1000:1000 --network none --cap-drop ALL --security-opt no-new-privileges:true --security-opt seccomp=pagewright/manager/internal/spawner/docker/seccomp-worker.json "$@" --tmpfs /work:rw,nosuid,nodev,size=268435456,mode=0700,uid=1000,gid=1000 -e TMPDIR=/work "$cli_image"
