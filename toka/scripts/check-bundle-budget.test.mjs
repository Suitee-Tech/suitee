import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { randomBytes } from 'node:crypto';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
const script = fileURLToPath(new URL('./check-bundle-budget.mjs', import.meta.url));
test('sums static imports, ignores lazy chunks and prevents hiding excess in small chunks', () => {
  const dir = mkdtempSync(join(tmpdir(), 'suitee-bundle-'));
  try {
    mkdirSync(join(dir, '.vite'));
    for (const file of ['entry.js', 'shared.js', 'lazy.js']) writeFileSync(join(dir, file), randomBytes(1200));
    writeFileSync(join(dir, '.vite/manifest.json'), JSON.stringify({
      entry: { isEntry: true, file: 'entry.js', imports: ['shared'], dynamicImports: ['lazy'] },
      shared: { file: 'shared.js' }, lazy: { file: 'lazy.js' },
    }));
    const run = total => spawnSync(process.execPath, [script], { encoding: 'utf8', env: { ...process.env, BUNDLE_DIST_DIR: dir, INITIAL_CHUNK_BUDGET_KB: '2', INITIAL_TOTAL_BUDGET_KB: total } });
    assert.equal(run('3').status, 0); // lazy.js must not count
    const rejected = run('2');
    assert.equal(rejected.status, 1);
    assert.match(rejected.stderr, /soma dos chunks/);
  } finally { rmSync(dir, { recursive: true, force: true }); }
});
