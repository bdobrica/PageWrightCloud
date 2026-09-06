import { useEffect, useState } from 'react';
import { apiClient } from '../api/client';
import { buildFailureDetail, startJobPolling } from '../api/jobPolling';
import type { PollState } from '../api/jobPolling';

export function BuildHistory({ fqdn, onCompleted }: { fqdn: string; onCompleted: () => void }) {
  const [page, setPage] = useState(1);
  const [retry, setRetry] = useState(0);
  const [result, setResult] = useState<PollState & { page: number }>({ page: 0, message: 'Loading build history…', paused: false });
  useEffect(() => startJobPolling({
    loadSite: signal => apiClient.getSite(fqdn, signal),
    loadPage: signal => apiClient.listJobs(fqdn, page, signal),
    loadJob: (id, signal) => apiClient.getJob(fqdn, id, signal),
    onState: state => setResult(previous => ({ ...state, page, data: state.data ?? (previous.page === page ? previous.data : undefined) })),
    onCompleted,
  }), [fqdn, page, retry, onCompleted]);
  const data = result.page === page ? result.data : undefined;
  return <section aria-label="Build history">
    <h3>Build history</h3>
    <p role="status">{result.page === page ? result.message : 'Loading build history…'}</p>
    <button className="pure-button" onClick={() => setRetry(n => n + 1)}>Refresh history / resume checks</button>
    {data && data.total_count === 0 && <p>No saved builds yet.</p>}
    {data && <ul>{data.data.map(job => <li key={job.job_id}>
      <strong>{job.status}</strong> — version {job.target_version}, based on {job.source_version}.
      {' '}<time dateTime={job.created_at}>{new Date(job.created_at).toLocaleString()}</time>
      <div>Job: <code>{job.job_id}</code></div>
      {job.status === 'failed' && <span>{buildFailureDetail(job.error_code)} ({job.error_code || 'build_failed'})</span>}
      {job.dispatch_state === 'dispatching' && <span> Dispatch awaiting confirmation; do not resubmit as a new build.</span>}
      {job.recovery_error && <span> Recovery needs attention ({job.recovery_error}).</span>}
    </li>)}</ul>}
    <button className="pure-button" disabled={page <= 1} onClick={() => setPage(n => n - 1)}>Newer builds</button>
    <button className="pure-button" disabled={!data || page >= data.total_pages} onClick={() => setPage(n => n + 1)}>Older builds</button>
  </section>;
}
