import { useState, useEffect, useMemo } from "react";
import { ShieldAlert, TrendingUp, Loader2 } from "lucide-react";
import { clsx } from "clsx";
import { toast } from "sonner";
import type { Contract } from "../contracts/types";
import {
  contractService,
  TerminateContractRequest,
  ReadjustContractRequest,
  AmendContractRequest,
  TerminationFeeBreakdown,
} from "../services/contract.service";
import { Dialog, DialogContent, DialogTitle } from "../ui/dialog";
import { MoneyInput } from "../ui/money-input";
import { PercentInput } from "../ui/percent-input";

interface BaseModalProps {
  open: boolean;
  onClose: () => void;
  contract: Contract;
  onCompleted?: (updated: Contract) => void;
}

const formatCurrency = (val: number) =>
  new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL" }).format(val || 0);

function ModalShell({
  open,
  onClose,
  title,
  icon: Icon,
  iconClass,
  children,
}: {
  open: boolean;
  onClose: () => void;
  title: string;
  icon: React.ComponentType<{ className?: string }>;
  iconClass: string;
  children: React.ReactNode;
}) {
  return (
    <Dialog open={open} onOpenChange={(next) => { if (!next) onClose(); }}>
      <DialogContent className="gap-0 overflow-hidden p-0 sm:max-w-lg">
        <div className="flex items-start gap-4 px-6 pt-6 pr-12">
          <div className={clsx("p-3 rounded-xl flex-shrink-0", iconClass)}>
            <Icon className="w-6 h-6" />
          </div>
          <div className="flex-1 min-w-0">
            <DialogTitle className="text-lg font-bold text-foreground">{title}</DialogTitle>
          </div>
        </div>
        {children}
      </DialogContent>
    </Dialog>
  );
}

export function TerminateContractModal({ open, onClose, contract, onCompleted }: BaseModalProps) {
  const [reason, setReason] = useState("");
  const [fee, setFee] = useState<number>(0);
  const [depositReturn, setDepositReturn] = useState<number>(contract.security_deposit || 0);
  const [deductions, setDeductions] = useState<number>(0);
  const [requireExit, setRequireExit] = useState(true);
  const isRental = (contract.type || "").toLowerCase() === "locacao";
  const [submitting, setSubmitting] = useState(false);
  const [previewBreakdown, setPreviewBreakdown] = useState<TerminationFeeBreakdown | null>(null);
  const [previewLoading, setPreviewLoading] = useState(false);

  useEffect(() => {
    if (open) {
      setReason("");
      setFee(0);
      setDepositReturn(contract.security_deposit || 0);
      setDeductions(0);
      setRequireExit(true);
      setPreviewLoading(true);
      contractService
        .previewTerminationFee(contract.id)
        .then((b) => setPreviewBreakdown(b))
        .catch(() => setPreviewBreakdown(null))
        .finally(() => setPreviewLoading(false));
    }
  }, [open, contract.id, contract.security_deposit]);

  const suggestedFee = useMemo(() => {
    if (!previewBreakdown) return 0;
    return Math.round(previewBreakdown.fee_cents) / 100;
  }, [previewBreakdown]);

  const previewExplanation = useMemo(() => {
    if (!previewBreakdown) return "";
    if (previewBreakdown.reason === "contract_already_ended") {
      return "Contrato já cumpriu o prazo — multa zero.";
    }
    if (previewBreakdown.reason === "no_monthly_rent") {
      return "Aluguel zero — multa zero.";
    }
    if (previewBreakdown.reason === "no_end_date") {
      return "Contrato sem prazo definido — sugerido o teto de 3 alugueis.";
    }
    const { months_remaining, total_months } = previewBreakdown;
    if (total_months > 0) {
      return `3 alugueis × ${months_remaining}/${total_months} meses restantes (Lei 8245/91 Art. 4)`;
    }
    return "";
  }, [previewBreakdown]);

  const netReturn = Math.max(0, depositReturn - deductions);

  const handleSubmit = async () => {
    if (!reason.trim()) {
      toast.error("Informe o motivo do encerramento.");
      return;
    }
    setSubmitting(true);
    try {
      const req: TerminateContractRequest = {
        reason: reason.trim(),
        early_termination_fee: fee || 0,
        security_deposit_return: isRental ? netReturn : 0,
        security_deposit_deductions: isRental ? deductions || 0 : 0,
        require_exit_inspection: isRental ? requireExit : false,
      };
      const updated = await contractService.terminate(contract.id, req);
      toast.success("Contrato encerrado. Cobranças futuras serão canceladas.");
      onCompleted?.(updated);
      onClose();
    } catch (err: any) {
      const status = err?.response?.status;
      const message =
        status === 409
          ? "Vistoria de saída obrigatória — registre uma vistoria EXIT concluída com laudo antes de encerrar."
          : err?.response?.data?.message || "Erro ao encerrar contrato.";
      toast.error(message);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <ModalShell
      open={open}
      onClose={onClose}
      title="Encerrar Contrato"
      icon={ShieldAlert}
      iconClass="bg-status-critical-soft text-status-critical"
    >
      <div className="px-6 py-5 space-y-4">
        <p className="text-sm text-muted-foreground">
          Ao confirmar, o contrato será marcado como{" "}
          <span className="font-semibold text-status-critical">TERMINATED</span>. Cobranças futuras pendentes serão
          canceladas. Cobranças já pagas e despesas lançadas permanecem no histórico.
        </p>

        <div>
          <label className="block text-xs font-bold text-foreground uppercase tracking-wider mb-1">
            Motivo do encerramento *
          </label>
          <textarea
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            placeholder="Ex.: locatário entregou as chaves; rescisão acordada"
            rows={2}
            className="w-full px-4 py-2.5 bg-muted border border-border rounded-xl text-sm focus:bg-card focus:ring-2 focus:ring-primary-500 outline-none resize-none"
          />
        </div>

        <div className={isRental ? "grid grid-cols-2 gap-3" : ""}>
          <div>
            <label className="block text-xs font-bold text-foreground uppercase tracking-wider mb-1">
              Multa por rescisão (R$)
            </label>
            <MoneyInput
              withSymbol
              placeholder="R$ 0,00"
              value={typeof fee === 'number' ? fee : undefined}
              onChange={(v) => setFee(v ?? 0)}
              className="w-full px-4 py-2.5 bg-muted border border-border rounded-xl text-sm focus:bg-card focus:ring-2 focus:ring-primary-500 outline-none"
            />
            {previewLoading ? (
              <p className="mt-1 text-xs text-muted-foreground flex items-center gap-1">
                <Loader2 className="w-3 h-3 animate-spin" /> Calculando...
              </p>
            ) : suggestedFee > 0 ? (
              <>
                <button
                  type="button"
                  onClick={() => setFee(suggestedFee)}
                  className="mt-1 text-xs text-primary-600 hover:text-primary-700 underline"
                >
                  Sugerir {formatCurrency(suggestedFee)}
                </button>
                {previewExplanation && <p className="mt-0.5 text-[11px] text-muted-foreground">{previewExplanation}</p>}
              </>
            ) : previewExplanation ? (
              <p className="mt-1 text-xs text-status-ok">{previewExplanation}</p>
            ) : null}
          </div>

          {isRental && (
            <div>
              <label className="block text-xs font-bold text-foreground uppercase tracking-wider mb-1">
                Devolução de caução (R$)
              </label>
              <MoneyInput
                withSymbol
                placeholder="R$ 0,00"
                value={typeof depositReturn === 'number' ? depositReturn : undefined}
                onChange={(v) => setDepositReturn(v ?? 0)}
                className="w-full px-4 py-2.5 bg-muted border border-border rounded-xl text-sm focus:bg-card focus:ring-2 focus:ring-primary-500 outline-none"
              />
            </div>
          )}
        </div>

        {isRental && (
          <div>
            <label className="block text-xs font-bold text-foreground uppercase tracking-wider mb-1">
              Descontos / danos identificados (R$)
            </label>
            <MoneyInput
              withSymbol
              placeholder="R$ 0,00"
              value={typeof deductions === 'number' ? deductions : undefined}
              onChange={(v) => setDeductions(v ?? 0)}
              className="w-full px-4 py-2.5 bg-muted border border-border rounded-xl text-sm focus:bg-card focus:ring-2 focus:ring-primary-500 outline-none"
            />
            <p className="text-xs text-muted-foreground mt-1">
              Líquido a devolver: <span className="font-semibold text-foreground">{formatCurrency(netReturn)}</span>
            </p>
          </div>
        )}

        {isRental && (
          <label className="flex items-start gap-3 cursor-pointer">
            <input
              type="checkbox"
              checked={requireExit}
              onChange={(e) => setRequireExit(e.target.checked)}
              className="mt-0.5"
            />
            <div>
              <p className="text-sm font-medium text-foreground">Exigir vistoria de saída concluída</p>
              <p className="text-xs text-muted-foreground">
                Bloqueia o encerramento até existir vistoria EXIT com status "Concluída" e laudo anexado.
              </p>
            </div>
          </label>
        )}
      </div>

      <div className="px-6 py-4 border-t border-border bg-muted/40 flex items-center justify-end gap-3">
        <button
          onClick={onClose}
          disabled={submitting}
          className="px-5 py-2.5 border border-border text-sm font-bold rounded-xl text-foreground hover:bg-muted transition-colors disabled:opacity-50"
        >
          Cancelar
        </button>
        <button
          onClick={handleSubmit}
          disabled={submitting || !reason.trim()}
          className="px-6 py-2.5 bg-status-critical/90 hover:bg-status-critical text-white text-sm font-bold rounded-xl transition-colors shadow-sm flex items-center gap-2 disabled:opacity-50"
        >
          {submitting ? <Loader2 className="w-4 h-4 animate-spin" /> : <ShieldAlert className="w-4 h-4" />}
          Encerrar Contrato
        </button>
      </div>
    </ModalShell>
  );
}

// O modal de reajuste saiu daqui: aplicava o reajuste direto no clique, sem
// confirmacao, e virou ReadjustContractPanel (painel lateral + AlertDialog).
