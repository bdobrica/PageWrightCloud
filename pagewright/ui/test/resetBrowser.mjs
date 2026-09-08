import assert from 'node:assert/strict';

export async function auditReset(browser, origin) {
  const context = await browser.newContext();
  const page = await context.newPage();
  const token = 'a'.repeat(64);
  const requests = [];
  let attempts = 0;
  let resets = 0;
  await page.route('**/auth/**', async route => {
    const request = route.request();
    if (request.method() === 'OPTIONS') return route.fulfill({ status: 200, headers: { 'Access-Control-Allow-Origin': '*', 'Access-Control-Allow-Headers': 'Content-Type' } });
    const path = new URL(request.url()).pathname;
    let status = 200;
    let body;
    if (path === '/auth/forgot-password') {
      attempts++;
      status = attempts === 1 ? 429 : 200;
      body = status === 429 ? { message: 'Wait a minute before requesting another reset' } : { message: 'If the email exists, a password reset link will be sent' };
    } else if (path === '/auth/reset-password') {
      assert.deepEqual(request.postDataJSON(), { token, password: 'replacement-password' });
      resets++;
      body = { message: 'Password successfully reset' };
    } else { throw Error('Unexpected auth request'); }
    await route.fulfill({ status, headers: { 'Access-Control-Allow-Origin': '*' }, contentType: 'application/json', body: JSON.stringify(body) });
  });
  page.on('request', request => requests.push(request.url()));
  try {
    await page.goto(`${origin}/forgot-password`);
    await page.getByLabel('Email', { exact: true }).fill('reset@example.test');
    await page.getByRole('button', { name: 'Send Reset Link' }).click();
    await page.getByRole('alert').filter({ hasText: 'Wait a minute' }).waitFor();
    await page.getByRole('button', { name: 'Send Reset Link' }).click();
    await page.getByRole('status').filter({ hasText: 'If the email exists' }).waitFor();
    await page.goto(`${origin}/reset-password#token=${token}`);
    await page.getByLabel('New Password', { exact: true }).fill('short');
    await page.getByLabel('Confirm Password', { exact: true }).fill('short');
    await page.getByRole('button', { name: 'Reset Password', exact: true }).click();
    await page.getByRole('alert').filter({ hasText: '72 UTF-8 bytes' }).waitFor();
    assert.equal(resets, 0);
    assert.equal(new URL(page.url()).hash, '');
    await page.getByLabel('New Password', { exact: true }).fill('replacement-password');
    await page.getByLabel('Confirm Password', { exact: true }).fill('replacement-password');
    await page.getByRole('button', { name: 'Reset Password', exact: true }).click();
    await page.getByRole('status').filter({ hasText: 'Password reset successful' }).waitFor();
    assert.equal(resets, 1);
    assert.ok(requests.every(url => !url.includes(token)), 'Reset token leaked into a network URL');
    await page.getByRole('link', { name: 'Login', exact: true }).click();
    await page.getByRole('button', { name: 'Login', exact: true }).waitFor();
    console.log('PASS: rendered reset throttling, password policy, fragment-token handling and login recovery link');
  } finally { await context.close(); }
}
