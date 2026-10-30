import type { LucideIcon } from "lucide-react";
import {
  Banknote,
  Building2,
  ClipboardCheck,
  FileQuestion,
  FileSignature,
  Handshake,
  Landmark,
  ListChecks,
  ShieldCheck,
  Users,
  Wrench,
} from "lucide-react";
import { t } from "../lib/i18n";
import type { DocumentSource } from "./types";

/**
 * Origem = de onde o documento veio dentro da operação da imobiliária
 * (contrato, negócio, imóvel, cliente, vistoria...). É uma dimensão
 * diferente de "tipo" (categoria do arquivo) e de "status" (assinatura) e
 * existe para que a Central de Documentos funcione como um drive da
 * operação: primeiro se escolhe o contexto (origem), depois o registro
 * específico (contrato X, imóvel Y, cliente Z) e só então os arquivos.
 */
export type DocumentOriginId =
  | "contract"
  | "deal"
  | "property"
  | "client"
  | "inspection"
  | "checklist"
  | "analysis"
  | "fiscal"
  | "financial"
  | "maintenance"
  | "unassigned";

export interface DocumentOriginDefinition {
  id: DocumentOriginId;
  label: string;
  description: string;
  icon: LucideIcon;
}

export const DOCUMENT_ORIGIN_IDS: readonly DocumentOriginId[] = [
  "contract",
  "deal",
  "property",
  "client",
  "inspection",
  "analysis",
  "checklist",
  "financial",
  "fiscal",
  "maintenance",
  "unassigned",
] as const;

const ORIGIN_ICONS: Record<DocumentOriginId, LucideIcon> = {
  contract: FileSignature,
  deal: Handshake,
  property: Building2,
  client: Users,
  inspection: ClipboardCheck,
  checklist: ListChecks,
  analysis: ShieldCheck,
  fiscal: Landmark,
  financial: Banknote,
  maintenance: Wrench,
  unassigned: FileQuestion,
};

const ORIGIN_META: Record<DocumentOriginId, DocumentOriginDefinition> = DOCUMENT_ORIGIN_IDS.reduce(
  (acc, id) => {
    acc[id] = {
      id,
      label: t(`documents.origin.${id}`),
      description: t(`documents.origin.${id}.hint`),
      icon: ORIGIN_ICONS[id],
    };
    return acc;
  },
  {} as Record<DocumentOriginId, DocumentOriginDefinition>,
);

export function getDocumentOrigin(id: DocumentOriginId): DocumentOriginDefinition {
  return ORIGIN_META[id] ?? ORIGIN_META.unassigned;
}

export function getDocumentOriginLabel(id: DocumentOriginId): string {
  return getDocumentOrigin(id).label;
}

export const DOCUMENT_ORIGINS: readonly DocumentOriginDefinition[] = DOCUMENT_ORIGIN_IDS.map(
  (id) => ORIGIN_META[id],
);

/** Campos do documento usados para descobrir a origem. */
export interface DocumentOriginSource {
  entityType?: string | null;
  entityId?: string | null;
  contractId?: string | null;
  source?: DocumentSource | string | null;
}

/**
 * `entity_type` é gravado por cada serviço e historicamente usa variações de
 * caixa/plural. Normalizamos antes de comparar para não perder vínculos.
 */
function normalizeKey(value: string | null | undefined): string {
  return (value ?? "").trim().toLowerCase();
}

const ENTITY_TYPE_ORIGINS: Record<string, DocumentOriginId> = {
  contract: "contract",
  contracts: "contract",
  deal: "deal",
  deals: "deal",
  negotiation: "deal",
  property: "property",
  properties: "property",
  listing: "property",
  lead: "client",
  leads: "client",
  client: "client",
  customer: "client",
  inspection: "inspection",
  inspections: "inspection",
  checklist: "checklist",
  checklistitem: "checklist",
  analysis: "analysis",
  duediligence: "analysis",
  credit_analysis: "analysis",
  fiscal: "fiscal",
  tax: "fiscal",
  maintenance: "maintenance",
  ticket: "maintenance",
  tickets: "maintenance",
  financial: "financial",
  finance: "financial",
  charge: "financial",
  general: "unassigned",
  document: "unassigned",
};

const SOURCE_ORIGINS: Record<string, DocumentOriginId> = {
  "toca-deal": "deal",
  "toca-core": "unassigned",
  "toca-financial": "financial",
  "toca-ops": "maintenance",
  "toca-comm": "client",
  portal: "client",
  manual: "unassigned",
};

/**
 * Resolve a origem de um documento. A ordem importa: o vínculo explícito
 * (`entity_type`) manda; depois o contrato; por último o serviço que originou
 * o upload. Nunca inventa vínculo — sem nenhuma evidência o documento fica em
 * "Sem vínculo" (arquivos avulsos), que é o balde honesto para o operador.
 */
export function resolveDocumentOrigin(doc: DocumentOriginSource): DocumentOriginId {
  const direct = ENTITY_TYPE_ORIGINS[normalizeKey(doc.entityType)];
  if (direct && direct !== "unassigned") return direct;

  if (normalizeKey(doc.contractId)) return "contract";

  const fromSource = SOURCE_ORIGINS[normalizeKey(doc.source)];
  if (fromSource) return fromSource;

  return "unassigned";
}

/** Chave estável de um registro dentro de uma origem (contrato/imóvel/etc). */
export function originEntityKey(origin: DocumentOriginId, entityId: string | null | undefined): string {
  const id = (entityId ?? "").trim();
  return id ? `${origin}:${id}` : `${origin}:__none__`;
}
