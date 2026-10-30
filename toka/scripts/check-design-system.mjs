import { readdirSync, readFileSync, writeFileSync } from 'node:fs';
import path from 'node:path';
const root = path.resolve('src');
const baselinePath = new URL('./design-system-baseline.json', import.meta.url);
const counts = {};
const patterns = {
  'raw-color': /\b(?:bg|text|border|ring|fill|stroke|from|via|to)-(?:slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose)-\d{2,3}\b/g,
  'arbitrary-font-size': /\btext-\[\d+(?:\.\d+)?px\]/g,
  // Pega `confirm(` e `window.confirm(`, mas não `confirmAction(`/`onConfirm(`.
  'native-confirm': /(?<![\w])confirm\s*\(/g,
};
// Fluxo que coleta comprovante é formulário: roda no painel lateral padrão
// (FinancialActionPanel). AlertDialog fica reservado à confirmação de ação
// crítica, e por isso não pode dividir arquivo com upload de comprovante —
// ConfirmDialog (confirmação crítica sem dado) continua permitido.
const proofFlow = /\b(?:useProofUpload|ProofAttachmentFields)\b/;
const alertDialogPrimitive = /from\s+['"][^'"]*ui\/alert-dialog['"]/g;
function visit(dir) {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const file = path.join(dir, entry.name);
    if (entry.isDirectory()) { if (file !== path.join(root, 'app')) visit(file); continue; }
    if (!/\.(ts|tsx)$/.test(file) || /\.(test|spec)\./.test(file)) continue;
    const source = readFileSync(file, 'utf8');
    for (const [rule, pattern] of Object.entries(patterns)) {
      const count = [...source.matchAll(pattern)].length;
      if (count) counts[`${path.relative(process.cwd(), file)} :: ${rule}`] = count;
    }
    const alertCount = [...source.matchAll(alertDialogPrimitive)].length;
    if (alertCount && proofFlow.test(source)) {
      counts[`${path.relative(process.cwd(), file)} :: proof-flow-in-alert-dialog`] = alertCount;
    }
  }
}
visit(root);
if (process.argv.includes('--write-baseline')) {
  if (Object.keys(counts).some(key => key.endsWith('native-confirm'))) throw new Error('Confirmações nativas não podem ser aceitas na baseline.');
  if (Object.keys(counts).some(key => key.endsWith('proof-flow-in-alert-dialog'))) throw new Error('Fluxo de comprovante não pode usar AlertDialog; use o painel lateral padrão.');
  writeFileSync(baselinePath, JSON.stringify(Object.fromEntries(Object.entries(counts).sort()), null, 2) + '\n');
  console.log('Inventário do design system atualizado. Revise o diff.');
} else {
  const baseline = JSON.parse(readFileSync(baselinePath, 'utf8'));
  const regressions = Object.entries(counts).filter(([key, count]) => count > (baseline[key] || 0));
  for (const [key, count] of regressions) console.error(`${key}: ${count} (limite ${baseline[key] || 0})`);
  process.exitCode = regressions.length ? 1 : 0;
  if (!regressions.length) console.log('Design system: nenhuma regressão de cores, tipografia ou confirmações.');
}
