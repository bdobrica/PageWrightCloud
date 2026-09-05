import type { AcceptedBuildResponse, BuildResponse, JobSnapshot, JobStatus } from '../types/api.ts';

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
  return {
    job_id: stringField(value, 'job_id'),
    site_id: stringField(value, 'site_id'),
    owner_id: stringField(value, 'owner_id'),
    source_version: stringField(value, 'source_version'),
    target_version: stringField(value, 'target_version'),
    status: statusField(value.status),
  };
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
  for (const key of ['result', 'error_message', 'manifest_path'] as const) {
    if (key in value) job[key] = stringField(value, key, true);
  }
  if (job.status === 'failed') job.error_message = stringField(value, 'error_message');
  if (job.status !== 'failed' && job.error_message) {
    throw new Error('error_message is only valid for failed jobs');
  }
  if (job.status !== 'completed' && job.manifest_path) {
    throw new Error('manifest_path is only valid for completed jobs');
  }
  return job;
}
