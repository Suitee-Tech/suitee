#!/usr/bin/env node

import { existsSync, mkdirSync, readFileSync, readdirSync, statSync, writeFileSync } from "node:fs";
import { dirname, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const scriptDir = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(scriptDir, "../..");
const docsRoot = resolve(repoRoot, "docs");
const generatedPath = resolve(docsRoot, "architecture/generated/routes-events.md");
const linkBaselinePath = resolve(docsRoot, ".linkcheck-baseline.json");
const writeMode = process.argv.includes("--write");
const writeLinkBaseline = process.argv.includes("--write-link-baseline");

// Diretórios que nunca contêm o que este script indexa (Go/Markdown) e que
// podem estar em estados estranhos no checkout de quem desenvolve — um symlink
// quebrado dentro de node_modules derrubava o walk com ENOENT.
const SKIP_DIRS = new Set(["node_modules", ".next", ".git", "dist", "test-results", "playwright-report"]);

function walk(root, predicate = () => true) {
  const output = [];
  for (const name of readdirSync(root)) {
    if (SKIP_DIRS.has(name)) continue;
    const path = resolve(root, name);
    const stat = statSync(path);
    if (stat.isDirectory()) output.push(...walk(path, predicate));
    else if (predicate(path)) output.push(path);
  }
  return output;
}

const markdownFiles = [resolve(repoRoot, "README.md"), ...walk(docsRoot, (path) => path.endsWith(".md"))]
  .filter(existsSync)
  .sort();
const errors = [];
const missingLinks = [];
const linkBaseline = existsSync(linkBaselinePath)
  ? new Set(JSON.parse(readFileSync(linkBaselinePath, "utf8")))
  : new Set();

function cleanLinkTarget(raw) {
  let target = raw.trim().replace(/^<|>$/g, "");
  if (/^(https?:|mailto:|tel:|data:)/i.test(target) || target.startsWith("#")) return "";
  target = target.split("#", 1)[0].split("?", 1)[0];
  try { target = decodeURIComponent(target); } catch { /* report the original path below */ }
  return target.replace(/:(\d+)(?::\d+)?$/, "");
}

function linkExists(sourceFile, target) {
  if (!target) return true;
  if (target.startsWith("file://")) {
    try { return existsSync(fileURLToPath(target)); } catch { return false; }
  }
  const withoutLeadingSlash = target.replace(/^\//, "");
  const candidates = [
    resolve(dirname(sourceFile), target),
    resolve(repoRoot, withoutLeadingSlash),
    resolve(repoRoot, withoutLeadingSlash.replace(/^toca\//, "")),
  ];
  return candidates.some(existsSync);
}

for (const file of markdownFiles) {
  const content = readFileSync(file, "utf8");
  const rel = relative(repoRoot, file);
  for (const match of content.matchAll(/!?\[[^\]]*\]\(([^)]+)\)/g)) {
    const target = cleanLinkTarget(match[1]);
    if (target && !linkExists(file, target)) {
      const line = content.slice(0, match.index).split("\n").length;
      const id = `${rel}::${target}`;
      missingLinks.push(id);
      if (!linkBaseline.has(id) && !writeLinkBaseline) {
        errors.push(`${rel}:${line}: link local inexistente novo: ${target}`);
      }
    }
  }

  const fences = [...content.matchAll(/^```mermaid\s*\n([\s\S]*?)^```\s*$/gm)];
  const opens = (content.match(/^```mermaid\s*$/gm) || []).length;
  if (opens !== fences.length) errors.push(`${rel}: bloco Mermaid sem fechamento`);
  for (const fence of fences) {
    const first = fence[1].split("\n").map((line) => line.trim()).find(Boolean) || "";
    if (!/^(flowchart|graph|sequenceDiagram|classDiagram|stateDiagram(?:-v2)?|erDiagram|journey|gantt|pie|mindmap|timeline|gitGraph|quadrantChart|xychart-beta|block-beta|architecture-beta)\b/.test(first)) {
      const line = content.slice(0, fence.index).split("\n").length;
      errors.push(`${rel}:${line}: tipo Mermaid ausente ou não reconhecido: ${first || "<vazio>"}`);
    }
  }
}

const codeRoots = readdirSync(repoRoot)
  .filter((name) => name === "gateway" || name === "pkg" || name.startsWith("toca-"))
  .map((name) => resolve(repoRoot, name))
  .filter(existsSync);
const goFiles = codeRoots.flatMap((root) => walk(root, (path) => path.endsWith(".go") && !path.endsWith("_test.go"))).sort();
const routes = [];
const events = [];

for (const file of goFiles) {
  const content = readFileSync(file, "utf8");
  const rel = relative(repoRoot, file);
  for (const match of content.matchAll(/\.(GET|POST|PUT|PATCH|DELETE|OPTIONS|HEAD|Any)\(\s*"([^"]+)"/g)) {
    routes.push({ method: match[1].toUpperCase(), path: match[2], file: rel, line: content.slice(0, match.index).split("\n").length });
  }
  for (const match of content.matchAll(/func\s*\([^)]*\)\s*Name\(\)\s*string\s*\{\s*return\s*"([a-z][a-z0-9_-]*(?:\.[a-z0-9_-]+)+)"/gs)) {
    events.push({ name: match[1], kind: "publisher", file: rel, line: content.slice(0, match.index).split("\n").length });
  }
  for (const match of content.matchAll(/SubscribeWithQueue\(\s*"[^"]+"\s*,\s*"([^"]+)"/g)) {
    events.push({ name: match[1], kind: "subscriber", file: rel, line: content.slice(0, match.index).split("\n").length });
  }
  for (const match of content.matchAll(/\.Subscribe\(\s*"([^"]+)"/g)) {
    events.push({ name: match[1], kind: "subscriber", file: rel, line: content.slice(0, match.index).split("\n").length });
  }
}

routes.sort((a, b) => a.path.localeCompare(b.path) || a.method.localeCompare(b.method) || a.file.localeCompare(b.file));
events.sort((a, b) => a.name.localeCompare(b.name) || a.kind.localeCompare(b.kind) || a.file.localeCompare(b.file));
const generated = [
  "# Inventário gerado de rotas e eventos",
  "",
  "> Gerado por `node scripts/docs/check-docs.mjs --write`. Não editar manualmente.",
  "> Os paths são os literais registrados no arquivo; prefixos de grupos Gin devem ser lidos no router indicado.",
  "",
  `- Rotas declaradas: **${routes.length}**`,
  `- Publishers/subscribers declarados: **${events.length}**`,
  "",
  "## Rotas HTTP",
  "",
  "| Método | Path literal | Fonte |",
  "|---|---|---|",
  ...routes.map((route) => `| ${route.method} | \`${route.path.replaceAll("|", "\\|")}\` | [${route.file}:${route.line}](../../../${route.file}#L${route.line}) |`),
  "",
  "## Eventos",
  "",
  "| Evento | Papel | Fonte |",
  "|---|---|---|",
  ...events.map((event) => `| \`${event.name}\` | ${event.kind} | [${event.file}:${event.line}](../../../${event.file}#L${event.line}) |`),
  "",
].join("\n");

if (writeMode) {
  mkdirSync(dirname(generatedPath), { recursive: true });
  writeFileSync(generatedPath, generated);
} else if (!existsSync(generatedPath) || readFileSync(generatedPath, "utf8") !== generated) {
  errors.push(`${relative(repoRoot, generatedPath)}: inventário desatualizado; execute node scripts/docs/check-docs.mjs --write`);
}

if (writeLinkBaseline) {
  writeFileSync(linkBaselinePath, `${JSON.stringify([...new Set(missingLinks)].sort(), null, 2)}\n`);
}

if (errors.length > 0) {
  console.error(errors.join("\n"));
  process.exit(1);
}
console.log(`Documentação validada: ${markdownFiles.length} Markdown, ${routes.length} rotas, ${events.length} vínculos de eventos, ${missingLinks.length} links legados no baseline.`);
