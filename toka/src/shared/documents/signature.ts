import type { LucideIcon } from "lucide-react";
import { AlertCircle, Ban, BadgeCheck, FileSignature, FileText, Files, Hourglass, PenTool, X } from "lucide-react";
import { t } from "../lib/i18n";
import type { ContractDocument, DocumentCategory, DocumentStatus } from "../types/contractDocument";

/**
 * Assinatura é uma dimensão própria do documento, não um sinônimo de status.
 *
 * O backend grava `pendente` em *todo* upload genérico (`document.New`), e
 * reserva `pendente_assinatura` / `parcialmente_assinado` / `assinado` para
 * documentos que realmente entraram em um fluxo de assinatura (contratos e
 * aditivos, via Clicksign). Tratar `pendente` como "aguardando assinatura"
 * fazia uma foto de fachada ou uma planilha interna parecerem pendências
 * jurídicas.
 *
 * Aqui a pergunta é sempre dupla: **este documento exige assinatura?** e, se
 * exigir, **em que ponto do fluxo ele está?**.
 */
export type DocumentSignatureState =
  | "signed"
  | "partially_signed"
  | "awaiting"
  | "not_started"
  | "not_required"
  | "draft"
  | "expired"
  | "cancelled";

/**
 * Categorias que são instrumentos formais: por natureza exigem assinatura,
 * mesmo antes de existir um envelope. Contrato e aditivo são o caso óbvio;
 * procuração e notificação também só valem assinadas.
 */
export const SIGNATURE_REQUIRED_CATEGORIES: readonly DocumentCategory[] = [
  "contrato",
  "aditivo",
  "procuracao",
  "notificacao",
] as const;

/** Estados que só existem dentro de um fluxo de assinatura. */
const SIGNATURE_FLOW_STATUSES: readonly DocumentStatus[] = [
  "pendente_assinatura",
  "parcialmente_assinado",
  "assinado",
] as const;

export interface SignatureSubject {
  status?: DocumentStatus | string | null;
  category?: DocumentCategory | string | null;
  signers?: { status?: string | null }[] | null;
  contractId?: string | null;
}

/**
 * `true` quando há evidência de que o documento participa (ou deve
 * participar) de um fluxo de assinatura. A ordem importa: um fluxo já criado
 * (signatários) ou um status de assinatura valem mais do que a categoria.
 */
export function documentRequiresSignature(doc: SignatureSubject): boolean {
  if ((doc.signers ?? []).length > 0) return true;
  if (SIGNATURE_FLOW_STATUSES.includes(doc.status as DocumentStatus)) return true;
  if (SIGNATURE_REQUIRED_CATEGORIES.includes(doc.category as DocumentCategory)) return true;
  return false;
}

/**
 * Estado de assinatura do documento. Nunca rotula como pendência de
 * assinatura um arquivo que não tem fluxo — esse é o ponto da classificação.
 */
export function resolveSignatureState(doc: SignatureSubject): DocumentSignatureState {
  switch (doc.status) {
    case "cancelado":
      return "cancelled";
    case "expirado":
      return "expired";
    case "rascunho":
      return "draft";
    case "assinado":
      return "signed";
    case "parcialmente_assinado":
      return "partially_signed";
    case "pendente_assinatura":
      return "awaiting";
    default:
      break;
  }

  // Status genérico (`pendente`) ou desconhecido: decide o que o documento é.
  const signers = doc.signers ?? [];
  if (signers.length > 0) {
    const signed = signers.filter((s) => s?.status === "assinado").length;
    if (signed === signers.length) return "signed";
    if (signed > 0) return "partially_signed";
    return "awaiting";
  }

  return documentRequiresSignature(doc) ? "not_started" : "not_required";
}

export interface SignatureProgress {
  signed: number;
  total: number;
  label: string;
}

/** Progresso legível dos signatários, quando existe fluxo. */
export function describeSignatureProgress(doc: SignatureSubject): SignatureProgress | null {
  const signers = doc.signers ?? [];
  if (signers.length === 0) return null;
  const signed = signers.filter((s) => s?.status === "assinado").length;
  return {
    signed,
    total: signers.length,
    label: t("documents.signature.progress", { signed, total: signers.length }),
  };
}

export interface SignatureStateMeta {
  key: DocumentSignatureState;
  label: string;
  description: string;
  badgeClass: string;
  icon: LucideIcon;
}

const SIGNATURE_STATE_META: Record<
  DocumentSignatureState,
  { badgeClass: string; icon: LucideIcon }
> = {
  signed: { badgeClass: "bg-status-ok-soft text-status-ok border-status-ok/30", icon: BadgeCheck },
  partially_signed: { badgeClass: "bg-status-info-soft text-status-info border-status-info/30", icon: PenTool },
  awaiting: { badgeClass: "bg-status-warning-soft text-status-warning border-status-warning/30", icon: Hourglass },
  // Borda tracejada: exige assinatura, mas nenhum fluxo foi criado ainda.
  not_started: {
    badgeClass: "bg-status-warning-soft text-status-warning border-dashed border-status-warning/50",
    icon: FileSignature,
  },
  not_required: { badgeClass: "bg-muted text-muted-foreground border-border", icon: FileText },
  draft: { badgeClass: "bg-muted text-muted-foreground border-border", icon: Files },
  expired: { badgeClass: "bg-status-critical-soft text-status-critical border-status-critical/30", icon: AlertCircle },
  cancelled: { badgeClass: "bg-muted text-muted-foreground border-border", icon: Ban },
};

const SIGNATURE_STATE_ICON_FALLBACK: Record<DocumentSignatureState, LucideIcon> = {
  signed: BadgeCheck,
  partially_signed: PenTool,
  awaiting: Hourglass,
  not_started: FileSignature,
  not_required: FileText,
  draft: Files,
  expired: AlertCircle,
  cancelled: X,
};

export const DOCUMENT_SIGNATURE_STATES: readonly DocumentSignatureState[] = [
  "signed",
  "partially_signed",
  "awaiting",
  "not_started",
  "not_required",
  "draft",
  "expired",
  "cancelled",
] as const;

export function getSignatureStateMeta(state: DocumentSignatureState): SignatureStateMeta {
  const meta = SIGNATURE_STATE_META[state] ?? SIGNATURE_STATE_META.not_required;
  return {
    key: state,
    label: t(`documents.signature.${state}`),
    description: t(`documents.signature.${state}.hint`),
    badgeClass: meta.badgeClass,
    icon: meta.icon ?? SIGNATURE_STATE_ICON_FALLBACK[state] ?? FileText,
  };
}

/** Atalho para o caso mais comum: classificar direto do documento. */
export function getDocumentSignatureMeta(doc: ContractDocument): SignatureStateMeta {
  return getSignatureStateMeta(resolveSignatureState(doc));
}

/** Coleções e cartões só precisam saber se ainda há assinatura em aberto. */
export function isSignatureOpen(state: DocumentSignatureState): boolean {
  return state === "awaiting" || state === "partially_signed";
}

/** Exige assinatura e ainda não foi assinado — inclui o que nem foi enviado. */
export function isSignaturePending(state: DocumentSignatureState): boolean {
  return state === "awaiting" || state === "partially_signed" || state === "not_started";
}
