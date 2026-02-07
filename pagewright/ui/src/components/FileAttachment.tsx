import React, { useRef, type ChangeEvent } from 'react';
import type { Dispatch, SetStateAction } from 'react';
import './FileAttachment.css';

interface FileAttachmentProps {
  files: File[];
  onFilesChange: Dispatch<SetStateAction<File[]>>;
  disabled: boolean;
}

export const FileAttachment: React.FC<FileAttachmentProps> = ({ files, onFilesChange, disabled }) => {
  const fileInputRef = useRef<HTMLInputElement>(null);

  const handleFileChange = (e: ChangeEvent<HTMLInputElement>) => {
    const fileList = e.target.files;
    if (fileList && fileList.length > 0) {
      const newFiles = Array.from(fileList);
      // Validate file types
      const validTypes = ['image/jpeg', 'image/png', 'image/gif', 'text/plain', 'application/pdf'];
      const invalidFiles = newFiles.filter(file => !validTypes.includes(file.type));
      if (invalidFiles.length > 0) {
        alert('Only JPG, PNG, GIF, TXT, and PDF files are allowed');
        return;
      }
      // Validate file sizes (10MB max each)
      const oversizedFiles = newFiles.filter(file => file.size > 10 * 1024 * 1024);
      if (oversizedFiles.length > 0) {
        alert('Each file must be less than 10MB');
        return;
      }
      onFilesChange(prev => [...prev, ...newFiles]);
    }
    // Reset input
    if (fileInputRef.current) {
      fileInputRef.current.value = '';
    }
  };

  const handleRemove = (index: number) => {
    onFilesChange(prev => prev.filter((_, i) => i !== index));
  };

  const handleClick = () => {
    if (!disabled) {
      fileInputRef.current?.click();
    }
  };

  return (
    <div className="file-attachment">
      <input
        ref={fileInputRef}
        type="file"
        accept=".jpg,.jpeg,.png,.gif,.txt,.pdf"
        onChange={handleFileChange}
        style={{ display: 'none' }}
        multiple
        disabled={disabled}
      />
      {files.length > 0 && (
        <div className="files-list">
          {files.map((file, index) => (
            <div key={index} className="file-item">
              <span className="file-name">{file.name}</span>
              <button
                type="button"
                className="pure-button button-error button-small"
                onClick={() => handleRemove(index)}
                disabled={disabled}
              >
                ×
              </button>
            </div>
          ))}
        </div>
      )}
      <button
        type="button"
        className="pure-button attach-button"
        onClick={handleClick}
        disabled={disabled}
      >
        📎 Attach Files
      </button>
    </div>
  );
};
