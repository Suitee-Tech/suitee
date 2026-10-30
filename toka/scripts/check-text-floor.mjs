import { readFileSync, readdirSync } from 'node:fs';
import path from 'node:path';

// P3-04: conteúdo operacional não pode descer a 8/9/10 px. O token
// `text-caption` (11 px) é o piso; `text-micro` continua disponível para
// metadados não operacionais, mas não para texto de leitura.
const MIN_ALLOWED = 11;
const violations = [];
const walk = (dir) => {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const file = path.join(dir, entry.name);
    if (entry.isDirectory()) { walk(file); continue; }
    if (!/\.(ts|tsx)$/.test(entry.name) || /\.(test|spec)\./.test(entry.name)) continue;
    const source = readFileSync(file, 'utf8');
    for (const match of source.matchAll(/text-\[(\d+)px\]/g)) {
      if (Number(match[1]) < MIN_ALLOWED) {
        const line = source.slice(0, match.index).split('\n').length;
        violations.push(`${path.relative(process.cwd(), file)}:${line}: ${match[0]}`);
      }
    }
  }
};
walk('src');
if (violations.length) {
  console.error(`Texto abaixo de ${MIN_ALLOWED}px:\n${violations.join('\n')}`);
  process.exitCode = 1;
} else {
  console.log(`Texto: nenhum tamanho arbitrário abaixo de ${MIN_ALLOWED}px.`);
}
