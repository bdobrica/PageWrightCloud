import assert from 'node:assert/strict';

// No mocked routes or injected credentials: provision through the operator CLI,
// then verify the production closed-signup boundary and actual UI login.
export async function checkPilotAccess(browser, ui, gateway, evidence) {
  const context = await browser.newContext();
  const page = await context.newPage();
  try {
    await page.goto(`${ui}/register`);
    await page.getByText('Accounts are provisioned by the operator.', { exact: false }).waitFor();
    assert.equal(await page.getByRole('button', { name: 'Register', exact: true }).count(), 0);
    const status = await page.evaluate(async api => {
      const result = await fetch(`${api}/auth/register`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email: 'uninvited@example.test', password: 'Acceptance-only-password-123!' }) });
      return result.status;
    }, gateway);
    assert.equal(status, 403);
    await page.getByRole('link', { name: 'Login', exact: true }).click();
    await page.getByLabel('Email', { exact: true }).fill('operator-provisioned@example.test');
    await page.getByLabel('Password', { exact: true }).fill('Acceptance-only-password-123!');
    await page.getByRole('button', { name: 'Login', exact: true }).click();
    await page.getByRole('heading', { name: 'My Sites' }).waitFor();
    await page.screenshot({ path: `${evidence}/operator-provisioned-login.png` });
    await page.getByRole('button', { name: 'Create New Site', exact: true }).click();
    await page.getByLabel('Subdomain').fill('pilot');
    await page.getByRole('button', { name: 'Create Site & Start Building' }).click();
    await page.waitForURL('**/chat/pilot.example.localhost');
    await page.getByRole('textbox', { name: 'Build request' }).fill('AI must remain disabled');
    const received = page.waitForResponse(r => r.request().method() === 'POST' && r.url().endsWith('/build'));
    await page.getByRole('button', { name: 'Send', exact: true }).click();
    assert.equal((await received).status(), 429);
    await page.getByRole('alert').filter({ hasText: 'Paid AI is disabled.' }).waitFor();
    console.log('PASS: closed signup, operator-provisioned UI login and disabled-AI 429 guidance');
  } finally { await context.close(); }
}
