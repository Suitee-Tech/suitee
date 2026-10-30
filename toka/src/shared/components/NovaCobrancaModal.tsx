import { useEffect, useMemo, useState } from "react";
import { useForm, useFieldArray } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { DollarSign, AlertCircle, ArrowUpRight, Pencil, Plus, Trash2 } from "lucide-react";
import { useNavigate } from "react-router";
import type { CreateChargeDTO, UpdateChargeDTO, Charge } from "../services/charge.service";
import { utilityAccountService, type UtilityAccount } from "../services/utilityAccount.service";
import { toast } from "sonner";
import { formatMoneyBR } from "../utils/format";
import { buildContractIssues, showContractIssueToasts } from "../utils/contractIssues";
import { canEditCharge } from "../utils/charge";
import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetTitle } from "../ui/sheet";
import { Button } from "../ui/button";
import { MoneyInput } from "../ui/money-input";
import { Form } from "../ui/form";
import { DateField, MoneyField, SelectField, TextField } from "../ui/fields";

interface CommonProps {
  initial?: Partial<CreateChargeDTO> & { id?: string; status?: string };
  onClose: () => void;
  contracts: any[];
  properties: Array<{ id: string; title?: string; ownerId?: string; owners?: Array<{ id: string }> }>;
}

interface CreateProps extends CommonProps {
  mode?: "create";
  onSave: (data: CreateChargeDTO) => Promise<void>;
}
interface EditProps extends CommonProps {
  mode: "edit";
  onSave: (data: UpdateChargeDTO, id: string) => Promise<void>;
  initial: Partial<CreateChargeDTO> & { id: string; status?: string };
}

type NovaCobrancaModalProps = CreateProps | EditProps;

const defaultReferenceMonth = () => {
  const d = new Date();
  const month = String(d.getMonth() + 1).padStart(2, "0");
  return `${d.getFullYear()}-${month}`;
};

const TYPE_OPTIONS = [
  { value: "RENT", label: "Aluguel" },
  { value: "MAINTENANCE", label: "Manutenção / Reparo" },
  { value: "CONDOMINIO", label: "Condomínio" },
  { value: "IPTU", label: "IPTU" },
  { value: "FEE", label: "Taxa Administrativa" },
  { value: "OTHER", label: "Outros" },
];

const LINE_SOURCE_OPTIONS = [
  { value: "rental_base", label: "Aluguel base / regra do contrato" },
  { value: "rental_bundled_item", label: "Item incluso na locação" },
  { value: "utility_account", label: "Conta de consumo cadastrada" },
  { value: "manual", label: "Origem manual (exceção)" },
];

const BUNDLED_ITEM_OPTIONS = [
  { value: "condominio", label: "Condomínio" },
  { value: "iptu", label: "IPTU" },
  { value: "admin_fee", label: "Taxa de administração" },
  { value: "seguro_fianca", label: "Seguro-fiança" },
  { value: "custom", label: "Outro item configurado" },
];

const SETTLEMENT_MODE_OPTIONS = [
  { value: "arrecadacao", label: "Arrecadação pela imobiliária" },
  { value: "liquidacao_direta", label: "Liquidação direta ao credor" },
  { value: "reembolso", label: "Reembolso" },
  { value: "compensacao", label: "Compensação" },
];

const UTILITY_TYPE_LABELS: Record<string, string> = {
  AGUA: "Água",
  ENERGIA: "Energia",
  GAS: "Gás",
  CONDOMINIO: "Condomínio",
  IPTU: "IPTU",
  INTERNET: "Internet",
  SEGURO: "Seguro",
  OUTROS: "Outros",
};

function withCurrentOption(options: { value: string; label: string; searchText?: string }[], current?: string) {
  if (!current || options.some((option) => option.value === current)) return options;
  return [{ value: current, label: `Valor existente: ${current}` }, ...options];
}

const lineItemSchema = z.object({
  id: z.string().optional(),
  description: z.string(),
  amount: z.number().optional(),
  source_type: z.string().optional(),
  source_id: z.string().optional(),
  economic_owner_id: z.string().optional(),
  settlement_mode: z.enum(["arrecadacao", "liquidacao_direta", "reembolso", "compensacao"]).optional(),
  rule_version_id: z.string().optional(),
});

type FormLineItem = z.infer<typeof lineItemSchema>;

type FormValues = {
  contract_id: string;
  type: string;
  reference_month: string;
  amount?: number;
  due_date: string; // yyyy-mm-dd
  description: string;
  discount?: number;
  line_items: FormLineItem[];
};

export function NovaCobrancaModal(props: NovaCobrancaModalProps) {
  const { initial, onClose, contracts, properties } = props;
  const isEdit = props.mode === "edit";
  const navigate = useNavigate();

  const blockedReason = useMemo(() => {
    if (!isEdit) return null;
    if (initial && initial.status && !canEditCharge({ status: initial.status } as Charge)) {
      return `Cobrança ${initial.status} não pode ser editada. Cancele e crie uma nova se for o caso.`;
    }
    return null;
  }, [isEdit, initial]);

  const schema = useMemo(
    () =>
      z
        .object({
          contract_id: z.string(),
          type: z.string(),
          reference_month: z.string(),
          amount: z.number().optional(),
          due_date: z.string().min(1, "Informe o vencimento"),
          description: z.string(),
          discount: z.number().optional(),
          line_items: z.array(lineItemSchema),
        })
        .superRefine((data, ctx) => {
          if (!isEdit && !data.contract_id) {
            ctx.addIssue({ path: ["contract_id"], code: z.ZodIssueCode.custom, message: "Selecione um contrato" });
          }
          if (data.line_items.length > 0) {
            data.line_items.forEach((li, i) => {
              if (!li.description.trim() || !li.amount || li.amount <= 0) {
                ctx.addIssue({
                  path: ["line_items", i, "amount"],
                  code: z.ZodIssueCode.custom,
                  message: "Descrição e valor > 0",
                });
              }
            });
          } else if (!data.amount || data.amount <= 0) {
            ctx.addIssue({ path: ["amount"], code: z.ZodIssueCode.custom, message: "Informe o valor" });
          }
        }),
    [isEdit],
  );

  const buildDefaults = (): FormValues => ({
    type: initial?.type || "RENT",
    reference_month: initial?.reference_month || defaultReferenceMonth(),
    contract_id: initial?.contract_id || "",
    amount: initial?.amount,
    due_date: initial?.due_date ? initial.due_date.split("T")[0] : "",
    description: initial?.description || "",
    discount: initial?.discount,
    line_items: (initial?.line_items ?? []).map((li) => ({
      id: li.id,
      description: li.description,
      amount: li.amount_cents / 100,
      source_type: li.source_type,
      source_id: li.source_id,
      economic_owner_id: li.economic_owner_id,
      settlement_mode: li.settlement_mode,
      rule_version_id: li.rule_version_id,
    })),
  });

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    mode: "onTouched",
    defaultValues: buildDefaults(),
  });
  const { fields: itemFields, append, remove } = useFieldArray({ control: form.control, name: "line_items" });

  // Re-seed quando o `initial` muda (modo edição reaproveita a mesma instância).
  useEffect(() => {
    if (!isEdit) return;
    form.reset(buildDefaults());
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    initial?.id,
    isEdit,
    initial?.amount,
    initial?.due_date,
    initial?.description,
    initial?.discount,
    initial?.contract_id,
    initial?.reference_month,
    initial?.type,
    initial?.line_items,
  ]);

  const w = form.watch();
  const contractId = w.contract_id;
  const [utilityAccounts, setUtilityAccounts] = useState<UtilityAccount[]>([]);

  const selectedContract = useMemo(
    () => (contractId ? contracts.find((c) => c.id === contractId) : undefined),
    [contractId, contracts],
  );
  const resolvedProperty = useMemo(
    () => (selectedContract?.property_id ? properties.find((p) => p.id === selectedContract.property_id) : undefined),
    [selectedContract?.property_id, properties],
  );
  const resolvedOwnerId = resolvedProperty?.ownerId || resolvedProperty?.owners?.[0]?.id || "";
  const resolvedTenantPartyId = selectedContract?.customer_id ?? "";
  const resolvedPropertyId = selectedContract?.property_id ?? "";

  const hasUtilityLineItem = (w.line_items ?? []).some((item) => item.source_type === "utility_account");
  useEffect(() => {
    if (!contractId || !hasUtilityLineItem) {
      setUtilityAccounts([]);
      return;
    }
    let cancelled = false;
    utilityAccountService
      .list(contractId)
      .then((accounts) => {
        if (!cancelled) setUtilityAccounts(Array.isArray(accounts) ? accounts : []);
      })
      .catch(() => {
        if (!cancelled) setUtilityAccounts([]);
      });
    return () => {
      cancelled = true;
    };
  }, [contractId, hasUtilityLineItem]);

  const issues = useMemo(
    () => (!isEdit && selectedContract ? buildContractIssues(selectedContract, resolvedProperty, navigate) : []),
    [isEdit, selectedContract, resolvedProperty, navigate],
  );

  const hasLineItems = (w.line_items?.length ?? 0) > 0;
  const lineItemsTotal = (w.line_items ?? []).reduce((sum, li) => sum + (li.amount ?? 0), 0);
  const effectiveAmount = hasLineItems ? lineItemsTotal : w.amount;

  const contractOptions = contracts.map((c) => ({
    value: c.id,
    label: `${c.property_title || "Imóvel sem título"} - ${c.client || "Sem cliente"}`,
    searchText: `${c.property_title || ""} ${c.client || ""} ${c.id}`,
  }));

  const onValid = async (values: FormValues) => {
    if (blockedReason) {
      toast.error(blockedReason);
      return;
    }
    if (!isEdit && !selectedContract) {
      toast.error("Contrato selecionado não encontrado.");
      return;
    }
    if (!isEdit && issues.length > 0) {
      showContractIssueToasts(issues);
      return;
    }
    const lineItemsPayload =
      values.line_items.length > 0
        ? values.line_items.map(({ amount, ...li }) => ({
            ...(li.id ? { id: li.id } : {}),
            description: li.description.trim(),
            amount_cents: Math.round((amount ?? 0) * 100),
            ...(li.source_type ? { source_type: li.source_type } : {}),
            ...(li.source_id ? { source_id: li.source_id } : {}),
            ...(li.economic_owner_id ? { economic_owner_id: li.economic_owner_id } : {}),
            ...(li.settlement_mode ? { settlement_mode: li.settlement_mode } : {}),
            ...(li.rule_version_id ? { rule_version_id: li.rule_version_id } : {}),
          }))
        : undefined;
    const finalAmount = values.line_items.length > 0 ? lineItemsTotal : values.amount;
    const dueDateISO = new Date(values.due_date).toISOString();

    try {
      if (props.mode === "edit") {
        const payload: UpdateChargeDTO = {
          type: values.type,
          description: values.description?.trim() || undefined,
          reference_month: values.reference_month || undefined,
          amount: finalAmount,
          due_date: dueDateISO,
          discount: values.discount,
          line_items: lineItemsPayload,
        };
        await props.onSave(payload, props.initial.id);
      } else {
        const payload: CreateChargeDTO = {
          contract_id: values.contract_id,
          tenant_party_id: resolvedTenantPartyId,
          owner_id: resolvedOwnerId,
          property_id: resolvedPropertyId,
          type: values.type || "RENT",
          description:
            values.description?.trim() ||
            `Cobrança ${values.type || "RENT"} ${values.reference_month || defaultReferenceMonth()}`,
          reference_month: values.reference_month || defaultReferenceMonth(),
          amount: finalAmount!,
          due_date: dueDateISO,
          discount: values.discount,
          line_items: lineItemsPayload,
        };
        await props.onSave(payload);
      }
      onClose();
    } catch (err) {
      console.error(err);
    }
  };

  const title = isEdit ? "Editar Cobrança" : "Nova Cobrança";
  const description = isEdit ? "Ajuste os termos da cobrança." : "Crie uma cobrança manual vinculada a um contrato";
  const Icon = isEdit ? Pencil : DollarSign;
  const submitLabel = isEdit ? "Salvar Alterações" : "Criar Cobrança";

  return (
    <Sheet
      open
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
    >
      <SheetContent
        side="right"
        expandable
        expandedWidth="sm:max-w-3xl lg:max-w-5xl"
        className="flex h-full w-full flex-col gap-0 overflow-hidden p-0 sm:max-w-lg"
      >
        <div className="flex items-center gap-3 border-b border-border bg-muted/50 px-6 py-4 pr-12">
          <div className="p-2 bg-primary-100 rounded-xl">
            <Icon className="w-5 h-5 text-primary-600" />
          </div>
          <div>
            <SheetTitle className="text-lg font-bold text-foreground">{title}</SheetTitle>
            <SheetDescription className="text-xs">{description}</SheetDescription>
          </div>
        </div>

        <Form {...form}>
          <form onSubmit={form.handleSubmit(onValid)} className="flex min-h-0 flex-1 flex-col">
            <div className="min-h-0 flex-1 space-y-6 overflow-y-auto p-6">
            {blockedReason && (
              <div className="p-3 bg-status-critical-soft border border-status-critical/30 rounded-xl text-xs text-status-critical flex gap-2">
                <AlertCircle className="w-4 h-4 flex-shrink-0 mt-0.5" />
                <p>{blockedReason}</p>
              </div>
            )}
            <div className="space-y-4">
              <div>
                {isEdit ? (
                  <>
                    <label className="block text-sm font-bold text-foreground mb-1">Contrato</label>
                    <div className="px-4 py-2.5 rounded-xl border border-border bg-muted/40 text-sm text-foreground/80">
                      {selectedContract
                        ? `${selectedContract.property_title || "Imóvel"} — ${selectedContract.client || "Cliente"}`
                        : contractId || "—"}
                      <span className="ml-2 text-xs text-muted-foreground">(imutável)</span>
                    </div>
                  </>
                ) : (
                  <SelectField
                    control={form.control}
                    name="contract_id"
                    label="Contrato"
                    required
                    placeholder="Selecione um contrato..."
                    options={contractOptions}
                    searchable
                    searchPlaceholder="Buscar contrato, cliente ou imóvel..."
                  />
                )}
                {!isEdit && selectedContract && issues.length > 0 && (
                  <div className="mt-2 p-3 bg-status-critical-soft border border-status-critical/30 rounded-xl text-xs text-status-critical">
                    <div className="flex items-start gap-2 mb-2">
                      <AlertCircle className="w-4 h-4 flex-shrink-0 mt-0.5" />
                      <p className="font-bold">Contrato incompleto — resolva antes de emitir:</p>
                    </div>
                    <ul className="space-y-1.5 ml-6">
                      {issues.map((issue) => (
                        <li key={issue.ref} className="flex items-center justify-between gap-3">
                          <span>{issue.message}</span>
                          <button
                            type="button"
                            onClick={issue.onCtaClick}
                            className="inline-flex items-center gap-1 font-bold underline hover:no-underline whitespace-nowrap"
                          >
                            {issue.ctaLabel}
                            <ArrowUpRight className="w-3 h-3" />
                          </button>
                        </li>
                      ))}
                    </ul>
                  </div>
                )}
                {!isEdit && selectedContract && (
                  <div className="mt-2 rounded-xl border border-border bg-muted/30 p-3 text-xs text-muted-foreground">
                    <p className="font-bold text-foreground">Relações que serão gravadas</p>
                    <div className="mt-2 grid grid-cols-1 gap-1 sm:grid-cols-3">
                      <span>Cliente: <strong className="text-foreground">{selectedContract.client || "não identificado"}</strong></span>
                      <span>Imóvel: <strong className="text-foreground">{resolvedProperty?.title || selectedContract.property_title || "não identificado"}</strong></span>
                      <span>Proprietário: <strong className="text-foreground">{resolvedOwnerId || "não identificado"}</strong></span>
                    </div>
                    {!resolvedTenantPartyId || !resolvedOwnerId || !resolvedPropertyId ? (
                      <p className="mt-2 text-status-critical">Complete cliente, imóvel e proprietário no contrato/imóvel antes de emitir.</p>
                    ) : null}
                  </div>
                )}
              </div>

              <div className="grid grid-cols-1 items-start gap-4 sm:grid-cols-2">
                {hasLineItems ? (
                  <div className="grid min-h-[82px] content-start gap-2">
                    <label className="text-sm font-bold text-foreground">Valor (R$) *</label>
                    <MoneyInput
                      withSymbol
                      readOnly
                      value={effectiveAmount}
                      onChange={() => {}}
                      className="w-full px-4 py-2.5 rounded-xl border border-border bg-muted/40 text-foreground/80 cursor-not-allowed transition-all outline-none"
                      placeholder="R$ 0,00"
                    />
                    <p className="text-xs text-muted-foreground">Calculado pela soma dos itens da composição.</p>
                  </div>
                ) : (
                  <MoneyField control={form.control} name="amount" label="Valor (R$)" required className="min-h-[76px] items-start" />
                )}
                <DateField control={form.control} name="due_date" label="Vencimento" required className="min-h-[76px] items-start" />
              </div>

              <MoneyField
                control={form.control}
                name="discount"
                label="Desconto (R$)"
                description="Total cobrado = valor − desconto."
                className="min-h-[76px] items-start"
              />

              <div className="rounded-xl border border-border bg-muted/30 p-3 space-y-3">
                <div className="flex items-center justify-between">
                  <div>
                    <p className="text-sm font-bold text-foreground">Composição da cobrança (itens inclusos)</p>
                    <p className="text-xs text-muted-foreground">
                      Opcional. Detalhe o que compõe o valor (ex.: Aluguel + Condomínio + IPTU).
                    </p>
                  </div>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => append({ description: "", amount: undefined })}
                  >
                    <Plus className="w-3.5 h-3.5" /> Item
                  </Button>
                </div>
                {itemFields.length > 0 && (
                  <div className="space-y-2">
                    {itemFields.map((field, idx) => {
                      const item = w.line_items?.[idx];
                      const sourceType = item?.source_type;
                      const sourceIdOptions = sourceType === "rental_bundled_item"
                        ? withCurrentOption(BUNDLED_ITEM_OPTIONS, item?.source_id)
                        : sourceType === "utility_account"
                          ? withCurrentOption(
                              utilityAccounts.map((account) => ({
                              value: account.id,
                              label: `${UTILITY_TYPE_LABELS[account.type] || account.type} · ${account.provider}`,
                              searchText: `${UTILITY_TYPE_LABELS[account.type] || account.type} ${account.provider} ${account.id}`,
                            })),
                              item?.source_id,
                            )
                          : [];
                      const economicOwnerOptions = withCurrentOption(
                        [
                          ...(resolvedOwnerId ? [{ value: resolvedOwnerId, label: "Proprietário do imóvel" }] : []),
                          ...(resolvedTenantPartyId ? [{ value: resolvedTenantPartyId, label: "Cliente / inquilino" }] : []),
                        ],
                        item?.economic_owner_id,
                      );

                      return (
                        <div key={field.id} className="space-y-2 rounded-lg border border-border/70 bg-card/60 p-2">
                          <div className="grid grid-cols-1 items-start gap-2 sm:grid-cols-[minmax(0,1fr)_8rem_auto]">
                            <TextField
                              control={form.control}
                              name={`line_items.${idx}.description`}
                              placeholder="Descrição (ex.: Condomínio)"
                              className="min-h-[72px] min-w-0 items-start"
                            />
                            <MoneyField
                              control={form.control}
                              name={`line_items.${idx}.amount`}
                              className="min-h-[72px] min-w-0 items-start"
                              inputClassName="text-right"
                              placeholder="0,00"
                            />
                            <Button
                              type="button"
                              variant="ghost"
                              size="icon"
                              className="h-9 w-9 shrink-0 text-status-critical"
                              onClick={() => remove(idx)}
                              title="Remover item"
                            >
                              <Trash2 className="w-3.5 h-3.5" />
                            </Button>
                          </div>

                          <div className="grid grid-cols-1 items-start gap-2 sm:grid-cols-2">
                            <SelectField
                              control={form.control}
                              name={`line_items.${idx}.source_type`}
                              label="Origem"
                              placeholder="Selecione a origem"
                              options={withCurrentOption(LINE_SOURCE_OPTIONS, sourceType)}
                              className="min-h-[76px] items-start"
                            />
                            <SelectField
                              control={form.control}
                              name={`line_items.${idx}.settlement_mode`}
                              label="Liquidação econômica"
                              placeholder="Padrão do contrato"
                              options={SETTLEMENT_MODE_OPTIONS}
                              className="min-h-[76px] items-start"
                            />
                          </div>

                          {sourceType === "rental_bundled_item" && (
                            <SelectField
                              control={form.control}
                              name={`line_items.${idx}.source_id`}
                              label="Item configurado"
                              placeholder="Selecione o item"
                              options={sourceIdOptions}
                              className="min-h-[76px] items-start"
                            />
                          )}
                          {sourceType === "utility_account" && (
                            <SelectField
                              control={form.control}
                              name={`line_items.${idx}.source_id`}
                              label="Conta cadastrada"
                              placeholder={utilityAccounts.length > 0 ? "Selecione a conta" : "Nenhuma conta cadastrada"}
                              options={sourceIdOptions}
                              disabled={utilityAccounts.length === 0}
                              searchable
                              searchPlaceholder="Buscar conta cadastrada..."
                              className="min-h-[76px] items-start"
                            />
                          )}
                          {(sourceType === "manual" || sourceType === "rental_base") && (
                            <TextField
                              control={form.control}
                              name={`line_items.${idx}.source_id`}
                              label="Identificador da origem"
                              placeholder={sourceType === "rental_base" ? "ID da regra do contrato" : "ID externo ou referência"}
                              description="Preencha apenas quando a origem não estiver no cadastro."
                              className="min-h-[76px] items-start"
                            />
                          )}

                          {economicOwnerOptions.length > 0 && (
                            <SelectField
                              control={form.control}
                              name={`line_items.${idx}.economic_owner_id`}
                              label="Titular econômico"
                              placeholder="Selecione o titular"
                              options={economicOwnerOptions}
                              description="Define quem é o dono econômico deste item no repasse/conciliação."
                              className="min-h-[76px] items-start"
                            />
                          )}
                          {item?.rule_version_id && (
                            <p className="text-[11px] text-muted-foreground">Regra preservada: {item.rule_version_id}</p>
                          )}
                        </div>
                      );
                    })}
                    <div className="flex items-center justify-between pt-1 text-sm border-t border-border">
                      <span className="text-muted-foreground">Soma dos itens</span>
                      <span className="font-bold text-foreground tabular-nums">
                        {formatMoneyBR(lineItemsTotal, { withSymbol: true, fractionDigits: 2 })}
                      </span>
                    </div>
                  </div>
                )}
              </div>

              <SelectField
                control={form.control}
                name="type"
                label="Tipo de Cobrança"
                options={TYPE_OPTIONS}
                className="min-h-[76px] items-start"
              />

              <TextField
                control={form.control}
                name="description"
                label="Descrição"
                placeholder="Ex: Reparo hidráulico banheiro"
                className="min-h-[76px] items-start"
              />

              <TextField
                control={form.control}
                name="reference_month"
                label="Mês de Referência"
                type="month"
                className="min-h-[76px] items-start"
              />
            </div>

            </div>
            <SheetFooter className="mt-auto flex-row justify-end gap-2 border-t border-border bg-background px-6 py-4">
              <Button type="button" variant="outline" onClick={onClose}>
                Cancelar
              </Button>
              <Button
                type="submit"
                loading={form.formState.isSubmitting}
                loadingText="Salvando..."
                disabled={!!blockedReason || (!isEdit && !!selectedContract && issues.length > 0)}
              >
                {submitLabel}
              </Button>
            </SheetFooter>
          </form>
        </Form>
      </SheetContent>
    </Sheet>
  );
}
