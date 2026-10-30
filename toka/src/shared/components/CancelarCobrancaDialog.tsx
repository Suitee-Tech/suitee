import { useEffect, useState, type FormEvent, type ReactNode } from "react";
import { CheckCircle2, ShieldAlert } from "lucide-react";

import { AlertDialog, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogTitle } from "../ui/alert-dialog";
import { Button } from "../ui/button";

interface CancelarCobrancaDialogProps {
  open: boolean;
  title?: string;
  description?: string;
  contextLine?: string;
  confirmLabel?: string;
  loadingText?: string;
  /** Painel de efeitos financeiros/contábeis (ex.: <SettlementEffects />). */
  effects?: ReactNode;
  /** Se falso, não exige motivo (usado em baixas/aprovações simples). */
  requireReason?: boolean;
  minReasonLength?: number;
  /** "destructive" (cancela/estorna) ou "default" (baixa/aprova/processa). */
  variant?: "destructive" | "default";
  /** Alarga o diálogo para acomodar o painel de efeitos. */
  wide?: boolean;
  onConfirm: (reason: string) => Promise<void>;
  onCancel: () => void;
}

const DEFAULT_MIN_REASON = 5;

export function CancelarCobrancaDialog({
  open,
  title = "Cancelar Cobrança",
  description = "Esta ação não pode ser desfeita. O link de pagamento será desativado e o lançamento ficará registrado como CANCELADO no histórico.",
  contextLine,
  confirmLabel = "Cancelar Cobrança",
  loadingText = "Cancelando...",
  effects,
  requireReason = true,
  minReasonLength = DEFAULT_MIN_REASON,
  variant = "destructive",
  wide = false,
  onConfirm,
  onCancel,
}: CancelarCobrancaDialogProps) {
  const [reason, setReason] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [touched, setTouched] = useState(false);

  useEffect(() => {
    if (!open) {
      setReason("");
      setSubmitting(false);
      setTouched(false);
    }
  }, [open]);

  const trimmed = reason.trim();
  const tooShort = trimmed.length < minReasonLength;
  const showError = requireReason && touched && tooShort;
  const destructive = variant === "destructive";

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (requireReason) {
      setTouched(true);
      if (tooShort) return;
    }
    setSubmitting(true);
    try {
      await onConfirm(requireReason ? trimmed : "");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <AlertDialog
      open={open}
      onOpenChange={(next) => {
        if (!next && !submitting) onCancel();
      }}
    >
      <AlertDialogContent className={wide ? "gap-4 sm:max-w-lg" : "gap-4"}>
        <div className="flex items-start gap-4">
          <div className={destructive ? "shrink-0 rounded-xl bg-status-critical-soft p-3" : "shrink-0 rounded-xl bg-status-ok-soft p-3"}>
            {destructive ? <ShieldAlert className="h-6 w-6 text-status-critical" /> : <CheckCircle2 className="h-6 w-6 text-status-ok" />}
          </div>
          <div className="min-w-0 flex-1">
            <AlertDialogTitle className="text-lg font-bold text-foreground">{title}</AlertDialogTitle>
            {contextLine && <p className="mt-1 truncate text-sm font-medium text-foreground/80">{contextLine}</p>}
            {description && <AlertDialogDescription className="mt-2">{description}</AlertDialogDescription>}
          </div>
        </div>

        {effects && <div className="mt-1">{effects}</div>}

        <form onSubmit={handleSubmit} className="space-y-2">
          {requireReason && (
            <>
              <label htmlFor="cancel-reason" className="block text-sm font-bold text-foreground">
                Motivo *
              </label>
              <textarea
                id="cancel-reason"
                value={reason}
                onChange={(e) => setReason(e.target.value)}
                onBlur={() => setTouched(true)}
                rows={3}
                placeholder="Ex: erro de digitação no valor, renegociação acordada, duplicado…"
                className={`w-full resize-y rounded-xl border bg-muted px-3 py-2 text-sm outline-none transition-all focus:bg-card ${
                  showError
                    ? "border-status-critical focus:ring-2 focus:ring-status-critical/20"
                    : "border-border focus:border-primary-500 focus:ring-2 focus:ring-primary-500/20"
                }`}
                autoFocus
                disabled={submitting}
              />
              {showError ? (
                <p className="text-xs text-status-critical">Descreva o motivo (mínimo {minReasonLength} caracteres) — ele entra no histórico de auditoria.</p>
              ) : (
                <p className="text-xs text-muted-foreground">O motivo é registrado no audit trail e fica visível a operadores futuros.</p>
              )}
            </>
          )}

          <AlertDialogFooter className="pt-2">
            <Button type="button" variant="outline" onClick={onCancel} disabled={submitting}>
              Voltar
            </Button>
            <Button
              type="submit"
              variant={destructive ? "destructive" : "success"}
              loading={submitting}
              loadingText={loadingText}
              disabled={requireReason && tooShort && touched}
            >
              {destructive && <ShieldAlert className="h-4 w-4" />} {confirmLabel}
            </Button>
          </AlertDialogFooter>
        </form>
      </AlertDialogContent>
    </AlertDialog>
  );
}
