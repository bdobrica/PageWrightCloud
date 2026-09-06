import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, unlinkSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { randomUUID, randomBytes, createHash } from 'node:crypto';
import { MODEL, MAX_RESERVED } from './provider-budget.mjs';

const args = process.argv.slice(2);
const offline = args.includes('--offline');
const dollars = args.find(a => a.startsWith('--authorize-usd='))?.split('=')[1];
const today = new Date().toISOString().slice(0, 10);
if (!offline && (!args.includes('--execute') || !/^\d+(\.\d{1,2})?$/.test(dollars ?? '') || Number(dollars) * 1e9 < MAX_RESERVED || Number(dollars) > 2 || !args.includes(`--price-verified-on=${today}`))) {
  console.error('Explicit invocation required: --execute --authorize-usd=1.62 --price-verified-on=YYYY-MM-DD [--key-env=PAGEWRIGHT_LLM_KEY]. Recheck documented model pricing first. Or use --offline (no key or provider).');
  process.exit(1);
}
// Parse only one dotenv value as data. Never source .env or print its contents.
const keyName = args.find(a => a.startsWith('--key-env='))?.split('=')[1] ?? 'PAGEWRIGHT_LLM_KEY';
if (!/^[A-Z_][A-Z0-9_]*$/.test(keyName)) throw Error('invalid key variable name');
let key = 'offline-fixture';
if (!offline) {
  const lines = readFileSync('.env', 'utf8').split(/\r?\n/).filter(line => line.startsWith(keyName + '='));
  if (lines.length !== 1) throw Error('expected one configured provider key');
  key = lines[0].slice(keyName.length + 1).trim();
  if ((key.startsWith('"') && key.endsWith('"')) || (key.startsWith("'") && key.endsWith("'"))) key = key.slice(1, -1);
  if (!/^sk-[A-Za-z0-9_-]+$/.test(key)) throw Error('provider key format unavailable; value withheld');
}
const root = mkdtempSync(join(tmpdir(), 'pagewright-provider-smoke-'));
const stateDir = join(root, 'state'); mkdirSync(stateDir, { mode: 0o700 });
const keyFile = join(root, 'provider-key'); writeFileSync(keyFile, key, { mode: 0o600 }); key = '';
const proxyToken = randomBytes(32).toString('hex');
const project = `pagewright-provider-${Date.now()}-${process.pid}`;
const env = { PATH: process.env.PATH, HOME: process.env.HOME, DOCKER_CONFIG: process.env.DOCKER_CONFIG,
  COMPOSE_PROJECT_NAME: project, SMOKE_PROXY_TOKEN: proxyToken, SMOKE_STATE_DIR: stateDir, SMOKE_KEY_FILE: keyFile,
  SMOKE_UID: String(process.getuid()), SMOKE_GID: String(process.getgid()),
  SMOKE_OFFLINE: offline ? '1' : '0', SMOKE_APPARMOR_PROFILE: process.env.PAGEWRIGHT_WORKER_APPARMOR_PROFILE ?? '' };
const docker = (argv, options = {}) => execFileSync('docker', argv, { env, timeout: 300000, maxBuffer: 2 * 1024 * 1024, ...options });
const compose = (...argv) => docker(['compose', '--env-file', '/dev/null', '-p', project, '-f', 'docker-compose.provider-smoke.yaml', ...argv]);
const endpoint = (service, port) => 'http://' + compose('port', service, String(port)).toString().trim();
const request = async (url, options = {}, status = 200) => {
  const response = await fetch(url, { ...options, redirect: 'error', signal: AbortSignal.timeout(10000) });
  if (response.status !== status) throw Error(`smoke HTTP ${response.status}; response withheld`);
  return response;
};
const json = value => ({ method: 'POST', headers: { 'content-type': 'application/json' }, body: JSON.stringify(value) });
const jobID = randomUUID(), siteID = randomUUID(), target = randomUUID();
const workerName = 'pagewright-job-' + createHash('sha256').update(jobID).digest('hex');
const heading = 'PageWright provider smoke verified';
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
let accepted = false;
let interrupted = false;
const interrupt = () => { interrupted = true; try { compose('stop', 'budget'); } catch {} };
process.on('SIGINT', interrupt);
process.on('SIGTERM', interrupt);
console.log(JSON.stringify({ mode: offline ? 'offline' : 'paid', project, model: MODEL, maximum_reserved_usd: MAX_RESERVED / 1e9, evidence_directory: root }));
try {
  compose('up', '-d', '--build', '--wait', '--wait-timeout', '120');
  const manager = endpoint('manager', 8081), storage = endpoint('storage', 8080), budget = endpoint('budget', 8090);
  const network = JSON.parse(docker(['network', 'inspect', `${project}_smoke`]))[0];
  assert.equal(network.Internal, true, 'worker network must have no external route');
  const source = join(root, 'source'); mkdirSync(join(source, 'content/home'), { recursive: true });
  writeFileSync(join(source, 'content/site.json'), '{"site_name":"Provider smoke"}');
  writeFileSync(join(source, 'content/home/index.md'), '# Original smoke heading\n');
  writeFileSync(join(source, 'manifest.json'), '{"schema_version":1,"theme_id":"starter","kind":"source"}');
  const archive = execFileSync('tar', ['-czf', '-', '-C', source, 'content', 'manifest.json']);
  const initialPath = `${storage}/sites/${siteID}/artifacts/initial`;
  await request(initialPath, { method: 'PUT', headers: { 'content-type': 'application/gzip' }, body: archive }, 201);
  await request(`${initialPath}/logs`, json({ content: 'Synthetic smoke bootstrap' }), 201);
  await request(`${initialPath}/manifest`, json({ site_id: siteID, build_id: 'initial', created_at: new Date().toISOString() }), 201);
  const submission = { job_id: jobID, site_id: siteID, owner_id: 'provider-smoke-owner', source_version: 'initial', target_version: target,
    prompt: `Change only content/home/index.md: replace its heading with exactly "${heading}". Do not change any other file. Do not research or browse. Finish after this single edit.` };
  // Exactly one admission call. Ambiguity is never retried by this harness.
  if (interrupted) throw Error('smoke interrupted before admission');
  accepted = true;
  await request(`${manager}/jobs`, json(submission), 201);
  let job;
  const deadline = Date.now() + 300000;
  do {
    if (interrupted) throw Error('smoke interrupted; spending stopped');
    job = await (await request(`${manager}/jobs/${jobID}`)).json();
    if (['completed', 'failed'].includes(job.status)) break;
    await sleep(1000);
  } while (Date.now() < deadline);
  const budgetState = await (await request(`${budget}/status`, { headers: { authorization: `Bearer ${proxyToken}` } })).json();
  writeFileSync(join(root, 'budget-summary.json'), JSON.stringify(budgetState, null, 2), { mode: 0o600 });
  if (job.status !== 'completed') throw Error(`job ${job.status}; code=${job.error_code ?? 'none'}; guard=${budgetState.blocked}; requests=${budgetState.requests}`);
  assert.ok(budgetState.requests > 0 && !budgetState.blocked);
  assert.ok(budgetState.reserved_nanodollars <= MAX_RESERVED);
  const path = `${storage}/sites/${siteID}/artifacts/${target}`;
  const manifest = await (await request(path + '/manifest')).json();
  assert.equal(manifest.checks_passed, true);
  assert.equal(manifest.browser_checks_performed, false);
  assert.equal(manifest.compiler_version, '0.1.0');
  assert.deepEqual(manifest.files_changed, ['content/home/index.md']);
  const built = Buffer.from(await (await request(path)).arrayBuffer());
  const html = execFileSync('tar', ['-xzOf', '-', 'public/index.html'], { input: built, maxBuffer: 1024 * 1024 }).toString();
  const edited = execFileSync('tar', ['-xzOf', '-', 'content/home/index.md'], { input: built, maxBuffer: 1024 * 1024 }).toString();
  assert.ok(edited.includes(`# ${heading}`));
  assert.ok(html.includes(heading) && !html.includes('Original smoke heading'));
  assert.deepEqual(Buffer.from(await (await request(initialPath)).arrayBuffer()), archive);
  const inspected = JSON.parse(docker(['inspect', workerName]))[0];
  assert.equal(inspected.Config.User, '1000:1000');
  assert.equal(inspected.HostConfig.Privileged, false);
  assert.equal(inspected.HostConfig.ReadonlyRootfs, true);
  assert.equal(inspected.HostConfig.NetworkMode, `${project}_smoke`);
  assert.deepEqual(inspected.HostConfig.CapDrop, ['ALL']);
  assert.equal(inspected.HostConfig.RestartPolicy.Name, 'no');
  assert.ok(!inspected.Config.Env.some(entry => entry.startsWith('PAGEWRIGHT_LLM_KEY=sk-')));
  const evidence = { mode: offline ? 'offline' : 'paid', job_id: jobID, site_id: siteID, target_version: target, image_id: inspected.Image,
    model: MODEL, requests: budgetState.requests, reserved_usd: budgetState.reserved_nanodollars / 1e9, token_cost_upper_bound_usd: budgetState.cost_nanodollars / 1e9,
    html_sha256: createHash('sha256').update(html).digest('hex'), heading, checks: ['real_worker', 'trusted_compilation', 'source_and_html_changed', 'initial_unchanged', 'worker_isolation', 'budget_guard'], observed_at: new Date().toISOString() };
  writeFileSync(join(root, 'acceptance.json'), JSON.stringify(evidence, null, 2), { mode: 0o600 });
  console.log(JSON.stringify(evidence));
} catch (error) {
  console.error(`Provider smoke did not pass: ${error.message}`);
  if (offline) { try { console.error(compose('logs', '--no-color', '--tail', '30', 'budget').toString()); } catch {} }
  process.exitCode = 1;
} finally {
  // Stop spending first. Only this generated project's worker and services.
  try { compose('stop', 'budget'); } catch { process.exitCode = 1; }
  if (accepted) {
    try {
      const worker = JSON.parse(docker(['inspect', workerName], { stdio: ['ignore', 'pipe', 'ignore'] }))[0];
      if (worker.Config.Labels['io.pagewright.job_id'] !== jobID || worker.Config.Labels['io.pagewright.network'] !== `${project}_smoke`) throw Error('cleanup identity mismatch');
      if (worker.State.Running) docker(['kill', worker.Id]);
      docker(['rm', worker.Id]);
    } catch { console.error('Worker cleanup unconfirmed; inspect only the recorded smoke job.'); process.exitCode = 1; }
  }
  try { compose('down', '--volumes', '--remove-orphans'); } catch { process.exitCode = 1; }
  unlinkSync(keyFile);
  console.log(`Removed disposable smoke services and temporary key; retained non-secret evidence at ${root}`);
}
