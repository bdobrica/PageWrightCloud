// Explicit smoke-test gateway, not a general production proxy. No dependencies.
import http from 'node:http';
import { openSync, writeSync, fsyncSync, closeSync, readFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';

export const MODEL = 'gpt-5.6-luna';
export const MAX_OUTPUT = 8192;
// Nanodollars: reserve the full 1.05M context as input, including worst-case
// 2x long-context input, 1.25x cache writes and 1.5x long-context output.
export const RESERVATION = 1050000 * 500 + MAX_OUTPUT * 1800;
export const MAX_REQUESTS = 3;
export const MAX_RESERVED = RESERVATION * MAX_REQUESTS; // $1.6192368

export function constrain(request) {
  if (!request || Array.isArray(request) || request.model !== MODEL) throw Error('model rejected');
  // CLI-local attribution is not needed by this synthetic acceptance request.
  const { client_metadata: ignoredMetadata, ...withoutMetadata } = request;
  request = withoutMetadata;
  const allowed = new Set(['model', 'input', 'instructions', 'tools', 'tool_choice', 'parallel_tool_calls', 'reasoning', 'text', 'stream', 'store', 'include', 'prompt_cache_key', 'max_output_tokens', 'service_tier']);
  if (Object.keys(request).some(key => !allowed.has(key))) {
    const error = Error('request fields rejected');
    error.fields = Object.keys(request).filter(key => !allowed.has(key));
    throw error;
  }
  const tools = request.tools ?? [];
  if (!Array.isArray(tools)) throw Error('tools rejected');
  const local = tool => tool && (
    ['function', 'custom', 'local_shell'].includes(tool.type) ||
    (tool.type === 'namespace' && Array.isArray(tool.tools) && tool.tools.every(local))
  );
  if (!tools.every(local)) throw Error('hosted tools rejected');
  // No server-managed histories, remote files, media or hosted tool invocation.
  const walk = value => {
    if (value && typeof value === 'object') {
      if (['input_image', 'input_file', 'computer_call', 'web_search_call', 'code_interpreter_call', 'image_generation_call', 'mcp_call'].includes(value.type)) throw Error('nonlocal content rejected');
      for (const child of Object.values(value)) walk(child);
    }
  };
  walk(request.input);
  return { ...request, model: MODEL, max_output_tokens: MAX_OUTPUT, service_tier: 'default', store: false, stream: true, reasoning: { effort: 'low' } };
}

export async function boundedBytes(stream, maximum) {
  const parts = [];
  let length = 0;
  for await (const part of stream) {
    length += part.length;
    if (length > maximum) throw Error('body limit');
    parts.push(Buffer.from(part));
  }
  return Buffer.concat(parts);
}

export function usageFromSSE(bytes) {
  const completed = bytes.toString().split('\n').filter(line => line.startsWith('data: ')).map(line => line.slice(6)).filter(line => line !== '[DONE]').map(line => JSON.parse(line)).filter(event => event.type === 'response.completed');
  if (completed.length !== 1) throw Error('completion unavailable');
  const response = completed[0].response;
  if (response.model !== MODEL || response.service_tier !== 'default' || response.status !== 'completed') throw Error('response identity rejected');
  const usage = response.usage;
  const input = usage?.input_tokens, output = usage?.output_tokens, cached = usage?.input_tokens_details?.cached_tokens;
  // If the API omits the cache-write breakdown, price every non-cached input
  // token as a cache write. This is an upper bound, never a budget refund.
  const written = usage?.input_tokens_details?.cache_write_tokens ?? (input - cached);
  if (![input, output, cached, written, usage?.total_tokens].every(n => Number.isSafeInteger(n) && n >= 0) || input > 1050000 || output > MAX_OUTPUT || cached + written > input || usage.total_tokens !== input + output) throw Error('usage rejected');
  const inputMultiplier = input > 272000 ? 2 : 1, outputMultiplier = input > 272000 ? 1.5 : 1;
  return { model: response.model, input_tokens: input, output_tokens: output, cached_tokens: cached, cache_write_tokens: usage.input_tokens_details.cache_write_tokens ?? null,
    cost_nanodollars: ((input - cached - written) * 200 + cached * 20 + written * 250) * inputMultiplier + output * 1200 * outputMultiplier };
}

export function createBudget({ key, token, journal, upstream = fetch, offlineDiagnostics = false }) {
  // Exclusive creation: a stopped/crashed run can never silently reset its budget.
  const fd = openSync(journal, 'wx', 0o600);
  const state = { model: MODEL, requests: 0, reserved_nanodollars: 0, cost_nanodollars: 0, blocked: false, observations: [] };
  const persist = () => { writeSync(fd, JSON.stringify(state) + '\n'); fsyncSync(fd); };
  persist();
  let busy = false;
  const server = http.createServer(async (req, res) => {
    const reject = (code, message) => { res.writeHead(code, { 'content-type': 'application/json' }); res.end(JSON.stringify({ error: { message, type: 'smoke_guard' } })); };
    if (req.method === 'GET' && req.url === '/health') { res.end('ok'); return; }
    if (req.headers.authorization !== `Bearer ${token}`) { reject(401, 'unauthorized'); return; }
    if (req.method === 'GET' && req.url === '/status') { res.setHeader('content-type', 'application/json'); res.end(JSON.stringify(state)); return; }
    if (req.method !== 'POST' || req.url !== '/v1/responses') { reject(404, 'unsupported endpoint'); return; }
    if (busy || state.blocked || state.requests >= MAX_REQUESTS) { reject(429, 'run unavailable'); return; }
    busy = true;
    let submitted = false;
    try {
      if (req.headers['content-encoding']) throw Error('encoded request rejected');
      const body = constrain(JSON.parse(await boundedBytes(req, 1024 * 1024)));
      state.requests++;
      state.reserved_nanodollars += RESERVATION;
      persist(); // Reservation is durable before the only upstream invocation.
      submitted = true;
      const upstreamResponse = await upstream('https://api.openai.com/v1/responses', {
        method: 'POST', redirect: 'error', signal: AbortSignal.timeout(120000),
        headers: { 'content-type': 'application/json', authorization: `Bearer ${key}` },
        body: JSON.stringify(body),
      });
      if (upstreamResponse.status !== 200) {
        state.observations.push({ upstream_status: upstreamResponse.status });
        // Keep only a known error category/parameter, never provider prose.
        try {
          const failure = JSON.parse(await boundedBytes(upstreamResponse.body, 65536)).error;
          const codes = ['model_not_found', 'invalid_api_key', 'insufficient_quota', 'unsupported_parameter', 'invalid_request_error', 'unsupported_value'];
          if (codes.includes(failure?.code)) state.observations.push({ upstream_code: failure.code });
          if (['model', 'max_output_tokens', 'tools', 'reasoning', 'include', 'text', 'service_tier'].includes(failure?.param)) state.observations.push({ upstream_parameter: failure.param });
        } catch {}
        throw Error('upstream rejected');
      }
      const bytes = await boundedBytes(upstreamResponse.body, 4 * 1024 * 1024);
      const usage = usageFromSSE(bytes);
      state.cost_nanodollars += usage.cost_nanodollars;
      state.observations.push(usage);
      persist();
      res.writeHead(200, { 'content-type': 'text/event-stream', 'cache-control': 'no-store' });
      res.end(bytes);
    } catch (error) {
      // Do not expose upstream errors, bodies, request text or credentials.
      if (submitted) state.blocked = true;
      const safeErrors = ['model rejected', 'request fields rejected', 'tools rejected', 'hosted tools rejected', 'nonlocal content rejected', 'encoded request rejected', 'body limit', 'completion unavailable', 'response identity rejected', 'usage rejected', 'upstream rejected'];
      state.observations.push({ guard_error: safeErrors.includes(error.message) ? error.message : (submitted ? 'upstream_or_usage_uncertain' : 'request_rejected') });
      if (offlineDiagnostics && error.fields) state.observations.push({ offline_rejected_fields: error.fields });
      try { persist(); } catch { state.blocked = true; }
      reject(400, 'smoke guard stopped request');
    } finally { busy = false; }
  });
  server.requestTimeout = 150000;
  server.headersTimeout = 10000;
  server.on('close', () => closeSync(fd));
  return { server, state };
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const key = readFileSync('/run/secrets/provider_key', 'utf8').trim();
  const token = process.env.SMOKE_PROXY_TOKEN;
  if (!key || !token) throw Error('smoke credentials required');
  let fixtureCalls = 0;
  const offline = async () => {
    const first = fixtureCalls++ === 0;
    const item = first ? { id: 'fc_smoke', type: 'function_call', call_id: 'call_smoke', name: 'exec_command', arguments: JSON.stringify({ cmd: "printf '# PageWright provider smoke verified\\n' > content/home/index.md", max_output_tokens: 1000 }) } : { id: 'msg_smoke', type: 'message', role: 'assistant', status: 'completed', content: [{ type: 'output_text', text: 'Updated requested heading.' }] };
    const response = { id: 'resp_smoke', model: MODEL, service_tier: 'default', status: 'completed', output: [item], usage: { input_tokens: 100, output_tokens: 20, total_tokens: 120, input_tokens_details: { cached_tokens: 0, cache_write_tokens: 0 } } };
    const events = [
      { type: 'response.created', response: { ...response, status: 'in_progress', output: [] } },
      { type: 'response.output_item.added', output_index: 0, item },
      { type: 'response.output_item.done', output_index: 0, item },
      { type: 'response.completed', response },
    ];
    return new Response(events.map(e => `event: ${e.type}\ndata: ${JSON.stringify(e)}\n\n`).join(''));
  };
  const { server } = createBudget({ key, token, journal: '/state/budget.jsonl', ...(process.env.SMOKE_OFFLINE === '1' ? { upstream: offline, offlineDiagnostics: true } : {}) });
  server.listen(8090, '0.0.0.0');
}
