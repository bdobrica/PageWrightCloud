// Build-time API origin is validated before interpolation into nginx syntax.
import { pathToFileURL } from 'node:url';

export function securityConfig(value) {
  if (!/^https?:\/\/[a-z0-9.-]+(?::[0-9]{1,5})?\/?$/.test(value)) throw Error('Invalid public API origin');
  const url = new URL(value);
  if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password
      || url.search || url.hash || url.pathname !== '/'
      || !/^[a-z0-9.-]+(?::[0-9]+)?$/.test(url.host)) throw Error('Invalid public API origin');
  if (url.port && Number(url.port) < 1) throw Error('Invalid public API port');
  if (url.hostname.length > 253 || url.hostname.split('.').some(label => !/^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(label))) throw Error('Invalid public API hostname');
  return `add_header Content-Security-Policy "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self' ${url.origin}; base-uri 'none'; object-src 'none'; frame-ancestors 'none'; form-action 'self'" always;
add_header X-Content-Type-Options "nosniff" always;
add_header X-Frame-Options "DENY" always;
add_header Referrer-Policy "no-referrer" always;
add_header Cross-Origin-Opener-Policy "same-origin" always;
add_header Origin-Agent-Cluster "?1" always;
add_header Permissions-Policy "camera=(), microphone=(), geolocation=(), payment=(), usb=()" always;
`;
}
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  process.stdout.write(securityConfig(process.env.VITE_PAGEWRIGHT_API_URL ?? 'http://localhost:8085'));
}
