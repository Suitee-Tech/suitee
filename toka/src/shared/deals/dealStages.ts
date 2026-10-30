import type { LucideIcon } from "lucide-react";
import { ArrowRightLeft, CheckCircle2, FileCheck, PenTool, UploadCloud, XCircle } from "lucide-react";
import type { DealStatus } from "../services/deal.service";

export const MAIN_STAGES: readonly DealStatus[] = [
  "NEGOTIATION",
  "PROPOSAL",
  "ANALYSIS",
  "CONTRACT",
  "CONVERTED",
] as const;

export interface StageMeta {
  key: DealStatus;
  label: string;
  accent: string;
  description: string;
  icon: LucideIcon;
  colorClass: string;
  bgClass: string;
}

const STAGE_META: Record<DealStatus, StageMeta> = {
  NEGOTIATION: {
    key: "NEGOTIATION",
    label: "Em Negociação",
    accent: "slate",
    description: "Conversas iniciais e visitas",
    icon: ArrowRightLeft,
    colorClass: "text-muted-foreground",
    bgClass: "bg-muted",
  },
  PROPOSAL: {
    key: "PROPOSAL",
    label: "Proposta",
    accent: "blue",
    description: "Proposta enviada e aguardando resposta",
    icon: FileCheck,
    colorClass: "text-primary-500",
    bgClass: "bg-primary-50",
  },
  ANALYSIS: {
    key: "ANALYSIS",
    label: "Análise",
    accent: "amber",
    description: "Documentação em análise",
    icon: UploadCloud,
    colorClass: "text-status-warning",
    bgClass: "bg-status-warning-soft",
  },
  CONTRACT: {
    key: "CONTRACT",
    label: "Contrato",
    accent: "purple",
    description: "Contrato pronto para assinatura",
    icon: PenTool,
    colorClass: "text-stage-contract",
    bgClass: "bg-stage-contract/10",
  },
  CONVERTED: {
    key: "CONVERTED",
    label: "Concluído",
    accent: "emerald",
    description: "Negócio concluído com sucesso",
    icon: CheckCircle2,
    colorClass: "text-status-ok",
    bgClass: "bg-status-ok-soft",
  },
  CANCELLED: {
    key: "CANCELLED",
    label: "Perdido",
    accent: "rose",
    description: "Negócio cancelado",
    icon: XCircle,
    colorClass: "text-status-critical",
    bgClass: "bg-status-critical-soft",
  },
};

export function getStageMeta(status: DealStatus): StageMeta {
  return STAGE_META[status];
}

export function getStageIndex(status: DealStatus): number {
  return MAIN_STAGES.indexOf(status);
}

export function isCancelled(status: DealStatus): boolean {
  return status === "CANCELLED";
}

const LABEL_TO_STATUS = MAIN_STAGES.concat("CANCELLED").reduce<Record<string, DealStatus>>((acc, status) => {
  acc[STAGE_META[status].label] = status;
  return acc;
}, {});

export function getStageMetaByLabel(label: string): StageMeta | undefined {
  const status = LABEL_TO_STATUS[label];
  return status ? STAGE_META[status] : undefined;
}

export const MAIN_STAGE_METAS: readonly StageMeta[] = MAIN_STAGES.map(getStageMeta);
export const ALL_STAGE_METAS: readonly StageMeta[] = [...MAIN_STAGE_METAS, STAGE_META.CANCELLED];
