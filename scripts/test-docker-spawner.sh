#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
spawner_project="pagewright-spawner-test-$(date +%s)-$$"
compose() {
    docker compose --env-file /dev/null -p "$spawner_project" -f docker-compose.test.yaml -f docker-compose.spawner-test.yaml "$@"
}
cleanup() {
    result=$?
    trap - EXIT INT TERM
    if [ "$result" -ne 0 ]; then compose logs --no-color --tail 40 || true; fi
    # Only workers labelled with this generated test network, never app workers.
    for container in $(docker ps -aq --filter "label=io.pagewright.network=${spawner_project}_default"); do
        docker rm -f "$container" || result=1
    done
    compose down --volumes --remove-orphans || result=1
    exit "$result"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
compose build
compose build spawn-fixture
compose up -d --wait --wait-timeout 120 redis manager storage
compose run --rm --no-deps tests
