// Offline file-target check. No remote requests or application secret reads.
import { existsSync, readFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

export function localLinks(markdown) {
  const prose = markdown.replace(/^\s*(`{3,}|~{3,})[^\n]*\n[\s\S]*?^\s*\1\s*$/gm, '');
  return [...prose.matchAll(/\]\(<?([^\s)>]+)>?(?:\s+"[^"]*")?\)/g)]
    .map(match => match[1])
    .filter(url => !/^(?:[a-z][a-z\d+.-]*:|\/\/|#)/i.test(url))
    .map(url => decodeURIComponent(url.split(/[?#]/)[0])).filter(Boolean);
}

export function checkLinks(files, root, read = readFileSync, exists = existsSync) {
  const failures = [];
  let checked = 0;
  for (const file of files) {
    for (const link of localLinks(read(resolve(root, file), 'utf8'))) {
      checked++;
      if (!exists(resolve(root, dirname(file), link))) failures.push(`${file}: ${link}`);
    }
  }
  return { checked, failures };
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const root = execFileSync('git', ['rev-parse', '--show-toplevel'], { encoding: 'utf8' }).trim();
  const files = [...new Set(execFileSync('git', ['ls-files', '-z', '--cached', '--others', '--exclude-standard'],
    { cwd: root, encoding: 'utf8' }).split('\0'))]
    .filter(file => file.endsWith('.md') && existsSync(resolve(root, file)))
    // Site/theme fixture Markdown has website routes, not repository links.
    .filter(file => !file.includes('/') || file.startsWith('docs/') ||
      file.endsWith('/README.md') || file.endsWith('/PROFILE_PROVENANCE.md'));
  const { checked, failures } = checkLinks(files, root);
  for (const failure of failures) console.error(`Missing local documentation target: ${failure}`);
  console.log(`Checked ${checked} local link targets in ${files.length} Markdown files; ${failures.length} missing.`);
  // Only inline file targets: external URLs, reference-style links and heading
  // anchors require separate review. Never fetch historical temporary evidence.
  process.exitCode = failures.length ? 1 : 0;
}
