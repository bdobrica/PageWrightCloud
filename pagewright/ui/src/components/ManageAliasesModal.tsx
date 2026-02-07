import React, { useEffect, useState } from 'react';
import { apiClient } from '../api/client';
import type { SiteAlias } from '../types/api';
import './Modal.css';

interface ManageAliasesModalProps {
  siteId: string;
  fqdn: string;
  onClose: () => void;
}

export const ManageAliasesModal: React.FC<ManageAliasesModalProps> = ({ siteId, fqdn, onClose }) => {
  const [aliases, setAliases] = useState<SiteAlias[]>([]);
  const [newAlias, setNewAlias] = useState('');
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    loadAliases();
  }, []);

  const loadAliases = async () => {
    try {
      const data = await apiClient.listAliases(fqdn);
      setAliases(data);
    } catch (err: any) {
      setError(err.response?.data?.message || 'Failed to load aliases');
    } finally {
      setIsLoading(false);
    }
  };

  const handleAdd = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');

    try {
      const alias = await apiClient.addAlias(fqdn, { alias: newAlias });
      setAliases([...aliases, alias]);
      setNewAlias('');
    } catch (err: any) {
      setError(err.response?.data?.message || 'Failed to add alias');
    }
  };

  const handleDelete = async (alias: string) => {
    if (!confirm(`Delete alias ${alias}?`)) return;

    try {
      await apiClient.deleteAlias(fqdn, alias);
      setAliases(aliases.filter((a) => a.alias !== alias));
    } catch (err: any) {
      setError(err.response?.data?.message || 'Failed to delete alias');
    }
  };

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal-content" onClick={(e) => e.stopPropagation()}>
        <div className="modal-header">
          <h2>Manage Aliases for {fqdn}</h2>
          <button className="modal-close" onClick={onClose}>
            ×
          </button>
        </div>

        <div className="modal-body">
          {error && <div className="error-message">{error}</div>}

          {isLoading ? (
            <p>Loading aliases...</p>
          ) : (
            <>
              <div className="aliases-list">
                {aliases.length === 0 ? (
                  <p>No aliases configured</p>
                ) : (
                  <ul>
                    {aliases.map((alias) => (
                      <li key={alias.id}>
                        <span>{alias.alias}</span>
                        <button className="pure-button button-error" onClick={() => handleDelete(alias.alias)}>
                          Delete
                        </button>
                      </li>
                    ))}
                  </ul>
                )}
              </div>

              <form onSubmit={handleAdd} className="pure-form">
                <h3>Add New Alias</h3>
                <input
                  type="text"
                  value={newAlias}
                  onChange={(e) => setNewAlias(e.target.value)}
                  placeholder="alias.example.com"
                  required
                />
                <button type="submit" className="pure-button pure-button-primary">
                  Add Alias
                </button>
              </form>
            </>
          )}
        </div>
      </div>
    </div>
  );
};
