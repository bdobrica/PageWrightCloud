import React, { useState } from 'react';
import { Version } from '../types/api';
import { formatTimestamp } from '../utils/format';
import './Modal.css';

interface VersionActionModalProps {
  version: Version;
  siteId: string;
  onClose: () => void;
  onPreview: () => void;
  onPromote: () => void;
  onDelete: () => void;
}

export const VersionActionModal: React.FC<VersionActionModalProps> = ({
  version,
  siteId,
  onClose,
  onPreview,
  onPromote,
  onDelete,
}) => {
  const [deleting, setDeleting] = useState(false);

  const handleDelete = async () => {
    if (window.confirm('Are you sure you want to delete this version? This action cannot be undone.')) {
      setDeleting(true);
      try {
        await onDelete();
        onClose();
      } catch (error) {
        console.error('Failed to delete version:', error);
        setDeleting(false);
      }
    }
  };

  const handlePromote = async () => {
    if (window.confirm('Promote this version to live? This will replace the current live version.')) {
      try {
        await onPromote();
        onClose();
      } catch (error) {
        console.error('Failed to promote version:', error);
      }
    }
  };

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal-content" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header">
          <h2>Version #{version.build_id}</h2>
          <button className="modal-close" onClick={onClose}>
            ×
          </button>
        </div>
        <div className="modal-body">
          <p>
            <strong>Created:</strong> {formatTimestamp(version.timestamp)}
          </p>
          <div className="modal-actions">
            <button className="pure-button pure-button-primary" onClick={onPreview}>
              Preview in New Tab
            </button>
            <button className="pure-button pure-button-primary" onClick={handlePromote}>
              Promote to Live
            </button>
            <button
              className="pure-button button-error"
              onClick={handleDelete}
              disabled={deleting}
            >
              {deleting ? 'Deleting...' : 'Delete Version'}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};
