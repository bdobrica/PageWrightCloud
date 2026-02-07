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
  const [selectedVersion, setSelectedVersion] = useState<Version | null>(null);

  useEffect(() => {
    const fetchVersions = async () => {
      try {
        setIsLoading(true);
        const response = await apiClient.listVersions(fqdn, 1, 10);
        setVersions(response.data);
      } catch (error) {
        console.error('Failed to fetch versions:', error);
      } finally {
        setIsLoading(false);
      }
    };

    fetchVersions();
  }, [fqdn, refresh]);

  const handleVersionClick = (version: Version) => {
    setSelectedVersion(version);
  };

  const handlePreview = () => {
    if (selectedVersion) {
      window.open(`https://${fqdn}/preview/${selectedVersion.build_id}`, '_blank');
    }
  };

  const handlePromote = async () => {
    if (selectedVersion) {
      await apiClient.deployVersion(fqdn, selectedVersion.build_id, { target: 'live' });
      setSelectedVersion(null);
    }
  };

  const handleDelete = async () => {
    if (selectedVersion) {
      await apiClient.deleteVersion(fqdn, selectedVersion.build_id);
      setVersions((prev) => prev.filter((v) => v.build_id !== selectedVersion.build_id));
      setSelectedVersion(null);
    }
  };

  return (
    <>
      <div className="versions-list">
        <h3>Versions</h3>
        {isLoading ? (
          <p className="no-versions">Loading...</p>
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
          version={selectedVersion}
          siteId={selectedVersion.site_id}
          onClose={() => setSelectedVersion(null)}
          onPreview={handlePreview}
          onPromote={handlePromote}
          onDelete={handleDelete}
        />
      )}
    </>
  );
};
