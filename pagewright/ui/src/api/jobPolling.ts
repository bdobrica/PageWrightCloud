import type { BuildHistoryItem, PaginatedResponse } from '../types/api.ts';

export const POLL_DELAYS = [2000, 4000, 8000, 15000] as const;
export const POLL_MAX_ROUNDS = 60;
export const POLL_MAX_FAILURES = 5;
export const POLL_MAX_ELAPSED = 15 * 60 * 1000;
export type HistoryPage = PaginatedResponse<BuildHistoryItem>;
export interface PollState { data?: HistoryPage; message: string; paused: boolean }

export function isActiveJob(job: BuildHistoryItem) {
  return job.status === 'pending' || job.status === 'running';
}

export function mergeJob(previous: BuildHistoryItem, next: BuildHistoryItem): BuildHistoryItem {
  if (previous.site_id !== next.site_id || previous.job_id !== next.job_id ||
      previous.source_version !== next.source_version || previous.target_version !== next.target_version) {
    throw new Error('Unexpected job identity');
  }
  // Never undo progress or revise a conservative terminal result.
  if (!isActiveJob(previous) || Date.parse(next.updated_at) < Date.parse(previous.updated_at) ||
      (previous.status === 'running' && next.status === 'pending')) return previous;
  return next;
}

interface PollOptions {
  loadSite(signal: AbortSignal): Promise<{ id: string }>;
  loadPage(signal: AbortSignal): Promise<HistoryPage>;
  loadJob(id: string, signal: AbortSignal): Promise<BuildHistoryItem>;
  onState(state: PollState): void;
  onCompleted(): void;
  schedule?: (fn: () => void, delay: number) => () => void;
  now?: () => number;
}

// One sequential request chain per mounted page, never an overlapping interval.
// Transport requests have their own timeout; cleanup aborts in-flight I/O.
export function startJobPolling(options: PollOptions): () => void {
  const controller = new AbortController();
  const now = options.now ?? Date.now;
  const deadline = now() + POLL_MAX_ELAPSED;
  const schedule = options.schedule ?? ((fn, delay) => {
    const timer = setTimeout(fn, delay);
    return () => clearTimeout(timer);
  });
  let cancelTimer = () => {};
  let data: HistoryPage | undefined;
  let rounds = 0, failures = 0;
  const emit = (message: string, paused = false) => {
    if (!controller.signal.aborted) options.onState({ data, message, paused });
  };
  const tick = async () => {
    if (controller.signal.aborted) return;
    if (now() >= deadline) {
      emit('Automatic checks paused after 15 minutes. Refresh to resume; the build has not been cancelled.', true);
      return;
    }
    let error: unknown;
    try {
      if (!data) {
        const site = await options.loadSite(controller.signal);
        if (controller.signal.aborted) return;
        if (!site.id) throw new Error('Missing site identity');
        const page = await options.loadPage(controller.signal);
        if (controller.signal.aborted) return;
        if (page.data.some(job => job.site_id !== site.id)) throw new Error('Unexpected site history');
        data = page;
        // The initial sidebar request may have raced durable reconciliation.
        if (data.data.some(job => job.status === 'completed')) options.onCompleted();
      } else {
        for (const previous of data.data.filter(isActiveJob)) {
          if (now() >= deadline) break;
          const next = await options.loadJob(previous.job_id, controller.signal);
          if (controller.signal.aborted) return;
          const merged = mergeJob(previous, next);
          data = { ...data, data: data.data.map(job => job.job_id === merged.job_id ? merged : job) };
          if (merged.status === 'completed') options.onCompleted();
          emit('Checking active builds on this page…');
        }
      }
      failures = 0;
    } catch (caught) {
      error = caught;
      failures++;
    }
    if (controller.signal.aborted) return;
    if (!error && data && !data.data.some(isActiveJob)) {
      emit('All builds on this page have finished.');
      return;
    }
    const status = (error as { response?: { status?: number } } | undefined)?.response?.status;
    if (status === 401 || status === 403 || status === 404) {
      emit('Status checks paused: access or job availability changed. Refresh to try again.', true);
      return;
    }
    if (failures >= POLL_MAX_FAILURES || rounds >= POLL_MAX_ROUNDS || now() >= deadline) {
      emit('Automatic checks paused. Saved status is unchanged; refresh to resume. This does not cancel the build.', true);
      return;
    }
    emit(error ? 'Could not check status. Keeping saved results and retrying…' : 'Checking active builds on this page…');
    const delay = POLL_DELAYS[Math.min(rounds, POLL_DELAYS.length - 1)];
    rounds++;
    cancelTimer = schedule(() => { void tick(); }, delay);
  };
  void tick();
  return () => { controller.abort(); cancelTimer(); };
}

export function buildFailureDetail(code?: string): string {
  switch (code) {
    case 'job_busy': return 'Another build was already active for this site.';
    case 'spawn_failed': return 'The worker could not be started.';
    case 'worker_exit': return 'The worker exited without a confirmed result.';
    case 'artifact_incomplete': return 'The output could not be verified. The build remains failed.';
    case 'job_conflict': return 'The saved build identity conflicts with the manager record. Contact the operator.';
    case 'manager_unavailable': return 'The manager was unavailable during submission.';
    default: return 'The build failed. Give the job ID to the operator for private diagnostic details.';
  }
}
