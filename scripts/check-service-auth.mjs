import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
for (const name of ['auth.go', 'auth_test.go']) {
 const expected = readFileSync(`pagewright/gateway/internal/serviceauth/${name}`, 'utf8');
 for (const module of ['manager', 'storage', 'serving']) assert.equal(readFileSync(`pagewright/${module}/internal/serviceauth/${name}`, 'utf8'), expected, `serviceauth drift: ${module}/${name}`);
}
console.log('PASS: shared internal auth implementation and conformance tests match across modules');
