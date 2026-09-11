import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdtempSync, writeFileSync, rmSync, existsSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

const script = fileURLToPath(new URL('./provider-smoke.mjs', import.meta.url));
test('remote Docker context is refused before paid credentials or resource actions', () => {
  const directory = mkdtempSync(join(tmpdir(), 'pagewright-smoke-preflight-'));
  try {
    const marker = join(directory, 'unexpected-action');
    // No actual Docker access and deliberately no .env in this test directory.
    writeFileSync(join(directory, 'docker'), `#!${process.execPath}\n` +
      `const fs=require('node:fs'); if(process.argv[2]==='context' && process.argv[3]==='inspect') process.stdout.write('ssh://remote.test'); else fs.writeFileSync(${JSON.stringify(marker)},'unexpected');\n`, { mode: 0o700 });
    const result = spawnSync(process.execPath, [script, '--execute', '--authorize-usd=2',
      '--price-verified-on=' + new Date().toISOString().slice(0, 10)], {
      cwd: directory, env: { PATH: directory }, encoding: 'utf8', timeout: 15000,
    });
    assert.ifError(result.error);
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /local Docker daemon required/);
    assert.doesNotMatch(result.stderr, /ENOENT|provider key/);
    assert.equal(existsSync(marker), false);
  } finally { rmSync(directory, { recursive: true, force: true }); }
});
