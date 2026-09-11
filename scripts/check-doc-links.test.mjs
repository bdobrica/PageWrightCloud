import test from 'node:test';
import assert from 'node:assert/strict';
import { localLinks, checkLinks } from './check-doc-links.mjs';

test('skip remote URLs, same-page anchors and fenced examples', () => {
  assert.deepEqual(localLinks('[a](../README.md#setup) [b](https://example.com) [c](#same)\n```md\n[x](missing.md)\n```\n[d](<file%20name.md>)'), ['../README.md', 'file name.md']);
});

test('resolve links from moved documents and reject missing targets', () => {
  const result = checkLinks(['docs/adr/0001.md'], '/repo', () => '[ok](../README.md) [bad](gone.md)',
    path => path === '/repo/docs/README.md');
  assert.equal(result.checked, 2);
  assert.deepEqual(result.failures, ['docs/adr/0001.md: gone.md']);
});
