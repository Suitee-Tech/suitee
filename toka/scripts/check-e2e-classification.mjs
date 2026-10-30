import { readFileSync, readdirSync } from 'node:fs';

const suites = JSON.parse(
  readFileSync(new URL('../e2e/suites.json', import.meta.url), 'utf8'),
);
const actual = readdirSync(new URL('../e2e', import.meta.url))
  .filter((file) => file.endsWith('.spec.ts'))
  .sort();
const mockedUi = [...suites.ciCritical, ...suites.mockedUiDeferred];
const classified = [...mockedUi, ...suites.fullStack];
const duplicates = classified.filter((file, index) => classified.indexOf(file) !== index);
const missing = actual.filter((file) => !classified.includes(file));
const stale = classified.filter((file) => !actual.includes(file));

if (duplicates.length || missing.length || stale.length) {
  const failures = [
    duplicates.length ? `duplicated: ${[...new Set(duplicates)].join(', ')}` : '',
    missing.length ? `unclassified: ${missing.join(', ')}` : '',
    stale.length ? `missing files: ${stale.join(', ')}` : '',
  ].filter(Boolean);
  throw new Error(`Invalid E2E suite classification (${failures.join('; ')})`);
}

for (const file of mockedUi) {
  const source = readFileSync(new URL(`../e2e/${file}`, import.meta.url), 'utf8');
  if (!source.includes('page.route(')) {
    throw new Error(`${file} is in deterministicUi but does not intercept browser API calls`);
  }
}

process.stdout.write(
  `E2E classification OK: ${suites.ciCritical.length} CI-critical specs, ` +
    `${suites.mockedUiDeferred.length} deferred mocked UI specs, ` +
    `${suites.fullStack.length} full-stack specs\n`,
);
