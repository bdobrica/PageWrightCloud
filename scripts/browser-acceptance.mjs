// Disposable production-stack acceptance: UI journey, then operator provisioning.
import { spawnSync } from 'node:child_process';
import { mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
import { runJourney } from '../pagewright/ui/test/journeyBrowser.mjs';
import { checkPilotAccess } from '../pagewright/ui/test/pilotBrowser.mjs';

const [modulePath, executablePath] = process.argv.slice(2);
if (!modulePath || !executablePath) throw Error('Usage: node scripts/browser-acceptance.mjs <playwright/index.mjs> <firefox executable>');
const { firefox } = await import(pathToFileURL(resolve(modulePath)).href);
const project = `pagewright-browser-${Date.now()}-${process.pid}`;
const evidence = mkdtempSync(`${tmpdir()}/pagewright-browser-`);
const env = {
  PATH: process.env.PATH, HOME: process.env.HOME, DOCKER_CONFIG: process.env.DOCKER_CONFIG,
  COMPOSE_PROJECT_NAME: project,
  PAGEWRIGHT_SIGNUP_MODE: 'development', PAGEWRIGHT_PROVIDER_TOKEN: 'browser-acceptance-private-proxy-token', PAGEWRIGHT_AI_ALLOWANCE_CENTS: '3000',
  PAGEWRIGHT_SITE_DOMAIN: 'example.localhost', PAGEWRIGHT_HOSTING_SCHEME: 'http',
  PAGEWRIGHT_LLM_KEY: 'browser-acceptance-dummy-key', PAGEWRIGHT_LLM_URL: 'http://fixture:8090/v1',
  PAGEWRIGHT_WORKER_IMAGE: `${project}-worker:acceptance`,
  PAGEWRIGHT_WORKER_TIMEOUT: '5m', PAGEWRIGHT_ACTIVE_BUILDS: '2',
  PAGEWRIGHT_WORKER_APPARMOR_PROFILE: process.env.PAGEWRIGHT_WORKER_APPARMOR_PROFILE ?? '',
  PAGEWRIGHT_JWT_SECRET: 'disposable-browser-acceptance-not-production',
};
for (const service of ['POSTGRES', 'REDIS', 'GATEWAY', 'MANAGER', 'STORAGE', 'SERVING', 'THEMES', 'NGINX', 'UI']) env[`PAGEWRIGHT_${service}_PORT`] = '127.0.0.1:0';
function docker(args, capture = false, required = true, input) {
  const result = spawnSync('docker', args, { env, input, encoding: 'utf8', stdio: capture ? 'pipe' : 'inherit', timeout: 900_000 });
  if (result.error || result.status !== 0) {
    const message = `docker ${args.join(' ')} failed: ${result.error ?? result.stderr ?? result.status}`;
    if (required) throw Error(message);
    console.error(message);
    process.exitCode = 1;
  }
  return result.stdout?.trim() ?? '';
}
const composeArgs = ['compose', '--env-file', '/dev/null', '-p', project, '-f', 'docker-compose.yaml', '-f', 'docker-compose.browser.yaml'];
const compose = (...args) => docker([...composeArgs, ...args]);
const port = (service, internal) => {
  const value = docker([...composeArgs, 'port', service, String(internal)], true).split(':').at(-1);
  if (!/^\d+$/.test(value) || Number(value) < 1 || Number(value) > 65535) throw Error(`No host port for ${service}: ${value}`);
  return value;
};
let browser;
// Refuse remote contexts: browser loopback and fixture bind mounts require the
// local daemon, and this task does not authorize deploying to another host.
if (!docker(['context', 'inspect', '--format', '{{.Endpoints.docker.Host}}'], true).startsWith('unix://')) throw Error('A local Unix-socket Docker context is required');
try {
  console.log(`Isolated project ${project}; evidence ${evidence}; provider spend $0`);
  docker(['build', '--target', 'production', '-t', env.PAGEWRIGHT_WORKER_IMAGE, '-f', 'pagewright/worker/Dockerfile', '.']);
  compose('up', '-d', '--build', '--wait', '--wait-timeout', '180', 'nginx', 'postgres', 'fixture', 'manager');
  env.PAGEWRIGHT_HOSTING_PORT = port('nginx', 80);
  compose('up', '-d', '--build', '--no-deps', '--wait', '--wait-timeout', '180', 'gateway');
  env.VITE_PAGEWRIGHT_API_URL = `http://localhost:${port('gateway', 8085)}`;
  compose('up', '-d', '--build', '--no-deps', '--wait', '--wait-timeout', '180', 'ui');
  browser = await firefox.launch({ executablePath: resolve(executablePath), headless: true,
    firefoxUserPrefs: { 'browser.cache.disk.enable': false, 'browser.cache.memory.enable': false } });
  await runJourney(browser, `http://localhost:${port('ui', 80)}`, evidence);
  env.PAGEWRIGHT_GATEWAY_PORT = `127.0.0.1:${port('gateway', 8085)}`;
  env.PAGEWRIGHT_SIGNUP_MODE = 'closed';
  env.PAGEWRIGHT_AI_ALLOWANCE_CENTS = '0';
  env.PAGEWRIGHT_LLM_URL = 'https://api.openai.com/v1';
  compose('up', '-d', '--no-deps', '--wait', '--wait-timeout', '180', 'gateway');
  docker([...composeArgs, 'exec', '-T', 'gateway', './users', 'create', '-email', 'operator-provisioned@example.test', '-password-stdin'], true, true, 'Acceptance-only-password-123!\n');
  await checkPilotAccess(browser, `http://localhost:${port('ui', 80)}`, env.VITE_PAGEWRIGHT_API_URL, evidence);
  console.log('PASS: real-service browser journey, sequential source edits, refresh, preview/live independence, failed build and rollback');
} catch (error) {
  docker([...composeArgs, 'logs', '--no-color', '--tail', '100'], false, false);
  throw error;
} finally {
  try { await browser?.close(); } catch (error) { console.error('Browser cleanup failed:', error.message); process.exitCode = 1; }
  // Stop dispatch before cleaning dynamically spawned workers belonging ONLY to
  // this unique network. Compose cannot otherwise discover manager-owned workers.
  docker([...composeArgs, 'stop', '-t', '5', 'manager'], false, false);
  const workers = docker(['ps', '-aq', '--filter', 'label=io.pagewright.role=worker', '--filter', `label=io.pagewright.network=${project}_pagewright`], true, false).split(/\s+/).filter(Boolean);
  if (workers.length) docker(['rm', '-f', ...workers], false, false);
  docker([...composeArgs, 'down', '--volumes', '--remove-orphans'], false, false);
  console.log(`${process.exitCode ? 'Cleanup reported errors; inspect the printed project before retrying' : 'Removed disposable stack and its volumes'}; browser evidence retained at ${evidence}`);
}
