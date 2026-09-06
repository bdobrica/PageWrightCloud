import React, { useState, useEffect } from 'react';
import type { Version, Site } from '../types/api';
import { versionLabel } from '../utils/versionLabel';
import { formatTimestamp } from '../utils/format';
import { apiClient } from '../api/client';
import { VersionActionModal } from './VersionActionModal';
import './VersionsList.css';

interface VersionsListProps {
  fqdn: string;
  refresh: number;
  onDeployed?: () => void;
  site?: Site | null;
}

export const VersionsList: React.FC<VersionsListProps> = ({ fqdn, refresh, onDeployed, site }) => {
  const [retry, setRetry] = useState(0);
  const [page, setPage] = useState(1);
  const [pages, setPages] = useState(0);
  const [versions, setVersions] = useState<Version[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [loadError, setLoadError] = useState(false);
  const [selectedVersion, setSelectedVersion] = useState<Version | null>(null);

  useEffect(() => {
    let active = true;
    const controller = new AbortController();
    const fetchVersions = async () => {
      try {
        setIsLoading(true);
        setVersions([]);
        setSelectedVersion(null);
        setLoadError(false);
        const response = await apiClient.listVersions(fqdn, page, 10, controller.signal);
        if (active) { setVersions(response.data); setPages(response.total_pages); }
      } catch (error) {
        console.error('Failed to fetch versions:', error);
        if (active) setLoadError(true);
      } finally {
        if (active) setIsLoading(false);
      }
    };

    fetchVersions();
    return () => { active = false; controller.abort(); };
  }, [fqdn, refresh, retry, page]);

  const handleVersionClick = (version: Version) => {
    setSelectedVersion(version);
  };

  const handlePreview = async () => {
    if (!selectedVersion) throw new Error('No version selected');
    try {
      const result = await apiClient.deployVersion(fqdn, selectedVersion.build_id, { target: 'preview' });
      return result.url;
    } finally { onDeployed?.(); }
  };

  const handlePromote = async () => {
    if (selectedVersion) {
      try {
        await apiClient.deployVersion(fqdn, selectedVersion.build_id, { target: 'live' });
        setSelectedVersion(null);
      } finally { onDeployed?.(); }
    }
  };

  return (
    <>
      <div className="versions-list">
        <h2 id="versions-heading" tabIndex={-1}>Versions</h2>
        {isLoading ? (
          <p className="no-versions">Loading...</p>
        ) : loadError ? (
          <p role="alert">Unable to load versions. Use Refresh versions to retry.</p>
        ) : versions.length === 0 ? (
          <p className="no-versions">No completed builds yet. Send a text request and follow Build history.</p>
        ) : (
          <div className="versions">
            {versions.map((version) => (
              <button type="button"
                key={version.build_id}
                className="version-item"
                onClick={() => handleVersionClick(version)}
              >
                <span className="version-id">{versionLabel(version.build_id, site)}</span>
                <span className="version-time"> — {formatTimestamp(version.created_at)}</span>
                <span> · {version.build_id}</span>
              </button>
            ))}
          </div>
        )}
        <button onClick={() => setRetry(n => n + 1)} disabled={isLoading}>Refresh versions</button>
        <button disabled={isLoading || page <= 1} onClick={() => setPage(n => n - 1)}>Newer versions</button>
        <button disabled={isLoading || loadError || page >= pages} onClick={() => setPage(n => n + 1)}>Older versions</button>
      </div>
      {selectedVersion && (
        <VersionActionModal
          key={selectedVersion.build_id}
          version={selectedVersion}
          label={versionLabel(selectedVersion.build_id, site)}
          siteId={selectedVersion.site_id}
          onClose={() => setSelectedVersion(null)}
          onPreview={handlePreview}
          onPromote={handlePromote}
        />
      )}
    </>
  );
};
