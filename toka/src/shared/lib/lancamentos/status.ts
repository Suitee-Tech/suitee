import type { Lancamento, LancamentoAction, LancamentoKind, LancamentoStatus } from "./types";

const CHARGE_STATUS: Record<string, LancamentoStatus> = {
  PENDING: "pendente",
  PAID: "pago",
  OVERDUE: "vencido",
  CANCELLED: "cancelado",
  PARTIALLY_PAID: "parcial",
  REVERSED: "revertido",
};

const COMMISSION_STATUS: Record<string, LancamentoStatus> = {
  PENDING: "pendente",
  APPROVED: "aprovado",
  PAID: "pago",
  REVERSED: "revertido",
  CANCELLED: "cancelado",
};

const TRANSFER_STATUS: Record<string, LancamentoStatus> = {
  PENDING: "pendente",
  APPROVED: "aprovado",
  PROCESSED: "processado",
  CANCELLED: "cancelado",
};

const ROYALTY_STATUS: Record<string, LancamentoStatus> = {
  DRAFT: "pendente",
  PENDING_REVIEW: "pendente",
  APPROVED: "aprovado",
  PAID: "pago",
  FAILED: "cancelado",
  CANCELLED: "cancelado",
};

const TRANSACTION_STATUS: Record<string, LancamentoStatus> = {
  PENDING: "pendente",
  PAID: "pago",
  OVERDUE: "vencido",
  CANCELLED: "cancelado",
  REVERSED: "revertido",
  CANCELADO: "cancelado",
  ESTORNADO: "revertido",
  PENDENTE: "pendente",
  PAGO: "pago",
  "EM ATRASO": "vencido",
};

const PTBR_STATUS: Record<string, LancamentoStatus> = {
  PENDENTE: "pendente",
  APROVADO: "aprovado",
  APROVADA: "aprovado",
  PAGO: "pago",
  PAGA: "pago",
  CONCLUIDO: "pago",
  CONCLUÍDO: "pago",
  PROCESSADO: "processado",
  PROCESSADA: "processado",
  CANCELADO: "cancelado",
  CANCELADA: "cancelado",
  ESTORNADO: "revertido",
  ESTORNADA: "revertido",
  REVERTIDO: "revertido",
  REVERTIDA: "revertido",
  REVERSADO: "revertido",
  REVERSADA: "revertido",
  VENCIDO: "vencido",
  VENCIDA: "vencido",
  "EM ATRASO": "vencido",
  PARCIAL: "parcial",
};

const KIND_MAPS: Record<LancamentoKind, Record<string, LancamentoStatus>> = {
  charge: CHARGE_STATUS,
  commission: COMMISSION_STATUS,
  transfer: TRANSFER_STATUS,
  royalty: ROYALTY_STATUS,
  transaction: TRANSACTION_STATUS,
};

export function normalizeStatus(kind: LancamentoKind, raw: string | null | undefined): LancamentoStatus | null {
  if (!raw) return null;
  const key = String(raw).trim().toUpperCase();
  return KIND_MAPS[kind][key] ?? PTBR_STATUS[key] ?? null;
}

export function normalizeStatusOrDefault(kind: LancamentoKind, raw: string | null | undefined): LancamentoStatus {
  return normalizeStatus(kind, raw) ?? "pendente";
}

const STATUS_LABELS: Record<LancamentoStatus, string> = {
  pendente: "Pendente",
  aprovado: "Aprovado",
  pago: "Pago",
  processado: "Processado",
  cancelado: "Cancelado",
  vencido: "Vencido",
  parcial: "Parcial",
  revertido: "Reversada",
};

export function statusLabel(status: LancamentoStatus): string {
  return STATUS_LABELS[status];
}

export interface StatusBadgeVariant {
  bg: string;
  text: string;
  border: string;
  iconName: "clock" | "check" | "alert" | "x" | null;
}

const BADGE_VARIANTS: Record<LancamentoStatus, StatusBadgeVariant> = {
  pendente: {
    bg: "bg-status-warning-soft",
    text: "text-status-warning",
    border: "border-status-warning/30",
    iconName: "clock",
  },
  aprovado: {
    bg: "bg-status-warning-soft",
    text: "text-status-warning",
    border: "border-status-warning/30",
    iconName: "clock",
  },
  parcial: {
    bg: "bg-status-warning-soft",
    text: "text-status-warning",
    border: "border-status-warning/30",
    iconName: "clock",
  },
  pago: {
    bg: "bg-status-ok-soft",
    text: "text-status-ok",
    border: "border-status-ok/30",
    iconName: "check",
  },
  processado: {
    bg: "bg-status-ok-soft",
    text: "text-status-ok",
    border: "border-status-ok/30",
    iconName: "check",
  },
  vencido: {
    bg: "bg-status-critical-soft",
    text: "text-status-critical",
    border: "border-status-critical/30",
    iconName: "alert",
  },
  cancelado: {
    bg: "bg-muted",
    text: "text-muted-foreground",
    border: "border-border",
    iconName: "x",
  },
  revertido: {
    bg: "bg-status-critical-soft",
    text: "text-status-critical",
    border: "border-status-critical/30",
    iconName: "x",
  },
};

export function statusBadgeVariant(status: LancamentoStatus): StatusBadgeVariant {
  return BADGE_VARIANTS[status];
}

export function availableActions(lancamento: Lancamento, permissions: Set<string>): LancamentoAction[] {
  const actions: LancamentoAction[] = [];
  const has = (p: string) => permissions.has(p);
  const { kind, status } = lancamento;

  if (kind === "charge") {
    if (status === "pendente" || status === "vencido" || status === "parcial") {
      if (has("finance:write")) {
        actions.push("markPaid", "notify", "cancel");
      }
    }
    if (status === "pago" || status === "parcial") {
      actions.push("viewReceipt");
    }
  }

  if (kind === "commission") {
    if (status === "pendente" && has("finance:write")) {
      actions.push("approve", "editDeductions", "cancel");
    }
    if (status === "aprovado" && has("finance:write")) {
      actions.push("markPaid", "editDeductions", "cancel");
    }
    if (status === "pago") {
      actions.push("viewReceipt");
    }
  }

  if (kind === "transfer") {
    if (status === "pendente" && has("finance:write")) {
      actions.push("cancel");
    }
    if (status === "pendente" && has("finance:approve_transfer")) {
      actions.push("approve");
    }
    if (status === "aprovado" && has("finance:approve_transfer")) {
      actions.push("processTransfer");
    }
    if (status === "processado") {
      actions.push("viewReceipt");
    }
  }

  if (kind === "royalty") {
    if (status === "pendente" && has("finance:approve_transfer")) {
      actions.push("approve", "cancel");
    }
    if (status === "aprovado" && has("finance:write")) {
      actions.push("markPaid", "cancel");
    }
    if (status === "pago") {
      actions.push("viewReceipt");
    }
  }

  if (kind === "transaction") {
    // Manual journal entry (entrada/saída): a single baixa marks it Pago.
    if ((status === "pendente" || status === "vencido") && has("finance:write")) {
      actions.push("settle");
    }
  }

  return actions;
}
