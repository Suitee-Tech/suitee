import type { Finding } from "../../services/inspection.service";

export type DiffStatus =
  | "worsened"
  | "unchanged"
  | "improved"
  | "missing_in_exit"
  | "added_in_exit";

export interface DiffPair {
  key: string;
  room: string;
  description: string;
  entry?: Finding;
  exit?: Finding;
  status: DiffStatus;
}

const CONDITION_RANK: Record<string, number> = {
  "Ótimo": 5,
  "Bom": 4,
  "Regular": 3,
  "Ruim": 2,
  "Danificado": 1,
  "": 0,
};

export function rankOf(condition: string | undefined): number {
  if (!condition) return 0;
  return CONDITION_RANK[condition] ?? 0;
}

export function diffInspections(entryFindings: Finding[], exitFindings: Finding[]): DiffPair[] {
  const entryByKey = new Map<string, { f: Finding; matched: boolean }>();
  const entryOrderByRoom = new Map<string, string[]>();

  for (const f of entryFindings) {
    const key = makeKey(f);
    if (!entryByKey.has(key)) {
      entryByKey.set(key, { f, matched: false });
      const arr = entryOrderByRoom.get(normRoom(f.room)) || [];
      arr.push(key);
      entryOrderByRoom.set(normRoom(f.room), arr);
    }
  }

  const pairs: DiffPair[] = [];
  const exitOnlyByRoom = new Map<string, DiffPair[]>();

  for (const ef of exitFindings) {
    const key = makeKey(ef);
    const found = entryByKey.get(key);
    if (found) {
      found.matched = true;
      pairs.push({
        key,
        room: found.f.room || ef.room,
        description: found.f.description || ef.description,
        entry: found.f,
        exit: ef,
        status: classify(found.f, ef),
      });
    } else {
      const room = normRoom(ef.room);
      const p: DiffPair = {
        key,
        room: ef.room,
        description: ef.description,
        exit: ef,
        status: "added_in_exit",
      };
      const arr = exitOnlyByRoom.get(room) || [];
      arr.push(p);
      exitOnlyByRoom.set(room, arr);
    }
  }

  for (const [key, { f, matched }] of entryByKey) {
    if (matched) continue;
    pairs.push({
      key,
      room: f.room,
      description: f.description,
      entry: f,
      status: "missing_in_exit",
    });
  }

  for (const arr of exitOnlyByRoom.values()) {
    pairs.push(...arr);
  }

  pairs.sort((a, b) => {
    const cmp = normRoom(a.room).localeCompare(normRoom(b.room), "pt-BR");
    if (cmp !== 0) return cmp;
    const ai = indexInEntry(entryOrderByRoom, a);
    const bi = indexInEntry(entryOrderByRoom, b);
    return ai - bi;
  });

  return pairs;
}

function indexInEntry(orderByRoom: Map<string, string[]>, pair: DiffPair): number {
  const arr = orderByRoom.get(normRoom(pair.room));
  if (!arr) return Number.MAX_SAFE_INTEGER;
  const idx = arr.indexOf(pair.key);
  return idx >= 0 ? idx : Number.MAX_SAFE_INTEGER;
}

function makeKey(f: Finding): string {
  return `${normRoom(f.room)}::${(f.description || "").trim().toLowerCase()}`;
}

function normRoom(room: string | undefined): string {
  return (room || "").trim().toLowerCase();
}

function classify(entry: Finding, exit: Finding): DiffStatus {
  const a = rankOf(entry.condition);
  const b = rankOf(exit.condition);
  if (a > b) return "worsened";
  if (a < b) return "improved";
  return "unchanged";
}

export interface DiffSummary {
  worsened: number;
  unchanged: number;
  improved: number;
  missingInExit: number;
  addedInExit: number;
  total: number;
}

export function summarize(pairs: DiffPair[]): DiffSummary {
  const s: DiffSummary = {
    worsened: 0,
    unchanged: 0,
    improved: 0,
    missingInExit: 0,
    addedInExit: 0,
    total: pairs.length,
  };
  for (const p of pairs) {
    switch (p.status) {
      case "worsened":
        s.worsened++;
        break;
      case "unchanged":
        s.unchanged++;
        break;
      case "improved":
        s.improved++;
        break;
      case "missing_in_exit":
        s.missingInExit++;
        break;
      case "added_in_exit":
        s.addedInExit++;
        break;
    }
  }
  return s;
}

export function groupByRoom(pairs: DiffPair[]): Array<{ room: string; pairs: DiffPair[] }> {
  const groups: Array<{ room: string; pairs: DiffPair[] }> = [];
  const byRoom = new Map<string, number>();
  for (const p of pairs) {
    const r = (p.room || "").trim() || "(sem cômodo)";
    if (!byRoom.has(r)) {
      byRoom.set(r, groups.length);
      groups.push({ room: r, pairs: [] });
    }
    groups[byRoom.get(r)!].pairs.push(p);
  }
  return groups;
}
