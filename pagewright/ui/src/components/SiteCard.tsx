import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { ManageAliasesModal } from './ManageAliasesModal';
import type { Site } from '../types/api';
import './SiteCard.css';

interface SiteCardProps {
  site: Site;
  onDelete: (fqdn: string) => void;
  onToggleEnabled: (site: Site) => void;
}

export const SiteCard: React.FC<SiteCardProps> = ({ site, onDelete, onToggleEnabled }) => {
  const [showAliases, setShowAliases] = useState(false);
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
          <a href={`https://${site.fqdn}`} target="_blank" rel="noopener noreferrer" className="pure-button">
            Live
          </a>
          <a href={`https://${site.fqdn}/preview`} target="_blank" rel="noopener noreferrer" className="pure-button">
            Preview
          </a>
          <button onClick={() => setShowAliases(true)} className="pure-button">
            Aliases
          </button>
          <button onClick={() => onToggleEnabled(site)} className="pure-button" disabled={pending}>
            {site.enabled ? 'Disable' : 'Enable'}
          </button>
          <button onClick={() => navigate(pending ? `/create-site?fqdn=${encodeURIComponent(site.fqdn)}` : `/chat/${site.fqdn}`)} className="pure-button pure-button-primary">
            {pending ? 'Resume Setup' : 'Build'}
          </button>
          <button onClick={() => onDelete(site.fqdn)} className="pure-button button-error">
            Delete
          </button>
        </div>
      </div>

      {showAliases && (
        <ManageAliasesModal siteId={site.id} fqdn={site.fqdn} onClose={() => setShowAliases(false)} />
      )}
    </>
  );
};
