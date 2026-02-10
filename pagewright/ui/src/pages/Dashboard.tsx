import React, { useEffect, useState } from 'react';
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

  useEffect(() => {
    loadSites();
  }, []);

  const loadSites = async () => {
    try {
      const response = await apiClient.listSites();
      setSites(response.data || []);
    } catch (err: any) {
      setError(err.response?.data?.message || 'Failed to load sites');
    } finally {
      setIsLoading(false);
    }
  };

  const handleDelete = async (fqdn: string) => {
    if (!confirm(`Delete site ${fqdn}?`)) return;

    try {
      await apiClient.deleteSite(fqdn);
      setSites(sites.filter((s) => s.fqdn !== fqdn));
    } catch (err: any) {
      alert(err.response?.data?.message || 'Failed to delete site');
    }
  };

  const handleToggleEnabled = async (site: Site) => {
    try {
      if (site.enabled) {
        await apiClient.disableSite(site.fqdn);
      } else {
        await apiClient.enableSite(site.fqdn);
      }
      setSites(sites.map((s) => (s.fqdn === site.fqdn ? { ...s, enabled: !s.enabled } : s)));
    } catch (err: any) {
      alert(err.response?.data?.message || 'Failed to update site');
    }
  };

  if (isLoading) {
    return (
      <Layout>
        <div>Loading sites...</div>
      </Layout>
    );
  }

  return (
    <Layout>
      <div className="dashboard">
        <div className="dashboard-header">
          <h1>My Sites</h1>
          <button className="pure-button pure-button-primary" onClick={() => navigate('/create-site')}>
            Create New Site
          </button>
        </div>

        {error && <div className="error-message">{error}</div>}

        {sites.length === 0 ? (
          <div className="empty-state">
            <p>No sites yet. Create your first site to get started!</p>
          </div>
        ) : (
          <div className="sites-grid">
            {sites.map((site) => (
              <SiteCard
                key={site.id}
                site={site}
                onDelete={handleDelete}
                onToggleEnabled={handleToggleEnabled}
              />
            ))}
          </div>
        )}
      </div>
    </Layout>
  );
};
