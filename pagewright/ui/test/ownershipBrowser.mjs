import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';

// Production router and services, real login/registration, no token injection or
// direct DB writes. Generated live/preview HTML is public; management is private.
export async function checkOwnership(browser, apiURL, appURL) {
  const context = await browser.newContext();
  try {
    const request = async (method, path, token, expected, data) => {
      const response = await context.request.fetch(`${apiURL}${path}`, {
        method, headers: { Origin: appURL, ...(token ? { Authorization: `Bearer ${token}` } : {}), 'Idempotency-Key': randomUUID() },
        ...(data === undefined ? {} : { data }),
      });
      assert.equal(response.status(), expected, `${method} ${path}: ${response.status()}`);
      return response;
    };
    const credentials = { email: 'journey@example.test', password: 'Acceptance-only-password-123!' };
    const owner = (await (await request('POST', '/auth/login', '', 200, credentials)).json()).token;
    const stranger = (await (await request('POST', '/auth/register', '', 200, { ...credentials, email: 'ownership@example.test' })).json()).token;
    assert.ok(owner && stranger && owner !== stranger);
    const base = '/sites/journey.example.localhost';
    const snapshot = async () => {
      const result = {};
      for (const suffix of ['', '/jobs', '/versions', '/deployment']) result[suffix] = await (await request('GET', base + suffix, owner, 200)).json();
      return result;
    };
    const before = await snapshot();
    const version = before[''].live_version_id;
    const job = before['/jobs'].data[0].job_id;
    assert.ok(version && job);
    const endpoints = [
      ['GET', ''], ['GET', '/jobs'], ['GET', `/jobs/${job}`], ['GET', '/versions'], ['GET', '/deployment'],
      ['GET', `/versions/${version}/download`], ['POST', '/enable'], ['POST', '/disable'],
      ['POST', '/build', { message: 'must not reach the provider' }],
      ['POST', `/versions/${version}/deploy`, { target: 'preview' }],
      ['POST', `/versions/${version}/deploy`, { target: 'live' }],
      ['DELETE', '', undefined, 501], ['DELETE', `/versions/${version}`, undefined, 501],
      ['GET', '/aliases', undefined, 501], ['POST', '/aliases', { alias: 'foreign.test' }, 501],
      ['DELETE', '/aliases/foreign.test', undefined, 501],
    ];
    for (const [method, suffix, data, disabled] of endpoints) {
      for (const token of ['', stranger]) {
        const response = await request(method, base + suffix, token, token ? (disabled ?? 403) : 401, data);
        const body = await response.text();
        for (const privateValue of [before[''].id, version, job]) assert.ok(!body.includes(privateValue), 'Denied response leaks private state');
      }
    }
    const empty = await (await request('GET', '/sites', stranger, 200)).json();
    assert.equal(empty.total_count, 0);
    assert.deepEqual(empty.data, []);
    await request('POST', '/sites', stranger, 201, { fqdn: 'ownership.example.localhost', template_id: 'starter' });
    const ownBase = '/sites/ownership.example.localhost';
    const own = await (await request('GET', ownBase, stranger, 200)).json();
    const list = await (await request('GET', '/sites', stranger, 200)).json();
    assert.equal(list.total_count, 1);
    assert.equal(list.data[0].id, own.id);
    await request('GET', ownBase, owner, 403); // symmetric ownership boundary
    await request('GET', `${ownBase}/jobs/${job}`, stranger, 404);
    // Version IDs are scoped to the checked site, not global artifact keys.
    await request('GET', `${ownBase}/versions/${version}/download`, stranger, 500);
    for (const target of ['preview', 'live']) await request('POST', `${ownBase}/versions/${version}/deploy`, stranger, 500, { target });
    const afterOwn = await (await request('GET', ownBase, stranger, 200)).json();
    assert.equal(afterOwn.live_version_id ?? null, null);
    assert.equal(afterOwn.preview_version_id ?? null, null);
    const ownVersions = await (await request('GET', `${ownBase}/versions`, stranger, 200)).json();
    assert.ok(!JSON.stringify(ownVersions).includes(version));
    const download = await request('GET', `${base}/versions/${version}/download`, owner, 200);
    assert.match(download.headers()['content-type'], /application\/gzip/);
    assert.ok((await download.body()).length > 0);
    assert.deepEqual(await snapshot(), before, 'Rejected cross-user requests changed owner state');
    for (const token of ['', stranger, owner]) {
      const socket = await request('GET', '/ws', token, 501);
      assert.equal(socket.headers().upgrade, undefined);
    }
    console.log('PASS: two-account site/job/version/download/deployment isolation, disabled deletion, unchanged owner state and retired event transport');
  } finally { await context.close(); }
}
