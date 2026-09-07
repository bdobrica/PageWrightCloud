// Runs inside the disposable fixture container, without a service/master secret.
import assert from 'node:assert/strict';
import net from 'node:net';
let input = ''; for await (const chunk of process.stdin) input += chunk;
const { tokens } = JSON.parse(input);
assert.ok(tokens.length >= 2);
const scopes = tokens.map(token => JSON.parse(Buffer.from(token.split('.')[1], 'base64url')));
const [a, b] = scopes;
async function check(origin, method, path, expected, token) {
 const response = await fetch(origin + path, { method, redirect: 'manual', headers: { ...(token ? { Authorization: `Bearer ${token}` } : {}), 'Content-Type': 'application/json' }, ...(method === 'POST' || method === 'PUT' ? { body: '{}' } : {}), signal: AbortSignal.timeout(5000) });
 await response.arrayBuffer();
 assert.equal(response.status, expected, `${method} ${origin}: unexpected authorization result`);
}
for (const [origin, method, path] of [
 ['http://manager:8081', 'POST', '/jobs'], ['http://manager:8081', 'GET', `/jobs/${a.Job}`],
 ['http://storage:8080', 'PUT', `/sites/${a.Site}/artifacts/initial`],
 ['http://storage:8080', 'GET', `/sites/${a.Site}/artifacts/${a.Target}/logs`],
 ['http://serving:8083', 'POST', '/sites/unauthorized.example.test/deployment'],
]) await check(origin, method, path, 401);
await check('http://manager:8081', 'GET', `/jobs/${a.Job}`, 200, tokens[0]);
await check('http://manager:8081', 'GET', `/jobs/${b.Job}`, 403, tokens[0]);
await check('http://manager:8081', 'POST', `/jobs/${b.Job}/result`, 403, tokens[0]);
await check('http://manager:8081', 'POST', `/jobs/${a.Job}/write-commit`, 403, tokens[0]);
await check('http://manager:8081', 'GET', `/jobs/${a.Job}`, 401, `${tokens[0]}x`);
await check('http://storage:8080', 'GET', `/sites/${a.Site}/artifacts/${a.Source}`, 200, tokens[0]);
await check('http://storage:8080', 'PUT', `/sites/${b.Site}/artifacts/${b.Target}`, 403, tokens[0]);
await check('http://storage:8080', 'PUT', `/sites/${a.Site}/artifacts/initial`, 403, tokens[0]);
await check('http://serving:8083', 'POST', '/sites/unauthorized.example.test/deployment', 403, tokens[0]);
await new Promise((resolve, reject) => {
 const socket = net.connect({ host: 'redis', port: 6379 }, () => socket.write('*1\r\n$4\r\nPING\r\n'));
 socket.setTimeout(5000, () => socket.destroy(new Error('Redis check timed out')));
 socket.once('error', reject);
 socket.once('data', data => { socket.destroy(); try { assert.match(data.toString(), /NOAUTH/); resolve(); } catch (e) { reject(e); } });
});
console.log('PASS: direct unauthenticated API/Redis access and cross-job/cross-version worker misuse rejected');
