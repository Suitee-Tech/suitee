import type { Agent } from "../types/agent";

const TTL_MS = 24 * 60 * 60 * 1000;
const KEY_PREFIX = "crm:agents:";

interface CacheEntry {
  agents: Agent[];
  storedAt: number;
}

function key(companyId: string): string {
  return `${KEY_PREFIX}${companyId}`;
}

function safeStorage(): Storage | null {
  try {
    if (typeof window === "undefined") return null;
    return window.sessionStorage;
  } catch {
    return null;
  }
}

export function readAgentsCache(companyId: string): Agent[] | null {
  if (!companyId) return null;
  const storage = safeStorage();
  if (!storage) return null;
  try {
    const raw = storage.getItem(key(companyId));
    if (!raw) return null;
    const entry = JSON.parse(raw) as CacheEntry;
    if (!entry || !Array.isArray(entry.agents)) return null;
    if (Date.now() - entry.storedAt > TTL_MS) return null;
    return entry.agents;
  } catch {
    return null;
  }
}

export function writeAgentsCache(companyId: string, agents: Agent[]): void {
  if (!companyId) return;
  const storage = safeStorage();
  if (!storage) return;
  try {
    const entry: CacheEntry = { agents, storedAt: Date.now() };
    storage.setItem(key(companyId), JSON.stringify(entry));
  } catch {
  }
}

export function clearAgentsCache(companyId?: string): void {
  const storage = safeStorage();
  if (!storage) return;
  try {
    if (companyId) {
      storage.removeItem(key(companyId));
      return;
    }
    const toRemove: string[] = [];
    for (let i = 0; i < storage.length; i++) {
      const k = storage.key(i);
      if (k && k.startsWith(KEY_PREFIX)) toRemove.push(k);
    }
    for (const k of toRemove) storage.removeItem(k);
  } catch {
  }
}
