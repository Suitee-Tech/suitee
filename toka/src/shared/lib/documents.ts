import type { LucideIcon } from "lucide-react";
import { AlertCircle, CheckCircle2, Clock, File as FileIcon, PenTool, X } from "lucide-react";
import { t } from "./i18n";
import {
  DOCUMENT_CATEGORIES,
  DOCUMENT_STATUSES,
  type DocumentCategory,
  type DocumentPersistenceStatus,
  type DocumentSource,
  type DocumentStatus,
  type EntityType,
  type SignerStatus,
} from "../documents/types";

export type { DocumentCategory, DocumentPersistenceStatus, DocumentSource, DocumentStatus, EntityType, SignerStatus };
export { DOCUMENT_CATEGORIES, DOCUMENT_STATUSES };

export interface CategoryMeta {
  key: DocumentCategory;
  label: string;
  badgeClass: string;
}

const CATEGORY_BADGE_CLASSES: Record<DocumentCategory, string> = {
  contrato: "bg-primary-50 text-primary-700 border-primary-200",
  aditivo: "bg-stage-contract/10 text-stage-contract border-stage-contract/30",
  laudo: "bg-teal-50 text-teal-700 border-teal-200",
  vistoria: "bg-status-warning-soft text-status-warning border-status-warning/30",
  procuracao: "bg-sky-50 text-sky-700 border-sky-200",
  recibo: "bg-status-ok-soft text-status-ok border-status-ok/30",
  notificacao: "bg-status-critical-soft text-status-critical border-status-critical/30",
  financeiro: "bg-emerald-50 text-emerald-700 border-emerald-200",
  juridico: "bg-indigo-50 text-indigo-700 border-indigo-200",
  fiscal: "bg-amber-50 text-amber-700 border-amber-200",
  identidade: "bg-violet-50 text-violet-700 border-violet-200",
  comprovante: "bg-cyan-50 text-cyan-700 border-cyan-200",
  foto: "bg-pink-50 text-pink-700 border-pink-200",
  outros: "bg-muted text-foreground border-border",
};

function buildCategoryMeta(key: DocumentCategory): CategoryMeta {
  return {
    key,
    label: t(`documents.category.${key}`),
    badgeClass: CATEGORY_BADGE_CLASSES[key],
  };
}

const CATEGORY_META: Record<DocumentCategory, CategoryMeta> = DOCUMENT_CATEGORIES.reduce(
  (acc, c) => {
    acc[c] = buildCategoryMeta(c);
    return acc;
  },
  {} as Record<DocumentCategory, CategoryMeta>,
);

export function getCategoryMeta(category: DocumentCategory): CategoryMeta {
  return CATEGORY_META[category] ?? CATEGORY_META.outros;
}

export function getCategoryLabel(category: DocumentCategory): string {
  return getCategoryMeta(category).label;
}

export const CATEGORY_OPTIONS: ReadonlyArray<{ value: DocumentCategory; label: string }> = DOCUMENT_CATEGORIES.map((c) => ({
  value: c,
  label: CATEGORY_META[c].label,
}));

export interface StatusMeta {
  key: DocumentStatus;
  label: string;
  badgeClass: string;
  icon: LucideIcon;
}

const STATUS_BADGE_AND_ICON: Record<DocumentStatus, { badgeClass: string; icon: LucideIcon }> = {
  rascunho: { badgeClass: "bg-muted text-muted-foreground border-border", icon: FileIcon },
  pendente: { badgeClass: "bg-muted text-muted-foreground border-border", icon: Clock },
  pendente_assinatura: { badgeClass: "bg-status-warning-soft text-status-warning border-status-warning/30", icon: Clock },
  parcialmente_assinado: { badgeClass: "bg-status-info-soft text-status-info border-status-info/30", icon: PenTool },
  assinado: { badgeClass: "bg-status-ok-soft text-status-ok border-status-ok/30", icon: CheckCircle2 },
  expirado: { badgeClass: "bg-status-critical-soft text-status-critical border-status-critical/30", icon: AlertCircle },
  cancelado: { badgeClass: "bg-muted text-muted-foreground border-border", icon: X },
};

const STATUS_META: Record<DocumentStatus, StatusMeta> = DOCUMENT_STATUSES.reduce(
  (acc, s) => {
    acc[s] = {
      key: s,
      label: t(`documents.status.${s}`),
      badgeClass: STATUS_BADGE_AND_ICON[s].badgeClass,
      icon: STATUS_BADGE_AND_ICON[s].icon,
    };
    return acc;
  },
  {} as Record<DocumentStatus, StatusMeta>,
);

export function getStatusMeta(status: DocumentStatus): StatusMeta {
  return STATUS_META[status] ?? STATUS_META.rascunho;
}

export function getSafeDocumentFallbackUrl(fileUrl: string | null | undefined): string | null {
  const value = fileUrl?.trim();
  if (!value) return null;

  try {
    const url = new URL(value, "https://suitee.invalid");
    if (url.protocol !== "http:" && url.protocol !== "https:") return null;

    const pathname = decodeURIComponent(url.pathname).replace(/\/+/g, "/");
    if (/\/storage\/v1\/object\/authenticated(?:\/|$)/i.test(pathname)) {
      return null;
    }
    return value;
  } catch {
    return null;
  }
}

/**
 * Normaliza o status vindo do backend para o vocabulário da UI.
 *
 * `pendente` **não** é convertido em `pendente_assinatura`: o backend grava
 * `pendente` em todo upload genérico (`document.New`), enquanto
 * `pendente_assinatura` só aparece quando o documento entrou de fato em um
 * fluxo de assinatura. Colapsar os dois fazia foto, comprovante e planilha
 * aparecerem como "aguardando assinatura". Quem decide se há assinatura
 * pendente é `shared/documents/signature.ts`.
 */
export function normalizeBackendStatus(raw: string | undefined | null): DocumentStatus {
  if (!raw) return "rascunho";
  if ((DOCUMENT_STATUSES as readonly string[]).includes(raw)) return raw as DocumentStatus;
  return "rascunho";
}

export type ChecklistItemCategoryName = "pessoal" | "financeiro" | "imovel" | "juridico" | "outros";

export function documentCategoryFromChecklistCategory(checklistCategory: string | undefined | null): DocumentCategory {
  switch (checklistCategory) {
    case "pessoal":
      return "identidade";
    case "financeiro":
      return "financeiro";
    case "imovel":
      return "juridico";
    case "juridico":
      return "juridico";
    case "outros":
      return "outros";
    default:
      return "outros";
  }
}
