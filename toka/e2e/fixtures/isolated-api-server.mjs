import { createServer } from 'node:http';

const host = '127.0.0.1';
const port = Number.parseInt(process.env.E2E_STUB_API_PORT || '4174', 10);
const seed = process.env.E2E_SEED || 'suitee-ci-v1';
let unexpectedRequests = [];

function json(response, status, body) {
  response.writeHead(status, {
    'access-control-allow-headers': 'authorization,content-type',
    'access-control-allow-methods': 'GET,POST,PUT,PATCH,DELETE,OPTIONS',
    'access-control-allow-origin': '*',
    'cache-control': 'no-store',
    'content-type': 'application/json; charset=utf-8',
  });
  response.end(JSON.stringify(body));
}

const server = createServer((request, response) => {
  const url = new URL(request.url || '/', `http://${host}:${port}`);

  if (request.method === 'GET' && url.pathname === '/__e2e/seed') {
    json(response, 200, {
      mode: 'isolated-deny-by-default',
      seed,
      fixtures: {
        companyId: 'company-1',
        userId: 'fake-user-id',
      },
    });
    return;
  }

  if (request.method === 'POST' && url.pathname === '/__e2e/reset') {
    unexpectedRequests = [];
    json(response, 200, { seed });
    return;
  }

  if (request.method === 'GET' && url.pathname === '/__e2e/state') {
    json(response, 200, { seed, unexpectedRequests });
    return;
  }

  // Deterministic unauthenticated boot used by login/signup/password recovery
  // pages. Authenticated specs override this endpoint with page.route().
  if (request.method === 'POST' && url.pathname === '/api/v1/auth/refresh') {
    json(response, 401, {
      success: false,
      statusCode: 401,
      message: 'No seeded browser session',
      seed,
    });
    return;
  }

  // Client-side error reporter. The app POSTs here from its global error handler
  // whenever any API call fails — including the failures specs provoke on
  // purpose (403/409/500/network). It is fire-and-forget telemetry that never
  // changes UI state, so it is answered locally instead of forcing every
  // error-path spec to intercept it: otherwise the suite fails its own teardown
  // over a request it deliberately caused. Like POST /auth/refresh above, the
  // response stays inside the containment server — nothing escapes.
  if (request.method === 'POST' && url.pathname === '/api/v1/client-errors') {
    json(response, 202, { success: true, statusCode: 202, seed });
    return;
  }

  // The successful login test redirects into the application shell. These
  // background reads receive the explicit empty seed instead of escaping to a
  // real backend. Mutations and every unknown read remain denied.
  if (request.method === 'GET' && url.pathname.startsWith('/api/v1/')) {
    json(response, 200, {
      success: true,
      statusCode: 200,
      data: [],
      seed,
    });
    return;
  }

  unexpectedRequests.push({ method: request.method || 'UNKNOWN', path: url.pathname });
  json(response, 501, {
    error: 'unmocked_e2e_request',
    message: 'Every API request in the deterministic UI suite must be intercepted by Playwright.',
    method: request.method,
    path: url.pathname,
    seed,
  });
});

// HTTPS through an HTTP proxy arrives as CONNECT. Refuse the tunnel before any
// socket can reach the public network and retain evidence for global teardown.
server.on('connect', (request, socket) => {
  unexpectedRequests.push({ method: 'CONNECT', path: request.url || 'unknown' });
  socket.end('HTTP/1.1 502 Bad Gateway\r\nConnection: close\r\n\r\n');
});

server.listen(port, host, () => {
  process.stdout.write(`Isolated E2E API listening on http://${host}:${port} (seed=${seed})\n`);
});

function shutdown() {
  server.closeAllConnections?.();
  server.close();
  process.exit(0);
}

process.on('SIGINT', shutdown);
process.on('SIGTERM', shutdown);
