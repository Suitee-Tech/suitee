import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

import { findSecretsInText } from './check-bundle-secrets.mjs';

const script = fileURLToPath(new URL('./check-bundle-secrets.mjs', import.meta.url));

const runWithDist = (files) => {
  const dir = mkdtempSync(join(tmpdir(), 'suitee-secrets-'));
  try {
    for (const [name, content] of Object.entries(files)) {
      const target = join(dir, name);
      const parent = name.includes('/') ? join(dir, name.split('/').slice(0, -1).join('/')) : dir;
      mkdirSync(parent, { recursive: true });
      writeFileSync(target, content);
    }
    return spawnSync(process.execPath, [script], {
      encoding: 'utf8',
      env: { ...process.env, BUNDLE_DIST_DIR: dir },
    });
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
};

test('detecta JWT, service_role, anon key e chaves com prefixo secreto', () => {
  const anon = ['eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0', 'eyJzdWIiOiJ0ZXN0Iiwicm9sZSI6ImFub24ifQ', 'c2lnbmF0dXJlLXRlc3Q'].join('.');
  assert.deepEqual(findSecretsInText(`window.__TOCA_ENV__={"k":"${anon}"}`), ['JWT']);
  assert.deepEqual(findSecretsInText('{"role":"service_role"}'), ['service_role']);
  assert.deepEqual(findSecretsInText('process.env.SUPABASE_SERVICE_ROLE_KEY'), ['SUPABASE_SERVICE_ROLE_KEY']);
  assert.deepEqual(findSecretsInText('VITE_SUPABASE_ANON_KEY'), ['SUPABASE_ANON_KEY']);
  assert.deepEqual(findSecretsInText('sb_secret_abcdefghijkl'), ['chave secreta Supabase (sb_secret_)']);
  assert.deepEqual(findSecretsInText('sk_live_abcdefghijkl'), ['chave secreta de pagamento (sk_live_/sk_test_)']);
});

test('não acusa env.js público nem placeholder de e2e', () => {
  const publicEnv =
    'window.__TOCA_ENV__ = {"VITE_API_URL":"https://api.example.com/api/v1","VITE_SUPABASE_URL":"https://x.supabase.co","VITE_MULTI_PARTY_UI":"true"};';
  assert.deepEqual(findSecretsInText(publicEnv), []);
  assert.deepEqual(findSecretsInText('anonKey:"isolated-e2e-anon-key"'), []);
});

test('exit 0 com bundle limpo e exit 1 com credencial no bundle', () => {
  const clean = runWithDist({
    'env.js': 'window.__TOCA_ENV__ = {"VITE_API_URL":"https://api.example.com/api/v1"};',
    'assets/app.js': 'const url = "/api/v1";',
  });
  assert.equal(clean.status, 0, clean.stderr);

  const leaked = runWithDist({
    'env.js': `window.__TOCA_ENV__ = {"VITE_SUPABASE_ANON_KEY":"${anon}"};`,
  });
  assert.equal(leaked.status, 1);
  assert.match(leaked.stderr, /credencial no bundle/);
});

test('exit 2 quando o dist não existe (não dá para validar)', () => {
  const run = spawnSync(process.execPath, [script], {
    encoding: 'utf8',
    env: { ...process.env, BUNDLE_DIST_DIR: join(tmpdir(), 'suitee-nao-existe-xyz') },
  });
  assert.equal(run.status, 2);
});
