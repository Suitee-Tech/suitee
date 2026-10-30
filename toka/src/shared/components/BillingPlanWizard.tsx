import { useEffect, useId, useMemo, useState, type FormEvent } from "react";
import { AlertCircle, ClipboardList, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "../ui/sheet";
import { Button } from "../ui/button";
import { formatMoneyBR, parseMoneyBR } from "../utils/format";
import { MoneyInput } from "../ui/money-input";
import { FinancialFlowSummary } from "./finance/FinancialFlowSummary";
import { rentalChargeConfigService, BundledItem } from "../services/rentalChargeConfig.service";
import {
  saleBillingPlanService,
  CreateSaleBillingPlanInput,
  SaleBillingPlanMode,
  UpsertDraftInput,
} from "../services/saleBillingPlan.service";

type ContractType = "RENT" | "SALE";

// 'commit' (default) cria o SaleBillingPlan/RentalChargeConfig de
// verdade. 'draft' grava só a escolha do operador num holding
// pré-assinatura (sale_billing_plan_drafts) — usado na esteira antes
// do contract.signed. Em 'draft', dealId é obrigatório e contractId é
// ignorado (pode ser "").
type WizardMode = "commit" | "draft";

interface BillingPlanWizardProps {
  contractId: string;
  contractType: ContractType;
  defaultMonthlyAmount?: number;
  defaultTotalAmount?: number;
  onClose: () => void;
  onSaved: () => void;
  // Optional draft-mode props. When mode='draft' the wizard:
  //  - exibe SALE-only (RENT ainda usa contract direto, sem draft)
  //  - chama upsertDraft(dealId, ...) em vez de create(contractId, ...)
  //  - pré-carrega draft ativo do deal se houver
  mode?: WizardMode;
  dealId?: string;
}

// Well-known bundled-item slots the operator can toggle in two clicks
// instead of typing a description. Custom items still fall through the
// "Adicionar item" button.
const BUNDLED_SLOTS: Array<{ key: string; label: string }> = [
  { key: "condominio", label: "Condomínio" },
  { key: "iptu", label: "IPTU" },
  { key: "seguro_fianca", label: "Seguro Fiança" },
];

export function BillingPlanWizard({
  contractId,
  contractType,
  defaultMonthlyAmount,
  defaultTotalAmount,
  onClose,
  onSaved,
  mode = "commit",
  dealId,
}: BillingPlanWizardProps) {
  const isDraftMode = mode === "draft";
  const fieldId = useId();
  const [saving, setSaving] = useState(false);

  // RENT state — bundled items only (the base rent + dates come from the
  // existing rental_charge_config that the contract.signed handler
  // created at signing; this wizard surfaces only the extras).
  const [bundledItems, setBundledItems] = useState<BundledItem[]>([]);
  const [rentalConfigId, setRentalConfigId] = useState<string | null>(null);
  const [rentalConfigLoading, setRentalConfigLoading] = useState(contractType === "RENT" && !isDraftMode);

  // SALE state — mode + parcelas + datas.
  const [saleMode, setSaleMode] = useState<SaleBillingPlanMode>("SINGLE");
  const [installmentCount, setInstallmentCount] = useState(12);
  const [downPayment, setDownPayment] = useState("R$ 0,00");
  const [installmentAmount, setInstallmentAmount] = useState("");
  const [totalAmount, setTotalAmount] = useState(defaultTotalAmount ? formatMoneyBR(defaultTotalAmount) : "");
  const [firstDueDate, setFirstDueDate] = useState(() => {
    const d = new Date();
    d.setDate(d.getDate() + 5);
    return d.toISOString().slice(0, 10);
  });
  // Composite legs for SALE — toggled in the UI; zero means "not used".
  const [dacaoAmount, setDacaoAmount] = useState("R$ 0,00");
  const [dacaoDescription, setDacaoDescription] = useState("");
  const [trocaAmount, setTrocaAmount] = useState("R$ 0,00");
  const [trocaDescription, setTrocaDescription] = useState("");
  const [saldoAmount, setSaldoAmount] = useState("R$ 0,00");
  const [saldoDueDate, setSaldoDueDate] = useState("");
  const [saldoDescription, setSaldoDescription] = useState("");

  // Bootstrap: for RENT, fetch the existing config so we can show the
  // current bundled items the user is editing.
  useEffect(() => {
    if (contractType !== "RENT" || isDraftMode) return;
    rentalChargeConfigService
      .getByContractId(contractId)
      .then((cfg) => {
        if (!cfg) return;
        setRentalConfigId(cfg.id);
        const items = cfg.bundled_items;
        if (items && items.length > 0) setBundledItems(items);
      })
      .catch(() => {
        // Not configured yet: leave empty. The wizard then acts as
        // first-time setup once the user enables an item.
      })
      .finally(() => setRentalConfigLoading(false));
  }, [contractType, contractId, isDraftMode]);

  // Bootstrap draft mode (SALE only): pré-carrega o draft ativo
  // anteriormente gravado para o deal, para que o operador continue de
  // onde parou em vez de re-preencher tudo.
  useEffect(() => {
    if (!isDraftMode || !dealId || contractType !== "SALE") return;
    saleBillingPlanService
      .getDraft(dealId)
      .then((d) => {
        if (!d) return;
        setSaleMode(d.mode);
        setInstallmentCount(d.installment_count);
        setDownPayment(formatMoneyBR(d.down_payment_cents / 100));
        setInstallmentAmount(formatMoneyBR(d.installment_amount_cents / 100));
        setTotalAmount(formatMoneyBR(d.total_amount_cents / 100));
        setFirstDueDate((d.first_due_date || "").slice(0, 10));
        const c = d.composite || {};
        setDacaoAmount(formatMoneyBR((c.dacao_amount_cents ?? 0) / 100));
        setDacaoDescription(c.dacao_description ?? "");
        setTrocaAmount(formatMoneyBR((c.troca_amount_cents ?? 0) / 100));
        setTrocaDescription(c.troca_description ?? "");
        setSaldoAmount(formatMoneyBR((c.saldo_amount_cents ?? 0) / 100));
        setSaldoDueDate((c.saldo_due_date || "").slice(0, 10));
        setSaldoDescription(c.saldo_description ?? "");
      })
      .catch(() => {
        // Sem draft ainda — wizard abre com defaults.
      });
  }, [isDraftMode, dealId, contractType]);

  const enableSlot = (key: string, label: string) => {
    setBundledItems((prev) => {
      if (prev.find((p) => p.key === key)) return prev;
      return [...prev, { key, description: label, amount_cents: 0, enabled: true }];
    });
  };

  const addCustomItem = () => {
    setBundledItems((prev) => [
      ...prev,
      { key: `custom_${prev.length + 1}`, description: "Outro", amount_cents: 0, enabled: true },
    ]);
  };

  const updateItem = (idx: number, patch: Partial<BundledItem>) => {
    setBundledItems((prev) => prev.map((it, i) => (i === idx ? { ...it, ...patch } : it)));
  };

  const removeItem = (idx: number) => {
    setBundledItems((prev) => prev.filter((_, i) => i !== idx));
  };

  const totalBundled = useMemo(
    () => bundledItems.filter((i) => i.enabled).reduce((acc, i) => acc + i.amount_cents, 0),
    [bundledItems]
  );

  const monthlyPreview = (defaultMonthlyAmount ?? 0) + totalBundled / 100;

  const handleRentSave = async () => {
    if (!rentalConfigId) {
      toast.error("Configuração de cobrança ainda não criada. Tente novamente em instantes.");
      return;
    }
    setSaving(true);
    try {
      const result = await rentalChargeConfigService.updateBundledItems(rentalConfigId, bundledItems);
      toast.success(
        result.charges_created > 0
          ? `Composição atualizada. ${result.charges_created} cobranças futuras foram reemitidas.`
          : "Composição mensal atualizada.",
      );
      onSaved();
      onClose();
    } catch (err) {
      toast.error("Falha ao atualizar pacote.");
      console.error(err);
    } finally {
      setSaving(false);
    }
  };

  // Sum of every composite leg (in BRL) so the operator sees in real
  // time whether the composition matches the total. In SINGLE mode the
  // one cash installment is the remainder after entrada/non-cash/saldo;
  // leaving it at zero made ordinary cash sales impossible to save.
  const compositeBreakdown = useMemo(() => {
    const total = parseMoneyBR(totalAmount) || 0;
    const down = parseMoneyBR(downPayment) || 0;
    const dacao = parseMoneyBR(dacaoAmount) || 0;
    const troca = parseMoneyBR(trocaAmount) || 0;
    const saldo = parseMoneyBR(saldoAmount) || 0;
    let installmentValue = parseMoneyBR(installmentAmount) || 0;
    const count = saleMode === "SINGLE" ? 1 : installmentCount;
    if (saleMode === "SINGLE") {
      const remainder = total - down - dacao - troca - saldo;
      installmentValue = remainder > 0 ? remainder : 0;
    } else if (installmentValue === 0) {
      const remainder = total - down - dacao - troca - saldo;
      installmentValue = remainder > 0 ? remainder / count : 0;
    }
    const installmentsTotal = installmentValue * count;
    const composedTotal = down + installmentsTotal + dacao + troca + saldo;
    return { total, down, dacao, troca, saldo, installmentsTotal, installmentValue, composedTotal };
  }, [totalAmount, downPayment, dacaoAmount, trocaAmount, saldoAmount, installmentAmount, installmentCount, saleMode]);

  const handleSaleSave = async (e: FormEvent) => {
    e.preventDefault();
    const { total, down, dacao, troca, saldo, installmentValue } = compositeBreakdown;
    if (total <= 0) {
      toast.error("Informe o valor total da venda.");
      return;
    }
    if (!firstDueDate || Number.isNaN(Date.parse(`${firstDueDate}T00:00:00Z`))) {
      toast.error("Informe a data da primeira cobrança.");
      return;
    }
    if (saldo > 0 && !saldoDueDate) {
      toast.error("Informe a data de vencimento do saldo.");
      return;
    }
    const allowedRoundingDelta = Math.max(0.01, (saleMode === "SINGLE" ? 1 : installmentCount) / 100);
    if (Math.abs(compositeBreakdown.composedTotal - total) > allowedRoundingDelta) {
      toast.error("A composição precisa fechar com o valor total da venda antes de salvar.");
      return;
    }
    const input: CreateSaleBillingPlanInput = {
      // O negócio vinculado é necessário para a composição econômica da venda
      // (repasse ao proprietário + comissões) ser aplicada às cobranças.
      deal_id: dealId || undefined,
      mode: saleMode,
      total_amount: total,
      down_payment_amount: down,
      installment_amount: installmentValue,
      installment_count: saleMode === "SINGLE" ? 1 : installmentCount,
      first_due_date: new Date(firstDueDate).toISOString(),
      recurrence_interval_days: 30,
      dacao_amount: dacao || undefined,
      dacao_description: dacaoDescription.trim() || undefined,
      troca_amount: troca || undefined,
      troca_description: trocaDescription.trim() || undefined,
      saldo_amount: saldo || undefined,
      saldo_due_date: saldo > 0 ? new Date(saldoDueDate).toISOString() : undefined,
      saldo_description: saldoDescription.trim() || undefined,
    };
    setSaving(true);
    try {
      if (isDraftMode) {
        if (!dealId) {
          toast.error("Deal não identificado para gravar a forma de pagamento.");
          return;
        }
        const draftInput: UpsertDraftInput = { ...input };
        await saleBillingPlanService.upsertDraft(dealId, draftInput);
        toast.success("Plano financeiro salvo. Nenhum lançamento foi criado nesta etapa.");
      } else {
        const result = await saleBillingPlanService.create(contractId, input);
        toast.success(`Plano criado. ${result.charges_created} cobranças geradas.`);
      }
      onSaved();
      onClose();
    } catch (err: any) {
      const msg =
        err?.response?.data?.message ?? err?.response?.data?.error ??
        (isDraftMode ? "Falha ao salvar forma de pagamento." : "Falha ao criar plano de venda.");
      toast.error(msg);
      console.error(err);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Sheet
      open
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
    >
      <SheetContent side="right" className="flex w-full flex-col gap-0 overflow-hidden p-0 sm:max-w-2xl">
        <SheetHeader className="flex flex-row items-center gap-3 border-b border-border bg-muted/50 px-6 py-4 pr-12 space-y-0">
          <div className="p-2 bg-primary-100 rounded-xl">
            <ClipboardList className="w-5 h-5 text-primary-600" />
          </div>
          <div className="text-left">
            <SheetTitle className="text-lg font-bold text-foreground">
              {contractType === "RENT" ? "Composição mensal da locação" : "Forma de recebimento da venda"}
            </SheetTitle>
            <SheetDescription className="text-xs">
              {contractType === "RENT"
                ? "Defina quais valores fixos acompanham o aluguel na mesma cobrança."
                : isDraftMode
                  ? "Salve o cronograma que acompanhará a minuta. Nenhum lançamento será criado agora."
                  : "Defina como o comprador pagará e materialize os recebíveis do contrato."}
            </SheetDescription>
          </div>
        </SheetHeader>

        {contractType === "RENT" ? (
          <div className="flex-1 overflow-y-auto p-6 space-y-6">
            <FinancialFlowSummary flow="rent" compact />
            <div className="rounded-xl border border-primary-100 bg-primary-50 p-4 text-sm text-primary-800">
              Valores adicionados aqui são <strong>somados ao aluguel</strong> e discriminados na mesma cobrança.
              Consumos variáveis devem ser vinculados em Contas e lançados por competência. A taxa de administração
              não é um item adicional: ela é receita da imobiliária deduzida do aluguel recebido.
            </div>

            {!rentalConfigLoading && !rentalConfigId && (
              <div className="flex items-start gap-3 rounded-xl border border-status-warning/30 bg-status-warning-soft p-4 text-status-warning">
                <AlertCircle className="mt-0.5 size-5 shrink-0" />
                <div>
                  <p className="text-sm font-bold">Configure primeiro o plano-base da locação</p>
                  <p className="mt-1 text-xs leading-relaxed">
                    O calendário, o primeiro aluguel e o dia de vencimento precisam existir antes da composição mensal.
                  </p>
                </div>
              </div>
            )}

            <div className="flex flex-wrap gap-2">
              {BUNDLED_SLOTS.map((slot) => {
                const exists = bundledItems.find((b) => b.key === slot.key);
                return (
                  <Button
                    key={slot.key}
                    variant={exists ? "secondary" : "outline"}
                    size="sm"
                    onClick={() => !exists && enableSlot(slot.key, slot.label)}
                    disabled={!!exists}
                  >
                    <Plus className="w-3.5 h-3.5 mr-1" />
                    {slot.label}
                  </Button>
                );
              })}
              <Button variant="outline" size="sm" onClick={addCustomItem}>
                <Plus className="w-3.5 h-3.5 mr-1" /> Item personalizado
              </Button>
            </div>

            <div className="space-y-3">
              {rentalConfigLoading ? (
                <div className="h-24 animate-pulse rounded-xl bg-muted" />
              ) : bundledItems.length === 0 ? (
                <div className="text-sm text-muted-foreground italic">
                  Nenhum item incluso. Adicione itens acima para que o sistema some ao aluguel mensal.
                </div>
              ) : (
                bundledItems.map((it, idx) => (
                  <div key={`${it.key}-${idx}`} className="flex items-center gap-3 rounded-md border border-border p-3">
                    <input
                      type="checkbox"
                      aria-label={`Incluir item ${idx + 1}`}
                      checked={it.enabled}
                      onChange={(e) => updateItem(idx, { enabled: e.target.checked })}
                      className="h-4 w-4"
                    />
                    <input
                      type="text"
                      aria-label={`Descrição do item ${idx + 1}`}
                      value={it.description}
                      onChange={(e) => updateItem(idx, { description: e.target.value })}
                      className="flex-1 rounded-md border border-border bg-background px-2 py-1 text-sm"
                    />
                    <MoneyInput
                      aria-label={`Valor do item ${idx + 1}`}
                      withSymbol
                      placeholder="R$ 0,00"
                      value={it.amount_cents / 100}
                      onChange={(v) =>
                        updateItem(idx, {
                          amount_cents: Math.round((v ?? 0) * 100),
                        })
                      }
                      className="w-32 rounded-md border border-border bg-background px-2 py-1 text-sm text-right"
                    />
                    <Button variant="ghost" size="icon" aria-label={`Remover item ${idx + 1}`} onClick={() => removeItem(idx)}>
                      <Trash2 className="w-4 h-4 text-destructive" />
                    </Button>
                  </div>
                ))
              )}
            </div>

            <div className="rounded-xl border border-border bg-card p-4 text-sm">
              <div className="mb-3 text-xs font-bold uppercase tracking-wide text-muted-foreground">Prévia da cobrança mensal</div>
              <div className="flex justify-between">
                <span>Aluguel base</span>
                <span className="font-mono">{formatMoneyBR(defaultMonthlyAmount ?? 0)}</span>
              </div>
              <div className="flex justify-between">
                <span>Itens inclusos</span>
                <span className="font-mono">{formatMoneyBR(totalBundled / 100)}</span>
              </div>
              <div className="mt-2 flex justify-between border-t pt-2 font-bold">
                <span>Total mensal</span>
                <span className="font-mono">{formatMoneyBR(monthlyPreview)}</span>
              </div>
            </div>
          </div>
        ) : (
          <form onSubmit={handleSaleSave} className="flex-1 overflow-y-auto p-6 space-y-5">
            <FinancialFlowSummary flow="sale" compact />
            <div>
              <label className="block text-sm font-bold mb-1">Estrutura do recebimento</label>
              <div role="group" aria-label="Estrutura do recebimento" className="flex gap-2">
                <Button
                  type="button"
                  aria-pressed={saleMode === "SINGLE"}
                  variant={saleMode === "SINGLE" ? "secondary" : "outline"}
                  onClick={() => setSaleMode("SINGLE")}
                >
                  À vista
                </Button>
                <Button
                  type="button"
                  aria-pressed={saleMode === "INSTALLMENTS"}
                  variant={saleMode === "INSTALLMENTS" ? "secondary" : "outline"}
                  onClick={() => setSaleMode("INSTALLMENTS")}
                >
                  Parcelado
                </Button>
              </div>
            </div>
            <div>
              <label htmlFor={`${fieldId}-total`} className="block text-sm font-bold mb-1">Valor total da venda <span aria-hidden="true" className="text-status-critical">*</span></label>
              <MoneyInput
                id={`${fieldId}-total`}
                aria-required="true"
                withSymbol
                placeholder="R$ 0,00"
                value={totalAmount === "" ? undefined : (parseMoneyBR(totalAmount) ?? undefined)}
                onChange={(v) => setTotalAmount(v === undefined ? "" : formatMoneyBR(v))}
                className="w-full rounded-md border border-border bg-background px-3 py-2"
              />
            </div>
            <div>
              <label htmlFor={`${fieldId}-down-payment`} className="block text-sm font-bold mb-1">Entrada (opcional)</label>
              <MoneyInput
                id={`${fieldId}-down-payment`}
                withSymbol
                placeholder="R$ 0,00"
                value={downPayment === "" ? undefined : (parseMoneyBR(downPayment) ?? undefined)}
                onChange={(v) => setDownPayment(v === undefined ? "" : formatMoneyBR(v))}
                className="w-full rounded-md border border-border bg-background px-3 py-2"
              />
            </div>
            {saleMode === "INSTALLMENTS" && (
              <>
                <div>
                  <label htmlFor={`${fieldId}-installments`} className="block text-sm font-bold mb-1">Número de parcelas <span aria-hidden="true" className="text-status-critical">*</span></label>
                  <input
                    id={`${fieldId}-installments`}
                    aria-required="true"
                    type="number"
                    min={2}
                    max={360}
                    value={installmentCount}
                    onChange={(e) => setInstallmentCount(Number(e.target.value))}
                    className="w-full rounded-md border border-border bg-background px-3 py-2"
                  />
                </div>
                <div>
                  <label htmlFor={`${fieldId}-installment-amount`} className="block text-sm font-bold mb-1">Valor por parcela</label>
                  <MoneyInput
                    id={`${fieldId}-installment-amount`}
                    withSymbol
                    placeholder="Deixe em branco para calcular"
                    value={installmentAmount === "" ? undefined : (parseMoneyBR(installmentAmount) ?? undefined)}
                    onChange={(v) => setInstallmentAmount(v === undefined ? "" : formatMoneyBR(v))}
                    className="w-full rounded-md border border-border bg-background px-3 py-2"
                  />
                  <p className="text-xs text-muted-foreground mt-1">Se vazio: (total − entrada) / número de parcelas.</p>
                </div>
              </>
            )}
            <div>
              <label htmlFor={`${fieldId}-first-due`} className="block text-sm font-bold mb-1">Primeira cobrança <span aria-hidden="true" className="text-status-critical">*</span></label>
              <input
                id={`${fieldId}-first-due`}
                aria-required="true"
                type="date"
                value={firstDueDate}
                onChange={(e) => setFirstDueDate(e.target.value)}
                className="w-full rounded-md border border-border bg-background px-3 py-2"
              />
            </div>

            {/* Composite legs: dação, troca, saldo. These are optional —
                zero means "not used". The dação/troca legs are
                informational (no charge is emitted); saldo is a deferred
                cash charge with its own vencimento. */}
            <fieldset className="rounded-md border border-border p-4 space-y-3">
              <legend className="px-2 text-xs font-bold uppercase text-muted-foreground">Composição (opcional)</legend>
              <p className="text-xs text-muted-foreground">
                Use estes campos quando o valor da venda não é totalmente em dinheiro: dação em pagamento e troca não
                geram cobrança (apenas registro contratual); saldo gera uma única cobrança com vencimento próprio.
              </p>

              <div className="space-y-2">
                <div className="flex items-center gap-2">
                  <label className="w-32 text-sm">Dação em pagamento</label>
                  <MoneyInput
                    withSymbol
                    placeholder="R$ 0,00"
                    value={dacaoAmount === "" ? undefined : (parseMoneyBR(dacaoAmount) ?? undefined)}
                    onChange={(v) => setDacaoAmount(v === undefined ? "" : formatMoneyBR(v))}
                    className="w-32 rounded-md border border-border bg-background px-2 py-1.5 text-right text-sm"
                  />
                </div>
                <input
                  type="text"
                  aria-label="Descrição do bem de dação em pagamento"
                  placeholder="Descrição do bem (ex: imóvel em Praia Grande)"
                  value={dacaoDescription}
                  onChange={(e) => setDacaoDescription(e.target.value)}
                  className="w-full rounded-md border border-border bg-background px-2 py-1.5 text-sm"
                />
              </div>

              <div className="space-y-2">
                <div className="flex items-center gap-2">
                  <label className="w-32 text-sm">Troca</label>
                  <MoneyInput
                    withSymbol
                    placeholder="R$ 0,00"
                    value={trocaAmount === "" ? undefined : (parseMoneyBR(trocaAmount) ?? undefined)}
                    onChange={(v) => setTrocaAmount(v === undefined ? "" : formatMoneyBR(v))}
                    className="w-32 rounded-md border border-border bg-background px-2 py-1.5 text-right text-sm"
                  />
                </div>
                <input
                  type="text"
                  aria-label="Descrição da propriedade trocada"
                  placeholder="Descrição da propriedade trocada"
                  value={trocaDescription}
                  onChange={(e) => setTrocaDescription(e.target.value)}
                  className="w-full rounded-md border border-border bg-background px-2 py-1.5 text-sm"
                />
              </div>

              <div className="space-y-2">
                <div className="flex items-center gap-2">
                  <label className="w-32 text-sm">Saldo</label>
                  <MoneyInput
                    withSymbol
                    placeholder="R$ 0,00"
                    value={saldoAmount === "" ? undefined : (parseMoneyBR(saldoAmount) ?? undefined)}
                    onChange={(v) => setSaldoAmount(v === undefined ? "" : formatMoneyBR(v))}
                    className="w-32 rounded-md border border-border bg-background px-2 py-1.5 text-right text-sm"
                  />
                </div>
                {(parseMoneyBR(saldoAmount) ?? 0) > 0 && (
                  <>
                    <div className="flex items-center gap-2">
                      <label htmlFor={`${fieldId}-saldo-due`} className="w-32 text-sm">Vencimento do saldo <span aria-hidden="true" className="text-status-critical">*</span></label>
                      <input
                        id={`${fieldId}-saldo-due`}
                        aria-required="true"
                        type="date"
                        value={saldoDueDate}
                        onChange={(e) => setSaldoDueDate(e.target.value)}
                        className="w-full rounded-md border border-border bg-background px-2 py-1.5 text-sm"
                      />
                    </div>
                    <input
                      type="text"
                      aria-label="Descrição do saldo"
                      placeholder="Descrição (ex: Saldo pós-financiamento)"
                      value={saldoDescription}
                      onChange={(e) => setSaldoDescription(e.target.value)}
                      className="w-full rounded-md border border-border bg-background px-2 py-1.5 text-sm"
                    />
                  </>
                )}
              </div>
            </fieldset>

            {/* Live composition summary. Helps the operator see if entrada
                + parcelas + dação + troca + saldo == total before sending. */}
            <div className="rounded-xl border border-border bg-muted/30 p-4 text-sm">
              <div className="mb-1 text-xs font-bold uppercase text-muted-foreground">Conferência do plano</div>
              <div className="flex justify-between">
                <span>Entrada</span>
                <span className="font-mono">{formatMoneyBR(compositeBreakdown.down)}</span>
              </div>
              <div className="flex justify-between">
                <span>{saleMode === "INSTALLMENTS" ? `Parcelas (${installmentCount}×)` : "Pagamento à vista"}</span>
                <span className="font-mono">{formatMoneyBR(compositeBreakdown.installmentsTotal)}</span>
              </div>
              {compositeBreakdown.dacao > 0 && (
                <div className="flex justify-between">
                  <span>Dação em pagamento</span>
                  <span className="font-mono">{formatMoneyBR(compositeBreakdown.dacao)}</span>
                </div>
              )}
              {compositeBreakdown.troca > 0 && (
                <div className="flex justify-between">
                  <span>Troca</span>
                  <span className="font-mono">{formatMoneyBR(compositeBreakdown.troca)}</span>
                </div>
              )}
              {compositeBreakdown.saldo > 0 && (
                <div className="flex justify-between">
                  <span>Saldo</span>
                  <span className="font-mono">{formatMoneyBR(compositeBreakdown.saldo)}</span>
                </div>
              )}
              <div className="mt-2 flex justify-between border-t pt-2 font-bold">
                <span>Composto</span>
                <span className="font-mono">{formatMoneyBR(compositeBreakdown.composedTotal)}</span>
              </div>
              <div className="flex justify-between">
                <span>Total da venda</span>
                <span className="font-mono">{formatMoneyBR(compositeBreakdown.total)}</span>
              </div>
              {Math.abs(compositeBreakdown.composedTotal - compositeBreakdown.total) >
                  Math.max(0.01, (saleMode === "SINGLE" ? 1 : installmentCount) / 100) &&
                compositeBreakdown.total > 0 && (
                  <div className="mt-2 flex items-start gap-1.5 rounded-lg bg-status-warning-soft px-2.5 py-2 text-xs text-status-warning">
                    <AlertCircle className="mt-0.5 size-3.5 shrink-0" />
                    <span>A composição não bate com o total da venda. Ajuste os valores antes de salvar.</span>
                  </div>
                )}
            </div>
          </form>
        )}

        <SheetFooter className="flex flex-row justify-end gap-2 border-t border-border bg-muted/30 px-6 py-3">
          <Button variant="ghost" onClick={onClose} disabled={saving}>
            Cancelar
          </Button>
          <Button
            onClick={(e) => (contractType === "SALE" ? handleSaleSave(e as any) : handleRentSave())}
            disabled={saving || (contractType === "RENT" && (rentalConfigLoading || !rentalConfigId))}
          >
            {saving
              ? "Salvando..."
              : contractType === "SALE"
                ? isDraftMode
                  ? "Salvar plano financeiro"
                  : "Materializar recebíveis"
                : "Salvar composição"}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}
