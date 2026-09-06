import assert from 'node:assert/strict';
import test from 'node:test';
import { readFileSync, readdirSync } from 'node:fs';
import { parseBuildResponse, parseJobSnapshot, parseVersionPage, parseBuildHistory, parseBuildHistoryItem } from '../src/api/contracts.ts';

test('polling MVP has no socket transport or socket URL configuration', () => {
 const root = new URL('../src/', import.meta.url);
 for (const file of readdirSync(root, {recursive:true}).filter(file => /\.(ts|tsx)$/.test(file))) {
  const source = readFileSync(new URL(file, root), 'utf8');
  assert.doesNotMatch(source, /\bWebSocket\b|useWebSocket|wsUrl|VITE_PAGEWRIGHT_WS_URL/, file);
 }
 const chat = readFileSync(new URL('../src/pages/Chat.tsx', import.meta.url), 'utf8');
 assert.match(chat, /<BuildHistory/);
 const gateway = readFileSync(new URL('../../gateway/cmd/gateway/main.go', import.meta.url), 'utf8');
 assert.match(gateway, /HandleFunc\("\/ws", handlers.WebSocketDisabled\)/);
 assert.doesNotMatch(gateway, /wsHub|NewWebSocketHandler|internal\/websocket/);
 for (const file of ['../Dockerfile','../../../docker-compose.yaml','../../../docker-compose.local-domain.yaml','../../../.env.example']) {
  assert.doesNotMatch(readFileSync(new URL(file,import.meta.url),'utf8'), /VITE_PAGEWRIGHT_WS_URL/);
 }
});

test('durable history restores all lifecycle states with strict pagination and a public allowlist', () => {
 const job = {job_id:'job',site_id:'site',source_version:'initial',target_version:'version',status:'pending',dispatch_state:'ready',created_at:'2026-09-06T12:00:00Z',updated_at:'2026-09-06T12:00:00Z'};
 for (const status of ['pending','running','completed','failed']) {
  const data = {...job,status};
  assert.deepEqual(parseBuildHistoryItem({...data,prompt:'private',request_key:'private'}),data);
  assert.deepEqual(parseBuildHistory({data:[data],page:1,page_size:25,total_count:1,total_pages:1}).data,[data]);
 }
 for (const bad of [{status:'success'},{dispatch_state:'unknown'},{updated_at:'invalid'},{job_id:''}]) assert.throws(()=>parseBuildHistoryItem({...job,...bad}));
 const page = {data:[job],page:1,page_size:25,total_count:1,total_pages:1};
 for (const bad of [{page:0},{page_size:101},{total_pages:2},{data:[]},{data:[job,job]}]) assert.throws(()=>parseBuildHistory({...page,...bad}));
 assert.deepEqual(parseBuildHistory({...page,page:2,data:[]}).data,[]);
 const component = readFileSync(new URL('../src/components/BuildHistory.tsx',import.meta.url),'utf8');
 assert.match(component,/apiClient.listJobs\(fqdn, page, signal\)/);
 assert.match(component,/useEffect\(\(\) => startJobPolling/);
 const chat = readFileSync(new URL('../src/pages/Chat.tsx',import.meta.url),'utf8');
 assert.match(chat,/<ChatSession key=\{fqdn\}/);
 assert.match(chat,/<BuildHistory key=\{historyRefresh\}/);
});
import { CHAT_ROUTE, chatPath } from '../src/routes.ts';
import { matchRoutes } from 'react-router-dom';

test('chat exposes the accepted immutable source version', () => {
 const chat = readFileSync(new URL('../src/pages/Chat.tsx', import.meta.url), 'utf8');
 assert.match(chat, /Build submitted from version \$\{response.source_version\}/);
 assert.match(chat, /Based on \$\{response.source_version\}/);
 const history = readFileSync(new URL('../src/components/BuildHistory.tsx', import.meta.url), 'utf8');
 assert.match(history, /based on \{job.source_version\}/);
});

test('chat route and links use the FQDN consumed by Chat',()=>{
 const app=readFileSync(new URL('../src/App.tsx',import.meta.url),'utf8');
 const chat=readFileSync(new URL('../src/pages/Chat.tsx',import.meta.url),'utf8');
 const card=readFileSync(new URL('../src/components/SiteCard.tsx',import.meta.url),'utf8');
 assert.match(app,/path=\{CHAT_ROUTE\}/);
 assert.match(chat,/useParams<\{ fqdn: string \}>/);
 assert.match(card,/chatPath\(site.fqdn\)/);
 const matches=matchRoutes([{path:CHAT_ROUTE}],chatPath('blog.example.test'));
 assert.equal(matches[0].params.fqdn,'blog.example.test');
 assert.equal(matches[0].params.siteId,undefined);
});

test('version pages have canonical artifact identity, status and timestamp',()=>{
 const version={id:'v1',build_id:'v1',site_id:'site',status:'completed',created_at:'2026-09-06T12:00:00Z'};
 const page={data:[version],page:1,page_size:10,total_count:1,total_pages:1};
 assert.deepEqual(parseVersionPage(page),page);
 for(const mutation of [{status:'success'},{status:'pending'},{created_at:'bad'},{created_at:undefined,timestamp:version.created_at},{site_id:undefined},{id:'database-id'}]){
  assert.throws(()=>parseVersionPage({...page,data:[{...version,...mutation}]}));
 }
 for(const mutation of [{page:0},{page_size:0},{total_pages:2},{data:null},{data:[version,version]}]){
  assert.throws(()=>parseVersionPage({...page,...mutation}));
 }
 assert.deepEqual(parseVersionPage({...page,data:[],page:999}),{...page,data:[],page:999});
 assert.deepEqual(parseVersionPage({...page,data:[],total_count:0,total_pages:0}),{...page,data:[],total_count:0,total_pages:0});
});
import { createSubmissionIdentity, isRejectedSubmission } from '../src/api/submission.ts';

test('site creation selects starter and offers pending setup recovery', () => {
  const create = readFileSync(new URL('../src/pages/CreateSite.tsx', import.meta.url), 'utf8');
  assert.match(create, /useState\('starter'\)/);
  assert.doesNotMatch(create, /template-1/);
  const card = readFileSync(new URL('../src/components/SiteCard.tsx', import.meta.url), 'utf8');
  assert.match(card, /Resume Setup/);
  assert.match(card, /initialization_status === 'pending'/);
});

test('MVP version deletion has no UI action or API client method', () => {
  for (const file of ['../src/components/VersionActionModal.tsx', '../src/components/VersionsList.tsx', '../src/api/client.ts']) {
    const source = readFileSync(new URL(file, import.meta.url), 'utf8');
    assert.doesNotMatch(source, /onDelete|handleDelete|deleteVersion|Delete Version/);
  }
});

const accepted = {
  job_id: 'job-1', site_id: 'site-1', owner_id: 'owner-1',
  source_version: 'initial', target_version: 'version-2', status: 'running',
};
const snapshot = {
  ...accepted, prompt: 'Change the title',
  created_at: '2026-09-05T12:00:00Z', updated_at: '2026-09-05T12:01:00Z',
};

test('parses both clarification and accepted build responses', () => {
  const clarification = { question: 'Which title?', conversation_id: 'conversation-1' };
  assert.deepEqual(parseBuildResponse(clarification), clarification);
  assert.deepEqual(parseBuildResponse(accepted), accepted);
});

test('accepts additive fields without exposing internal response fields to consumers', () => {
  assert.deepEqual(parseBuildResponse({ ...accepted, future_field: true }), accepted);
  assert.deepEqual(parseJobSnapshot({ ...snapshot, future_field: true }), snapshot);
});

test('accepts every canonical job status and terminal result fields', () => {
  for (const status of ['pending', 'running', 'completed', 'failed']) {
    const value = {
      ...snapshot, status, result: 'Title updated',
      ...(status === 'completed' ? { manifest_path: '/manifest.json' } : {}),
      ...(status === 'failed' ? { error_message: 'Compile failed' } : {}),
    };
    assert.deepEqual(parseJobSnapshot(value), value);
    assert.equal(parseBuildResponse({ ...accepted, status,
      ...(status === 'failed' ? { error_message: 'Compile failed' } : {}),
    }).status, status);
  }
});

test('rejects old and unknown statuses on both API and socket contracts', () => {
  for (const status of ['queued', 'success', 'cancelled', '', null, 2]) {
    assert.throws(() => parseBuildResponse({ ...accepted, status }), /status/);
    assert.throws(() => parseJobSnapshot({ ...snapshot, status }), /status/);
  }
});

test('requires every identity and source/target version in API and socket responses', () => {
  for (const key of ['job_id', 'site_id', 'owner_id', 'source_version', 'target_version']) {
    for (const invalid of [undefined, '', ' ', null, 7]) {
      assert.throws(() => parseBuildResponse({ ...accepted, [key]: invalid }), new RegExp(key));
      assert.throws(() => parseJobSnapshot({ ...snapshot, [key]: invalid }), new RegExp(key));
    }
  }
});

test('rejects malformed objects and ambiguous or incomplete clarification responses', () => {
  for (const input of [null, [], 'text', 3, {}, { question: 'What?' },
    { conversation_id: 'conversation-1' }, { question: '', conversation_id: 'conversation-1' },
    { ...accepted, question: 'What?', conversation_id: 'conversation-1' }]) {
    assert.throws(() => parseBuildResponse(input));
  }
  for (const input of [null, [], 'text', { type: 'job', data: snapshot }]) {
    assert.throws(() => parseJobSnapshot(input));
  }
});

test('requires snapshot prompt and timestamps; validates optional result types', () => {
  for (const key of ['prompt', 'created_at', 'updated_at']) {
    assert.throws(() => parseJobSnapshot({ ...snapshot, [key]: undefined }), new RegExp(key));
    assert.throws(() => parseJobSnapshot({ ...snapshot, [key]: '' }), new RegExp(key));
  }
  for (const key of ['result', 'error_message', 'manifest_path']) {
    assert.throws(() => parseJobSnapshot({ ...snapshot, [key]: {} }), new RegExp(key));
  }
});

test('failed snapshots require a nonblank error message', () => {
  for (const error_message of [undefined, '', ' ']) {
    assert.throws(() => parseJobSnapshot({ ...snapshot, status: 'failed', error_message }), /error_message/);
  }
});

test('terminal metadata must agree with the job status', () => {
  for (const status of ['pending', 'running', 'completed']) {
    assert.throws(() => parseJobSnapshot({ ...snapshot, status, error_message: 'Failed' }), /error_message/);
  }
  for (const status of ['pending', 'running', 'failed']) {
    assert.throws(() => parseJobSnapshot({
      ...snapshot, status, manifest_path: '/manifest.json',
      ...(status === 'failed' ? { error_message: 'Failed' } : {}),
    }), /manifest_path/);
  }
});

test('timestamps must have an ISO date, time and timezone', () => {
  for (const key of ['created_at', 'updated_at']) {
    for (const invalid of [
      'yesterday', '2026-09-05', '2026-09-05T12:00:00', '2026-99-05T12:00:00Z',
      '2026-02-30T12:00:00Z', '2026-02-29T12:00:00Z', '1900-02-29T12:00:00Z',
      '2026-04-31T12:00:00Z', '2026-09-00T12:00:00Z', '2026-09-05T24:00:00Z',
      '2026-09-05T12:60:00Z', '2026-09-05T12:00:60Z', '2026-09-05T12:00:00+24:00',
      '2026-09-05T12:00:00+03:60',
    ]) {
      assert.throws(() => parseJobSnapshot({ ...snapshot, [key]: invalid }), new RegExp(key));
    }
    for (const valid of [
      '2026-09-05T12:00:00.123456789Z', '2026-09-05T15:00:00+03:00',
      '2024-02-29T23:59:59Z', '2000-02-29T12:00:00Z',
    ]) {
      assert.equal(parseJobSnapshot({ ...snapshot, [key]: valid })[key], valid);
    }
  }
});

test('clarification cannot contain any accepted job identity fields', () => {
  const clarification = { question: 'Which title?', conversation_id: 'conversation-1' };
  for (const [key, value] of Object.entries(accepted)) {
    assert.throws(() => parseBuildResponse({ ...clarification, [key]: value }), /cannot mix/);
  }
  assert.deepEqual(parseBuildResponse({ ...clarification, future_field: true }), clarification);
});

test('failed accepted responses preserve errors and reject mismatched error metadata', () => {
  assert.equal(parseBuildResponse({ ...accepted, status: 'failed', error_message: 'Worker failed' }).error_message, 'Worker failed');
  assert.throws(() => parseBuildResponse({ ...accepted, status: 'failed' }), /error_message/);
  assert.throws(() => parseBuildResponse({ ...accepted, error_message: 'Worker failed' }), /error_message/);
});

const payload = { fqdn: 'site.example.test', message: 'Update title', conversation_id: 'conversation-1',
  files: [{ name: 'photo.png', size: 12, lastModified: 123 }] };
function identity() {
  let counter = 0;
  return createSubmissionIdentity(() => `key-${++counter}`);
}

test('ambiguous retries reuse identity and synchronous duplicate sends are blocked', () => {
  const retry = identity();
  const key = retry.begin(payload);
  assert.equal(retry.begin(payload), null);
  assert.equal(retry.begin({ ...payload, message: 'Another title' }), null);
  retry.finish('uncertain');
  assert.equal(retry.begin({ ...payload }), key);
});

test('every changed payload field rotates identity after an uncertain attempt', () => {
  for (const changed of [
    { ...payload, fqdn: 'other.example.test' }, { ...payload, message: 'New title' },
    { ...payload, conversation_id: 'conversation-2' }, { ...payload, files: [] },
    ...[{ name: 'other.png' }, { size: 13 }, { lastModified: 124 }].map(change =>
      ({ ...payload, files: [{ ...payload.files[0], ...change }] })),
  ]) {
    const retry = identity();
    const first = retry.begin(payload);
    retry.finish('uncertain');
    assert.notEqual(retry.begin(changed), first);
  }
});

test('successful responses and definite rejection reset retry identity', () => {
  for (const outcome of ['success', 'rejected']) {
    const retry = identity();
    const first = retry.begin(payload);
    retry.finish(outcome);
    assert.notEqual(retry.begin(payload), first);
  }
});

test('only explicit server rejection clears an unsuccessful submission', () => {
  assert.equal(isRejectedSubmission({ response: { status: 503, data: { submission_state: 'rejected' } } }), true);
  for (const error of [new Error('Network failed'), undefined,
    { response: { status: 503, data: { submission_state: 'dispatching' } } },
    { response: { status: 400, data: { message: 'Unclassified failure' } } },
  ]) assert.equal(isRejectedSubmission(error), false);
});
