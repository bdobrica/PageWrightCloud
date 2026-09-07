// Read-only Compose validation. Never load .env or print expanded configuration.
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';

const base = { PATH: process.env.PATH, HOME: process.env.HOME };
const secrets = { PAGEWRIGHT_POSTGRES_PASSWORD: 'fixture:@/?#%&=+database-password', PAGEWRIGHT_JWT_SECRET: 'fixture-signing-secret-not-for-production', PAGEWRIGHT_SERVICE_TOKEN: 'fixture-internal-service-token-not-production', PAGEWRIGHT_REDIS_PASSWORD: 'fixture-private-redis-password' };
function compose(env, file = 'docker-compose.yaml') {
  return spawnSync('docker', ['compose', '--env-file', '/dev/null', '-f', file, 'config', '--format', 'json'], { env: { ...base, ...env }, encoding: 'utf8', timeout: 30_000 });
}
for (const missing of Object.keys(secrets)) {
  const env = { ...secrets }; delete env[missing];
  const result = compose(env);
  assert.notEqual(result.status, 0, 'Missing required secrets must fail Compose validation');
  assert.match(result.stderr, /Set PAGEWRIGHT_/);
  for (const secret of Object.values(secrets)) assert.ok(!result.stderr.includes(secret), 'Secret leaked by config failure');
}
const result = compose({ ...secrets, PAGEWRIGHT_DATABASE_URL: 'postgres://ignored:legacy@external/db' });
assert.equal(result.status, 0, 'Explicit fixture configuration must validate');
const services = JSON.parse(result.stdout).services;
for (const service of ['postgres', 'redis', 'manager', 'storage', 'serving', 'themes']) assert.equal(services[service].ports, undefined, `${service} must not publish ports`);
assert.equal(services.postgres.environment.POSTGRES_PASSWORD, secrets.PAGEWRIGHT_POSTGRES_PASSWORD);
assert.equal(services.gateway.environment.PAGEWRIGHT_POSTGRES_PASSWORD, secrets.PAGEWRIGHT_POSTGRES_PASSWORD);
assert.equal(services.gateway.environment.PAGEWRIGHT_DATABASE_HOST, 'postgres:5432');
assert.equal(services.gateway.environment.PAGEWRIGHT_DATABASE_USER, services.postgres.environment.POSTGRES_USER);
assert.equal(services.gateway.environment.PAGEWRIGHT_DATABASE_NAME, services.postgres.environment.POSTGRES_DB);
assert.equal(services.gateway.environment.PAGEWRIGHT_DATABASE_URL, undefined);
assert.equal(services.manager.environment.PAGEWRIGHT_JWT_SECRET, undefined);
assert.equal(services.manager.environment.PAGEWRIGHT_POSTGRES_PASSWORD, undefined);
assert.equal(services.redis.environment.REDISCLI_AUTH, secrets.PAGEWRIGHT_REDIS_PASSWORD);
assert.equal(services.manager.environment.PAGEWRIGHT_REDIS_PASSWORD, secrets.PAGEWRIGHT_REDIS_PASSWORD);
for (const service of ['gateway', 'manager', 'storage', 'serving']) assert.equal(services[service].environment.PAGEWRIGHT_SERVICE_TOKEN, secrets.PAGEWRIGHT_SERVICE_TOKEN);
const legacy = 'pagewright/gateway/docker-compose.yaml';
assert.notEqual(compose({}, legacy).status, 0);
const legacyResult = compose(secrets, legacy);
assert.equal(legacyResult.status, 0);
const legacyServices = JSON.parse(legacyResult.stdout).services;
assert.equal(legacyServices.bff.environment.PAGEWRIGHT_POSTGRES_PASSWORD, legacyServices.postgres.environment.POSTGRES_PASSWORD);
assert.equal(legacyServices.bff.environment.PAGEWRIGHT_JWT_SECRET, secrets.PAGEWRIGHT_JWT_SECRET);
console.log('PASS: missing-secret rejection, single-source PostgreSQL wiring, literal punctuation and no legacy URL override');
