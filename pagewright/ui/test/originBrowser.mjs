import assert from 'node:assert/strict';

export async function checkOriginBoundaries(browser, appURL, apiURL, siteURL) {
  const context = await browser.newContext();
  try {
    for (const origin of [siteURL, siteURL.replace('://journey.', '://preview.journey.'), 'null', 'https://foreign.example', `${appURL}.evil.test`]) {
      const response = await context.request.get(`${apiURL}/capabilities`, { headers: { Origin: origin, Authorization: 'Bearer rejected-before-auth' } });
      assert.equal(response.status(), 403);
      assert.equal(response.headers()['access-control-allow-origin'], undefined);
    }
    const preflight = await context.request.fetch(`${apiURL}/sites`, { method: 'OPTIONS', headers: {
      Origin: appURL, 'Access-Control-Request-Method': 'POST', 'Access-Control-Request-Headers': 'Authorization,Content-Type,Idempotency-Key',
    } });
    assert.equal(preflight.status(), 200);
    assert.equal(preflight.headers()['access-control-allow-origin'], appURL);
    assert.equal(preflight.headers()['access-control-allow-credentials'], undefined);
    const unauthorized = await context.request.get(`${apiURL}/sites`, { headers: { Origin: appURL } });
    assert.equal(unauthorized.status(), 401);
    assert.equal(unauthorized.headers()['access-control-allow-origin'], appURL);
    for (const [origin, status] of [[appURL, 501], [siteURL, 403]]) {
      const response = await context.request.get(`${apiURL}/ws`, { headers: { Origin: origin } });
      assert.equal(response.status(), status);
      assert.equal(response.headers()['upgrade'], undefined);
    }
    const page = await context.newPage();
    // COOP process switches can lose Firefox lifecycle notifications. Wait for
    // the response commit, then assert actual rendered/application readiness.
    const app = await page.goto(`${appURL}/login`, { waitUntil: 'commit' });
    await page.getByRole('button', { name: 'Login', exact: true }).waitFor();
    assert.match(app.headers()['content-security-policy'], /script-src 'self';/);
    assert.equal(app.headers()['cross-origin-opener-policy'], 'same-origin');
    assert.equal(await page.evaluate(async url => (await fetch(`${url}/capabilities`)).status, apiURL), 200);
    const asset = await page.locator('script[src]').first().getAttribute('src');
    const assetResponse = await context.request.get(new URL(asset, appURL).href);
    assert.equal(assetResponse.headers()['x-frame-options'], 'DENY');
    assert.match(assetResponse.headers()['content-security-policy'], /frame-ancestors 'none'/);
    const generatedPage = await context.newPage();
    const generated = await generatedPage.goto(siteURL, { waitUntil: 'commit' });
    await generatedPage.getByRole('heading', { name: 'Journey first heading', exact: true }).waitFor();
    assert.equal(generated.status(), 200);
    assert.match(generated.headers()['content-security-policy'], /connect-src 'self';/);
    assert.equal(generated.headers()['x-frame-options'], 'DENY');
    const blocked = await generatedPage.evaluate(async url => {
      const violation = new Promise(resolve => {
        document.addEventListener('securitypolicyviolation', event => resolve(event.effectiveDirective), { once: true });
        setTimeout(() => resolve('no CSP violation observed'), 3000);
      });
      let denied = false;
      try { await fetch(`${url}/capabilities`); } catch { denied = true; }
      return { denied, directive: await violation };
    }, apiURL);
    assert.deepEqual(blocked, { denied: true, directive: 'connect-src' }, 'Generated scripts cannot fetch the application API');
    // Firefox resolves *.localhost itself; Node's request client need not.
    const missing = await generatedPage.evaluate(async () => {
      const response = await fetch('/not-present');
      return { status: response.status, nosniff: response.headers.get('x-content-type-options') };
    });
    assert.equal(missing.status, 404);
    assert.equal(missing.nosniff, 'nosniff');
    console.log('PASS: exact CORS origins, retired sockets, application/asset/error headers and generated-content API isolation');
  } finally { await context.close(); }
}
