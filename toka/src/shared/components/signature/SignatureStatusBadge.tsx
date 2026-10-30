import { clsx } from "clsx";
import {
  AlarmClockOff,
  AlertTriangle,
  Ban,
  CheckCircle2,
  Clock,
  Eye,
  FileCheck,
  FileText,
  Hourglass,
  XCircle,
  type LucideIcon,
} from "lucide-react";
import type { ReactNode } from "react";
import type { StatusTone } from "../../tokens/types";
import type { SignatureEnvelopeStatus, SignatureSignerStatus } from "../../contracts/types";

type StatusEntry = { label: string; tone: StatusTone; Icon: LucideIcon };

const ENVELOPE_STATUS: Record<SignatureEnvelopeStatus, StatusEntry> = {
  PENDING: { label: "Preparando", tone: "neutral", Icon: Clock },
  DRAFT: { label: "Rascunho", tone: "neutral", Icon: FileText },
  RUNNING: { label: "Aguardando assinatura", tone: "warning", Icon: Hourglass },
  PARTIALLY_SIGNED: { label: "Parcialmente assinado", tone: "info", Icon: FileCheck },
  FINISHED: { label: "Assinado", tone: "ok", Icon: CheckCircle2 },
  REFUSED: { label: "Recusado", tone: "critical", Icon: XCircle },
  CANCELED: { label: "Cancelado", tone: "neutral", Icon: Ban },
  EXPIRED: { label: "Expirado", tone: "critical", Icon: AlarmClockOff },
  ERROR: { label: "Erro", tone: "critical", Icon: AlertTriangle },
};

const SIGNER_STATUS: Record<SignatureSignerStatus, StatusEntry> = {
  PENDING: { label: "Pendente", tone: "warning", Icon: Clock },
  VIEWED: { label: "Visualizado", tone: "info", Icon: Eye },
  SIGNED: { label: "Assinou", tone: "ok", Icon: CheckCircle2 },
  REFUSED: { label: "Recusou", tone: "critical", Icon: XCircle },
};

type ContractLikeStatus =
  | SignatureEnvelopeStatus
  | "PENDING_SIGNATURE"
  | "SIGNED"
  | "ACTIVE"
  | "TERMINATED";

const CONTRACT_OVERRIDES: Partial<Record<ContractLikeStatus, StatusEntry>> = {
  PENDING_SIGNATURE: { label: "Aguardando assinatura", tone: "warning", Icon: Hourglass },
  SIGNED: { label: "Assinado", tone: "ok", Icon: CheckCircle2 },
  ACTIVE: { label: "Ativo", tone: "info", Icon: CheckCircle2 },
  TERMINATED: { label: "Encerrado", tone: "neutral", Icon: Ban },
};

const SIGNER_ALIASES: Record<string, SignatureSignerStatus> = {
  REJECTED: "REFUSED",
  ASSINADO: "SIGNED",
  PENDENTE: "PENDING",
};

const TONE_CLASS: Record<StatusTone, string> = {
  ok: "bg-status-ok-soft text-status-ok border-status-ok/30",
  pending: "bg-status-pending-soft text-status-pending border-status-pending/30",
  warning: "bg-status-warning-soft text-status-warning border-status-warning/30",
  critical: "bg-status-critical-soft text-status-critical border-status-critical/30",
  info: "bg-status-info-soft text-status-info border-status-info/30",
  neutral: "bg-muted text-muted-foreground border-border",
};

type Size = "sm" | "md";
const SIZE_CLASS: Record<Size, string> = {
  sm: "text-[11px] font-bold px-2 py-0.5 gap-1",
  md: "text-xs font-bold px-2.5 py-1 gap-1.5",
};
const ICON_SIZE: Record<Size, string> = {
  sm: "w-3 h-3",
  md: "w-3.5 h-3.5",
};

function resolveEntry(kind: "envelope" | "signer" | "contract", raw: string | undefined): StatusEntry | null {
  if (!raw) return null;
  const key = raw.toUpperCase();
  if (kind === "envelope") {
    return ENVELOPE_STATUS[key as SignatureEnvelopeStatus] ?? null;
  }
  if (kind === "signer") {
    const aliased = SIGNER_ALIASES[key] ?? key;
    return SIGNER_STATUS[aliased as SignatureSignerStatus] ?? null;
  }
  return CONTRACT_OVERRIDES[key as ContractLikeStatus] ?? ENVELOPE_STATUS[key as SignatureEnvelopeStatus] ?? null;
}

export function SignatureStatusBadge({
  status,
  kind = "envelope",
  size = "md",
  hideIcon,
  className,
  fallback,
}: {
  status: string | undefined;
  kind?: "envelope" | "signer" | "contract";
  size?: Size;
  hideIcon?: boolean;
  className?: string;
  fallback?: ReactNode;
}) {
  const entry = resolveEntry(kind, status);
  if (!entry) {
    if (fallback !== undefined) return <>{fallback}</>;
    return status ? <span className={className}>{status}</span> : null;
  }
  const { label, tone, Icon } = entry;
  return (
    <span className={clsx("inline-flex items-center rounded-lg border", SIZE_CLASS[size], TONE_CLASS[tone], className)}>
      {!hideIcon && <Icon className={ICON_SIZE[size]} aria-hidden="true" />}
      {label}
    </span>
  );
}

export function getSignatureStatusEntry(kind: "envelope" | "signer" | "contract", status: string | undefined): StatusEntry | null {
  return resolveEntry(kind, status);
}

