import { useEffect, useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { FileText, CheckCircle2, Save, AlertCircle, Clock, Pencil } from "lucide-react";
import { motion } from "motion/react";
import { clsx } from "clsx";
import type { Transaction } from "../types/transaction";
import { useContractsContext } from "../context/ContractsContext";
import { useFinanceContext } from "../context/FinanceContext";
import { toast } from "sonner";
import { ExpandableSidePanel } from "./ExpandableSidePanel";
import { Form } from "../ui/form";
import { DateField, MoneyField, SelectField, TextField } from "../ui/fields";

const CATEGORIES = ["Aluguel", "Condominio", "IPTU", "Energia", "Agua", "Gas", "Seguro", "Multa", "Outros"];

const PAYMENT_METHODS = ["PIX", "Boleto", "Transferencia", "Debito Automatico", "Cartao de Credito", "Dinheiro"];
const PAYMENT_METHOD_OPTIONS = PAYMENT_METHODS.map((m) => ({ value: m, label: m }));

const STATUS_OPTIONS: { value: "Pendente" | "Pago" | "Em Atraso"; label: string; color: string; icon: typeof Clock }[] = [
  { value: "Pendente", label: "Pendente", color: "bg-status-warning-soft text-status-warning border-status-warning/30", icon: Clock },
  { value: "Pago", label: "Pago / Concluido", color: "bg-status-ok-soft text-status-ok border-status-ok/30", icon: CheckCircle2 },
  { value: "Em Atraso", label: "Em Atraso", color: "bg-status-critical-soft text-status-critical border-status-critical/30", icon: AlertCircle },
];

const normalizeStatus = (s?: Transaction["status"]): "Pendente" | "Pago" | "Em Atraso" => {
  switch (s) {
    case "Pago":
    case "PAID":
      return "Pago";
    case "Em Atraso":
    case "OVERDUE":
      return "Em Atraso";
    default:
      return "Pendente";
  }
};

interface TransactionEditModalProps {
  transaction: Transaction | null;
  mode: "cobranca" | "repasse";
  onClose: () => void;
}

const schema = z.object({
  description: z.string(),
  value: z.number().positive("Valor inválido"),
  status: z.enum(["Pendente", "Pago", "Em Atraso"]),
  date: z.string(),
  dueDate: z.string(),
  paymentMethod: z.string(),
  paymentDate: z.string(),
  category: z.string(),
  month: z.string(),
  adminFee: z.number().optional(),
  maintenanceDeductions: z.number().optional(),
  utilityDeductions: z.number().optional(),
});
type FormValues = z.infer<typeof schema>;

const toDefaults = (t: Transaction | null): FormValues => ({
  description: t?.description || "",
  value: t ? Math.abs(t.value) : 0,
  status: normalizeStatus(t?.status),
  date: t?.date || "",
  dueDate: t?.dueDate || "",
  paymentMethod: t?.paymentMethod || "",
  paymentDate: t?.paymentDate || "",
  category: t?.category || "",
  month: t?.month || "",
  adminFee: t?.adminFee || undefined,
  maintenanceDeductions: t?.maintenanceDeductions || undefined,
  utilityDeductions: t?.utilityDeductions || undefined,
});

export function TransactionEditModal({ transaction, mode, onClose }: TransactionEditModalProps) {
  const { updateTransaction } = useFinanceContext();
  const { contracts } = useContractsContext();
  const [saving, setSaving] = useState(false);

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    mode: "onTouched",
    defaultValues: toDefaults(transaction),
  });

  useEffect(() => {
    if (transaction) form.reset(toDefaults(transaction));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [transaction]);

  if (!transaction) return null;

  const contract = contracts.find((c) => c.id === transaction.contractId);

  const formatCurrency = (val: number) => {
    if (isNaN(val) || val == null) return "R$ 0,00";
    return new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL" }).format(Math.abs(val));
  };

  const w = form.watch();
  const categoryOptions = [
    ...CATEGORIES.map((category) => ({ value: category, label: category })),
    ...(w.category && !CATEGORIES.includes(w.category)
      ? [{ value: w.category, label: `Categoria atual: ${w.category}` }]
      : []),
  ];
  const grossValue = transaction.grossValue || Math.abs(transaction.value);
  const computedNet =
    mode === "repasse"
      ? grossValue - (w.adminFee ?? 0) - (w.maintenanceDeductions ?? 0) - (w.utilityDeductions ?? 0)
      : (w.value ?? 0);

  const onSubmit = async (values: FormValues) => {
    setSaving(true);
    const updates: Partial<Transaction> = {
      description: values.description,
      value: values.value,
      status: values.status,
      date: values.date,
    };

    if (mode === "cobranca") {
      updates.dueDate = values.dueDate || undefined;
      updates.paymentMethod = values.status === "Pago" ? values.paymentMethod || null : null;
      updates.paymentDate = values.status === "Pago" ? values.paymentDate || null : null;
      updates.category = values.category || undefined;
    }

    if (mode === "repasse") {
      updates.month = values.month || undefined;
      updates.adminFee = values.adminFee;
      updates.maintenanceDeductions = values.maintenanceDeductions;
      updates.utilityDeductions = values.utilityDeductions;

      const gross = transaction.grossValue || values.value;
      const fee = values.adminFee ?? 0;
      const maint = values.maintenanceDeductions ?? 0;
      const util = values.utilityDeductions ?? 0;
      updates.value = gross - fee - maint - util;
      updates.grossValue = gross;
    }

    try {
      const applied = await updateTransaction(transaction.id, updates);
      if (!applied) return;
    } catch {
      return;
    } finally {
      setSaving(false);
    }

    toast.success("Transacao atualizada", { description: `${transaction.id} editado com sucesso.` });
    onClose();
  };

  return (
    <ExpandableSidePanel
      open={true}
      onClose={onClose}
      defaultSize="xl"
      expandedSize="full"
      renderHeader={({ expandButton, closeButton }) => (
        <div className="px-6 py-4 border-b border-border flex items-center justify-between bg-card flex-shrink-0">
          <div className="flex items-center gap-3">
            <div className={clsx("p-2.5 rounded-xl", mode === "cobranca" ? "bg-primary-50" : "bg-status-ok-soft")}>
              <Pencil className={clsx("w-5 h-5", mode === "cobranca" ? "text-primary-600" : "text-status-ok")} />
            </div>
            <div>
              <h2 className="text-lg font-bold text-foreground">Editar {mode === "cobranca" ? "Cobrança" : "Repasse"}</h2>
              <p className="text-xs text-muted-foreground">
                {transaction.id} {contract ? `- ${contract.client}` : ""}
              </p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            {expandButton}
            {closeButton}
          </div>
        </div>
      )}
    >
      <Form {...form}>
        <form onSubmit={form.handleSubmit(onSubmit)} className="flex flex-1 flex-col min-h-0">
          <div className="flex-1 overflow-y-auto p-6 space-y-6">
            {contract && (
              <div className="bg-muted rounded-xl p-4 border border-border flex items-center gap-4">
                <FileText className="w-5 h-5 text-muted-foreground" />
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-bold text-foreground truncate">{contract.property_title}</p>
                  <p className="text-xs text-muted-foreground">
                    {contract.id} - {contract.type === "locacao" ? "Locação" : "Venda"} - {contract.client}
                  </p>
                </div>
              </div>
            )}

            {/* Status (button group) */}
            <div>
              <label className="block text-sm font-bold text-foreground mb-2">Status</label>
              <div className="flex gap-2">
                {STATUS_OPTIONS.map((opt) => (
                  <button
                    key={opt.value}
                    type="button"
                    onClick={() => form.setValue("status", opt.value)}
                    className={clsx(
                      "flex-1 flex items-center justify-center gap-2 px-3 py-2.5 rounded-xl text-xs font-bold border transition-all",
                      w.status === opt.value
                        ? `${opt.color} ring-2 ring-offset-1 ${opt.value === "Pendente" ? "ring-amber-300" : opt.value === "Pago" ? "ring-emerald-300" : "ring-rose-300"}`
                        : "bg-card border-border text-muted-foreground hover:bg-muted",
                    )}
                  >
                    <opt.icon className="w-3.5 h-3.5" />
                    {opt.label}
                  </button>
                ))}
              </div>
            </div>

            <TextField control={form.control} name="description" label="Descrição" />

            {mode === "cobranca" && (
              <>
                <SelectField
                  control={form.control}
                  name="category"
                  label="Tipo de conta / categoria"
                  placeholder="Selecione uma categoria"
                  options={categoryOptions}
                />

                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                  <MoneyField control={form.control} name="value" label="Valor (R$)" required />
                  <TextField control={form.control} name="dueDate" label="Vencimento" placeholder="dd/mm/aaaa" />
                </div>

                <DateField control={form.control} name="date" label="Data de Referência" />

                {w.status === "Pago" && (
                  <motion.div
                    initial={{ opacity: 0, height: 0 }}
                    animate={{ opacity: 1, height: "auto" }}
                    className="space-y-4 bg-status-ok-soft/50 rounded-xl p-4 border border-status-ok/20"
                  >
                    <p className="text-xs font-bold text-status-ok uppercase tracking-wider flex items-center gap-2">
                      <CheckCircle2 className="w-3.5 h-3.5" /> Dados do Pagamento
                    </p>
                    <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                      <SelectField
                        control={form.control}
                        name="paymentMethod"
                        label="Método de Pagamento"
                        placeholder="Selecione"
                        options={PAYMENT_METHOD_OPTIONS}
                      />
                      <TextField control={form.control} name="paymentDate" label="Data do Pagamento" placeholder="dd/mm/aaaa" />
                    </div>
                  </motion.div>
                )}
              </>
            )}

            {mode === "repasse" && (
              <>
                <TextField control={form.control} name="month" label="Referência (Mês/Ano)" placeholder="Março/2026" />

                <div>
                  <label className="block text-sm font-bold text-foreground mb-1.5">Valor Bruto (Aluguel)</label>
                  <input
                    type="text"
                    disabled
                    value={formatCurrency(grossValue)}
                    className="w-full px-4 py-2.5 bg-muted border border-border rounded-xl text-sm font-bold text-foreground"
                  />
                </div>

                <div className="space-y-3">
                  <p className="text-sm font-bold text-foreground">Deduções</p>
                  <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
                    <MoneyField control={form.control} name="adminFee" label="Taxa Admin. (R$)" />
                    <MoneyField control={form.control} name="maintenanceDeductions" label="Manutenção (R$)" />
                    <MoneyField control={form.control} name="utilityDeductions" label="Consumo (R$)" />
                  </div>
                </div>

                <div className="bg-muted rounded-xl p-5 border border-border space-y-3">
                  <div className="flex justify-between text-sm">
                    <span className="text-muted-foreground">Valor Bruto</span>
                    <span className="font-bold text-foreground">{formatCurrency(grossValue)}</span>
                  </div>
                  <div className="flex justify-between text-sm">
                    <span className="text-muted-foreground">(-) Taxa Administrativa</span>
                    <span className="font-bold text-status-critical">-{formatCurrency(w.adminFee ?? 0)}</span>
                  </div>
                  <div className="flex justify-between text-sm">
                    <span className="text-muted-foreground">(-) Manutenção</span>
                    <span className="font-bold text-status-critical">-{formatCurrency(w.maintenanceDeductions ?? 0)}</span>
                  </div>
                  <div className="flex justify-between text-sm">
                    <span className="text-muted-foreground">(-) Consumo</span>
                    <span className="font-bold text-status-critical">-{formatCurrency(w.utilityDeductions ?? 0)}</span>
                  </div>
                  <div className="h-px bg-muted w-full" />
                  <div className="flex justify-between items-center">
                    <span className="font-bold text-foreground">Valor Liquido</span>
                    <span className="font-bold text-status-ok text-xl">{formatCurrency(computedNet)}</span>
                  </div>
                </div>

                <DateField control={form.control} name="date" label="Data do Repasse" />

                {w.status === "Pago" && (
                  <motion.div
                    initial={{ opacity: 0, height: 0 }}
                    animate={{ opacity: 1, height: "auto" }}
                    className="space-y-3 bg-status-ok-soft/50 rounded-xl p-4 border border-status-ok/20"
                  >
                    <p className="text-xs font-bold text-status-ok uppercase tracking-wider flex items-center gap-2">
                      <CheckCircle2 className="w-3.5 h-3.5" /> Dados do Pagamento
                    </p>
                    <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                      <SelectField
                        control={form.control}
                        name="paymentMethod"
                        label="Método"
                        placeholder="Selecione"
                        options={PAYMENT_METHOD_OPTIONS}
                      />
                      <TextField control={form.control} name="paymentDate" label="Data Pgto" placeholder="dd/mm/aaaa" />
                    </div>
                  </motion.div>
                )}
              </>
            )}
          </div>

          <div className="bg-card px-6 py-4 border-t border-border flex items-center justify-end gap-3 flex-shrink-0">
            <button
              type="button"
              onClick={onClose}
              className="px-5 py-2.5 bg-card border border-border text-foreground text-sm font-bold rounded-xl hover:bg-muted transition-colors"
            >
              Cancelar
            </button>
            <button
              type="submit"
              disabled={saving}
              className={clsx(
                "flex items-center gap-2 px-6 py-2.5 text-sm font-bold rounded-xl transition-all shadow-sm",
                saving ? "bg-muted text-muted-foreground cursor-wait" : "bg-primary text-primary-foreground hover:bg-primary/90",
              )}
            >
              <Save className="w-4 h-4" />
              {saving ? "Salvando..." : "Salvar Alterações"}
            </button>
          </div>
        </form>
      </Form>
    </ExpandableSidePanel>
  );
}
