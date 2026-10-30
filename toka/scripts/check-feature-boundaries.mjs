import { readdir, readFile } from "node:fs/promises";
import path from "node:path";
import { findBoundaryViolations } from "./import-boundaries.mjs";

const ROOT = path.resolve(process.cwd());
const FEATURES_DIR = path.join(ROOT, "src", "features");
const CONFIG_PATH = path.join(ROOT, "scripts", "feature-boundaries.json");

async function readConfig() {
  try {
    const raw = await readFile(CONFIG_PATH, "utf8");
    const parsed = JSON.parse(raw);
    const enforcedPaths = Array.isArray(parsed?.enforced_paths) ? parsed.enforced_paths.filter(Boolean) : [];
    const enforced = Array.isArray(parsed?.enforced_features) ? parsed.enforced_features.filter(Boolean) : [];
    const forbidden = Array.isArray(parsed?.forbidden_roots) ? parsed.forbidden_roots.filter(Boolean) : [];
    return {
      enforced_paths: enforcedPaths,
      enforced_features: enforced.length > 0 ? enforced : ["esteira"],
      forbidden_roots: forbidden.length > 0 ? forbidden : ["app"],
    };
  } catch {
    return { enforced_paths: [], enforced_features: ["esteira"], forbidden_roots: ["app"] };
  }
}

async function listFeatureDirs() {
  const entries = await readdir(FEATURES_DIR, { withFileTypes: true });
  return entries
    .filter((e) => e.isDirectory())
    .map((e) => e.name)
    .sort((a, b) => a.localeCompare(b));
}

async function listFiles(dir) {
  const entries = await readdir(dir, { withFileTypes: true });
  const out = [];
  for (const entry of entries) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      out.push(...(await listFiles(full)));
      continue;
    }
    if (/\.(ts|tsx|js|jsx)$/.test(entry.name)) out.push(full);
  }
  return out;
}

const config = await readConfig();
const forbiddenRoots = config.forbidden_roots;

const args = new Set(process.argv.slice(2));
const enforcedFeatures = args.has("--all") ? await listFeatureDirs() : config.enforced_features;
const enforcedPaths = (() => {
  if (args.has("--all")) {
    return ["src/shared", ...enforcedFeatures.map((f) => path.join("src", "features", f))];
  }
  if (Array.isArray(config.enforced_paths) && config.enforced_paths.length > 0) {
    return config.enforced_paths;
  }
  return enforcedFeatures.map((f) => path.join("src", "features", f));
})();

const failures = [];

for (const relPath of enforcedPaths) {
  const targetDir = path.join(ROOT, relPath);
  const files = await listFiles(targetDir);
  for (const file of files) {
    const content = await readFile(file, "utf8");
    const violations = findBoundaryViolations(content, file, ROOT, forbiddenRoots);
    for (const v of violations) {
      failures.push({ file, ...v });
    }
  }
}

if (failures.length > 0) {
  const lines = failures
    .map((f) => `${path.relative(ROOT, f.file)}:${f.line} -> ${f.spec}`)
    .join("\n");
  process.stderr.write(`Feature boundary violation(s):\n${lines}\n`);
  process.exit(1);
}

process.stdout.write("Feature boundary check passed.\n");
