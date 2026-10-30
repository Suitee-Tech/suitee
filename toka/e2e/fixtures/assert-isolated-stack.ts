const stubApiUrl = process.env.E2E_STUB_API_URL || 'http://127.0.0.1:4174';

export default async function assertIsolatedStack() {
  const stateResponse = await fetch(`${stubApiUrl}/__e2e/state`);
  const state = (await stateResponse.json()) as {
    unexpectedRequests?: Array<{ method: string; path: string }>;
  };
  const unexpected = state.unexpectedRequests || [];
  if (unexpected.length > 0) {
    const details = unexpected.map(({ method, path }) => `${method} ${path}`).join(', ');
    throw new Error(`Deterministic UI suite made unmocked API requests: ${details}`);
  }
}
