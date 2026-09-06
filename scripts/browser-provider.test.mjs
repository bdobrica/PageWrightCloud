import assert from 'node:assert/strict';
import test from 'node:test';
import vm from 'node:vm';
import { responseFor } from './browser-provider.mjs';

const input = [{ role: 'user', content: 'Apply JOURNEY_SECOND to source.' }];
test('chat fixture preserves the request identity through clarity and instruction generation', () => {
  for (const marker of ['JOURNEY_FIRST', 'JOURNEY_SECOND', 'JOURNEY_FAILURE']) {
    assert.equal(responseFor({ messages: [{ content: `Determine if this request ${marker}` }] }).choices[0].message.content, `CLEAR: ${marker}`);
    assert.match(responseFor({ messages: [{ content: marker }] }).choices[0].message.content, new RegExp(marker));
  }
});
test('Responses fixture asks the actual CLI to edit source and preserves prior content', () => {
  const events = responseFor({ input }).trim().split('\n\n').map(event => JSON.parse(event.split('\ndata: ')[1]));
  assert.deepEqual(events.map(e => e.sequence_number), [0, 1, 2, 3]);
  const item = events.at(-1).response.output[0];
  assert.equal(item.name, 'exec_command');
  assert.equal(item.call_id, 'call_JOURNEY_SECOND');
  const { cmd } = JSON.parse(item.arguments);
  assert.match(cmd, /readFileSync/);
  assert.match(cmd, /Previous source missing/);
  assert.doesNotMatch(cmd, /public\/|index\.html|psql|docker/);
});
test('completion requires successful matching tool output, not a global call counter', () => {
  const result = responseFor({ input: [...input, { type: 'function_call_output', call_id: 'call_JOURNEY_SECOND', output: 'Process exited with code 0' }] });
  assert.match(result, /SUMMARY: Applied JOURNEY_SECOND/);
  assert.throws(() => responseFor({ input: [...input, { type: 'function_call_output', call_id: 'call_JOURNEY_SECOND', output: 'Process exited with code 1' }] }), /Sandbox source edit failed/);
  assert.match(responseFor({ input }), /exec_command/);
  assert.throws(() => responseFor({ input: [{ content: 'Unexpected request' }] }), /Unknown acceptance request/);
});
test('fixed tool programs edit only source, preserve both changes, and make invalid config', () => {
  const files = new Map([['content/home/index.md', '# Original starter\n'], ['content/site.json', '{}']]);
  const fs = {
    readFileSync: path => { assert.ok(files.has(path)); return files.get(path); },
    writeFileSync: (path, value) => { assert.ok(files.has(path)); files.set(path, value); },
  };
  for (const marker of ['JOURNEY_FIRST', 'JOURNEY_SECOND', 'JOURNEY_FAILURE']) {
    const events = responseFor({ input: [{ content: marker }] }).trim().split('\n\n');
    const item = JSON.parse(events.at(-1).split('\ndata: ')[1]).response.output[0];
    const { cmd } = JSON.parse(item.arguments);
    assert.ok(cmd.startsWith("node -e '") && cmd.endsWith("'"));
    vm.runInNewContext(cmd.slice(9, -1), { require: name => { assert.equal(name, 'node:fs'); return fs; } }, { timeout: 1000 });
  }
  assert.match(files.get('content/home/index.md'), /Original starter[\s\S]*Journey first heading[\s\S]*Journey second paragraph/);
  assert.throws(() => JSON.parse(files.get('content/site.json')));
});
