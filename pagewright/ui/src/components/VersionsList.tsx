import React from 'react';
import { Version } from '../types/api';
import { formatTimestamp } from '../utils/format';
import './VersionsList.css';

interface VersionsListProps {
  versions: Version[];
  onVersionClick: (version: Version) => void;
}

export const VersionsList: React.FC<VersionsListProps> = ({ versions, onVersionClick }) => {
  return (
    <div className="versions-list">
      <h3>Versions</h3>
      {versions.length === 0 ? (
        <p className="no-versions">No versions yet</p>
      ) : (
        <div className="versions">
          {versions.map((version) => (
            <div
              key={version.build_id}
              className="version-item"
              onClick={() => onVersionClick(version)}
            >
              <div className="version-id">#{version.build_id}</div>
              <div className="version-time">{formatTimestamp(version.timestamp)}</div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
};
