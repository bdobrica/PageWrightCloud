#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
cli_image="pagewright-cli-test:$(date +%s)-$$"
set --
case "${PAGEWRIGHT_WORKER_APPARMOR_PROFILE:-}" in
  '') ;;
  pagewright-worker) set -- --security-opt apparmor=pagewright-worker ;;
  pagewright-worker-proc) ;;
  *) echo 'Unsupported worker AppArmor profile' >&2; exit 1 ;;
esac
docker build --target cli-acceptance -f pagewright/worker/Dockerfile -t "$cli_image" .
if [ "${PAGEWRIGHT_WORKER_APPARMOR_PROFILE:-}" = pagewright-worker-proc ]; then
  cd pagewright/manager
  TEST_WORKER_ACCEPTANCE_IMAGE="$cli_image" go test -tags dockerintegration ./internal/spawner/docker -run '^TestRealWorkerAcceptance$' -count=1 -v -timeout 180s
  exit
fi
docker run --rm --init --cpus 1 --memory 1g --memory-swap 1g --pids-limit 128 --read-only --shm-size 16m --ulimit core=0 --ulimit nofile=1024:1024 --tmpfs /tmp:rw,nosuid,nodev,noexec,size=67108864,mode=1777 --user 1000:1000 --network none --cap-drop ALL --security-opt no-new-privileges:true --security-opt seccomp=pagewright/manager/internal/spawner/docker/seccomp-worker.json "$@" --tmpfs /work:rw,nosuid,nodev,size=268435456,mode=0700,uid=1000,gid=1000 -e PAGEWRIGHT_EXPECT_APPARMOR="${PAGEWRIGHT_WORKER_APPARMOR_PROFILE:-}" -e TMPDIR=/work "$cli_image"
