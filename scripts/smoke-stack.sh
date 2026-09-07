#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
smoke_project="pagewright-smoke-$(date +%s)-$$"
unset COMPOSE_PROFILES
# Ignore app .env and inherited endpoints/credentials; all state belongs to this run.
export PAGEWRIGHT_POSTGRES_PASSWORD='smoke:@/?#%&=+database-password'
export PAGEWRIGHT_STORAGE_URL=http://storage:8080 PAGEWRIGHT_MANAGER_URL=http://manager:8081
export PAGEWRIGHT_SERVING_URL=http://serving:8083 PAGEWRIGHT_REDIS_ADDR=redis:6379
export PAGEWRIGHT_REDIS_PASSWORD= PAGEWRIGHT_REDIS_DB=0 PAGEWRIGHT_STORAGE_BACKEND=nfs
export PAGEWRIGHT_QUEUE_BACKEND=redis PAGEWRIGHT_WORKER_SPAWNER=docker
export PAGEWRIGHT_DISPATCH_CONCURRENCY=4 PAGEWRIGHT_DISPATCH_CLAIM_TTL=30s
export PAGEWRIGHT_JWT_SECRET=isolated-smoke-test-secret-not-production PAGEWRIGHT_JWT_EXPIRATION=15m
export PAGEWRIGHT_DEFAULT_PAGE_SIZE=25
export VITE_PAGEWRIGHT_API_URL=http://localhost:8085
export PAGEWRIGHT_SITE_DOMAIN=example.test
export PAGEWRIGHT_SIGNUP_MODE=development PAGEWRIGHT_AI_ALLOWANCE_CENTS=0 PAGEWRIGHT_PROVIDER_TOKEN=
export PAGEWRIGHT_USER_DAILY_BUILDS=10 PAGEWRIGHT_SITE_DAILY_BUILDS=5 PAGEWRIGHT_ACTIVE_BUILDS=2
export PAGEWRIGHT_LLM_URL=https://api.openai.com/v1
export PAGEWRIGHT_LLM_KEY= PAGEWRIGHT_GOOGLE_CLIENT_ID= PAGEWRIGHT_GOOGLE_CLIENT_SECRET=
export PAGEWRIGHT_WORKER_LLM_KEY= PAGEWRIGHT_WORKER_LLM_URL=https://api.openai.com/v1
# A unique missing image exercises durable dispatch failure without executing AI.
export PAGEWRIGHT_WORKER_IMAGE="pagewright-missing-smoke:$smoke_project"
export PAGEWRIGHT_WWW_ROOT=/var/www PAGEWRIGHT_NGINX_SITES_ENABLED=/etc/nginx/sites-enabled
export PAGEWRIGHT_NGINX_RELOAD_COMMAND='nginx -s reload'
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
    node scripts/smoke-stack.mjs "$1" "$(url gateway 8085)" "$(url storage 8080)" "$(url ui 80)" "$(url themes 80)" "$(url manager 8081)" "$(url nginx 80)"
    compose exec -T nginx nginx -t
    compose exec -T serving nginx -t
    compose exec -T manager ./recovery-audit
}
compose up -d --build --wait --wait-timeout 180
verify fresh
# Fault only this disposable container's nginx master. The supervisor must stop
# its API sibling and exit; Compose must restart a healthy pair from saved config.
serving_container=$(compose ps -q serving)
restart_before=$(docker inspect -f '{{.RestartCount}}' "$serving_container")
compose exec -T serving sh -c 'kill -KILL "$(cat /run/nginx.pid)"'
supervised_recovery=no
for attempt in $(seq 1 40); do
    restart_after=$(docker inspect -f '{{.RestartCount}}' "$serving_container")
    serving_health=$(docker inspect -f '{{.State.Health.Status}}' "$serving_container")
    if [ "$restart_after" -gt "$restart_before" ] && [ "$serving_health" = healthy ]; then supervised_recovery=yes; break; fi
    sleep 1
done
[ "$supervised_recovery" = yes ] || { echo 'Hosting supervisor recovery failed' >&2; exit 1; }
compose exec -T serving nginx -t
# Persist gateway crash-window fixtures without calling a provider. The first
# manager job is already terminal; the second was claimed but never received.
compose exec -T postgres psql -v ON_ERROR_STOP=1 -U pagewright -d pagewright <<'SQL'
INSERT INTO versions(site_id,build_id,status)
 SELECT id,v.build,'pending' FROM sites CROSS JOIN (VALUES ('smoke-dispatch'),('smoke-missing')) v(build) WHERE fqdn='smoke.example.test';
INSERT INTO build_submissions(job_id,site_id,owner_id,source_version,target_version,prompt,request_key,request_hash,dispatch_state,updated_at)
 SELECT v.job::uuid,id,user_id,'initial',v.build,'Smoke dispatch',v.job::uuid,repeat('a',64),'dispatching',now()-interval '2 minutes'
 FROM sites CROSS JOIN (VALUES
 ('692982f4-3a86-45f6-a84a-c749081c0b22','smoke-dispatch'),
 ('692982f4-3a86-45f6-a84a-c749081c0b23','smoke-missing')) v(job,build)
 WHERE fqdn='smoke.example.test';
SQL
# Abruptly stop only this generated project's writers/Redis, then recreate with
# the same volumes. This exercises AOF recovery, not just graceful shutdown.
compose kill -s SIGKILL gateway manager redis serving
compose down
compose up -d --wait --wait-timeout 180
verify restored
for attempt in $(seq 1 40); do
    recovered=$(compose exec -T postgres psql -At -U pagewright -d pagewright -c "SELECT count(*) FROM build_submissions WHERE (job_id='692982f4-3a86-45f6-a84a-c749081c0b22' AND status='failed') OR (job_id='692982f4-3a86-45f6-a84a-c749081c0b23' AND dispatch_state='dispatching' AND status='pending' AND recovery_error='manager_evidence_missing_operator_required')")
    if [ "$recovered" = 2 ]; then break; fi
    sleep 2
done
[ "$recovered" = 2 ] || { echo 'Durable history reconciliation failed' >&2; exit 1; }
history=$(compose exec -T postgres psql -At -U pagewright -d pagewright -c "SELECT count(*) FROM job_history WHERE job_id='692982f4-3a86-45f6-a84a-c749081c0b22' AND status='failed'")
[ "$history" = 1 ] || { echo 'Terminal history missing or duplicated' >&2; exit 1; }
echo "Fresh startup, abrupt Redis/gateway/manager restart, durable history and missing-evidence checks passed."
