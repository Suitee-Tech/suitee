import { useEffect, useId, useMemo, useState, type ReactNode } from "react";
import {
  AlertCircle,
  Building2,
  Droplet,
  FileText,
  Flame,
  Receipt,
  ShieldCheck,
  Wallet,
  Zap,
} from "lucide-react";
import type { Contract } from "../contracts/types";
import type { RentalChargeConfig } from "../rentals/types";
import { useCreateUtilityAccountMutation } from "../hooks/useContracts";
import { parseMoneyBR, formatMoneyBR } from "../utils/format";
import { MoneyInput } from "../ui/money-input";
import { Button } from "../ui/button";
import { SearchableSelect } from "../ui/searchable-select";
import type {
  UtilityAccountType,
  UtilityResponsibleParty,
  UtilityBillingMode,
} from "../services/utilityAccount.service";
import { ExpandableSidePanel } from "./ExpandableSidePanel";

interface UtilityAccountModalProps {
  contract: Contract;
  rentalConfig?: RentalChargeConfig | null;
  onConfigurePlan?: () => void;
  onClose: () => void;
}

const TYPE_OPTIONS: { value: UtilityAccountType; label: string; icon: ReactNode }[] = [
  { value: "AGUA", label: "Água", icon: <Droplet className="size-4" /> },
  { value: "ENERGIA", label: "Energia", icon: <Zap className="size-4" /> },
  { value: "GAS", label: "Gás", icon: <Flame className="size-4" /> },
  { value: "CONDOMINIO", label: "Condomínio", icon: <Building2 className="size-4" /> },
  { value: "IPTU", label: "IPTU", icon: <Receipt className="size-4" /> },
  { value: "INTERNET", label: "Internet", icon: <Zap className="size-4" /> },
  { value: "SEGURO", label: "Seguro", icon: <ShieldCheck className="size-4" /> },
  { value: "OUTROS", label: "Outros", icon: <FileText className="size-4" /> },
];

const RESPONSIBILITY_OPTIONS: Array<{
  value: UtilityResponsibleParty;
  label: string;
  description: string;
}> = [
  { value: "TENANT", label: "Inquilino", description: "Gera cobrança ao inquilino quando o modo for variável." },
  { value: "OWNER", label: "Proprietário", description: "Vira dedução no repasse da competência." },
  { value: "AGENCY", label: "Imobiliária", description: "Vira despesa operacional da imobiliária." },
];

const BILLING_OPTIONS: Array<{
  value: UtilityBillingMode;
  label: string;
  description: string;
}> = [
  {
    value: "VARIABLE_SEPARATE",
    label: "Valor variável por mês",
    description: "O operador informa a fatura de cada competência; o destino depende do responsável.",
  },
  {
    value: "BUNDLED",
    label: "Inclusa na cobrança mensal",
    description: "Usa um item fixo da composição mensal e aparece discriminada junto do aluguel.",
  },
  {
    value: "INFORMATIONAL",
    label: "Somente acompanhamento",
    description: "Mantém fornecedor, vencimento e responsabilidade sem gerar lançamento financeiro.",
  },
];

export function UtilityAccountModal({
  contract,
  rentalConfig,
  onConfigurePlan,
  onClose,
}: UtilityAccountModalProps) {
  const createMutation = useCreateUtilityAccountMutation();
  const fieldId = useId();
  const [form, setForm] = useState({
    type: "ENERGIA" as UtilityAccountType,
    provider: "",
    accountNumber: "",
    responsibleParty: "TENANT" as UtilityResponsibleParty,
    estimatedAmount: "",
    paymentMethod: "",
    autoPayEnabled: false,
    dueDay: "",
    notes: "",
    billingMode: "VARIABLE_SEPARATE" as UtilityBillingMode,
    bundledItemKey: "",
  });
  const [error, setError] = useState<string | null>(null);

  const bundledItems = useMemo(
    () => (rentalConfig?.bundled_items ?? []).filter((item) => item.enabled),
    [rentalConfig?.bundled_items],
  );

  useEffect(() => {
    setForm({
      type: "ENERGIA",
      provider: "",
      accountNumber: "",
      responsibleParty: "TENANT",
      estimatedAmount: "",
      paymentMethod: "",
      autoPayEnabled: false,
      dueDay: "",
      notes: "",
      billingMode: "VARIABLE_SEPARATE",
      bundledItemKey: "",
    });
    setError(null);
  }, [contract.id]);

  const selectedResponsibility = RESPONSIBILITY_OPTIONS.find((option) => option.value === form.responsibleParty);

  const handleSubmit = async () => {
    setError(null);
    if (!form.provider.trim()) {
      setError("Informe a concessionária ou fornecedor.");
      return;
    }
    if (!contract.property_id) {
      setError("Este contrato não possui imóvel vinculado.");
      return;
    }
    const dueDay = form.dueDay ? Number.parseInt(form.dueDay, 10) : undefined;
    if (dueDay !== undefined && (!Number.isInteger(dueDay) || dueDay < 1 || dueDay > 31)) {
      setError("O dia de vencimento deve estar entre 1 e 31.");
      return;
    }
    if (form.billingMode === "BUNDLED" && !form.bundledItemKey) {
      setError("Selecione o item correspondente da composição mensal.");
      return;
    }

    try {
      await createMutation.mutateAsync({
        contract_id: contract.id,
        property_id: contract.property_id,
        type: form.type,
        provider: form.provider.trim(),
        account_number: form.accountNumber.trim() || undefined,
        responsible_party: form.responsibleParty,
        estimated_amount: parseMoneyBR(form.estimatedAmount) ?? 0,
        payment_method: form.paymentMethod.trim() || undefined,
        auto_pay_enabled: form.autoPayEnabled,
        due_day: dueDay,
        notes: form.notes.trim() || undefined,
        billing_mode: form.billingMode,
        bundled_item_key: form.billingMode === "BUNDLED" ? form.bundledItemKey : undefined,
      });
      onClose();
    } catch {
      // A mutation já apresenta o erro retornado pela API.
    }
  };

  return (
    <ExpandableSidePanel
      open
      onClose={onClose}
      defaultSize="3xl"
      expandedSize="full"
      bgClass="bg-muted"
      renderHeader={({ expandButton, closeButton }) => (
        <div className="flex items-center justify-between gap-4 border-b border-border bg-card px-6 py-4">
          <div className="flex min-w-0 items-center gap-3">
            <div className="rounded-xl bg-primary-50 p-2 text-primary-700"><Building2 className="size-5" /></div>
            <div className="min-w-0">
              <h2 className="truncate text-lg font-bold text-foreground">Vincular conta à locação</h2>
              <p className="truncate text-sm text-muted-foreground">{contract.client} · {contract.property_title}</p>
            </div>
          </div>
          <div className="flex items-center gap-2">{expandButton}{closeButton}</div>
        </div>
      )}
    >
      <div className="flex flex-1 flex-col overflow-hidden">
        <div className="flex-1 space-y-7 overflow-y-auto p-6">
          <section>
            <div className="mb-3">
              <h3 className="text-sm font-bold text-foreground">1. Identificação da conta</h3>
              <p className="text-xs text-muted-foreground">Escolha o tipo e registre os dados usados para conciliação.</p>
            </div>
            <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
              {TYPE_OPTIONS.map((option) => (
                <button
                  key={option.value}
                  type="button"
                  aria-pressed={form.type === option.value}
                  onClick={() => setForm((current) => ({ ...current, type: option.value }))}
                  className={`flex items-center justify-center gap-2 rounded-xl border px-3 py-3 text-xs font-medium transition ${
                    form.type === option.value
                      ? "border-primary-300 bg-primary-50 text-primary-700 ring-2 ring-primary-600/20"
                      : "border-border bg-card text-muted-foreground hover:bg-muted"
                  }`}
                >
                  {option.icon}{option.label}
                </button>
              ))}
            </div>

            <div className="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div>
                <label htmlFor={`${fieldId}-provider`} className="mb-1 block text-xs font-medium text-foreground">Concessionária / fornecedor <span aria-hidden="true" className="text-status-critical">*</span></label>
                <input
                  id={`${fieldId}-provider`}
                  aria-required="true"
                  value={form.provider}
                  onChange={(event) => setForm((current) => ({ ...current, provider: event.target.value }))}
                  placeholder="Ex.: Enel, Sabesp, condomínio…"
                  className="h-11 w-full rounded-xl border border-border bg-card px-3 text-sm outline-none focus:border-primary-600 focus:ring-2 focus:ring-primary-600/20"
                />
              </div>
              <div>
                <label htmlFor={`${fieldId}-account`} className="mb-1 block text-xs font-medium text-foreground">Número da conta / unidade</label>
                <input
                  id={`${fieldId}-account`}
                  value={form.accountNumber}
                  onChange={(event) => setForm((current) => ({ ...current, accountNumber: event.target.value }))}
                  className="h-11 w-full rounded-xl border border-border bg-card px-3 text-sm outline-none focus:border-primary-600 focus:ring-2 focus:ring-primary-600/20"
                />
              </div>
            </div>
          </section>

          <section className="border-t border-border pt-6">
            <div className="mb-3">
              <h3 className="text-sm font-bold text-foreground">2. Efeito no fluxo financeiro</h3>
              <p className="text-xs text-muted-foreground">O responsável define quem absorve o valor; o modo define como ele vira lançamento.</p>
            </div>

            <div role="group" aria-label="Responsável pela conta" className="grid grid-cols-1 gap-3 sm:grid-cols-3">
              {RESPONSIBILITY_OPTIONS.map((option) => (
                <button
                  key={option.value}
                  type="button"
                  aria-pressed={form.responsibleParty === option.value}
                  onClick={() => setForm((current) => ({ ...current, responsibleParty: option.value }))}
                  className={`rounded-xl border p-4 text-left transition ${
                    form.responsibleParty === option.value
                      ? "border-primary-300 bg-primary-50 ring-2 ring-primary-600/20"
                      : "border-border bg-card hover:bg-muted"
                  }`}
                >
                  <span className="text-sm font-bold text-foreground">{option.label}</span>
                  <span className="mt-1 block text-xs leading-relaxed text-muted-foreground">{option.description}</span>
                </button>
              ))}
            </div>

            <div role="group" aria-label="Modo de cobrança" className="mt-4 grid grid-cols-1 gap-3">
              {BILLING_OPTIONS.map((option) => (
                <button
                  key={option.value}
                  type="button"
                  aria-pressed={form.billingMode === option.value}
                  onClick={() => setForm((current) => ({
                    ...current,
                    billingMode: option.value,
                    bundledItemKey: option.value === "BUNDLED" ? (current.bundledItemKey || bundledItems[0]?.key || "") : "",
                  }))}
                  className={`flex items-start gap-3 rounded-xl border p-4 text-left transition ${
                    form.billingMode === option.value
                      ? "border-primary-300 bg-primary-50 ring-2 ring-primary-600/20"
                      : "border-border bg-card hover:bg-muted"
                  }`}
                >
                  <span className={`mt-0.5 size-4 rounded-full border-4 ${form.billingMode === option.value ? "border-primary-600 bg-card" : "border-border bg-card"}`} />
                  <span>
                    <span className="block text-sm font-bold text-foreground">{option.label}</span>
                    <span className="mt-1 block text-xs leading-relaxed text-muted-foreground">{option.description}</span>
                  </span>
                </button>
              ))}
            </div>

            {form.billingMode === "BUNDLED" && (
              <div className="mt-4 rounded-xl border border-border bg-card p-4">
                <label htmlFor={`${fieldId}-bundled-item`} className="mb-1 block text-xs font-medium text-foreground">Item da composição mensal <span aria-hidden="true" className="text-status-critical">*</span></label>
                {bundledItems.length > 0 ? (
                  <>
                    <SearchableSelect
                      id={`${fieldId}-bundled-item`}
                      aria-label="Item da composição mensal"
                      value={form.bundledItemKey}
                      onValueChange={(value) => setForm((current) => ({ ...current, bundledItemKey: value }))}
                      options={bundledItems.map((item) => ({
                        value: item.key,
                        label: `${item.description} · ${formatMoneyBR(item.amount_cents / 100)}`,
                        searchText: `${item.description} ${item.key}`,
                      }))}
                      placeholder="Selecione o item já configurado…"
                      searchPlaceholder="Buscar na composição…"
                    />
                    <p className="mt-1 text-[11px] text-muted-foreground">O valor mensal vem deste item; a estimativa abaixo não é somada novamente.</p>
                  </>
                ) : (
                  <div className="flex flex-col gap-3 rounded-lg border border-status-warning/30 bg-status-warning-soft p-3 sm:flex-row sm:items-center sm:justify-between">
                    <p className="text-xs text-status-warning">Crie primeiro um item fixo na composição mensal do plano.</p>
                    {onConfigurePlan && <Button type="button" variant="outline" size="sm" onClick={onConfigurePlan}>Configurar composição</Button>}
                  </div>
                )}
              </div>
            )}

            <div className="mt-4 rounded-xl border border-primary-100 bg-primary-50 p-3 text-xs text-primary-800">
              <strong>{selectedResponsibility?.label}:</strong> {selectedResponsibility?.description}
            </div>
          </section>

          <section className="border-t border-border pt-6">
            <div className="mb-3">
              <h3 className="text-sm font-bold text-foreground">3. Previsão e operação</h3>
              <p className="text-xs text-muted-foreground">Dados de apoio para vencimento, previsão e conciliação.</p>
            </div>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div>
                <label htmlFor={`${fieldId}-estimated`} className="mb-1 block text-xs font-medium text-foreground">Valor estimado</label>
                <MoneyInput
                  id={`${fieldId}-estimated`}
                  withSymbol
                  value={form.estimatedAmount === "" ? undefined : (parseMoneyBR(form.estimatedAmount) ?? undefined)}
                  onChange={(value) => setForm((current) => ({ ...current, estimatedAmount: value === undefined ? "" : String(value) }))}
                  placeholder="R$ 0,00"
                  className="h-11 w-full rounded-xl border border-border bg-card px-3 text-sm outline-none focus:border-primary-600 focus:ring-2 focus:ring-primary-600/20"
                />
              </div>
              <div>
                <label htmlFor={`${fieldId}-due-day`} className="mb-1 block text-xs font-medium text-foreground">Dia de vencimento</label>
                <input
                  id={`${fieldId}-due-day`}
                  type="number"
                  min={1}
                  max={31}
                  value={form.dueDay}
                  onChange={(event) => setForm((current) => ({ ...current, dueDay: event.target.value }))}
                  className="h-11 w-full rounded-xl border border-border bg-card px-3 text-sm outline-none focus:border-primary-600 focus:ring-2 focus:ring-primary-600/20"
                />
              </div>
              <div className="sm:col-span-2">
                <label htmlFor={`${fieldId}-payment-method`} className="mb-1 block text-xs font-medium text-foreground">Forma recorrente cadastrada</label>
                <input
                  id={`${fieldId}-payment-method`}
                  value={form.paymentMethod}
                  onChange={(event) => setForm((current) => ({ ...current, paymentMethod: event.target.value }))}
                  placeholder="Ex.: débito em conta, cartão recorrente, boleto…"
                  className="h-11 w-full rounded-xl border border-border bg-card px-3 text-sm outline-none focus:border-primary-600 focus:ring-2 focus:ring-primary-600/20"
                />
                <label className="mt-3 flex items-start gap-2 text-xs text-foreground">
                  <input
                    type="checkbox"
                    checked={form.autoPayEnabled}
                    onChange={(event) => setForm((current) => ({ ...current, autoPayEnabled: event.target.checked }))}
                    className="mt-0.5 rounded border-border"
                  />
                  <span>
                    Forma recorrente conferida
                    <span className="block font-normal text-muted-foreground">Registro operacional; não movimenta dinheiro nem confirma pagamento sem integração.</span>
                  </span>
                </label>
              </div>
              <div className="sm:col-span-2">
                <label htmlFor={`${fieldId}-notes`} className="mb-1 block text-xs font-medium text-foreground">Observações</label>
                <textarea
                  id={`${fieldId}-notes`}
                  value={form.notes}
                  onChange={(event) => setForm((current) => ({ ...current, notes: event.target.value }))}
                  rows={3}
                  className="w-full resize-none rounded-xl border border-border bg-card px-3 py-2 text-sm outline-none focus:border-primary-600 focus:ring-2 focus:ring-primary-600/20"
                />
              </div>
            </div>
          </section>

          {error && (
            <div role="alert" className="flex items-start gap-2 rounded-xl border border-status-critical/30 bg-status-critical-soft p-3 text-xs font-medium text-status-critical">
              <AlertCircle className="mt-0.5 size-4 shrink-0" /> {error}
            </div>
          )}
        </div>

        <div className="flex justify-end gap-3 border-t border-border bg-card px-6 py-4">
          <Button variant="outline" onClick={onClose}>Cancelar</Button>
          <Button onClick={handleSubmit} loading={createMutation.isPending} loadingText="Vinculando…">
            <Wallet className="size-4" /> Vincular conta
          </Button>
        </div>
      </div>
    </ExpandableSidePanel>
  );
}
