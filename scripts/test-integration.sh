#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
# This name is generated here and never points at the user's application project.
test_project="pagewright-test-$(date +%s)-$$"
compose() {
    docker compose --env-file /dev/null -p "$test_project" -f docker-compose.test.yaml "$@"
}
cleanup() {
    result=$?
    trap - EXIT INT TERM
    if [ "$result" -ne 0 ]; then compose logs --no-color --tail 80 || true; fi
    compose down --volumes --remove-orphans || result=1
    exit "$result"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
compose build
compose up -d --wait --wait-timeout 120 postgres redis manager storage serving
compose run --rm tests
