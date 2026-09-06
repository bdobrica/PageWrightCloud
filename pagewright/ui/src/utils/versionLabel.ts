import type { Site } from '../types/api';

export function versionLabel(id: string, site?: Site | null): string {
  const labels: string[] = [];
  if (site?.live_version_id === id) labels.push('Live');
  if (site?.preview_version_id === id) labels.push('Preview');
  if (id === 'initial') labels.push('Starter source');
  return labels.length ? labels.join(' · ') : 'Saved build';
}
