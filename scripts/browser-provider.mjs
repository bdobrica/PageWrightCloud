// Deterministic provider boundary ONLY. Source writes run through the installed
// CLI's exec_command tool inside the production worker sandbox, never here.
import http from 'node:http';
import { pathToFileURL } from 'node:url';

export const TOKEN = 'browser-acceptance-dummy-key';
export function responseFor(body) {
  const text = JSON.stringify(body.messages ?? body.input);
  const marker = ['JOURNEY_FIRST', 'JOURNEY_SECOND', 'JOURNEY_FAILURE'].find(m => text.includes(m));
  if (!marker) throw Error('Unknown acceptance request');
  if (body.messages) {
    const content = text.includes('Determine if this request') ? `CLEAR: ${marker}` : `Apply ${marker} to the source.`;
    return { choices: [{ index: 0, message: { role: 'assistant', content }, finish_reason: 'stop' }] };
  }
  const callID = `call_${marker}`;
  const returned = body.input?.find(item => item.type === 'function_call_output' && item.call_id === callID);
  if (returned && !String(returned.output).includes('Process exited with code 0')) throw Error('Sandbox source edit failed');
  const source = marker === 'JOURNEY_FIRST'
    ? "const p='content/home/index.md';fs.writeFileSync(p,fs.readFileSync(p,'utf8')+'\\n# Journey first heading\\n');"
    : marker === 'JOURNEY_SECOND'
      ? "const p='content/home/index.md',s=fs.readFileSync(p,'utf8');if(!s.includes('Journey first heading'))throw Error('Previous source missing');fs.writeFileSync(p,s+'\\nJourney second paragraph\\n');"
      : "fs.writeFileSync('content/site.json','{ invalid JSON');";
  const cmd = `node -e '${'const fs=require("node:fs");' + source.replaceAll("'", '"')}'`;
  const item = returned
    ? { id: 'msg_fixture', type: 'message', role: 'assistant', status: 'completed', content: [{ type: 'output_text', text: `SUMMARY: Applied ${marker}` }] }
    : { id: 'fc_fixture', type: 'function_call', call_id: callID, name: 'exec_command', arguments: JSON.stringify({ cmd, max_output_tokens: 1000 }) };
  const response = { id: 'resp_fixture', status: 'completed', output: [item], usage: { input_tokens: 1, output_tokens: 1, total_tokens: 2 } };
  return [
    { type: 'response.created', response: { ...response, status: 'in_progress', output: [] } },
    { type: 'response.output_item.added', output_index: 0, item },
    { type: 'response.output_item.done', output_index: 0, item },
    { type: 'response.completed', response },
  ].map((event, sequence_number) => `event: ${event.type}\ndata: ${JSON.stringify({ ...event, sequence_number })}\n\n`).join('');
}

export function createFixture() {
  return http.createServer(async (req, res) => {
    if (req.method === 'GET' && req.url === '/health') return res.end('ok');
    if (req.method !== 'POST' || !['/v1/chat/completions', '/v1/responses'].includes(req.url)) { res.writeHead(404); return res.end(); }
    if (req.headers.authorization !== `Bearer ${TOKEN}`) { res.writeHead(401); return res.end(); }
    try {
      let raw = '';
      for await (const chunk of req) { raw += chunk; if (raw.length > 2_000_000) throw Error('Request too large'); }
      const result = responseFor(JSON.parse(raw));
      // Keep the first job observable across a browser reload without altering jobs.
      if (typeof result === 'string' && result.includes('function_call')) await new Promise(resolve => setTimeout(resolve, 4000));
      res.writeHead(200, { 'Content-Type': typeof result === 'string' ? 'text/event-stream' : 'application/json' });
      res.end(typeof result === 'string' ? result : JSON.stringify(result));
    } catch (error) { console.error(error.message); res.writeHead(400); res.end(JSON.stringify({ error: { message: error.message } })); }
  });
}
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) createFixture().listen(8090, '0.0.0.0');
