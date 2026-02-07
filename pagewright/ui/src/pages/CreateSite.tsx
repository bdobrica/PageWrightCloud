import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Layout } from '../components/Layout';
import { apiClient } from '../api/client';
import { config } from '../config';

export const CreateSite: React.FC = () => {
  const [mode, setMode] = useState<'fqdn' | 'subdomain'>('subdomain');
  const [fqdn, setFqdn] = useState('');
  const [subdomain, setSubdomain] = useState('');
  const [templateId] = useState('template-1');
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState('');
  const navigate = useNavigate();

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setIsLoading(true);

    const siteFqdn = mode === 'fqdn' ? fqdn : `${subdomain}.${config.defaultDomain}`;

    try {
      await apiClient.createSite({ fqdn: siteFqdn, template_id: templateId });
      navigate(`/chat/${siteFqdn}`);
    } catch (err: any) {
      setError(err.response?.data?.message || 'Failed to create site');
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <Layout>
      <div style={{ maxWidth: '600px' }}>
        <h1>Create New Site</h1>

        <form onSubmit={handleSubmit} className="pure-form pure-form-stacked">
          {error && <div className="error-message">{error}</div>}

          <label>Domain Type</label>
          <label className="pure-radio">
            <input
              type="radio"
              checked={mode === 'subdomain'}
              onChange={() => setMode('subdomain')}
              disabled={isLoading}
            />
            Use a subdomain of {config.defaultDomain}
          </label>
          <label className="pure-radio">
            <input
              type="radio"
              checked={mode === 'fqdn'}
              onChange={() => setMode('fqdn')}
              disabled={isLoading}
            />
            Use my own domain
          </label>

          {mode === 'subdomain' ? (
            <div>
              <label htmlFor="subdomain">Subdomain</label>
              <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                <input
                  id="subdomain"
                  type="text"
                  value={subdomain}
                  onChange={(e) => setSubdomain(e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, ''))}
                  required
                  disabled={isLoading}
                  placeholder="mysite"
                  style={{ flex: 1 }}
                />
                <span>.{config.defaultDomain}</span>
              </div>
            </div>
          ) : (
            <div>
              <label htmlFor="fqdn">Full Domain Name</label>
              <input
                id="fqdn"
                type="text"
                value={fqdn}
                onChange={(e) => setFqdn(e.target.value.toLowerCase())}
                required
                disabled={isLoading}
                placeholder="www.example.com"
              />
            </div>
          )}

          <label htmlFor="template">Template</label>
          <select id="template" disabled={isLoading}>
            <option value="template-1">Basic Template</option>
          </select>

          <button type="submit" className="pure-button pure-button-primary" disabled={isLoading}>
            {isLoading ? 'Creating...' : 'Create Site & Start Building'}
          </button>
        </form>
      </div>
    </Layout>
  );
};
