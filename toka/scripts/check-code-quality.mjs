import { ESLint } from 'eslint';
import { readFileSync, writeFileSync } from 'node:fs';
import path from 'node:path';
const baselinePath = new URL('./code-quality-baseline.json', import.meta.url);
const eslint = new ESLint();
const results = await eslint.lintFiles(['src']);
const counts = {};
for (const result of results) for (const message of result.messages) {
  if (message.fatal || message.ruleId === 'react-hooks/rules-of-hooks') throw new Error(`${result.filePath}:${message.line} ${message.message}`);
  const key = `${path.relative(process.cwd(), result.filePath)} :: ${message.ruleId}`;
  counts[key] = (counts[key] || 0) + 1;
}
if (process.argv.includes('--write-baseline')) {
  writeFileSync(baselinePath, JSON.stringify(Object.fromEntries(Object.entries(counts).sort()), null, 2) + '\n');
  console.log('Baseline registrada. Revise o diff antes de aceitar novos débitos.');
} else {
  const baseline = JSON.parse(readFileSync(baselinePath, 'utf8'));
  const regressions = Object.entries(counts).filter(([key, count]) => count > (baseline[key] || 0));
  if (regressions.length) {
    for (const [key, count] of regressions) console.error(`${key}: ${count} (limite ${baseline[key] || 0})`);
    process.exitCode = 1;
  } else console.log(`Qualidade aprovada sem aumento de violações (${Object.values(counts).reduce((a,b) => a+b, 0)} ocorrências legadas).`);
}
