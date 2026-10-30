#!/usr/bin/env node
// check-help-center.mjs — valida a Central de Ajuda (toka/src/help/docs):
// sincronia manifest × tópicos, frontmatter obrigatório, links internos entre
// tópicos e referências de imagens. A mesma pasta é empacotada pelo frontend
// (página /ajuda) e pelo toca-mcp (/help-corpus), então este checker protege a
// fonte única.

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const docsDir = path.join(root, "toka", "src", "help", "docs");
const topicsDir = path.join(docsDir, "topicos");
const imgDir = path.join(docsDir, "img");

const errors = [];
const warn = (msg) => console.warn(`[help-center] aviso: ${msg}`);
const fail = (msg) => errors.push(msg);

function parseManifest(raw) {
  const entries = [];
  let cur = null;
  for (const line of raw.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("#") || trimmed.startsWith("topics:")) continue;
    const item = /^\s*-\s+([a-z_]+):\s*(.*)$/.exec(line);
    const kv = /^([a-z_]+):\s*(.*)$/.exec(trimmed);
    const apply = (key, val) => {
      if (!cur) return;
      if (key === "file") cur.file = val;
      else if (key === "title") cur.title = val;
      else if (key === "summary") cur.summary = val;
      else if (key === "tags") cur.tags = val.replace(/[[\]]/g, "").split(",").map((s) => s.trim()).filter(Boolean);
      else if (key === "roles") cur.roles = val.replace(/[[\]]/g, "").split(",").map((s) => s.trim()).filter(Boolean);
    };
    if (item) {
      if (item[1] === "id") {
        cur = { id: item[2].trim(), file: "", title: "", summary: "", tags: [], roles: [] };
        entries.push(cur);
      } else apply(item[1], item[2].trim());
    } else if (kv) apply(kv[1], kv[2].trim());
  }
  return entries;
}

function stripFrontmatter(raw) {
  const lines = raw.split("\n");
  if (lines[0]?.trim() !== "---") return { meta: {}, body: raw };
  const end = lines.indexOf("---", 1);
  if (end < 0) return { meta: {}, body: raw };
  const meta = {};
  for (const line of lines.slice(1, end)) {
    const m = /^([a-z_]+):\s*(.*)$/.exec(line);
    if (m) meta[m[1]] = m[2].trim();
  }
  return { meta, body: lines.slice(end + 1).join("\n") };
}

const manifestRaw = fs.readFileSync(path.join(docsDir, "manifest.yaml"), "utf8");
const entries = parseManifest(manifestRaw);
if (entries.length === 0) fail("manifest.yaml sem tópicos");

const topicFiles = fs.existsSync(topicsDir)
  ? fs.readdirSync(topicsDir).filter((f) => f.endsWith(".md")).sort()
  : [];

const manifestFiles = new Set();
const ids = new Set();
// Primeira passada: valida o manifest e monta o conjunto de arquivos ANTES de
// checar links — links "para frente" (tópico 15 -> 16) precisam do mapa completo.
for (const e of entries) {
  if (!e.id) { fail("tópico sem id no manifest"); continue; }
  if (ids.has(e.id)) fail(`id duplicado no manifest: ${e.id}`);
  ids.add(e.id);
  if (!e.file) { fail(`tópico ${e.id} sem file no manifest`); continue; }
  manifestFiles.add(e.file);
  if (!e.title) fail(`tópico ${e.id} sem title no manifest`);
  if (!e.summary) fail(`tópico ${e.id} sem summary no manifest`);
  if (e.tags.length === 0) fail(`tópico ${e.id} sem tags no manifest`);
  if (e.roles.length === 0) fail(`tópico ${e.id} sem roles no manifest`);
}

for (const e of entries) {
  if (!e.file) continue;
  const full = path.join(docsDir, e.file);
  if (!fs.existsSync(full)) {
    fail(`tópico no manifest sem arquivo: ${e.file}`);
    continue;
  }
  const { meta, body } = stripFrontmatter(fs.readFileSync(full, "utf8"));
  for (const key of ["title", "summary", "tags", "roles"]) {
    if (!meta[key]) fail(`${e.file}: frontmatter sem ${key}`);
  }
  if (!body.trim()) fail(`${e.file}: corpo vazio`);

  // links internos entre tópicos: ](./NN-slug.md) e ](topicos/NN-slug.md).
  // Além de existir, o alvo precisa estar no manifest — é o mapa que o
  // frontend usa para reescrever o link para /ajuda?t=<id>.
  for (const m of body.matchAll(/\]\(([^)\s]+\.md)(?:#[^)]*)?\)/g)) {
    const target = m[1];
    if (/^(https?:)?\/\//.test(target)) continue;
    const rel = target.startsWith("topicos/") ? target : `topicos/${target.replace(/^\.\//, "")}`;
    const resolved = path.join(docsDir, rel);
    if (!fs.existsSync(resolved)) {
      fail(`${e.file}: link local inexistente: ${target}`);
      continue;
    }
    if (!manifestFiles.has(rel)) {
      fail(`${e.file}: link para tópico fora do manifest (não navegável em /ajuda): ${target}`);
    }
  }
  // imagens: ](../img/X.png) a partir de topicos/ e ](img/X.png) a partir do index
  for (const m of body.matchAll(/\]\(((?:\.\.\/)?img\/[^)]+\.png)\)/g)) {
    const name = m[1].replace(/^(?:\.\.\/)?img\//, "");
    if (!fs.existsSync(path.join(imgDir, name))) {
      fail(`${e.file}: imagem inexistente: img/${name}`);
    }
  }
}

// arquivos órfãos (não listados no manifest)
for (const f of topicFiles) {
  const rel = `topicos/${f}`;
  if (!manifestFiles.has(rel)) fail(`tópico sem entrada no manifest: ${rel}`);
}

// imagens órfãs (sem referência em index.md nem nos tópicos)
const usedImages = new Set();
const scanImages = (raw) => {
  for (const m of raw.matchAll(/\]\(((?:\.\.\/)?img\/[^)]+\.png)\)/g)) {
    usedImages.add(m[1].replace(/^(?:\.\.\/)?img\//, ""));
  }
};
if (fs.existsSync(path.join(docsDir, "index.md"))) {
  const indexRaw = fs.readFileSync(path.join(docsDir, "index.md"), "utf8");
  scanImages(indexRaw);
  // Links do índice (topicos/NN-slug.md) também precisam existir e estar no manifest.
  for (const m of indexRaw.matchAll(/\]\(([^)\s]+\.md)(?:#[^)]*)?\)/g)) {
    const target = m[1];
    if (/^(https?:)?\/\//.test(target)) continue;
    const rel = target.startsWith("topicos/") ? target : `topicos/${target.replace(/^\.\//, "")}`;
    if (!fs.existsSync(path.join(docsDir, rel))) {
      fail(`index.md: link local inexistente: ${target}`);
    } else if (!manifestFiles.has(rel)) {
      fail(`index.md: link para tópico fora do manifest: ${target}`);
    }
  }
} else {
  fail("index.md ausente");
}
for (const f of topicFiles) {
  scanImages(fs.readFileSync(path.join(topicsDir, f), "utf8"));
}
if (fs.existsSync(imgDir)) {
  for (const img of fs.readdirSync(imgDir).filter((f) => f.endsWith(".png"))) {
    if (!usedImages.has(img)) warn(`imagem sem referência em nenhum tópico: img/${img}`);
  }
} else {
  warn("pasta img/ ainda não existe (prints pendentes)");
}

// o MCP e o frontend devem enxergar exatamente o mesmo corpus: confere se o
// Dockerfile do toca-mcp copia a pasta canônica.
const mcpDockerfile = path.join(root, "toca-mcp", "Dockerfile");
if (fs.existsSync(mcpDockerfile)) {
  const df = fs.readFileSync(mcpDockerfile, "utf8");
  if (!df.includes("COPY toka/src/help/docs/ /help-corpus/")) {
    fail("toca-mcp/Dockerfile não copia toka/src/help/docs/ para /help-corpus/");
  }
} else {
  fail("toca-mcp/Dockerfile ausente");
}

if (errors.length > 0) {
  console.error(`[help-center] ${errors.length} erro(s):`);
  for (const e of errors) console.error(`  - ${e}`);
  process.exit(1);
}
console.log(
  `[help-center] OK: ${entries.length} tópicos, ${topicFiles.length} arquivos, ${usedImages.size} imagens referenciadas.`,
);
