import assert from 'node:assert/strict';
import test from 'node:test';
import { readFileSync } from 'node:fs';
import { parseBuildResponse, parseJobSnapshot } from '../src/api/contracts.ts';
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
