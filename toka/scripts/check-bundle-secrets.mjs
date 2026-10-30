// check-bundle-secrets varre o bundle de produção procurando credencial.
//
// É a última linha de defesa do env.js: o entrypoint só publica um allowlist
// público e o check-frontend-env trava o allowlist/compose/source, mas nada
// disso vê o artefato final. Se um valor de build (build arg, `.env` local
// copiado por engano, import novo do supabase-js) entrar no bundle, este check
// falha o `build:ci` — antes do push da imagem.
//
// Padrões detectados:
//   - JWT (`eyJ...`): anon key, access token, qualquer token do Supabase;
//   - `service_role` / `SUPABASE_SERVICE_ROLE_KEY`: a credencial que NUNCA pode
//     sair do servidor;
//   - `SUPABASE_ANON_KEY`: se voltar ao bundle, o navegador voltou a precisar
//     de uma chave pública (ver docs/deployment/hostinger.md § env.js);
//   - chaves de API por prefixo: `sb_secret_` (Supabase), `sk_live_`/`sk_test_`
//     (gateways de pagamento).
//
// Uso: node scripts/check-bundle-secrets.mjs   (dist/ já construído)
// Exit codes: 0 ok, 1 achou credencial, 2 uso/IO.

import { readdirSync, readFileSync, statSync } from "node:fs";
import { resolve, join, relative } from "node:path";

const distDir = resolve(process.cwd(), process.env.BUNDLE_DIST_DIR || "dist");

/** Padrões que nunca podem aparecer em artefato servido ao navegador. */
const SECRET_PATTERNS = [
  { name: "JWT", re: /eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}/ },
  { name: "service_role", re: /service_role/ },
  { name: "SUPABASE_SERVICE_ROLE_KEY", re: /SUPABASE_SERVICE_ROLE_KEY/ },
  { name: "SUPABASE_ANON_KEY", re: /SUPABASE_ANON_KEY/ },
  { name: "chave secreta Supabase (sb_secret_)", re: /sb_secret_[A-Za-z0-9_-]{8,}/ },
  { name: "chave secreta de pagamento (sk_live_/sk_test_)", re: /sk_(?:live|test)_[A-Za-z0-9]{8,}/ },
];

export function findSecretsInText(text) {
  return SECRET_PATTERNS.filter(({ re }) => re.test(text)).map(({ name }) => name);
}

function walk(dir, files = []) {
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) walk(full, files);
    else files.push(full);
  }
  return files;
}

function main() {
  let files;
  try {
    files = walk(distDir);
  } catch (error) {
    console.error(`check-bundle-secrets: dist não encontrado em ${distDir} (construa antes): ${error.message}`);
    process.exit(2);
  }

  const extensions = [".js", ".mjs", ".css", ".html", ".json", ".map", ".txt", ".webmanifest"];
  const findings = [];
  for (const file of files) {
    if (!extensions.some((ext) => file.endsWith(ext))) continue;
    const content = readFileSync(file, "utf8");
    for (const secret of findSecretsInText(content)) {
      findings.push(`${relative(distDir, file)}: ${secret}`);
    }
  }

  if (findings.length > 0) {
    console.error("check-bundle-secrets: credencial no bundle");
    for (const finding of findings) console.error("  -", finding);
    process.exit(1);
  }
  console.log(`check-bundle-secrets: ok (${files.length} arquivos sem credencial)`);
}

if (process.argv[1] && process.argv[1].endsWith("check-bundle-secrets.mjs")) {
  main();
}
