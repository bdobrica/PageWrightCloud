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
compose up -d --wait --wait-timeout 120 postgres redis manager storage serving nginx
compose run --rm tests

# Fail dependencies only after the stateful contracts finish. Test process
# liveness independently, then require readiness to recover without a rebuild.
probe() {
    compose run --rm --no-deps tests go run /workspace/tests/readiness.go "$@"
}
probe http://manager:8081/ready 200 http://storage:8080/ready 200 http://serving:8083/ready 200
compose stop redis
probe http://manager:8081/ready 503 http://manager:8081/health 200
compose start redis
compose up -d --wait --wait-timeout 120 redis manager
compose stop storage
probe http://serving:8083/ready 503 http://serving:8083/health 200
compose start storage
compose up -d --wait --wait-timeout 120 storage serving
probe http://manager:8081/ready 200 http://storage:8080/ready 200 http://serving:8083/ready 200
