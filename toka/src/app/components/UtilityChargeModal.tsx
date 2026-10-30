import { useState, type FormEvent } from "react";
import { Zap } from "lucide-react";
import { toast } from "sonner";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogTitle,
} from "./ui/dialog";
import { Button } from "./ui/button";
import { formatMoneyBR, parseMoneyBR } from "../utils/format";
import { utilityChargeService } from "../services/utilityCharge.service";
import { UtilityAccount } from "../services/utilityAccount.service";
import { notifyApiError } from "../../shared/services/errorToast";

interface UtilityChargeModalProps {
  account: UtilityAccount;
  onClose: () => void;
  onSubmitted: () => void;
}

// Emits a standalone TypeUtility charge from a VARIABLE_SEPARATE
// UtilityAccount. The wizard collects the consumption month (defaults to
// current), the amount the provider billed and the due date the operator
// wants for the tenant.
export function UtilityChargeModal({ account, onClose, onSubmitted }: UtilityChargeModalProps) {
  const today = new Date();
  const defaultMonth = `${today.getFullYear()}-${String(today.getMonth() + 1).padStart(2, "0")}`;
  const defaultDue = (() => {
    const d = new Date(today);
    d.setDate(account.due_day && account.due_day > 0 ? account.due_day : today.getDate() + 7);
    return d.toISOString().slice(0, 10);
  })();

  const [referenceMonth, setReferenceMonth] = useState(defaultMonth);
  const [amount, setAmount] = useState(formatMoneyBR((account.estimated_amount_cents || 0) / 100));
  const [dueDate, setDueDate] = useState(defaultDue);
  const [description, setDescription] = useState("");
  const [saving, setSaving] = useState(false);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    // parseMoneyBR returns number | null. Coalesce once so the rest of
    // the flow operates on a non-null amount; the explicit > 0 check
    // below still rejects empty/zero inputs.
    const value = parseMoneyBR(amount) ?? 0;
    if (value <= 0) {
      toast.error("Informe o valor do consumo.");
      return;
    }
    setSaving(true);
    try {
      await utilityChargeService.emit(account.id, {
        reference_month: referenceMonth,
        amount_cents: Math.round(value * 100),
        due_date: new Date(dueDate).toISOString(),
        description: description.trim() || undefined,
      });
      toast.success("Cobrança emitida.");
      onSubmitted();
      onClose();
    } catch (err: any) {
      notifyApiError(err, { context: "Emitir cobrança" });
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open onOpenChange={(next) => { if (!next) onClose(); }}>
      <DialogContent className="flex max-h-[90vh] flex-col gap-0 overflow-hidden p-0 sm:max-w-md">
        <div className="flex items-center gap-3 border-b border-border bg-muted/50 px-6 py-4 pr-12">
          <div className="p-2 bg-amber-100 rounded-xl">
            <Zap className="w-5 h-5 text-amber-600" />
          </div>
          <div>
            <DialogTitle className="text-lg font-bold">Lançar consumo</DialogTitle>
            <DialogDescription className="text-xs">
              {account.type} — {account.provider}
            </DialogDescription>
          </div>
        </div>
        <form onSubmit={handleSubmit} className="flex-1 overflow-y-auto p-6 space-y-4">
          <div>
            <label className="block text-xs font-medium mb-1">Mês de referência *</label>
            <input
              type="month"
              required
              value={referenceMonth}
              onChange={(e) => setReferenceMonth(e.target.value)}
              className="w-full px-3 py-2 bg-card border border-border rounded-xl text-sm"
            />
          </div>
          <div>
            <label className="block text-xs font-medium mb-1">Valor do consumo *</label>
            <input
              type="text"
              required
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
              placeholder="R$ 0,00"
              className="w-full px-3 py-2 bg-card border border-border rounded-xl text-sm"
            />
          </div>
          <div>
            <label className="block text-xs font-medium mb-1">Data de vencimento *</label>
            <input
              type="date"
              required
              value={dueDate}
              onChange={(e) => setDueDate(e.target.value)}
              className="w-full px-3 py-2 bg-card border border-border rounded-xl text-sm"
            />
          </div>
          <div>
            <label className="block text-xs font-medium mb-1">Descrição (opcional)</label>
            <input
              type="text"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder={`${account.provider} — ${referenceMonth}`}
              className="w-full px-3 py-2 bg-card border border-border rounded-xl text-sm"
            />
          </div>
        </form>
        <DialogFooter className="border-t border-border bg-muted/30 px-6 py-3">
          <Button variant="ghost" onClick={onClose} disabled={saving}>
            Cancelar
          </Button>
          <Button onClick={handleSubmit as any} disabled={saving}>
            {saving ? "Emitindo..." : "Emitir cobrança"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
