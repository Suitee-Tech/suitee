import { useEffect, useId, useMemo, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AlertCircle, CalendarDays, Droplet, Receipt } from "lucide-react";
import { toast } from "sonner";
import type { Contract } from "../contracts/types";
import type { UtilityAccount } from "../services/utilityAccount.service";
import { utilityChargeService } from "../services/utilityCharge.service";
import { contractsKeys } from "../contracts/queries";
import { utilityAccountsKey } from "../hooks/useContracts";
import { formatMoneyBR, parseMoneyBR, formatCompetencia } from "../utils/format";
import { Button } from "../ui/button";
import { MoneyInput } from "../ui/money-input";
import { ExpandableSidePanel } from "./ExpandableSidePanel";

interface UtilityChargeModalProps {
  contract: Contract;
  account: UtilityAccount;
  accountLabel: string;
  onClose: () => void;
}

function defaultDueDate(dueDay?: number): string {
  const now = new Date();
  const safeDay = Math.min(Math.max(dueDay || 10, 1), 28);
  const date = new Date(now.getFullYear(), now.getMonth(), safeDay);
  if (date < new Date(now.getFullYear(), now.getMonth(), now.getDate())) {
    date.setMonth(date.getMonth() + 1);
  }
  return date.toISOString().slice(0, 10);
}

export function UtilityChargeModal({ contract, account, accountLabel, onClose }: UtilityChargeModalProps) {
  const queryClient = useQueryClient();
  const fieldId = useId();
  const [referenceMonth, setReferenceMonth] = useState(() => new Date().toISOString().slice(0, 7));
  const [dueDate, setDueDate] = useState(() => defaultDueDate(account.due_day));
  const [amount, setAmount] = useState(() => String((account.estimated_amount_cents || 0) / 100));
  const [description, setDescription] = useState("");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setReferenceMonth(new Date().toISOString().slice(0, 7));
    setDueDate(defaultDueDate(account.due_day));
    setAmount(String((account.estimated_amount_cents || 0) / 100));
    setDescription("");
    setError(null);
  }, [account.id, account.due_day, account.estimated_amount_cents]);

  const consequence = useMemo(() => {
    switch (account.responsible_party) {
      case "OWNER":
        return {
          title: "Dedução no repasse do proprietário",
          description: "O valor será abatido do repasse da mesma competência. O repasse precisa existir para receber a dedução.",
        };
      case "AGENCY":
        return {
          title: "Despesa da imobiliária",
          description: "O lançamento será registrado como despesa operacional da imobiliária, sem cobrar o inquilino.",
        };
      default:
        return {
          title: "Cobrança separada ao inquilino",
          description: "Será criada uma cobrança própria, independente do aluguel mensal e identificada pela competência.",
        };
    }
  }, [account.responsible_party]);

  const emitMutation = useMutation({
    mutationFn: () => {
      const parsedAmount = parseMoneyBR(amount) ?? 0;
      return utilityChargeService.emit(account.id, {
        reference_month: referenceMonth,
        amount_cents: Math.round(parsedAmount * 100),
        due_date: new Date(`${dueDate}T12:00:00Z`).toISOString(),
        description: description.trim() || undefined,
      });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: contractsKeys.charges(contract.id) });
      queryClient.invalidateQueries({ queryKey: contractsKeys.transfers(contract.id) });
      queryClient.invalidateQueries({ queryKey: utilityAccountsKey(contract.id) });
      toast.success("Valor mensal registrado", { description: consequence.title });
      onClose();
    },
    onError: (requestError: any) => {
      toast.error(requestError?.response?.data?.message ?? "Não foi possível registrar o valor desta conta");
    },
  });

  const handleSubmit = () => {
    setError(null);
    if (account.billing_mode !== "VARIABLE_SEPARATE") {
      setError("Esta conta não está configurada como cobrança variável separada.");
      return;
    }
    if (!/^\d{4}-\d{2}$/.test(referenceMonth)) {
      setError("Informe uma competência válida.");
      return;
    }
    if (!dueDate) {
      setError("Informe o vencimento.");
      return;
    }
    if ((parseMoneyBR(amount) ?? 0) <= 0) {
      setError("O valor deve ser maior que zero.");
      return;
    }
    emitMutation.mutate();
  };

  return (
    <ExpandableSidePanel
      open
      onClose={onClose}
      defaultSize="2xl"
      expandedSize="4xl"
      bgClass="bg-muted"
      renderHeader={({ expandButton, closeButton }) => (
        <div className="flex items-center justify-between gap-4 border-b border-border bg-card px-6 py-4">
          <div className="flex min-w-0 items-center gap-3">
            <div className="rounded-xl bg-primary-50 p-2 text-primary-700"><Droplet className="size-5" /></div>
            <div className="min-w-0">
              <h2 className="truncate text-lg font-bold text-foreground">Lançar valor da conta</h2>
              <p className="truncate text-sm text-muted-foreground">{accountLabel} · {account.provider}</p>
            </div>
          </div>
          <div className="flex items-center gap-2">{expandButton}{closeButton}</div>
        </div>
      )}
    >
      <div className="flex flex-1 flex-col overflow-hidden">
        <div className="flex-1 space-y-6 overflow-y-auto p-6">
          <div className="rounded-xl border border-primary-100 bg-primary-50 p-4">
            <div className="flex items-start gap-3">
              <Receipt className="mt-0.5 size-5 shrink-0 text-primary-700" />
              <div>
                <h3 className="text-sm font-bold text-primary-900">{consequence.title}</h3>
                <p className="mt-1 text-xs leading-relaxed text-primary-800">{consequence.description}</p>
              </div>
            </div>
          </div>

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div>
              <label htmlFor={`${fieldId}-reference-month`} className="mb-1 block text-xs font-medium text-foreground">Competência <span aria-hidden="true" className="text-status-critical">*</span></label>
              <input
                id={`${fieldId}-reference-month`}
                type="month"
                aria-required="true"
                value={referenceMonth}
                onChange={(event) => setReferenceMonth(event.target.value)}
                className="h-11 w-full rounded-xl border border-border bg-card px-3 text-sm outline-none focus:border-primary-600 focus:ring-2 focus:ring-primary-600/20"
              />
              <p className="mt-1 text-[11px] text-muted-foreground">Também impede duplicidade para a mesma conta e mês.</p>
            </div>
            <div>
              <label htmlFor={`${fieldId}-due-date`} className="mb-1 block text-xs font-medium text-foreground">Vencimento <span aria-hidden="true" className="text-status-critical">*</span></label>
              <div className="relative">
                <input
                  id={`${fieldId}-due-date`}
                  type="date"
                  aria-required="true"
                  value={dueDate}
                  onChange={(event) => setDueDate(event.target.value)}
                  className="h-11 w-full rounded-xl border border-border bg-card px-3 pr-10 text-sm outline-none focus:border-primary-600 focus:ring-2 focus:ring-primary-600/20"
                />
                <CalendarDays className="pointer-events-none absolute right-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
              </div>
            </div>
            <div className="sm:col-span-2">
              <label htmlFor={`${fieldId}-amount`} className="mb-1 block text-xs font-medium text-foreground">Valor efetivo <span aria-hidden="true" className="text-status-critical">*</span></label>
              <MoneyInput
                id={`${fieldId}-amount`}
                aria-required="true"
                withSymbol
                value={amount === "" ? undefined : (parseMoneyBR(amount) ?? undefined)}
                onChange={(value) => setAmount(value === undefined ? "" : String(value))}
                placeholder="R$ 0,00"
                className="h-11 w-full rounded-xl border border-border bg-card px-3 text-sm outline-none focus:border-primary-600 focus:ring-2 focus:ring-primary-600/20"
              />
              <p className="mt-1 text-[11px] text-muted-foreground">
                Estimativa cadastrada: {formatMoneyBR((account.estimated_amount_cents || 0) / 100)}. Informe o valor da fatura do mês.
              </p>
            </div>
            <div className="sm:col-span-2">
              <label htmlFor={`${fieldId}-description`} className="mb-1 block text-xs font-medium text-foreground">Descrição no lançamento</label>
              <input
                id={`${fieldId}-description`}
                value={description}
                onChange={(event) => setDescription(event.target.value)}
                placeholder={`${accountLabel} — ${referenceMonth}`}
                className="h-11 w-full rounded-xl border border-border bg-card px-3 text-sm outline-none focus:border-primary-600 focus:ring-2 focus:ring-primary-600/20"
              />
            </div>
          </div>

          {error && (
            <div role="alert" className="flex items-start gap-2 rounded-xl border border-status-critical/30 bg-status-critical-soft p-3 text-xs font-medium text-status-critical">
              <AlertCircle className="mt-0.5 size-4 shrink-0" /> {error}
            </div>
          )}
        </div>

        <div className="flex justify-end gap-3 border-t border-border bg-card px-6 py-4">
          <Button variant="outline" onClick={onClose}>Cancelar</Button>
          <Button onClick={handleSubmit} loading={emitMutation.isPending} loadingText="Registrando…">
            Registrar valor de {formatCompetencia(referenceMonth) || "competência"}
          </Button>
        </div>
      </div>
    </ExpandableSidePanel>
  );
}
