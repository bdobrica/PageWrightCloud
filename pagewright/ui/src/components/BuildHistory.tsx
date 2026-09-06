import { useEffect, useState } from 'react';
import { apiClient } from '../api/client';
import type { BuildHistoryItem, PaginatedResponse } from '../types/api';

// Mounting a site (including browser refresh) reads durable server history.
// No timers here: bounded active-job polling belongs to M3.2.
export function BuildHistory({ fqdn, refresh }: { fqdn: string; refresh: number }) {
  const [page, setPage] = useState(1);
  const [retry, setRetry] = useState(0);
  const [result, setResult] = useState<{ data?: PaginatedResponse<BuildHistoryItem>; error?: string; page: number }>({ page: 0 });
  useEffect(() => {
    let cancelled = false;
    apiClient.listJobs(fqdn, page).then(data => {
      if (!cancelled) setResult({ data, page });
    }).catch(() => {
      if (!cancelled) setResult({ error: 'Build history could not be loaded. Your saved builds have not been removed.', page });
    });
    return () => { cancelled = true; };
  }, [fqdn, page, refresh, retry]);
  const data = result.page === page ? result.data : undefined;
  return <section aria-label="Build history">
    <h3>Build history</h3>
    <p>Last saved status; automatic updates are not enabled yet.</p>
    <button className="pure-button" onClick={() => setRetry(n => n + 1)}>Refresh history</button>
    {result.error && <p role="alert">{result.error}</p>}
    {!data && !result.error && <p role="status">Loading build history…</p>}
    {data && data.total_count === 0 && <p>No saved builds yet.</p>}
    {data && <ul>{data.data.map(job => <li key={job.job_id}>
      <strong>{job.status}</strong> — version {job.target_version}, based on {job.source_version}.
      {' '}<time dateTime={job.created_at}>{new Date(job.created_at).toLocaleString()}</time>
      {job.status === 'failed' && <span> Build failed ({job.error_code || 'build_failed'}).</span>}
      {job.dispatch_state === 'dispatching' && <span> Dispatch awaiting confirmation; do not resubmit as a new build.</span>}
      {job.recovery_error && <span> Recovery needs attention ({job.recovery_error}).</span>}
    </li>)}</ul>}
    <button className="pure-button" disabled={page <= 1} onClick={() => setPage(n => n - 1)}>Newer builds</button>
    <button className="pure-button" disabled={!data || page >= data.total_pages} onClick={() => setPage(n => n + 1)}>Older builds</button>
  </section>;
}
