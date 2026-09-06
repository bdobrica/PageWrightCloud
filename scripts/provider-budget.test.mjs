import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { once } from 'node:events';
import { MODEL, MAX_OUTPUT, MAX_REQUESTS, MAX_RESERVED, RESERVATION, constrain, createBudget, usageFromSSE } from './provider-budget.mjs';

const request = { model: MODEL, input: 'Change one heading', tools: [{ type: 'function', name: 'exec_command', parameters: {} }] };
const event = (changes = {}) => 'data: ' + JSON.stringify({ type: 'response.completed', response: {
  model: MODEL, status: 'completed', service_tier: 'default', usage: { input_tokens: 100, output_tokens: 20, total_tokens: 120, input_tokens_details: { cached_tokens: 40, cache_write_tokens: 0 } }, ...changes,
} }) + '\n\n';

test('fixed pricing envelope and no hosted tools or mutable history', () => {
  assert.equal(MAX_RESERVED, 1619236800);
  assert.equal(RESERVATION, 539745600);
  const safe = constrain({ ...request, max_output_tokens: 999999, service_tier: 'priority', store: true });
  assert.equal(safe.max_output_tokens, MAX_OUTPUT);
  assert.equal(safe.service_tier, 'default');
  assert.equal(safe.store, false);
  for (const extra of [{ model: 'other' }, { background: true }, { previous_response_id: 'resp' }, { tools: [{ type: 'web_search' }] }, { tools: [{ type: 'namespace', tools: [{ type: 'code_interpreter' }] }] }, { input: [{ type: 'input_file', file_id: 'file' }] }]) {
    assert.throws(() => constrain({ ...request, ...extra }));
  }
  assert.equal(usageFromSSE(Buffer.from(event())).cost_nanodollars, 36800);
  assert.equal(usageFromSSE(Buffer.from(event({ usage: { input_tokens: 300000, output_tokens: 100, total_tokens: 300100, input_tokens_details: { cached_tokens: 0, cache_write_tokens: 300000 } } }))).cost_nanodollars, 150180000);
  assert.equal(usageFromSSE(Buffer.from(event({ usage: { input_tokens: 100, output_tokens: 20, total_tokens: 120, input_tokens_details: { cached_tokens: 0 } } }))).cost_nanodollars, 49000);
  for (const changes of [{ model: 'other' }, { service_tier: 'priority' }, { usage: null }, { status: 'incomplete' }]) assert.throws(() => usageFromSSE(Buffer.from(event(changes))));
});

test('durable preflight reservation, three requests maximum, no automatic retry', async t => {
  const dir = mkdtempSync(join(tmpdir(), 'pw-budget-test-'));
  t.after(() => rmSync(dir, { recursive: true }));
  let calls = 0;
  const journal = join(dir, 'budget.jsonl');
  const { server, state } = createBudget({ key: 'private-canary', token: 'test', journal, upstream: async (url, options) => {
    calls++;
    assert.equal(url, 'https://api.openai.com/v1/responses');
    assert.equal(options.redirect, 'error');
    const durable = JSON.parse(readFileSync(journal, 'utf8').trim().split('\n').at(-1));
    assert.equal(durable.requests, calls);
    assert.equal(durable.reserved_nanodollars, calls * RESERVATION);
    assert.equal(JSON.parse(options.body).model, MODEL);
    return new Response(event(), { status: 200 });
  } });
  server.listen(0, '127.0.0.1'); await once(server, 'listening');
  t.after(() => server.close());
  const url = `http://127.0.0.1:${server.address().port}/v1/responses`;
  const send = (body = request, token = 'test') => fetch(url, { method: 'POST', headers: { authorization: `Bearer ${token}` }, body: JSON.stringify(body) });
  assert.equal((await send(request, 'wrong')).status, 401);
  assert.equal((await send({ ...request, tools: [{ type: 'web_search' }] })).status, 400);
  assert.equal(calls, 0);
  for (let i = 0; i < MAX_REQUESTS; i++) assert.equal((await send()).status, 200);
  assert.equal((await send()).status, 429);
  assert.equal(calls, MAX_REQUESTS);
  assert.equal(state.reserved_nanodollars, MAX_RESERVED);
  assert.ok(!readFileSync(journal, 'utf8').includes('private-canary'));
  assert.throws(() => createBudget({ key: 'unused', token: 'test', journal }));
});

for (const failure of ['network', 'status', 'usage']) test(`uncertain ${failure} permanently blocks and retains reservation`, async t => {
  const dir = mkdtempSync(join(tmpdir(), 'pw-budget-test-'));
  t.after(() => rmSync(dir, { recursive: true }));
  let calls = 0;
  const { server, state } = createBudget({ key: 'private-canary', token: 'test', journal: join(dir, 'budget.jsonl'), upstream: async () => {
    calls++;
    if (failure === 'network') throw Error('private-canary');
    return new Response(failure === 'usage' ? 'data: {}\n\n' : 'private-canary', { status: failure === 'status' ? 503 : 200 });
  } });
  server.listen(0, '127.0.0.1'); await once(server, 'listening');
  t.after(() => server.close());
  const send = () => fetch(`http://127.0.0.1:${server.address().port}/v1/responses`, { method: 'POST', headers: { authorization: 'Bearer test' }, body: JSON.stringify(request) });
  const result = await send();
  assert.equal(result.status, 400);
  assert.ok(!(await result.text()).includes('private-canary'));
  assert.equal((await send()).status, 429);
  assert.equal(calls, 1);
  assert.equal(state.blocked, true);
  assert.equal(state.reserved_nanodollars, RESERVATION);
});

test('concurrent and oversized requests cannot reserve extra calls', async t => {
  const dir = mkdtempSync(join(tmpdir(), 'pw-budget-test-'));
  t.after(() => rmSync(dir, { recursive: true }));
  let release, entered;
  const pending = new Promise(resolve => { release = resolve; });
  const started = new Promise(resolve => { entered = resolve; });
  let calls = 0;
  const { server, state } = createBudget({ key: 'unused', token: 'test', journal: join(dir, 'budget.jsonl'), upstream: async () => {
    calls++; entered(); await pending; return new Response(event());
  } });
  server.listen(0, '127.0.0.1'); await once(server, 'listening');
  t.after(() => { release(); server.close(); });
  const send = body => fetch(`http://127.0.0.1:${server.address().port}/v1/responses`, { method: 'POST', headers: { authorization: 'Bearer test' }, body: JSON.stringify(body) });
  // Request-body limits may reset the connection instead of returning a body.
  await send({ ...request, input: 'x'.repeat(1024 * 1024) }).catch(() => null);
  assert.equal(calls, 0);
  const first = send(request);
  await started;
  assert.equal((await send(request)).status, 429);
  assert.equal(state.requests, 1);
  release(); assert.equal((await first).status, 200);
  assert.equal(calls, 1);
});
