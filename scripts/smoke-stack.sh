#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
smoke_project="pagewright-smoke-$(date +%s)-$$"
unset COMPOSE_PROFILES
# Ignore app .env and inherited endpoints/credentials; all state belongs to this run.
export PAGEWRIGHT_DATABASE_URL='postgres://pagewright:pagewright@postgres:5432/pagewright?sslmode=disable'
export PAGEWRIGHT_STORAGE_URL=http://storage:8080 PAGEWRIGHT_MANAGER_URL=http://manager:8081
export PAGEWRIGHT_SERVING_URL=http://serving:8083 PAGEWRIGHT_REDIS_ADDR=redis:6379
export PAGEWRIGHT_REDIS_PASSWORD= PAGEWRIGHT_REDIS_DB=0 PAGEWRIGHT_STORAGE_BACKEND=nfs
export PAGEWRIGHT_QUEUE_BACKEND=redis PAGEWRIGHT_WORKER_SPAWNER=docker
export PAGEWRIGHT_DISPATCH_CONCURRENCY=4 PAGEWRIGHT_DISPATCH_CLAIM_TTL=30s
export PAGEWRIGHT_JWT_SECRET=isolated-smoke-test-secret PAGEWRIGHT_JWT_EXPIRATION=15m
export PAGEWRIGHT_DEFAULT_PAGE_SIZE=25
export VITE_PAGEWRIGHT_API_URL=http://localhost:8085 VITE_PAGEWRIGHT_WS_URL=ws://localhost:8085/ws
export VITE_PAGEWRIGHT_DEFAULT_DOMAIN=example.test
export PAGEWRIGHT_LLM_KEY= PAGEWRIGHT_GOOGLE_CLIENT_ID= PAGEWRIGHT_GOOGLE_CLIENT_SECRET=
export PAGEWRIGHT_WORKER_LLM_KEY= PAGEWRIGHT_WORKER_LLM_URL=https://api.openai.com/v1
# A unique missing image exercises durable dispatch failure without executing AI.
export PAGEWRIGHT_WORKER_IMAGE="pagewright-missing-smoke:$smoke_project"
export PAGEWRIGHT_WWW_ROOT=/var/www PAGEWRIGHT_NGINX_SITES_ENABLED=/etc/nginx/sites-enabled
export PAGEWRIGHT_POSTGRES_PORT=0 PAGEWRIGHT_REDIS_PORT=0 PAGEWRIGHT_GATEWAY_PORT=0
export PAGEWRIGHT_MANAGER_PORT=0 PAGEWRIGHT_STORAGE_PORT=0 PAGEWRIGHT_SERVING_PORT=0
export PAGEWRIGHT_UI_PORT=0 PAGEWRIGHT_THEMES_PORT=0 PAGEWRIGHT_NGINX_PORT=0
compose() {
    docker compose --env-file /dev/null -p "$smoke_project" -f docker-compose.yaml "$@"
}
cleanup() {
    result=$?
    trap - EXIT INT TERM
    if [ "$result" -ne 0 ]; then compose logs --no-color --tail 60 || true; fi
    compose down --volumes --remove-orphans || result=1
    exit "$result"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
url() {
    address=$(compose port "$1" "$2" | head -n 1)
    printf 'http://127.0.0.1:%s' "${address##*:}"
}
verify() {
    node scripts/smoke-stack.mjs "$1" "$(url gateway 8085)" "$(url storage 8080)" "$(url ui 80)" "$(url themes 80)" "$(url manager 8081)"
    compose exec -T nginx nginx -t
}
compose up -d --build --wait --wait-timeout 180
verify fresh
# Recreate containers and network while retaining this run's named volumes.
compose down
compose up -d --wait --wait-timeout 180
verify restored
echo "Fresh startup and persistent-volume recreation checks passed."
