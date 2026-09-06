// The gateway owns the namespace. Never fall back to a build-time domain when
// capabilities cannot be loaded; that could reserve the wrong site on retry.
export function parseSiteDomain(value: unknown): string {
  if (!value || typeof value !== 'object' || !('mode' in value) || value.mode !== 'mvp'
    || !('site_domain' in value) || typeof value.site_domain !== 'string') {
    throw new Error('Unsupported server capabilities');
  }
  const domain = value.site_domain;
  if (domain.length > 181 || !domain.includes('.') || !domain.split('.').every(label =>
    /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(label))) {
    throw new Error('Invalid platform domain');
  }
  return domain;
}

export function platformLabel(fqdn: string, domain: string): string | null {
  const suffix = '.' + domain;
  if (!fqdn.endsWith(suffix)) return null;
  const label = fqdn.slice(0, -suffix.length);
  return /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(label)
    && !['preview', 'www', 'api', 'app'].includes(label) ? label : null;
}
