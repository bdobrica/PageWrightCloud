import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
for (const name of ['auth.go', 'auth_test.go']) {
 const expected = readFileSync(`pagewright/gateway/internal/serviceauth/${name}`, 'utf8');
 for (const module of ['manager', 'storage', 'serving']) assert.equal(readFileSync(`pagewright/${module}/internal/serviceauth/${name}`, 'utf8'), expected, `serviceauth drift: ${module}/${name}`);
}
console.log('PASS: shared internal auth implementation and conformance tests match across modules');
for (const name of ['runtime.go', 'runtime_test.go']) {
 const expected = readFileSync(`pagewright/gateway/internal/runtimehttp/${name}`, 'utf8');
 for (const module of ['manager', 'storage', 'serving']) assert.equal(readFileSync(`pagewright/${module}/internal/runtimehttp/${name}`, 'utf8'), expected, `runtimehttp drift: ${module}/${name}`);
}
console.log('PASS: runtime HTTP helpers and conformance tests match across modules');
