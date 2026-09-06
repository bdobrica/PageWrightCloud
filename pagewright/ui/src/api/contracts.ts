import type { AcceptedBuildResponse, BuildResponse, BuildHistoryItem, JobSnapshot, JobStatus, Version, PaginatedResponse } from '../types/api.ts';

export function parseDeployment(input: unknown, fqdn: string, version: string, target: 'live' | 'preview') {
  const value = object(input);
  if (value.status !== 'deployed' || value.target !== target || value.version_id !== version) throw new Error('Unexpected deployment identity');
  const address = new URL(stringField(value, 'url'));
  if (!['http:', 'https:'].includes(address.protocol) || address.hostname !== fqdn.toLowerCase() || address.username || address.password || address.search || address.hash || address.pathname !== (target === 'preview' ? '/preview/' : '/')) throw new Error('Invalid deployment URL');
  return { url: address.href, version_id: version, target };
}

export function parseBuildHistoryItem(input: unknown): BuildHistoryItem {
  const v = object(input);
  const state = v.dispatch_state;
  if (state !== 'ready' && state !== 'dispatching' && state !== 'accepted' && state !== 'rejected') throw new Error('Invalid dispatch state');
  const item: BuildHistoryItem = {
    job_id: stringField(v, 'job_id'), site_id: stringField(v, 'site_id'),
    source_version: stringField(v, 'source_version'), target_version: stringField(v, 'target_version'),
    status: statusField(v.status), dispatch_state: state,
    created_at: timestampField(v, 'created_at'), updated_at: timestampField(v, 'updated_at'),
  };
  for (const key of ['error_code', 'recovery_error'] as const) {
    if (key in v) item[key] = stringField(v, key, true);
  }
  return item;
}

export function parseBuildHistory(input: unknown): PaginatedResponse<BuildHistoryItem> {
  const v = object(input);
  for (const key of ['page', 'page_size', 'total_count', 'total_pages']) {
    if (!Number.isSafeInteger(v[key]) || (v[key] as number) < (key.startsWith('total') ? 0 : 1)) throw new Error('Invalid history pagination');
  }
  const page = v.page as number, page_size = v.page_size as number;
  const total_count = v.total_count as number, total_pages = v.total_pages as number;
  if (page_size > 100 || total_pages !== Math.ceil(total_count / page_size) || !Array.isArray(v.data)) throw new Error('Invalid history pagination');
  const data = v.data.map(parseBuildHistoryItem);
  const expected = page > total_pages ? 0 : Math.min(page_size, total_count - (page - 1) * page_size);
  if (data.length !== expected || new Set(data.map(j => j.job_id)).size !== data.length || new Set(data.map(j => j.site_id)).size > 1) throw new Error('Invalid history page');
  return { data, page, page_size, total_count, total_pages };
}

export function parseVersionPage(input: unknown): PaginatedResponse<Version> {
  const value = object(input);
  const integer = (key: string, min: number, max = Number.MAX_SAFE_INTEGER): number => {
    const n = value[key];
    if (typeof n !== 'number' || !Number.isSafeInteger(n) || n < min || n > max) {
      throw new Error(`Invalid response field: ${key}`);
    }
    return n;
  };
  const page = integer('page', 1);
  const page_size = integer('page_size', 1, 100);
  const total_count = integer('total_count', 0);
  const total_pages = integer('total_pages', 0);
  if (total_pages !== Math.ceil(total_count / page_size) || !Array.isArray(value.data)) {
    throw new Error('Invalid version pagination');
  }
  const seen = new Set<string>();
  const data = value.data.map((item): Version => {
    const v = object(item);
    const id = stringField(v, 'id');
    const build_id = stringField(v, 'build_id');
    const site_id = stringField(v, 'site_id');
    if (id !== build_id || seen.has(id) || v.status !== 'completed') {
      throw new Error('Invalid version identity or status');
    }
    seen.add(id);
    return { id, build_id, site_id, status: 'completed', created_at: timestampField(v, 'created_at') };
  });
  const expected = page > total_pages ? 0 : Math.min(page_size, total_count - (page - 1) * page_size);
  if (data.length !== expected) throw new Error('Invalid version page length');
  return { data, page, page_size, total_count, total_pages };
}

function object(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    throw new Error('Expected a response object');
  }
  return value as Record<string, unknown>;
}

function stringField(value: Record<string, unknown>, key: string, allowEmpty = false): string {
  const field = value[key];
  if (typeof field !== 'string' || (!allowEmpty && !field.trim())) {
    throw new Error(`Invalid response field: ${key}`);
  }
  return field;
}

function statusField(value: unknown): JobStatus {
  if (value === 'pending' || value === 'running' || value === 'completed' || value === 'failed') {
    return value;
  }
  throw new Error('Invalid response field: status');
}

function timestampField(value: Record<string, unknown>, key: string): string {
  const timestamp = stringField(value, key);
  const parts = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d+)?(?:Z|[+-](\d{2}):(\d{2}))$/.exec(timestamp);
  if (!parts) {
    throw new Error(`Invalid response field: ${key}`);
  }
  const [, year, month, day, hour, minute, second, offsetHour = '0', offsetMinute = '0'] = parts;
  const leapYear = Number(year) % 4 === 0 && (Number(year) % 100 !== 0 || Number(year) % 400 === 0);
  const daysInMonth = [31, leapYear ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
  if (Number(month) < 1 || Number(month) > 12 ||
      Number(day) < 1 || Number(day) > daysInMonth[Number(month) - 1] ||
      Number(hour) > 23 || Number(minute) > 59 || Number(second) > 59 ||
      Number(offsetHour) > 23 || Number(offsetMinute) > 59 ||
      !Number.isFinite(Date.parse(timestamp))) {
    throw new Error(`Invalid response field: ${key}`);
  }
  return timestamp;
}

function acceptedBuild(value: Record<string, unknown>): AcceptedBuildResponse {
  const accepted: AcceptedBuildResponse = {
    job_id: stringField(value, 'job_id'),
    site_id: stringField(value, 'site_id'),
    owner_id: stringField(value, 'owner_id'),
    source_version: stringField(value, 'source_version'),
    target_version: stringField(value, 'target_version'),
    status: statusField(value.status),
  };
  if ('error_message' in value) accepted.error_message = stringField(value, 'error_message', true);
  if (accepted.status === 'failed') accepted.error_message = stringField(value, 'error_message');
  if (accepted.status !== 'failed' && accepted.error_message) {
    throw new Error('error_message is only valid for failed jobs');
  }
  return accepted;
}

export function parseBuildResponse(input: unknown): BuildResponse {
  const value = object(input);
  if ('question' in value || 'conversation_id' in value) {
    if (['job_id', 'site_id', 'owner_id', 'source_version', 'target_version', 'status'].some(key => key in value)) {
      throw new Error('Build response cannot mix clarification and accepted job fields');
    }
    return {
      question: stringField(value, 'question'),
      conversation_id: stringField(value, 'conversation_id'),
    };
  }
  return acceptedBuild(value);
}

export function parseJobSnapshot(input: unknown): JobSnapshot {
  const value = object(input);
  const job: JobSnapshot = {
    ...acceptedBuild(value),
    prompt: stringField(value, 'prompt'),
    created_at: timestampField(value, 'created_at'),
    updated_at: timestampField(value, 'updated_at'),
  };
  for (const key of ['result', 'manifest_path'] as const) {
    if (key in value) job[key] = stringField(value, key, true);
  }
  if (job.status !== 'completed' && job.manifest_path) {
    throw new Error('manifest_path is only valid for completed jobs');
  }
  return job;
}
