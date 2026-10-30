// Fonte única da Central de Ajuda: carrega toka/src/help/docs (mesmo conteúdo
// que o toca-mcp embute em /help-corpus) e expõe navegação, busca e resolução
// de imagens para a página /ajuda.

export interface HelpTopic {
  id: string;
  file: string;
  title: string;
  summary: string;
  tags: string[];
  roles: string[];
  /** Markdown do corpo, sem frontmatter, com imagens e links já resolvidos. */
  body: string;
  /** Mesmo conteúdo sem marcação: usado na busca e no preview dos resultados. */
  plainBody: string;
}

// Vite 8 (rolldown): `as: 'raw'` não acrescenta a query ?raw nos imports
// eager — os .md acabam parseados como JS. `query: '?raw'` é o caminho
// compatível desta versão.
const topicModules = import.meta.glob("./docs/topicos/*.md", {
  query: "?raw",
  import: "default",
  eager: true,
});
const imageModules = import.meta.glob("./docs/img/*.png", {
  import: "default",
  eager: true,
});
const manifestModule = import.meta.glob("./docs/manifest.yaml", {
  query: "?raw",
  import: "default",
  eager: true,
});
const indexModule = import.meta.glob("./docs/index.md", {
  query: "?raw",
  import: "default",
  eager: true,
});

function splitList(s: string): string[] {
  const inner = /\[(.*)\]/.exec(s.trim());
  if (inner) s = inner[1];
  return s
    .split(",")
    .map((p) => p.trim().replace(/^["']|["']$/g, ""))
    .filter(Boolean);
}

interface ManifestEntry {
  id: string;
  file: string;
  title: string;
  summary: string;
  tags: string[];
  roles: string[];
}

function parseManifest(raw: string): ManifestEntry[] {
  const entries: ManifestEntry[] = [];
  let cur: ManifestEntry | null = null;
  for (const line of raw.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("#") || trimmed.startsWith("topics:")) continue;
    const item = /^\s*-\s+([a-z_]+):\s*(.*)$/.exec(line);
    const kv = /^([a-z_]+):\s*(.*)$/.exec(trimmed);
    if (item) {
      const [, key, val] = item;
      if (key === "id") {
        cur = { id: val.trim(), file: "", title: "", summary: "", tags: [], roles: [] };
        entries.push(cur);
      } else if (cur) {
        applyKey(cur, key, val.trim());
      }
    } else if (kv && cur) {
      applyKey(cur, kv[1], kv[2].trim());
    }
  }
  return entries;
}

function applyKey(entry: ManifestEntry, key: string, val: string): void {
  switch (key) {
    case "file":
      entry.file = val;
      break;
    case "title":
      entry.title = val;
      break;
    case "summary":
      entry.summary = val;
      break;
    case "tags":
      entry.tags = splitList(val);
      break;
    case "roles":
      entry.roles = splitList(val);
      break;
  }
}

function stripFrontmatter(raw: string): string {
  const lines = raw.split("\n");
  if (lines[0]?.trim() !== "---") return raw;
  const end = lines.indexOf("---", 1);
  if (end < 0) return raw;
  return lines.slice(end + 1).join("\n");
}

/** Substitui referências a ../img/*.png (e img/*.png) pelas URLs empacotadas. */
export function resolveHelpImages(markdown: string): string {
  return markdown.replace(/\(([^)]*\.\.\/img\/[^)]+\.png|img\/[^)]+\.png)\)/g, (match, ref) => {
    const name = ref.replace(/^(?:\.\.\/)?img\//, "");
    const url = imageModules[`./docs/img/${name}`];
    return url ? `(${url})` : match;
  });
}

/**
 * Converte o markdown do tópico em texto puro: o preview da busca não deve
 * mostrar `##`, `**`, `|` de tabela, links crus nem blocos de código.
 */
export function markdownToPlainText(markdown: string): string {
  return markdown
    .replace(/```[\s\S]*?```/g, " ")
    .replace(/!\[[^\]]*\]\([^)]*\)/g, " ")
    .replace(/\[([^\]]+)\]\([^)]*\)/g, "$1")
    .replace(/^\s{0,3}#{1,6}\s+/gm, "")
    .replace(/^\s{0,3}>\s?/gm, "")
    .replace(/^\s*[-*+]\s+/gm, "")
    .replace(/^\s*\d+\.\s+/gm, "")
    .replace(/^\s*\|?[\s:|-]{3,}\|?\s*$/gm, " ")
    .replace(/\|/g, " ")
    .replace(/`([^`]+)`/g, "$1")
    .replace(/\*\*([^*]+)\*\*/g, "$1")
    .replace(/\*([^*]+)\*/g, "$1")
    .replace(/__([^_]+)__/g, "$1")
    .replace(/_([^_]+)_/g, "$1")
    .replace(/^\s*([-*_])\1{2,}\s*$/gm, " ")
    .replace(/\s+/g, " ")
    .trim();
}

/**
 * Reescreve links entre tópicos (`./03-leads.md` nos tópicos e
 * `topicos/03-leads.md` no índice) para a rota interna da Central de Ajuda
 * (`/ajuda?t=leads`). Sem isso o react-markdown emite `href` cru para o
 * arquivo .md, que o roteador do SPA não resolve e o link quebra.
 */
export function resolveHelpLinks(markdown: string, fileToId: Map<string, string>): string {
  return markdown.replace(/\]\(([^)\s]+?\.md)(#[^)\s]*)?\)/g, (match, target, hash = "") => {
    const candidates = [
      target,
      target.replace(/^\.\//, ""),
      target.replace(/^\.\//, "topicos/"),
      target.replace(/^topicos\//, ""),
      `topicos/${target.replace(/^\.\//, "")}`,
    ];
    const id = candidates.map((c) => fileToId.get(c)).find(Boolean);
    if (!id) return match;
    return `](/ajuda?t=${id}${hash})`;
  });
}

function buildTopics(): HelpTopic[] {
  const manifestRaw = Object.values(manifestModule)[0] ?? "";
  const entries = parseManifest(manifestRaw);
  // Mapa arquivo -> id do manifest: base para reescrever os links entre tópicos.
  const fileToId = new Map<string, string>();
  for (const entry of entries) {
    if (!entry.id || !entry.file) continue;
    const base = entry.file.replace(/^topicos\//, "");
    fileToId.set(entry.file, entry.id);
    fileToId.set(base, entry.id);
    fileToId.set(`./${base}`, entry.id);
  }
  const topics: HelpTopic[] = [];
  for (const entry of entries) {
    if (!entry.file) continue;
    const rel = `./docs/${entry.file}`;
    const raw = topicModules[rel];
    if (raw === undefined) {
      // Tópico no manifest sem arquivo: falha visível em dev, omitido em prod.
      if (import.meta.env.DEV) {
        console.error(`[help] tópico sem arquivo empacotado: ${entry.file}`);
      }
      continue;
    }
    const body = resolveHelpLinks(resolveHelpImages(stripFrontmatter(raw)), fileToId);
    topics.push({
      id: entry.id,
      file: entry.file,
      title: entry.title,
      summary: entry.summary,
      tags: entry.tags,
      roles: entry.roles,
      body,
      plainBody: markdownToPlainText(body),
    });
  }
  helpFileToId = fileToId;
  return topics;
}

// Preenchido por buildTopics; usado também para o índice.
let helpFileToId = new Map<string, string>();

export const helpTopics: HelpTopic[] = buildTopics();

export const helpIndexBody: string = resolveHelpLinks(
  resolveHelpImages(stripFrontmatter(Object.values(indexModule)[0] ?? "")),
  helpFileToId,
);

export function getHelpTopic(id: string): HelpTopic | undefined {
  return helpTopics.find((t) => t.id === id);
}

const STOPWORDS = new Set([
  "a", "o", "e", "é", "de", "da", "do", "em", "no", "na", "um", "uma", "para",
  "com", "como", "que", "por", "os", "as", "ao", "à", "se", "eu", "meu",
  "minha", "fazer", "faz", "mais", "sobre", "qual", "quais", "onde", "quando",
  "tenho", "quero",
]);

function tokenize(s: string): string[] {
  const fields = s
    .toLowerCase()
    .split(/[^a-z0-9\-]+/)
    .filter((f) => f.length >= 2 && !STOPWORDS.has(f));
  return [...new Set(fields)];
}

function countTokens(haystack: string, terms: string[]): number {
  const hay = ` ${haystack.toLowerCase()} `;
  let score = 0;
  for (const t of terms) score += hay.split(t).length - 1;
  return score;
}

export interface HelpSearchHit {
  topic: HelpTopic;
  score: number;
  excerpt: string;
}

function excerptFor(topic: HelpTopic, terms: string[]): string {
  // O preview usa o texto puro: nada de `##`, `**` ou links crus na busca.
  const body = topic.plainBody.toLowerCase();
  let best = -1;
  for (const term of terms) {
    const i = body.indexOf(term);
    if (i >= 0 && (best === -1 || i < best)) best = i;
  }
  if (best < 0) return topic.summary;
  const start = Math.max(0, best - 60);
  const end = Math.min(topic.plainBody.length, start + 220);
  let out = topic.plainBody.slice(start, end).replace(/\s+/g, " ").trim();
  if (start > 0) out = `…${out}`;
  if (end < topic.plainBody.length) out = `${out}…`;
  return out;
}

export function searchHelp(query: string, limit = 6): HelpSearchHit[] {
  const terms = tokenize(query);
  if (terms.length === 0) return [];
  const phrase = terms.join(" ");
  const hits: HelpSearchHit[] = [];
  for (const topic of helpTopics) {
    const score =
      countTokens(topic.title, terms) * 8 +
      countTokens(topic.tags.join(" "), terms) * 6 +
      countTokens(topic.summary, terms) * 3 +
      countTokens(topic.plainBody, terms);
    if (score <= 0) continue;
    if (` ${topic.title.toLowerCase()} `.includes(` ${phrase} `)) hits.push({ topic, score: score + 4, excerpt: excerptFor(topic, terms) });
    else hits.push({ topic, score, excerpt: excerptFor(topic, terms) });
  }
  hits.sort((a, b) => b.score - a.score || a.topic.id.localeCompare(b.topic.id));
  return hits.slice(0, limit);
}
