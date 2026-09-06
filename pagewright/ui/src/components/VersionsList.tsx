import React, { useState, useEffect } from 'react';
import type { Version } from '../types/api';
import { formatTimestamp } from '../utils/format';
import { apiClient } from '../api/client';
import { VersionActionModal } from './VersionActionModal';
import './VersionsList.css';

interface VersionsListProps {
  fqdn: string;
  refresh: number;
}

export const VersionsList: React.FC<VersionsListProps> = ({ fqdn, refresh }) => {
  const [versions, setVersions] = useState<Version[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [loadError, setLoadError] = useState(false);
  const [selectedVersion, setSelectedVersion] = useState<Version | null>(null);

  useEffect(() => {
    let active = true;
    const fetchVersions = async () => {
      try {
        setIsLoading(true);
        setVersions([]);
        setSelectedVersion(null);
        setLoadError(false);
        const response = await apiClient.listVersions(fqdn, 1, 10);
        if (active) setVersions(response.data);
      } catch (error) {
        console.error('Failed to fetch versions:', error);
        if (active) setLoadError(true);
      } finally {
        if (active) setIsLoading(false);
      }
    };

    fetchVersions();
    return () => { active = false; };
  }, [fqdn, refresh]);

  const handleVersionClick = (version: Version) => {
    setSelectedVersion(version);
  };

  const handlePreview = async () => {
    if (!selectedVersion) throw new Error('No version selected');
    const result = await apiClient.deployVersion(fqdn, selectedVersion.build_id, { target: 'preview' });
    return result.url;
  };

  const handlePromote = async () => {
    if (selectedVersion) {
      await apiClient.deployVersion(fqdn, selectedVersion.build_id, { target: 'live' });
      setSelectedVersion(null);
    }
  };

  return (
    <>
      <div className="versions-list">
        <h3>Versions</h3>
        {isLoading ? (
          <p className="no-versions">Loading...</p>
        ) : loadError ? (
          <p role="alert">Unable to load versions.</p>
        ) : versions.length === 0 ? (
          <p className="no-versions">No versions yet</p>
        ) : (
          <div className="versions">
            {versions.map((version) => (
              <div
                key={version.build_id}
                className="version-item"
                onClick={() => handleVersionClick(version)}
              >
                <div className="version-id">#{version.build_id}</div>
                <div className="version-time">{formatTimestamp(version.created_at)}</div>
              </div>
            ))}
          </div>
        )}
      </div>
      {selectedVersion && (
        <VersionActionModal
          key={selectedVersion.build_id}
          version={selectedVersion}
          siteId={selectedVersion.site_id}
          onClose={() => setSelectedVersion(null)}
          onPreview={handlePreview}
          onPromote={handlePromote}
        />
      )}
    </>
  );
};
