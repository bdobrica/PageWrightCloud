import { getErrorMessage } from '../utils/errors';
import React, { useEffect, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { Layout } from '../components/Layout';
import { apiClient } from '../api/client';
import { platformLabel } from '../api/capabilities';

export const CreateSite: React.FC = () => {
  const [searchParams] = useSearchParams();
  const resumeFqdn = searchParams.get('fqdn') || '';
  const [domain, setDomain] = useState('');
  const [subdomain, setSubdomain] = useState('');
  const [attempt, setAttempt] = useState(0);
  const [configError, setConfigError] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState('');
  const navigate = useNavigate();

  useEffect(() => {
    const controller = new AbortController();
    let active = true;
    setDomain('');
    setConfigError('');
    apiClient.getSiteDomain(controller.signal).then(value => {
      if (active) setDomain(value);
    }).catch(err => {
      if (active) setConfigError(getErrorMessage(err, 'Could not load the platform domain'));
    });
    return () => { active = false; controller.abort(); };
  }, [attempt]);

  const resumeLabel = domain && resumeFqdn ? platformLabel(resumeFqdn, domain) : null;
  const label = resumeFqdn ? resumeLabel ?? '' : subdomain;
  const valid = !!domain && platformLabel(label + '.' + domain, domain) !== null;
  const unsupportedResume = !!domain && !!resumeFqdn && resumeLabel === null;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!valid || isLoading) return;
    setError('');
    setIsLoading(true);
    try {
      const site = await apiClient.createSite({ fqdn: label + '.' + domain, template_id: 'starter' });
      navigate('/chat/' + encodeURIComponent(site.fqdn));
    } catch (err: unknown) {
      setError(getErrorMessage(err, 'Failed to create site'));
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <Layout>
      <div style={{ maxWidth: '600px' }}>
        <h1>{resumeFqdn ? 'Resume Site Setup' : 'Create New Site'}</h1>
        <p>The MVP supports platform subdomains and text-only editing. Custom domains and aliases are unavailable.</p>
        {!domain && !configError && <p>Loading platform domain…</p>}
        {configError && <div role="alert">{configError} <button onClick={() => setAttempt(n => n + 1)}>Retry</button></div>}
        {unsupportedResume && <p role="alert">This site's domain is outside the current platform namespace. Ask the operator to restore its original domain configuration; no replacement site has been created.</p>}
        <form onSubmit={handleSubmit} className="pure-form pure-form-stacked">
          {error && <div className="error-message" role="alert">{error}</div>}
          <label htmlFor="subdomain">Subdomain</label>
          <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
            <input id="subdomain" type="text" value={label}
              onChange={e => setSubdomain(e.target.value.toLowerCase())}
              required maxLength={63}
              disabled={isLoading || !domain || !!resumeFqdn} placeholder="mysite" />
            <span>{domain ? '.' + domain : ''}</span>
          </div>
          <p>Use letters, numbers and internal hyphens. Platform and infrastructure names (such as app, api, www, preview and admin) are reserved; internationalized names are not supported.</p>
          <label htmlFor="template">Template</label>
          <select id="template" disabled><option value="starter">Starter</option></select>
          <button type="submit" className="pure-button pure-button-primary" disabled={isLoading || !valid}>
            {isLoading ? 'Creating...' : resumeFqdn ? 'Resume Setup' : 'Create Site & Start Building'}
          </button>
        </form>
      </div>
    </Layout>
  );
};
