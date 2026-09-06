import assert from 'node:assert/strict';
import { gzipSync, gunzipSync } from 'node:zlib';

const [stage, gateway, storage, ui, themes, manager] = process.argv.slice(2);
assert.ok(['fresh', 'restored'].includes(stage));
async function request(base, path, options = {}, expected = 200) {
  const response = await fetch(base + path, {
    ...options,
    signal: AbortSignal.timeout(15000),
  });
  assert.equal(response.status, expected, `${stage}: ${path} returned ${response.status}`);
  return response;
}
const credentials = { email: 'smoke@example.test', password: 'SmokeTestPassword123!' };
const json = (value, token) => ({
  method: 'POST',
  headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}) },
  body: JSON.stringify(value),
});
await request(gateway, '/health');
await request(storage, '/health');
await request(ui, '/health');
const html = await (await request(ui, '/')).text();
assert.match(html, /id="root"/);
const entry = html.match(/src="([^"]+\.js)"/);
assert.ok(entry, 'UI must reference its built JavaScript');
const script = await request(ui, entry[1]);
assert.match(script.headers.get('content-type') ?? '', /javascript/);
assert.ok((await script.text()).length > 100, 'UI bundle must not be empty');
const registry = await (await request(themes, '/')).json();
assert.ok(registry.themes.some(theme => theme.id === 'starter'));
if (stage === 'fresh') await request(gateway, '/auth/register', json(credentials));
const auth = await (await request(gateway, '/auth/login', json(credentials))).json();
assert.ok(auth.token);
if (stage === 'fresh') {
  await request(gateway, '/sites', json({ fqdn: 'smoke.example.test', template_id: 'starter' }, auth.token), 201);
}
const sites = await (await request(gateway, '/sites', {
  headers: { Authorization: `Bearer ${auth.token}` },
})).json();
assert.equal(sites.total_count, 1);
assert.equal(sites.data[0].fqdn, 'smoke.example.test');
assert.equal(sites.data[0].initialization_status, 'ready');
const initialPath = `/sites/${sites.data[0].id}/artifacts/initial`;
const initial = gunzipSync(Buffer.from(await (await request(storage, initialPath)).arrayBuffer())).toString();
assert.ok(initial.includes('content/site.json') && initial.includes('content/home/index.md'));
assert.equal((await (await request(storage, initialPath + '/manifest')).json()).compiled, false);
const retriedSite = await (await request(gateway, '/sites', json({ fqdn: 'smoke.example.test', template_id: 'starter' }, auth.token), 201)).json();
assert.equal(retriedSite.id, sites.data[0].id);
// Bootstrap is the only intentionally unfenced write. Normal worker writes
// require a live attempt; those paths are covered by the service integration suite.
const artifactPath = initialPath;
const archive = Buffer.from(await (await request(storage, artifactPath)).arrayBuffer());
const metadata = {};
for (const part of ['logs', 'manifest']) metadata[part] = await (await request(storage, `${artifactPath}/${part}`)).text();
const versionsPath = `/sites/${sites.data[0].id}/versions`;
// The same checks run before and after recreation: retries are idempotent,
// replacements conflict, and neither API exposes destructive version deletion.
await request(storage, artifactPath, { method: 'PUT', headers: { 'Content-Type': 'application/gzip' }, body: archive }, 201);
for (const part of ['logs', 'manifest']) await request(storage, `${artifactPath}/${part}`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: metadata[part] }, 201);
await request(storage, artifactPath, { method: 'PUT', headers: { 'Content-Type': 'application/gzip' }, body: gzipSync('replacement') }, 409);
await request(storage, artifactPath + '/logs', json({ content: 'replacement' }), 409);
await request(storage, artifactPath + '/manifest', json({ ...JSON.parse(metadata.manifest), prompt: 'replacement' }), 409);
await request(storage, artifactPath, { method: 'DELETE' }, 405);
await request(gateway, '/sites/smoke.example.test/versions/smoke-v1', { method: 'DELETE', headers: { Authorization: `Bearer ${auth.token}` } }, 501);
const artifact = await request(storage, artifactPath);
assert.deepEqual(Buffer.from(await artifact.arrayBuffer()), archive);
for (const [suffix, expected] of Object.entries(metadata)) {
  const response = await request(storage, `${artifactPath}/${suffix}`);
  assert.equal(response.headers.get('cache-control'), 'no-store');
  assert.equal(await response.text(), expected);
}
const versions = await (await request(storage, versionsPath)).json();
assert.equal(versions.count, 1);
assert.equal(versions.versions[0].status, 'completed');
assert.ok(!JSON.stringify(versions).includes('private'));
// Persist an acknowledged job across full Redis/container recreation. The smoke
// stack deliberately selects a missing image, so no worker or provider executes.
const submission = {
  job_id: '692982f4-3a86-45f6-a84a-c749081c0b22',
  owner_id: sites.data[0].user_id, site_id: sites.data[0].id,
  source_version: 'initial', target_version: 'smoke-dispatch', prompt: 'Smoke dispatch',
};
if (stage === 'fresh') {
  const accepted = await (await request(manager, '/jobs', json(submission), 201)).json();
  assert.equal(accepted.status, 'pending');
}
let job;
for (let attempt = 0; attempt < 100; attempt++) {
  job = await (await request(manager, `/jobs/${submission.job_id}`)).json();
  if (job.status === 'failed') break;
  await new Promise(resolve => setTimeout(resolve, 100));
}
assert.equal(job.status, 'failed');
assert.equal(job.error_code, 'spawn_failed');
for (const [key, value] of Object.entries(submission)) assert.equal(job[key], value);
await request(manager, '/jobs', json(submission), 502);
assert.deepEqual(await (await request(manager, `/jobs/${submission.job_id}`)).json(), job);
if (stage === 'restored') {
  await request(manager, '/jobs/692982f4-3a86-45f6-a84a-c749081c0b23', {}, 404);
}
console.log(`${stage}: health, UI, theme registry, auth, site, artifact, private metadata and acknowledged job persistence passed`);
