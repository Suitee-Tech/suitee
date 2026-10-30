import { spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const vite = fileURLToPath(new URL('../node_modules/vite/bin/vite.js', import.meta.url));
const appPort = process.env.E2E_APP_PORT || '4173';
let activeChild;

function start(args) {
  activeChild = spawn(process.execPath, [vite, ...args], {
    env: process.env,
    stdio: 'inherit',
  });
  return new Promise((resolve) => {
    activeChild.once('exit', (code, signal) => resolve({ code, signal }));
  });
}

for (const signal of ['SIGINT', 'SIGTERM']) {
  process.on(signal, () => activeChild?.kill(signal));
}

if (process.env.E2E_SKIP_BUILD !== '1') {
  const build = await start(['build', '--config', 'vite.e2e.config.ts']);
  if (build.code !== 0) process.exit(build.code || 1);
}

const preview = await start([
  'preview',
  '--config',
  'vite.e2e.config.ts',
  '--host',
  '127.0.0.1',
  '--port',
  appPort,
  '--strictPort',
]);
process.exitCode = preview.code || 0;
