export type DocumentCategory =
  | "contrato"
  | "aditivo"
  | "laudo"
  | "vistoria"
  | "procuracao"
  | "recibo"
  | "notificacao"
  | "financeiro"
  | "juridico"
  | "fiscal"
  | "identidade"
  | "comprovante"
  | "foto"
  | "outros";

export type DocumentStatus =
  | "rascunho"
  | "pendente"
  | "pendente_assinatura"
  | "parcialmente_assinado"
  | "assinado"
  | "expirado"
  | "cancelado";

export type SignerStatus = "pendente" | "assinado" | "recusado";

export type EntityType =
  | "contract"
  | "deal"
  | "property"
  | "lead"
  | "inspection"
  | "fiscal"
  | "checklist"
  | "analysis"
  | "maintenance"
  | "financial"
  | "general";

export type DocumentSource =
  | "toca-deal"
  | "toca-core"
  | "toca-financial"
  | "toca-ops"
  | "toca-comm"
  | "portal"
  | "manual";

export type DocumentPersistenceStatus = "pending" | "stored" | "failed";

/**
 * Critério da análise de crédito/due diligence ao qual o arquivo serve de
 * evidência. Permite rotular o documento pelo que ele realmente comprova
 * ("CND", "Score", "Parecer") em vez de um nome de arquivo cru.
 */
export type AnalysisCriterion = "score" | "cnd" | "legal_opinion" | "income";

export const ANALYSIS_CRITERIA: readonly AnalysisCriterion[] = ["score", "cnd", "legal_opinion", "income"] as const;

export const ANALYSIS_CRITERION_LABELS: Record<AnalysisCriterion, string> = {
  score: "Score",
  cnd: "CND",
  legal_opinion: "Parecer",
  income: "Renda",
};

export const DOCUMENT_CATEGORIES: readonly DocumentCategory[] = [
  "contrato",
  "aditivo",
  "laudo",
  "vistoria",
  "procuracao",
  "recibo",
  "notificacao",
  "financeiro",
  "juridico",
  "fiscal",
  "identidade",
  "comprovante",
  "foto",
  "outros",
] as const;

export const DOCUMENT_STATUSES: readonly DocumentStatus[] = [
  "rascunho",
  "pendente",
  "pendente_assinatura",
  "parcialmente_assinado",
  "assinado",
  "expirado",
  "cancelado",
] as const;
