import React, { useEffect, useId, useRef, useState } from 'react';
import { openActivatedPreview } from '../api/preview';
import type { Version } from '../types/api';
import { formatTimestamp } from '../utils/format';
import './Modal.css';

interface VersionActionModalProps {
  version: Version;
  label?: string;
  siteId: string;
  onClose: () => void;
  onPreview: () => Promise<string>;
  onPromote: () => Promise<void>;
}

export const VersionActionModal: React.FC<VersionActionModalProps> = ({
  version,
  label = 'Saved build',
  onClose,
  onPreview,
  onPromote,
}) => {
  const [previewPending, setPreviewPending] = useState(false);
  const [promotePending, setPromotePending] = useState(false);
  const [previewError, setPreviewError] = useState('');
  const [promoteError, setPromoteError] = useState('');
  const [previewURL, setPreviewURL] = useState('');
  const previewBusy = useRef(false);
  const mounted = useRef(true);
  const dialog = useRef<HTMLDialogElement>(null);
  const titleId = useId();
  useEffect(() => {
    const element = dialog.current!;
    const opener = document.activeElement;
    const overflow = document.body.style.overflow;
    element.showModal();
    document.body.style.overflow = 'hidden';
    return () => {
      element.close();
      document.body.style.overflow = overflow;
      if (opener instanceof HTMLElement && opener.isConnected) opener.focus();
      else document.getElementById('versions-heading')?.focus();
    };
  }, []);
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
      setPromoteError('');
      try {
        await onPromote();
        if (mounted.current) onClose();
      } catch (error) {
        console.error('Failed to promote version:', error);
        if (mounted.current) setPromoteError('Publishing could not be confirmed. Refresh hosting state and retry this same version; it may already be live.');
      } finally {
        previewBusy.current = false;
        if (mounted.current) setPromotePending(false);
      }
    }
  };

  return (
    <dialog ref={dialog} className="version-dialog" aria-labelledby={titleId}
      onCancel={(event) => { event.preventDefault(); onClose(); }}
      onClick={(event) => {
        if (event.target !== event.currentTarget) return;
        const rect = event.currentTarget.getBoundingClientRect();
        if (event.clientX < rect.left || event.clientX > rect.right ||
            event.clientY < rect.top || event.clientY > rect.bottom) onClose();
      }}>
      <div className="modal-content">
        <div className="modal-header">
          <h2 id={titleId}>{label} — {version.build_id}</h2>
          <button className="modal-close" onClick={onClose} aria-label="Close version actions" autoFocus>
            ×
          </button>
        </div>
        <div className="modal-body">
          <p>
            <strong>Created:</strong> {formatTimestamp(version.created_at)}
          </p>
          <div className="modal-actions">
            <p role="status">{previewPending ? 'Preparing preview…' : promotePending ? 'Publishing…' : ''}</p>
            <button className="pure-button pure-button-primary" onClick={handlePreview} disabled={previewPending || promotePending}>
              {previewPending ? 'Preparing preview…' : 'Preview in New Tab'}
            </button>
            <button className="pure-button pure-button-primary" onClick={handlePromote} disabled={previewPending || promotePending}>
              {promotePending ? 'Publishing…' : 'Promote to Live'}
            </button>
            <p>Version deletion is unavailable in this MVP.</p>
            {previewError && <p role="alert">{previewError}</p>}
            {promoteError && <p role="alert">{promoteError}</p>}
            {previewURL && <p>Preview activated. <a href={previewURL} target="_blank" rel="noopener noreferrer">Open preview</a> if a new tab did not open.</p>}
          </div>
        </div>
      </div>
    </dialog>
  );
};
