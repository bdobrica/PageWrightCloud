import assert from 'node:assert/strict';

export async function runJourney(browser, baseURL, evidence) {
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.tracing.start({ screenshots: true, snapshots: true });
  const page = await context.newPage();
  page.setDefaultTimeout(30_000);
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  page.on('dialog', dialog => dialog.accept());
  const version = id => page.locator('.version-item').filter({ hasText: id });
  const history = id => page.getByRole('region', { name: 'Build history', exact: true }).locator('li').filter({ hasText: id });
  async function submit(marker, expected, reload = false) {
    await page.getByRole('textbox', { name: 'Build request' }).fill(marker);
    const received = page.waitForResponse(r => r.request().method() === 'POST' && r.url().endsWith('/build'));
    await page.getByRole('button', { name: 'Send', exact: true }).click();
    const response = await received;
    assert.equal(response.status(), 200);
    const job = await response.json();
    assert.ok(job.job_id && job.target_version && job.source_version);
    if (reload) {
      await history(job.job_id).waitFor();
      assert.match(await history(job.job_id).innerText(), /pending|running/);
      await page.reload();
      await history(job.job_id).waitFor();
    }
    const terminal = history(job.job_id).locator('strong').filter({ hasText: /^(completed|failed)$/ });
    await terminal.waitFor({ timeout: 180_000 });
    assert.equal(await terminal.innerText(), expected, await history(job.job_id).innerText());
    if (expected === 'completed') await version(job.target_version).waitFor();
    else assert.equal(await version(job.target_version).count(), 0);
    console.log(`${marker}: ${expected}, source ${job.source_version}, target ${job.target_version}`);
    return job;
  }
  async function preview(id) {
    await version(id).click();
    await page.getByRole('button', { name: 'Preview in New Tab' }).click();
    const link = page.getByRole('link', { name: 'Open preview', exact: true });
    await link.waitFor();
    const url = await link.getAttribute('href');
    await page.getByRole('button', { name: 'Close version actions' }).click();
    return url;
  }
  async function publish(id) {
    await version(id).click();
    await page.getByRole('button', { name: 'Promote to Live' }).click();
    await page.getByRole('dialog').waitFor({ state: 'hidden' });
    await version(id).filter({ hasText: 'Live' }).waitFor();
  }
  async function hosted(url, second, status = 200) {
    const tab = await context.newPage();
    try {
      const response = await tab.goto(url);
      assert.equal(response.status(), status, url);
      if (status !== 200) return;
      await tab.getByRole('heading', { name: 'Journey first heading', exact: true }).waitFor();
      assert.equal((await tab.locator('body').innerText()).includes('Journey second paragraph'), second);
      const assets = await tab.locator('link[rel=stylesheet],script[src]').evaluateAll(nodes => nodes.map(n => n.href || n.src));
      assert.ok(assets.length > 0, 'Compiler emitted theme assets');
      for (const asset of assets) {
        assert.equal(new URL(asset).origin, new URL(url).origin);
        const response = await tab.evaluate(async url => {
          const result = await fetch(url, { cache: 'no-store' });
          return { status: result.status, length: (await result.arrayBuffer()).byteLength };
        }, asset);
        assert.equal(response.status, 200, asset);
        assert.ok(response.length);
      }
      await tab.screenshot({ path: `${evidence}/${second ? 'second' : 'first'}-${new URL(url).hostname}.png` });
    } finally { await tab.close(); }
  }
  try {
    await page.goto(`${baseURL}/register`);
    await page.getByLabel('Email', { exact: true }).fill('journey@example.test');
    await page.getByLabel('Password', { exact: true }).fill('Acceptance-only-password-123!');
    await page.getByLabel('Confirm Password', { exact: true }).fill('Acceptance-only-password-123!');
    await page.getByRole('button', { name: 'Register', exact: true }).click();
    await page.waitForURL(url => !url.pathname.includes('register'));
    await page.getByRole('button', { name: 'Create New Site', exact: true }).click();
    await page.getByLabel('Subdomain').fill('journey');
    await page.getByRole('button', { name: 'Create Site & Start Building' }).click();
    await page.waitForURL('**/chat/journey.example.localhost');
    await version('initial').waitFor();
    assert.match(await version('initial').innerText(), /Starter source/);
    assert.equal(await page.locator('.version-item').count(), 1);
    const first = await submit('JOURNEY_FIRST', 'completed', true);
    assert.equal(first.source_version, 'initial');
    const previewURL = await preview(first.target_version);
    const liveURL = previewURL.replace('preview.', '');
    await hosted(previewURL, false);
    await hosted(liveURL, false, 404);
    // Bootstrap deliberately creates disabled metadata. Once preview has
    // provisioned routing, enable through the supported dashboard action.
    await page.getByRole('link', { name: 'Dashboard', exact: true }).click();
    await page.getByRole('button', { name: 'Enable', exact: true }).click();
    await page.getByRole('button', { name: 'Disable', exact: true }).waitFor();
    await page.getByRole('button', { name: 'Build', exact: true }).click();
    await publish(first.target_version);
    assert.equal(await page.getByRole('link', { name: 'View Live', exact: true }).getAttribute('href'), liveURL);
    assert.equal(await page.getByRole('link', { name: 'View Preview', exact: true }).getAttribute('href'), previewURL);
    const openedLive = context.waitForEvent('page');
    await page.getByRole('link', { name: 'View Live', exact: true }).click();
    const liveTab = await openedLive;
    await liveTab.waitForURL(liveURL);
    await liveTab.getByRole('heading', { name: 'Journey first heading', exact: true }).waitFor();
    await liveTab.close();
    await hosted(liveURL, false);
    const second = await submit('JOURNEY_SECOND', 'completed');
    assert.equal(second.source_version, first.target_version);
    await hosted(previewURL, false);
    await hosted(liveURL, false);
    assert.equal(await preview(second.target_version), previewURL);
    await hosted(previewURL, true);
    await hosted(liveURL, false);
    await page.reload();
    await history(second.job_id).getByText('completed', { exact: true }).waitFor();
    assert.match(await version(first.target_version).innerText(), /Live/);
    assert.match(await version(second.target_version).innerText(), /Preview/);
    const failure = await submit('JOURNEY_FAILURE', 'failed');
    assert.equal(failure.source_version, second.target_version, 'Unpublished newest source is selected');
    await hosted(previewURL, true);
    await hosted(liveURL, false);
    await publish(second.target_version);
    await hosted(liveURL, true);
    await publish(first.target_version);
    await hosted(liveURL, false);
    await hosted(previewURL, true);
    await page.reload();
    await history(failure.job_id).getByText('failed', { exact: true }).waitFor();
    assert.equal(await page.locator('.version-item').count(), 3); // starter source + two compiled builds
    assert.match(await version(first.target_version).innerText(), /Live/);
    assert.match(await version(second.target_version).innerText(), /Preview/);
    await page.getByRole('link', { name: 'Dashboard', exact: true }).click();
    await page.getByText(first.target_version, { exact: false }).waitFor();
    assert.equal(await page.getByRole('link', { name: 'View Live', exact: true }).getAttribute('href'), liveURL);
    assert.equal(await page.getByRole('link', { name: 'View Preview', exact: true }).getAttribute('href'), previewURL);
    await page.screenshot({ path: `${evidence}/dashboard-rollback.png` });
    assert.deepEqual(errors, []);
  } catch (error) {
    await page.screenshot({ path: `${evidence}/failure.png`, fullPage: true });
    console.error(await page.locator('body').innerText());
    throw error;
  } finally {
    await context.tracing.stop({ path: `${evidence}/trace.zip` });
    await context.close();
  }
}
