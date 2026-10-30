const stubApiUrl = process.env.E2E_STUB_API_URL || 'http://127.0.0.1:4174';
const expectedSeed = process.env.E2E_SEED || 'suitee-ci-v1';

export default async function setupIsolatedStack() {
  const resetResponse = await fetch(`${stubApiUrl}/__e2e/reset`, { method: 'POST' });
  if (!resetResponse.ok) {
    throw new Error(`Could not reset isolated E2E API: HTTP ${resetResponse.status}`);
  }

  const seedResponse = await fetch(`${stubApiUrl}/__e2e/seed`);
  const seedState = (await seedResponse.json()) as { seed?: string; mode?: string };
  if (!seedResponse.ok || seedState.seed !== expectedSeed) {
    throw new Error(
      `Isolated E2E seed mismatch: expected ${expectedSeed}, received ${seedState.seed || 'none'}`,
    );
  }
  if (seedState.mode !== 'isolated-deny-by-default') {
    throw new Error(`Unexpected E2E API mode: ${seedState.mode || 'none'}`);
  }
}
