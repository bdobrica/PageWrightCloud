import React, { useEffect, useRef, useState } from 'react';
import { openActivatedPreview } from '../api/preview';
import type { Version } from '../types/api';
import { formatTimestamp } from '../utils/format';
import './Modal.css';

interface VersionActionModalProps {
  version: Version;
  siteId: string;
  onClose: () => void;
  onPreview: () => Promise<string>;
  onPromote: () => void;
}

export const VersionActionModal: React.FC<VersionActionModalProps> = ({
  version,
  onClose,
  onPreview,
  onPromote,
}) => {
  const [previewPending, setPreviewPending] = useState(false);
  const [promotePending, setPromotePending] = useState(false);
  const [previewError, setPreviewError] = useState('');
  const [previewURL, setPreviewURL] = useState('');
  const previewBusy = useRef(false);
  const mounted = useRef(true);
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; }; }, []);
  const handlePreview = async () => {
    if (previewBusy.current) return;
    previewBusy.current = true;
    setPreviewPending(true);
    setPreviewError('');
    setPreviewURL('');
    try {
      const url = await openActivatedPreview(onPreview, address => { window.open(address, '_blank', 'noopener,noreferrer'); }, () => mounted.current);
      if (url) setPreviewURL(url);
    } catch {
      if (mounted.current) setPreviewError('Preview could not be confirmed. Retry this version; activation may already have occurred.');
    } finally {
      previewBusy.current = false;
      if (mounted.current) setPreviewPending(false);
    }
  };
  const handlePromote = async () => {
    if (previewBusy.current) return;
    if (window.confirm('Promote this version to live? This will replace the current live version.')) {
      previewBusy.current = true;
      setPromotePending(true);
      try {
        await onPromote();
        onClose();
      } catch (error) {
        console.error('Failed to promote version:', error);
      } finally {
        previewBusy.current = false;
        if (mounted.current) setPromotePending(false);
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
            <strong>Created:</strong> {formatTimestamp(version.created_at)}
          </p>
          <div className="modal-actions">
            <button className="pure-button pure-button-primary" onClick={handlePreview} disabled={previewPending || promotePending}>
              {previewPending ? 'Preparing preview…' : 'Preview in New Tab'}
            </button>
            <button className="pure-button pure-button-primary" onClick={handlePromote} disabled={previewPending || promotePending}>
              Promote to Live
            </button>
            <p>Version deletion is unavailable in this MVP.</p>
            {previewError && <p role="alert">{previewError}</p>}
            {previewURL && <p>Preview activated. <a href={previewURL} target="_blank" rel="noopener noreferrer">Open preview</a> if a new tab did not open.</p>}
          </div>
        </div>
      </div>
    </div>
  );
};
