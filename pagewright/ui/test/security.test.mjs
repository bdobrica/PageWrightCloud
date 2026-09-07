import assert from 'node:assert/strict';
import test from 'node:test';
import { securityConfig } from '../scripts/security-config.mjs';
test('CSP restricts UI scripts and exact API destination without config injection', () => {
  const headers = securityConfig('https://api.pagewright.io');
  assert.match(headers, /connect-src 'self' https:\/\/api.pagewright.io;/);
  assert.match(headers, /script-src 'self';/);
  assert.match(headers, /frame-ancestors 'none'/);
  for (const bad of ['javascript:alert(1)', 'https://user:pass@api.test', 'https://api.test/path', 'https://api.test?x=1', 'https://api.test?', 'https://api.test#', 'http://bad;host', 'http://bad$host', 'http://api.test:0', 'http://api.test:99999', 'http://bad..test', 'http://api.test\n']) {
    assert.throws(() => securityConfig(bad));
  }
});
