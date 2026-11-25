// Conservative PR classification. Unknown/empty/mixed product changes require full CI.
import { execFileSync } from 'node:child_process';
import { pathToFileURL } from 'node:url';

const docs = /^docs\/esterium-2\/[a-zA-Z0-9_/-]+\.md$/;
const windows = new Set([
  '.github/workflows/esterium-windows-setup.yml',
  'web/smoke/installed-lab.spec.ts',
  'web/playwright.installed-lab.config.ts',
]);
export function classify(paths, event = 'pull_request') {
  if (event !== 'pull_request' || !Array.isArray(paths) || paths.length === 0) return 'full';
  if (paths.some(p => typeof p !== 'string' || !p || p.split('/').some(x => x === '..' || x === '.' || !x))) return 'full';
  if (paths.every(p => docs.test(p))) return 'docs';
  if (paths.every(p => docs.test(p) || windows.has(p)) && paths.some(p => windows.has(p))) return 'windows';
  return 'full';
}
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  let scope = 'full';
  try {
    const [event, base, head] = process.argv.slice(2);
    if (event === 'pull_request' && /^[0-9a-f]{40}$/.test(base) && /^[0-9a-f]{40}$/.test(head)) {
      // No renames: both old and new paths participate. Never use a truncated API file list.
      const raw = execFileSync('git', ['diff', '--name-only', '-z', '--no-renames', base + '...' + head, '--'], { encoding: 'utf8', maxBuffer: 16 * 1024 * 1024 });
      if (!raw || raw.endsWith('\0')) scope = classify(raw.split('\0').filter(Boolean), event);
    }
  } catch {
    console.error('CI scope unavailable; running full validation.');
  }
  console.log(scope);
}
