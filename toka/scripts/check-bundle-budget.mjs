import { gzipSync } from "node:zlib";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

const distDir = resolve(process.cwd(), process.env.BUNDLE_DIST_DIR || "dist");
const manifestPath = resolve(distDir, ".vite/manifest.json");
const totalBudgetKb = Number(process.env.INITIAL_TOTAL_BUDGET_KB || 400);
if (!Number.isFinite(totalBudgetKb) || totalBudgetKb <= 0) throw new Error("INITIAL_TOTAL_BUDGET_KB inválido");
const budgetKb = Number(process.env.INITIAL_CHUNK_BUDGET_KB || 500);

if (!Number.isFinite(budgetKb) || budgetKb <= 0) {
  throw new Error(`INITIAL_CHUNK_BUDGET_KB inválido: ${process.env.INITIAL_CHUNK_BUDGET_KB}`);
}

const manifest = JSON.parse(readFileSync(manifestPath, "utf8"));
const entryKeys = Object.entries(manifest)
  .filter(([, value]) => value.isEntry)
  .map(([key]) => key);

if (entryKeys.length === 0) {
  throw new Error(`Nenhum entry chunk encontrado em ${manifestPath}`);
}

const initialKeys = new Set();
const visit = (key) => {
  if (initialKeys.has(key)) return;
  const item = manifest[key];
  if (!item) throw new Error(`Import ausente no manifest: ${key}`);
  initialKeys.add(key);
  for (const imported of item.imports || []) visit(imported);
};
for (const key of entryKeys) visit(key);

const rows = [...initialKeys]
  .map((key) => {
    const file = manifest[key].file;
    const bytes = readFileSync(resolve(distDir, file));
    return { file, gzipKb: gzipSync(bytes).byteLength / 1024 };
  })
  .sort((a, b) => b.gzipKb - a.gzipKb);

for (const row of rows) {
  console.log(`${row.gzipKb.toFixed(1).padStart(7)} kB gzip  ${row.file}`);
}

const totalKb = rows.reduce((sum, row) => sum + row.gzipKb, 0);
console.log(`Total inicial: ${totalKb.toFixed(1)} KiB gzip (limite ${totalBudgetKb} KiB).`);
if (totalKb > totalBudgetKb) {
  console.error("A soma dos chunks iniciais excede o orçamento.");
  process.exit(1);
}

const violations = rows.filter((row) => row.gzipKb > budgetKb);
if (violations.length > 0) {
  console.error(`\nBundle inicial excede o orçamento de ${budgetKb} kB gzip:`);
  for (const row of violations) console.error(`- ${row.file}: ${row.gzipKb.toFixed(1)} kB`);
  process.exit(1);
}

console.log(`\nBundle inicial aprovado: ${rows.length} chunks, limite ${budgetKb} kB gzip por chunk.`);
