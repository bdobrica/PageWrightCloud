import React from 'react';
import { chatPath } from '../routes';
import { useNavigate } from 'react-router-dom';
import { HostingLinks } from './HostingLinks';
import type { Site } from '../types/api';
import './SiteCard.css';

interface SiteCardProps {
  site: Site;
  onToggleEnabled: (site: Site) => void;
}

export const SiteCard: React.FC<SiteCardProps> = ({ site, onToggleEnabled }) => {
  const navigate = useNavigate();
  const pending = site.initialization_status === 'pending';

  return (
    <>
      <div className="site-card">
        <div className="site-card-header">
          <h3>{site.fqdn}</h3>
          <span className={`status-badge ${site.enabled ? 'enabled' : 'disabled'}`}>
            {site.enabled ? 'Enabled' : 'Disabled'}
          </span>
        </div>

        <div className="site-card-info">
          {pending && <p>Initialization incomplete. Resume setup to safely retry.</p>}
          <p><strong>Template:</strong> {site.template_id}</p>
          <p><strong>Live Version:</strong> {site.live_version_id || 'None'}</p>
          <p><strong>Preview Version:</strong> {site.preview_version_id || 'None'}</p>
        </div>

        <div className="site-card-actions">
          <HostingLinks site={site} />
          <span>Aliases and site deletion are unavailable in this MVP.</span>
          <button onClick={() => onToggleEnabled(site)} className="pure-button" disabled={pending}>
            {site.enabled ? 'Disable' : 'Enable'}
          </button>
          <button onClick={() => navigate(pending ? `/create-site?fqdn=${encodeURIComponent(site.fqdn)}` : chatPath(site.fqdn))} className="pure-button pure-button-primary">
            {pending ? 'Resume Setup' : 'Build'}
          </button>
        </div>
      </div>
    </>
  );
};
