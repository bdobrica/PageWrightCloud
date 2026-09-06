import type { Site } from '../types/api';

export function HostingLinks({ site }: { site: Site | null }) {
  return <>
    {(['live', 'preview'] as const).map(target => {
      const label = target === 'live' ? 'View Live' : 'View Preview';
      const url = site?.[`${target}_url`];
      return site?.enabled && site[`${target}_version_id`] && url
        ? <a key={target} href={url} target="_blank" rel="noopener noreferrer" className="pure-button">{label}</a>
        : <button key={target} disabled className="pure-button" title="No active deployment or hosting unavailable">{label}</button>;
    })}
  </>;
}
