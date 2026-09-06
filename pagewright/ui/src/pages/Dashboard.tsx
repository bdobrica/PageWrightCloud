import { getErrorMessage } from '../utils/errors';
import React, { useEffect, useState, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { Layout } from '../components/Layout';
import { SiteCard } from '../components/SiteCard';
import { apiClient } from '../api/client';
import type { Site } from '../types/api';
import './Dashboard.css';

export const Dashboard: React.FC = () => {
  const [sites, setSites] = useState<Site[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState('');
  const navigate = useNavigate();
  const [refresh, setRefresh] = useState(0);
  const [actionError, setActionError] = useState('');
  const [busy, setBusy] = useState<string[]>([]);
  const pending = useRef(new Set<string>());
  const mounted = useRef(true);

  useEffect(() => {
    mounted.current = true;
    const reload = () => { if (!document.hidden) setRefresh(n => n + 1); };
    window.addEventListener('focus', reload);
    document.addEventListener('visibilitychange', reload);
    return () => {
      mounted.current = false;
      window.removeEventListener('focus', reload);
      document.removeEventListener('visibilitychange', reload);
    };
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    let active = true;
    setIsLoading(true);
    setError('');
    apiClient.listSites(1, 25, controller.signal).then(response => {
      if (active) setSites(response.data);
    }).catch(err => {
      if (active) setError(getErrorMessage(err, 'Failed to load sites. Retry refresh.'));
    }).finally(() => { if (active) setIsLoading(false); });
    return () => { active = false; controller.abort(); };
  }, [refresh]);

  const handleToggleEnabled = async (site: Site) => {
    if (pending.current.has(site.fqdn)) return;
    pending.current.add(site.fqdn);
    setBusy([...pending.current]);
    setActionError('');
    try {
      if (site.enabled) {
        await apiClient.disableSite(site.fqdn);
      } else {
        await apiClient.enableSite(site.fqdn);
      }
    } catch (err: unknown) {
      if (mounted.current) setActionError(getErrorMessage(err, 'Site update could not be confirmed. Refresh state before retrying.'));
    } finally {
      pending.current.delete(site.fqdn);
      if (mounted.current) {
        setBusy([...pending.current]);
        setRefresh(n => n + 1);
      }
    }
  };

  return (
    <Layout>
      <div className="dashboard">
        <div className="dashboard-header">
          <h1>My Sites</h1>
          <button className="pure-button pure-button-primary" onClick={() => navigate('/create-site')}>
            Create New Site
          </button>
        </div>

        <button onClick={() => setRefresh(n => n + 1)} disabled={isLoading}>Refresh sites</button>
        {isLoading && <p role="status">Refreshing site and deployment state…</p>}
        {error && <div className="error-message" role="alert">{error} Displayed state may be stale.</div>}
        {actionError && <p role="alert">{actionError}</p>}

        {sites.length === 0 && !isLoading && !error ? (
          <div className="empty-state">
            <p>No sites yet. Create your first site to get started!</p>
          </div>
        ) : (
          <div className="sites-grid">
            {sites.map((site) => (
              <SiteCard
                key={site.id}
                site={site}
                onToggleEnabled={handleToggleEnabled}
                busy={busy.includes(site.fqdn) || isLoading || !!error}
              />
            ))}
          </div>
        )}
      </div>
    </Layout>
  );
};
