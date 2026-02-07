import React, { useRef, ChangeEvent } from 'react';
import './FileAttachment.css';

interface FileAttachmentProps {
  onFileSelect: (file: File | null) => void;
  selectedFile: File | null;
}

export const FileAttachment: React.FC<FileAttachmentProps> = ({ onFileSelect, selectedFile }) => {
  const fileInputRef = useRef<HTMLInputElement>(null);

  const handleFileChange = (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0] || null;
    if (file) {
      // Validate file type
      const validTypes = ['image/jpeg', 'image/png', 'image/gif'];
      if (!validTypes.includes(file.type)) {
        alert('Only JPG, PNG, and GIF files are allowed');
        return;
      }
      // Validate file size (10MB max)
      if (file.size > 10 * 1024 * 1024) {
        alert('File size must be less than 10MB');
        return;
      }
    }
    onFileSelect(file);
  };

  const handleRemove = () => {
    onFileSelect(null);
    if (fileInputRef.current) {
      fileInputRef.current.value = '';
    }
  };

  const handleClick = () => {
    fileInputRef.current?.click();
  };

  return (
    <div className="file-attachment">
      <input
        ref={fileInputRef}
        type="file"
        accept=".jpg,.jpeg,.png,.gif"
        onChange={handleFileChange}
        style={{ display: 'none' }}
      />
      {selectedFile ? (
        <div className="file-preview">
          <img
            src={URL.createObjectURL(selectedFile)}
            alt="Preview"
            className="preview-image"
          />
          <div className="file-info">
            <span className="file-name">{selectedFile.name}</span>
            <button
              type="button"
              className="pure-button button-error button-small"
              onClick={handleRemove}
            >
              Remove
            </button>
          </div>
        </div>
      ) : (
        <button
          type="button"
          className="pure-button attach-button"
          onClick={handleClick}
        >
          📎 Attach Image
        </button>
      )}
    </div>
  );
};
