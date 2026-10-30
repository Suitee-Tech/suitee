import type { LucideIcon } from "lucide-react";
import { Ban, BadgeCheck, Clock, FileSignature, FileText, FolderOpen, Files, Hourglass } from "lucide-react";
import { t } from "../lib/i18n";
import { getCategoryMeta } from "../lib/documents";
import type { ContractDocument, DocumentCategory } from "../types/contractDocument";
import {
  getDocumentOrigin,
  getDocumentOriginLabel,
  originEntityKey,
  resolveDocumentOrigin,
  type DocumentOriginId,
} from "./origins";
import type { DocumentActor, DocumentReference, DocumentReferenceResolver } from "./references";
import {
  getSignatureStateMeta,
  isSignatureOpen,
  resolveSignatureState,
  type DocumentSignatureState,
} from "./signature";

/** Janela usada pela coleção "Adicionados recentemente". */
export const RECENT_WINDOW_DAYS = 7;

/**
 * Coleções inteligentes: recortes por estado do documento. São o primeiro
 * nível do drive porque respondem à pergunta "o que precisa da minha atenção
 * agora?".
 *
 * As coleções de assinatura são separadas de propósito. "Aguardando
 * assinatura" é só o que já foi enviado; "Assinatura não iniciada" é o
 * contrato/aditivo que exige assinatura e ninguém enviou; "Não exige
 * assinatura" é o arquivo de apoio que nunca passará por fluxo nenhum.
 */
export type DocumentCollectionId =
  | "all"
  | "recent"
  | "awaiting"
  | "not_started"
  | "signed"
  | "not_required"
  | "draft"
  | "attention";

export interface DocumentCollectionDefinition {
  id: DocumentCollectionId;
  label: string;
  description: string;
  icon: LucideIcon;
}

const COLLECTION_ICONS: Record<DocumentCollectionId, LucideIcon> = {
  all: FolderOpen,
  recent: Clock,
  awaiting: Hourglass,
  not_started: FileSignature,
  signed: BadgeCheck,
  not_required: FileText,
  draft: Files,
  attention: Ban,
};

export const DOCUMENT_COLLECTION_IDS: readonly DocumentCollectionId[] = [
  "all",
  "recent",
  "awaiting",
  "not_started",
  "signed",
  "not_required",
  "draft",
  "attention",
] as const;

export const DOCUMENT_COLLECTIONS: readonly DocumentCollectionDefinition[] = DOCUMENT_COLLECTION_IDS.map((id) => ({
  id,
  label: t(`documents.collection.${id}`),
  description: t(`documents.collection.${id}.hint`),
  icon: COLLECTION_ICONS[id],
}));

export function getDocumentCollection(id: DocumentCollectionId): DocumentCollectionDefinition {
  return DOCUMENT_COLLECTIONS.find((c) => c.id === id) ?? DOCUMENT_COLLECTIONS[0];
}

export type DocumentSortId = "recent" | "oldest" | "name" | "size";

export const DOCUMENT_SORTS: readonly { id: DocumentSortId; label: string }[] = [
  { id: "recent", label: t("documents.sort.recent") },
  { id: "oldest", label: t("documents.sort.oldest") },
  { id: "name", label: t("documents.sort.name") },
  { id: "size", label: t("documents.sort.size") },
];

export type DocumentGroupId = "none" | "origin" | "category" | "status" | "actor";

export const DOCUMENT_GROUPS: readonly { id: DocumentGroupId; label: string }[] = [
  { id: "none", label: t("documents.group.none") },
  { id: "origin", label: t("documents.group.origin") },
  { id: "category", label: t("documents.group.category") },
  { id: "status", label: t("documents.group.status") },
  { id: "actor", label: t("documents.group.actor") },
];

/**
 * Seleção ativa da barra lateral. `origin-entity` é o nível mais útil do
 * drive: um contrato, um imóvel ou um cliente específico.
 */
export type DocumentFolderKey =
  | { kind: "collection"; id: DocumentCollectionId }
  | { kind: "category"; category: DocumentCategory }
  | { kind: "origin"; origin: DocumentOriginId }
  | { kind: "origin-entity"; origin: DocumentOriginId; entityId: string }
  | { kind: "tag"; tag: string };

export function documentFolderKeyToString(key: DocumentFolderKey): string {
  switch (key.kind) {
    case "collection":
      return `collection:${key.id}`;
    case "category":
      return `cat:${key.category}`;
    case "origin":
      return `origin:${key.origin}`;
    case "origin-entity":
      return `origin:${key.origin}:${key.entityId}`;
    case "tag":
      return `tag:${key.tag}`;
  }
}

/**
 * Pastas fixadas foram gravadas antes deste redesenho com as chaves
 * `all` / `smart:*`. Traduzimos na leitura para que a preferência salva do
 * usuário continue válida em vez de sumir da barra lateral.
 */
const LEGACY_FOLDER_KEY_ALIASES: Record<string, string> = {
  all: "collection:all",
  "smart:recent": "collection:recent",
  "smart:waiting": "collection:awaiting",
  "smart:rascunho": "collection:draft",
  // Chaves gravadas pela primeira versão do drive, quando "Aguardando
  // assinatura" ainda misturava documentos sem fluxo de assinatura.
  "collection:waiting": "collection:awaiting",
};

export function normalizeFolderKeyString(key: string): string {
  return LEGACY_FOLDER_KEY_ALIASES[key] ?? key;
}

export interface DocumentEntityNode {
  origin: DocumentOriginId;
  entityId: string;
  label: string;
  detail: string;
  href?: string;
  count: number;
}

export interface DriveEntry {
  doc: ContractDocument;
  origin: DocumentOriginId;
  /** Nome legível do registro ao qual o documento pertence. */
  reference: DocumentReference;
  actor: DocumentActor;
  categoryLabel: string;
  /** Estado real de assinatura — não confundir com `doc.status` cru. */
  signatureState: DocumentSignatureState;
  signatureLabel: string;
  /** Texto normalizado usado pela busca livre. */
  searchText: string;
}

export interface DocumentDriveCounts {
  total: number;
  collections: Record<DocumentCollectionId, number>;
  categories: Partial<Record<DocumentCategory, number>>;
  origins: Partial<Record<DocumentOriginId, number>>;
  tags: Record<string, number>;
}

export interface DocumentDrive {
  entries: DriveEntry[];
  counts: DocumentDriveCounts;
  /** Registros (contrato/imóvel/cliente...) de cada origem, com contagem. */
  entitiesByOrigin: Record<DocumentOriginId, DocumentEntityNode[]>;
}

export interface DocumentDriveSummary {
  counts: DocumentDriveCounts;
  entitiesByOrigin: Record<DocumentOriginId, DocumentEntityNode[]>;
}

/** Recalcula contagens e nós de origem para um subconjunto de entradas. */
export function summarizeDriveEntries(
  entries: readonly DriveEntry[],
  now: number = Date.now(),
): DocumentDriveSummary {
  const collections: Record<DocumentCollectionId, number> = {
    all: 0,
    recent: 0,
    awaiting: 0,
    not_started: 0,
    signed: 0,
    not_required: 0,
    draft: 0,
    attention: 0,
  };
  const categories: Partial<Record<DocumentCategory, number>> = {};
  const origins: Partial<Record<DocumentOriginId, number>> = {};
  const tags: Record<string, number> = {};
  const entitiesByOrigin = {} as Record<DocumentOriginId, DocumentEntityNode[]>;
  const entityIndex = new Map<string, DocumentEntityNode>();

  for (const entry of entries) {
    for (const collection of DOCUMENT_COLLECTION_IDS) {
      if (matchesCollection(entry, collection, now)) collections[collection] += 1;
    }
    categories[entry.doc.category] = (categories[entry.doc.category] ?? 0) + 1;
    origins[entry.origin] = (origins[entry.origin] ?? 0) + 1;
    for (const tag of entry.doc.tags ?? []) {
      const value = tag.trim();
      if (value) tags[value] = (tags[value] ?? 0) + 1;
    }

    const key = originEntityKey(entry.origin, entry.reference.entityId);
    const existing = entityIndex.get(key);
    if (existing) {
      existing.count += 1;
      continue;
    }
    const node: DocumentEntityNode = {
      origin: entry.origin,
      entityId: entry.reference.entityId,
      label: entry.reference.groupLabel,
      detail: entry.reference.groupDetail ?? entry.reference.detail,
      href: entry.reference.href,
      count: 1,
    };
    entityIndex.set(key, node);
    (entitiesByOrigin[entry.origin] ??= []).push(node);
  }

  for (const origin of Object.keys(entitiesByOrigin) as DocumentOriginId[]) {
    entitiesByOrigin[origin].sort((a, b) => b.count - a.count || a.label.localeCompare(b.label, "pt-BR"));
  }

  return { counts: { total: entries.length, collections, categories, origins, tags }, entitiesByOrigin };
}

function uploadTimestamp(doc: ContractDocument): number {
  const raw = doc.uploadDate || doc.createdAt || "";
  const ts = Date.parse(raw);
  return Number.isNaN(ts) ? 0 : ts;
}

export function isRecentlyAdded(doc: ContractDocument, now: number = Date.now()): boolean {
  const ts = uploadTimestamp(doc);
  if (ts <= 0) return false;
  return now - ts <= RECENT_WINDOW_DAYS * 24 * 60 * 60 * 1000;
}

/**
 * A coleção é decidida pelo estado de assinatura, não pelo status cru: é isso
 * que impede uma foto de fachada de aparecer como "aguardando assinatura".
 */
export function matchesCollection(
  entry: DriveEntry,
  collection: DocumentCollectionId,
  now: number = Date.now(),
): boolean {
  const state = entry.signatureState;
  switch (collection) {
    case "all":
      return true;
    case "recent":
      return isRecentlyAdded(entry.doc, now);
    case "awaiting":
      return isSignatureOpen(state);
    case "not_started":
      return state === "not_started";
    case "signed":
      return state === "signed";
    case "not_required":
      return state === "not_required";
    case "draft":
      return state === "draft";
    case "attention":
      return state === "expired" || state === "cancelled";
    default:
      return true;
  }
}

/** Remove acentos e caixa para que "imovel" encontre "Imóvel". */
export function normalizeSearchText(value: string): string {
  return value
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "")
    .toLowerCase()
    .replace(/\s+/g, " ")
    .trim();
}

function buildEntry(doc: ContractDocument, resolver: DocumentReferenceResolver): DriveEntry {
  const reference = resolver.resolve(doc);
  const origin = resolveDocumentOrigin(doc);
  const actor = resolver.resolveActor(doc.uploadedBy);
  const categoryLabel = getCategoryMeta(doc.category).label;
  const signatureState = resolveSignatureState(doc);
  const signatureLabel = getSignatureStateMeta(signatureState).label;

  const searchText = normalizeSearchText(
    [
      doc.name,
      doc.notes ?? "",
      doc.fileType,
      categoryLabel,
      signatureLabel,
      getDocumentOriginLabel(origin),
      reference.label,
      reference.detail,
      reference.entityId,
      doc.contractId ?? "",
      doc.entityId ?? "",
      actor.label,
      (doc.tags ?? []).join(" "),
      doc.signers.map((s) => s.name).join(" "),
    ]
      .filter(Boolean)
      .join(" "),
  );

  return { doc, origin, reference, actor, categoryLabel, signatureState, signatureLabel, searchText };
}

/** Constrói o índice do drive: entradas normalizadas + contagens exatas. */
export function buildDocumentDrive(
  documents: readonly ContractDocument[],
  resolver: DocumentReferenceResolver,
  now: number = Date.now(),
): DocumentDrive {
  const entries = documents.map((doc) => buildEntry(doc, resolver));
  const summary = summarizeDriveEntries(entries, now);
  return { entries, ...summary };
}

export function matchesFolder(entry: DriveEntry, folder: DocumentFolderKey, now: number = Date.now()): boolean {
  switch (folder.kind) {
    case "collection":
      return matchesCollection(entry, folder.id, now);
    case "category":
      return entry.doc.category === folder.category;
    case "origin":
      return entry.origin === folder.origin;
    case "origin-entity":
      return entry.origin === folder.origin && (entry.reference.entityId || "") === folder.entityId;
    case "tag":
      return (entry.doc.tags ?? []).includes(folder.tag);
    default:
      return true;
  }
}

export function matchesQuery(entry: DriveEntry, query: string): boolean {
  const needle = normalizeSearchText(query);
  if (!needle) return true;
  const terms = needle.split(" ").filter(Boolean);
  return terms.every((term) => entry.searchText.includes(term));
}

export function filterByFolder(
  entries: readonly DriveEntry[],
  folder: DocumentFolderKey,
  now: number = Date.now(),
): DriveEntry[] {
  return entries.filter((entry) => matchesFolder(entry, folder, now));
}

/** Busca livre + pasta em uma passada (usado por consumidores simples). */
export function filterDriveEntries(
  entries: readonly DriveEntry[],
  folder: DocumentFolderKey,
  query: string,
  now: number = Date.now(),
): DriveEntry[] {
  return filterByFolder(entries, folder, now).filter((entry) => matchesQuery(entry, query));
}

export function sortDriveEntries(entries: readonly DriveEntry[], sort: DocumentSortId): DriveEntry[] {
  const out = [...entries];
  switch (sort) {
    case "oldest":
      out.sort((a, b) => uploadTimestamp(a.doc) - uploadTimestamp(b.doc));
      break;
    case "name":
      out.sort((a, b) => a.doc.name.localeCompare(b.doc.name, "pt-BR", { sensitivity: "base" }));
      break;
    case "size":
      out.sort((a, b) => (b.doc.fileSize ?? 0) - (a.doc.fileSize ?? 0));
      break;
    case "recent":
    default:
      out.sort((a, b) => uploadTimestamp(b.doc) - uploadTimestamp(a.doc));
      break;
  }
  return out;
}

export interface DriveSection {
  key: string;
  label: string;
  hint?: string;
  count: number;
  entries: DriveEntry[];
}

/**
 * Agrupa o resultado da busca em seções nomeadas. É o que transforma a lista
 * plana em um drive navegável ("Contratos", "Imóveis", "Clientes"...).
 */
export function groupDriveEntries(
  entries: readonly DriveEntry[],
  groupBy: DocumentGroupId,
): DriveSection[] {
  if (groupBy === "none") return [];

  const groups = new Map<string, DriveSection>();
  for (const entry of entries) {
    let key: string;
    let label: string;
    let hint: string | undefined;

    switch (groupBy) {
      case "origin": {
        const def = getDocumentOrigin(entry.origin);
        key = entry.origin;
        label = def.label;
        hint = def.description;
        break;
      }
      case "category":
        key = entry.doc.category;
        label = entry.categoryLabel;
        break;
      case "status":
        key = entry.signatureState;
        label = entry.signatureLabel;
        hint = getSignatureStateMeta(entry.signatureState).description;
        break;
      case "actor":
        key = entry.actor.label;
        label = entry.actor.label;
        break;
      default:
        key = "all";
        label = t("documents.group.none");
    }

    const existing = groups.get(key);
    if (existing) {
      existing.entries.push(entry);
      existing.count += 1;
      continue;
    }
    groups.set(key, { key, label, hint, count: 1, entries: [entry] });
  }

  return Array.from(groups.values()).sort(
    (a, b) => b.count - a.count || a.label.localeCompare(b.label, "pt-BR"),
  );
}
