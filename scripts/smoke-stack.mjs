import assert from 'node:assert/strict';
import { gzipSync, gunzipSync } from 'node:zlib';

const [stage, gateway, storage, ui, themes] = process.argv.slice(2);
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
const artifactPath = `/sites/${sites.data[0].id}/artifacts/smoke-v1`;
const marker = 'M0 persistent artifact smoke check';
if (stage === 'fresh') {
  await request(storage, artifactPath, { method: 'PUT', headers: { 'Content-Type': 'application/gzip' }, body: gzipSync(marker) }, 201);
}
const artifact = await request(storage, artifactPath);
assert.equal(gunzipSync(Buffer.from(await artifact.arrayBuffer())).toString(), marker);
console.log(`${stage}: health, UI, theme registry, auth, site and artifact persistence passed`);
