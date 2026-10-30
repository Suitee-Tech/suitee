import { useMemo, useState } from "react";
import {
  AlertTriangle,
  Building2,
  Calendar,
  CheckCircle2,
  Copy,
  DollarSign,
  FileText,
  History as HistoryIcon,
  Home,
  ListTree,
  Pencil,
  Receipt,
  Send,
  User,
  Wrench,
  X,
  XCircle,
} from "lucide-react";
import { clsx } from "clsx";
import { toast } from "sonner";
import { useNavigate } from "react-router";

import { Button } from "../ui/button";
import { Sheet, SheetContent, SheetDescription, SheetTitle } from "../ui/sheet";
import { TooltipText } from "../ui/TooltipText";
import { formatCompetencia, formatISODateBR, formatDateTimeBR } from "../utils/format";
import { DivergenceResolutionCard } from "./lancamentos/DivergenceResolutionCard";
import { chargeToLancamento } from "../lib/lancamentos/mappers";
import type { Charge } from "../services/charge.service";
import { availableChargeActions, mapChargeStatus, normalizeChargeStatus, type ChargeAction } from "../utils/charge";
import { usePermissionsContext } from "../context/PermissionsContext";
import { useRepairRentalAllocationsEligibilityQuery, useRepairRentalAllocationsMutation } from "../hooks/useContracts";
import { CancelarCobrancaDialog } from "./CancelarCobrancaDialog";
import { getChargeComposition } from "../utils/chargeComposition";
import { getChargeContextLabels } from "../lib/finance/chargeLabels";

export interface ChargeDetailContractInfo {
  id?: string;
  property_title?: string;
  client?: string;
}

interface ChargeDetailDrawerProps {
  charge: Charge | null;
  contract?: ChargeDetailContractInfo;
  ownerLabel?: string;
  tenantLabel?: string;
  propertyLabel?: string;
  onClose: () => void;
  onAction: (action: ChargeAction, charge: Charge) => void;
  onResolveDivergence?: () => void;
}

const formatBRL = (cents: number | undefined) => {
  if (cents === undefined || cents === null || isNaN(cents)) return "—";
  return cents.toLocaleString("pt-BR", { style: "currency", currency: "BRL" });
};

const formatDate = (iso?: string) => {
  if (!iso) return "—";
  const out = formatISODateBR(iso);
  return out || "—";
};

const formatDateTime = (iso?: string) => {
  if (!iso) return "—";
  const out = formatDateTimeBR(iso);
  return out || "—";
};

const CHARGE_TYPE_LABEL: Record<string, string> = {
  RENT: "Aluguel",
  MAINTENANCE: "Manutenção",
  CONDOMINIO: "Condomínio",
  IPTU: "IPTU",
  FEE: "Taxa Administrativa",
  OTHER: "Outros",
  SALE: "Venda",
  UTILITY: "Concessionária",
};

const SETTLEMENT_MODE_LABEL: Record<string, string> = {
  arrecadacao: "Arrecadação",
  liquidacao_direta: "Liquidação direta",
  reembolso: "Reembolso",
  compensacao: "Compensação",
};

const ALLOCATION_TYPE_LABEL: Record<string, string> = {
  accounts_receivable: "Contas a receber",
  revenue: "Receita da imobiliária",
  owner_payable: "Repasse ao proprietário",
  commission_payable: "Comissão a pagar",
  partner_payable: "Repasse ao parceiro",
  royalty_payable: "Royalty a pagar",
  tax_withheld: "Tributo retido",
};

const partyLabel = (partyId: string | undefined, charge: Charge, ownerLabel?: string) => {
  if (!partyId) return "Não informado";
  if (partyId === charge.owner_id) return ownerLabel || "Proprietário";
  if (partyId === charge.tenant_id) return "Imobiliária";
  return partyId;
};

// Raio econômico da cobrança de locação: quanto do valor colecionado vai para
// cada dono econômico. Prefere os ALLOCATIONS realizados (por pagamento, já
// com allocation_type); sem pagamentos, usa os LineItems como projeção.
export type EconomicBreakdown = {
  valor: number;
  desconto: number;
  total: number;
  pago: number;
  aReceber: number;
  imobiliaria: number;
  corretor: number;
  proprietario: number;
  parceiro: number;
  tributario: number;
  outros: number;
};

const isImobiliariaItem = (item: { allocation_type?: string; economic_owner_id?: string; settlement_mode?: string }) =>
  item.allocation_type === "revenue" || item.settlement_mode === "arrecadacao";

const classifyByAllocationType = (t: string | undefined): keyof EconomicBreakdown => {
  switch (t) {
    case "revenue":
      return "imobiliaria";
    case "owner_payable":
      return "proprietario";
    case "commission_payable":
      return "corretor";
    case "partner_payable":
      return "parceiro";
    case "tax_withheld":
      return "tributario";
    default:
      return "outros";
  }
};

export function computeEconomicBreakdown(charge: Charge): EconomicBreakdown {
  const valor = charge.amount ?? 0;
  const desconto = charge.discount ?? 0;
  const total = charge.total_amount ?? valor;
  const paid = charge.payments?.length
    ? charge.payments.reduce((sum, p) => sum + (p.amount_cents ?? 0), 0) / 100
    : (charge.paid_amount ?? 0);

  const buckets: EconomicBreakdown = {
    valor,
    desconto,
    total,
    pago: paid,
    aReceber: Math.max(total - paid, 0),
    imobiliaria: 0,
    corretor: 0,
    proprietario: 0,
    parceiro: 0,
    tributario: 0,
    outros: 0,
  };

  // Allocations realizados (fonte da verdade após a baixa).
  const allocs = (charge.payments ?? []).flatMap((p) => p.allocations ?? []);
  if (allocs.length > 0) {
    for (const a of allocs) {
      const key = classifyByAllocationType(a.allocation_type);
      buckets[key] = (buckets[key] ?? 0) + (a.amount_cents ?? 0) / 100;
    }
    return buckets;
  }

  // Sem pagamento ainda: projeção pelos LineItems (composição econômica).
  const items = charge.line_items ?? [];
  if (items.length === 0) return buckets;
  for (const li of items) {
    const cents = (li.amount_cents ?? 0) / 100;
    if (li.allocation_type) {
      const key = classifyByAllocationType(li.allocation_type);
      buckets[key] = (buckets[key] ?? 0) + cents;
    } else if (li.economic_owner_id && li.economic_owner_id === charge.tenant_id) {
      // A imobiliária é o dono econômico (ex.: taxa de administração).
      buckets.imobiliaria += cents;
    } else if (li.economic_owner_id && li.economic_owner_id === charge.owner_id) {
      // O aluguel base e encargos do proprietário vão para o repasse.
      buckets.proprietario += cents;
    } else if (isImobiliariaItem(li)) {
      buckets.imobiliaria += cents;
    } else {
      buckets.outros += cents;
    }
  }
  return buckets;
}

const StatusPill = ({ status }: { status: string }) => {
  const label = mapChargeStatus(status);
  const style = useMemo(() => {
    switch (label) {
      case "Pago":
        return "bg-status-ok-soft text-status-ok border-status-ok/30";
      case "Em Atraso":
        return "bg-status-critical-soft text-status-critical border-status-critical/30";
      case "Cancelada":
        return "bg-muted text-muted-foreground border-border";
      default:
        return "bg-status-warning-soft text-status-warning border-status-warning/30";
    }
  }, [label]);
  const Icon = label === "Pago" ? CheckCircle2 : label === "Cancelada" ? XCircle : AlertTriangle;
  return (
    <span className={clsx("inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-bold border", style)}>
      <Icon className="w-3.5 h-3.5" /> {label}
    </span>
  );
};

const ACTION_META: Record<ChargeAction, { label: string; icon: typeof DollarSign; variant?: "default" | "destructive" | "outline" | "warning" }> = {
  pay: { label: "Marcar Pago", icon: DollarSign, variant: "default" },
  edit: { label: "Editar", icon: Pencil, variant: "outline" },
  cancel: { label: "Cancelar", icon: XCircle, variant: "destructive" },
  notify: { label: "Notificar", icon: Send, variant: "outline" },
  receipt: { label: "Recibo", icon: Receipt, variant: "outline" },
};

export function ChargeDetailDrawer({ charge, contract, ownerLabel, tenantLabel, propertyLabel, onClose, onAction, onResolveDivergence }: ChargeDetailDrawerProps) {
  const navigate = useNavigate();
  const { can } = usePermissionsContext();
  const [repairOpen, setRepairOpen] = useState(false);
  const repairMutation = useRepairRentalAllocationsMutation(charge?.contract_id ?? null);
  const labels = getChargeContextLabels(charge?.type);

  // Pré-condições baratas (tipo/status/permissão) antes de consultar o backend:
  // a elegibilidade real (plano congelado com saldo + pagamento sem
  // intermediação) vem do endpoint read-only e é o gate do botão.
  const repairPrerequisitesMet =
    Boolean(charge) &&
    charge!.type === "RENT" &&
    normalizeChargeStatus(charge!.status) === "PAID" &&
    can("finance:approve_transfer");
  const repairEligibility = useRepairRentalAllocationsEligibilityQuery(
    charge?.id ?? null,
    repairPrerequisitesMet,
  );
  const canRepairIntermediation = repairPrerequisitesMet && repairEligibility.data?.eligible === true;

  const copyId = () => {
    if (!charge?.id) return;
    navigator.clipboard.writeText(charge.id).then(() => toast.success("ID copiado"));
  };

  const actions = charge ? availableChargeActions(charge.status) : [];
  const lancamento = charge ? chargeToLancamento(charge) : null;
  const breakdown = useMemo(() => (charge ? computeEconomicBreakdown(charge) : null), [charge]);
  const composition = charge ? getChargeComposition(charge) : null;

  return (
    <>
      {/*
        Primitiva compartilhada (Radix Dialog via Sheet): papel de diálogo,
        título associado, portal, contenção de foco, Escape e retorno do foco
        ao gatilho. O overlay artesanal com motion.div não tinha nenhum disso.
      */}
      <Sheet open={Boolean(charge)} onOpenChange={(next) => { if (!next) onClose(); }}>
      <SheetContent
        side="right"
        expandable
        className="w-full gap-0 overflow-hidden p-0 sm:max-w-lg"
        data-testid="charge-detail-drawer"
      >
        {charge && (
          <>
            <div className="px-6 py-4 border-b border-border flex items-start justify-between gap-3 pr-20">
              <div className="min-w-0">
                <SheetTitle className="text-xl font-bold text-foreground flex items-center gap-2">
                  <FileText className="w-5 h-5 text-primary-600" />
                  Detalhes da Cobrança
                </SheetTitle>
                <SheetDescription className="sr-only">
                  Composição, titularidade e ações da cobrança selecionada.
                </SheetDescription>
                <button
                  type="button"
                  onClick={copyId}
                  className="mt-1 inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors group"
                  aria-label={`Copiar ID da cobrança ${charge.id}`}
                >
                  <span className="font-mono truncate max-w-[260px]">{charge.id}</span>
                  <Copy className="w-3 h-3 opacity-50 group-hover:opacity-100" aria-hidden="true" />
                </button>
              </div>
            </div>

            <div className="flex-1 overflow-y-auto p-6 space-y-6">
              <div className="flex items-start justify-between gap-4 flex-wrap">
                <div className="min-w-0">
                  <p className="text-xs font-bold text-muted-foreground mb-1">Descrição</p>
                  <p className="text-base font-bold text-foreground break-words">{charge.description || "(sem descrição)"}</p>
                </div>
                <StatusPill status={charge.status} />
              </div>

              {lancamento && (
                <DivergenceResolutionCard
                  lancamento={lancamento}
                  actionLabel={onResolveDivergence ? "Abrir conciliação" : undefined}
                  onAction={onResolveDivergence}
                />
              )}

              <div className="grid grid-cols-2 gap-4">
                <FactCell icon={ListTree} label="Tipo" value={CHARGE_TYPE_LABEL[charge.type] || charge.type} />
                <FactCell icon={FileText} label="Contrato" value={contract?.property_title || charge.contract_id || "Avulso"} />
                <FactCell icon={Calendar} label="Vencimento" value={formatDate(charge.due_date)} />
                <FactCell icon={Calendar} label="Mês de Referência" value={formatCompetencia(charge.reference_month)} />
                {charge.paid_at && <FactCell icon={CheckCircle2} label="Data do Pagamento" value={formatDateTime(charge.paid_at)} accent="ok" />}
                {charge.payment_method && <FactCell icon={DollarSign} label="Método" value={charge.payment_method.toUpperCase()} />}
              </div>

              {charge.contract_id && (
                <div className="flex flex-wrap items-center gap-2">
                  <Button variant="outline" size="sm" onClick={() => { onClose(); navigate(`/contratos/${charge.contract_id}`); }}>
                    <FileText className="w-4 h-4" /> Ver contrato
                  </Button>
                  {charge.type !== "SALE" && (
                    <Button variant="outline" size="sm" onClick={() => { onClose(); navigate(`/locacao/${charge.contract_id}`); }}>
                      <Home className="w-4 h-4" /> Ver locação
                    </Button>
                  )}
                </div>
              )}

              <div className="bg-muted rounded-xl p-5 border border-border space-y-3">
                <RowKV label="Valor Original" value={formatBRL(charge.amount)} />
                {charge.late_fee > 0 && <RowKV label="Multa" value={formatBRL(charge.late_fee)} accent="warning" />}
                {charge.interest > 0 && <RowKV label="Juros" value={formatBRL(charge.interest)} accent="warning" />}
                {charge.discount > 0 && <RowKV label="Desconto" value={`- ${formatBRL(charge.discount)}`} accent="ok" />}
                <div className="h-px bg-border w-full" />
                <RowKV label="Total" value={formatBRL(charge.total_amount)} totalStyle />
                {charge.paid_amount !== undefined && <RowKV label="Pago" value={formatBRL(charge.paid_amount)} accent="ok" />}
              </div>

              <Section title="Contraparte">
                <FactCell icon={User} label={labels.buyer} value={tenantLabel || contract?.client || charge.tenant_party_id} />
                <FactCell icon={Home} label="Imóvel" value={propertyLabel || contract?.property_title || charge.property_id} />
                <FactCell icon={Building2} label={labels.seller} value={ownerLabel || charge.owner_id} />
              </Section>

              {breakdown && (
                <Section title="Destinos do valor">
                  <div className="col-span-2 rounded-xl border border-border overflow-hidden">
                    <BreakdownRow label={labels.original} value={breakdown.valor} />
                    {breakdown.desconto > 0 && <BreakdownRow label={labels.discounts} value={-breakdown.desconto} negative />}
                    <BreakdownRow label={labels.total} value={breakdown.total} bold />
                    <BreakdownRow label={labels.paid} value={breakdown.pago} tone={breakdown.pago > 0 ? "ok" : "muted"} />
                    <BreakdownRow label={labels.agencyFee} value={breakdown.imobiliaria} prefix="→" tone="primary" />
                    <BreakdownRow label={labels.brokerFee} value={breakdown.corretor} prefix="→" tone="primary" />
                    <BreakdownRow label={labels.ownerTransfer} value={breakdown.proprietario} prefix="→" tone="ok" />
                    {breakdown.parceiro > 0 && <BreakdownRow label={labels.partnerTransfer} value={breakdown.parceiro} prefix="→" />}
                    {breakdown.tributario > 0 && <BreakdownRow label={labels.taxWithheld} value={breakdown.tributario} prefix="→" />}
                    {breakdown.outros > 0 && <BreakdownRow label={labels.other} value={breakdown.outros} prefix="→" tone="muted" />}
                    {breakdown.aReceber > 0 && (
                      <BreakdownRow label={labels.outstanding} value={breakdown.aReceber} tone="warning" />
                    )}
                  </div>
                  <p className="col-span-2 text-micro text-muted-foreground">{labels.nature}</p>
                  {breakdown.total - breakdown.pago <= 0 && (breakdown.imobiliaria + breakdown.corretor + breakdown.proprietario) < breakdown.pago - 0.01 && (
                    <p className="col-span-2 text-xs text-status-warning">
                      Parte do valor pago ainda não foi alocada a um destino econômico — verifique a composição abaixo.
                    </p>
                  )}
                </Section>
              )}

              {charge.line_items && charge.line_items.length > 0 && (
                <Section title="Composição da cobrança">
                  <div className="col-span-2 rounded-xl border border-border overflow-hidden">
                    {composition?.items.map((item) => (
                      <BreakdownRow key={item.key} label={item.description} value={item.amountCents / 100} />
                    ))}
                  </div>
                  {composition?.rentalSplit && (
                    <p className="col-span-2 text-xs text-muted-foreground">
                      A taxa de administração de {formatBRL(composition.rentalSplit.administrationCents / 100)} está incluída no aluguel e é deduzida da parcela do proprietário. O aluguel líquido do proprietário é {formatBRL(composition.rentalSplit.ownerRentCents / 100)}.
                    </p>
                  )}
                </Section>
              )}

              {charge.line_items && charge.line_items.length > 0 && (
                <Section title="Composição econômica">
                  <div className="col-span-2 rounded-xl border border-border overflow-hidden">
                    {charge.line_items.map((li, i) => (
                      <div
                        key={li.id || `${li.source_id || i}-${li.description}`}
                        className={clsx("px-3 py-3 text-sm", i !== 0 && "border-t border-border")}
                      >
                        <div className="flex items-start justify-between gap-3">
                          <span className="font-bold text-foreground">{composition?.rentalSplit?.ownerItemIndex === i ? "Aluguel líquido do proprietário" : li.description}</span>
                          <span className="shrink-0 font-mono font-bold text-foreground">{formatBRL(li.amount_cents / 100)}</span>
                        </div>
                        <div className="mt-2 flex flex-wrap items-center gap-2 text-xs">
                          <span className="rounded-full border border-primary-200 bg-primary-50 px-2 py-0.5 font-bold text-primary-700">
                            {SETTLEMENT_MODE_LABEL[li.settlement_mode || ""] || li.settlement_mode || "Modo não informado"}
                          </span>
                          {li.allocation_type && (
                            <span className="rounded-full border border-status-ok/30 bg-status-ok-soft px-2 py-0.5 font-bold text-status-ok">
                              {ALLOCATION_TYPE_LABEL[li.allocation_type] || li.allocation_type}
                            </span>
                          )}
                          <span className="text-muted-foreground" title={li.economic_owner_id}>
                            Titular econômico: <strong className="text-foreground">{partyLabel(li.economic_owner_id, charge, ownerLabel)}</strong>
                          </span>
                        </div>
                        {(li.id || li.rule_version_id) && (
                          <TooltipText
                            text={[li.id && `Item ${li.id}`, li.rule_version_id && `Regra ${li.rule_version_id}`].filter(Boolean).join(" · ")}
                            className="mt-2 block truncate font-mono text-[11px] text-muted-foreground"
                          >
                            {li.id && `Item ${li.id}`}
                            {li.id && li.rule_version_id && " · "}
                            {li.rule_version_id && `Regra ${li.rule_version_id}`}
                          </TooltipText>
                        )}
                      </div>
                    ))}
                  </div>
                </Section>
              )}

              {charge.payments && charge.payments.length > 0 && (
                <Section title={`Pagamentos e composição alocada (${charge.payments.length})`}>
                  <div className="col-span-2 space-y-3">
                    {charge.payments.map((payment, paymentIndex) => {
                      const allocations = payment.allocations ?? [];
                      const allocatedCents = allocations.reduce((sum, allocation) => sum + allocation.amount_cents, 0);
                      const allocationMismatch = allocations.length > 0 && allocatedCents !== payment.amount_cents;
                      return (
                        <div key={payment.id || `${payment.paid_at}-${paymentIndex}`} className="rounded-xl border border-border p-3">
                          <div className="flex items-start justify-between gap-3">
                            <div>
                              <p className="text-sm font-bold text-foreground">Pagamento {paymentIndex + 1}</p>
                              <p className="text-xs text-muted-foreground">
                                {formatDateTime(payment.paid_at)} · {payment.method || "Método não informado"}
                                {payment.allocation_method && ` · ${payment.allocation_method}`}
                              </p>
                            </div>
                            <span className="shrink-0 font-mono text-sm font-bold text-status-ok">
                              {formatBRL(payment.amount_cents / 100)}
                            </span>
                          </div>

                          {allocations.length > 0 ? (
                            <div className="mt-3 overflow-hidden rounded-lg border border-border">
                              {allocations.map((allocation, allocationIndex) => {
                                const item = charge.line_items?.find((candidate) => candidate.id === allocation.charge_item_id);
                                return (
                                  <div
                                    key={allocation.id || `${allocation.charge_item_id}-${allocationIndex}`}
                                    className={clsx("flex items-start justify-between gap-3 px-3 py-2", allocationIndex !== 0 && "border-t border-border")}
                                  >
                                    <div className="min-w-0">
                                      <p className="text-xs font-bold text-foreground">
                                        {item?.description || `Item ${allocation.charge_item_id}`}
                                      </p>
                                      <p className="text-xs text-muted-foreground">
                                        {ALLOCATION_TYPE_LABEL[allocation.allocation_type] || allocation.allocation_type}
                                        {" · "}Beneficiário: {partyLabel(allocation.beneficiary_party_id, charge, ownerLabel)}
                                      </p>
                                      {allocation.rule_version_id && (
                                        <TooltipText
                                          text={allocation.rule_version_id}
                                          className="block truncate font-mono text-[11px] text-muted-foreground"
                                        >
                                          Regra {allocation.rule_version_id}
                                        </TooltipText>
                                      )}
                                    </div>
                                    <span className="shrink-0 font-mono text-xs font-bold text-foreground">
                                      {formatBRL(allocation.amount_cents / 100)}
                                    </span>
                                  </div>
                                );
                              })}
                            </div>
                          ) : (
                            <p className="mt-3 rounded-lg bg-muted/50 px-3 py-2 text-xs text-muted-foreground">
                              Pagamento sem composição econômica registrada (legado).
                            </p>
                          )}

                          {allocations.length > 0 && (
                            <p className={clsx("mt-2 text-right text-xs", allocationMismatch ? "font-bold text-status-warning" : "text-muted-foreground")}>
                              Alocado: {formatBRL(allocatedCents / 100)} de {formatBRL(payment.amount_cents / 100)}
                            </p>
                          )}
                        </div>
                      );
                    })}
                  </div>
                </Section>
              )}

              {charge.reminders_sent && charge.reminders_sent.length > 0 && (
                <Section title={`Lembretes Enviados (${charge.reminders_sent.length})`}>
                  <div className="space-y-2">
                    {charge.reminders_sent.map((r, i) => (
                      <div key={i} className="flex items-start gap-2 text-xs text-muted-foreground">
                        <HistoryIcon className="w-3.5 h-3.5 mt-0.5 shrink-0" />
                        <div>
                          <span className="font-bold text-foreground">{formatDateTime(r.sent_at)}</span>
                          {r.channels?.length > 0 && <span> · {r.channels.join(", ")}</span>}
                          {r.sent_by_label && <span> · por {r.sent_by_label}</span>}
                        </div>
                      </div>
                    ))}
                  </div>
                </Section>
              )}

              <Section title="Metadados">
                <FactCell icon={Calendar} label="Criado em" value={formatDateTime(charge.created_at)} />
                <FactCell icon={Calendar} label="Atualizado em" value={formatDateTime(charge.updated_at)} />
              </Section>
            </div>

            <div className="px-6 py-4 border-t border-border bg-muted/30 flex items-center justify-between flex-wrap gap-2">
              <Button variant="ghost" onClick={onClose}>
                Fechar
              </Button>
              <div className="flex items-center gap-2 flex-wrap justify-end">
                {canRepairIntermediation && (
                  <Button variant="outline" size="sm" onClick={() => setRepairOpen(true)}>
                    <Wrench className="w-4 h-4" /> Reparar alocações
                  </Button>
                )}
                {actions.length === 0 && !canRepairIntermediation ? (
                  <span className="text-xs text-muted-foreground italic">Nenhuma ação disponível para este status</span>
                ) : (
                  actions.map((a) => {
                    const meta = ACTION_META[a];
                    const Icon = meta.icon;
                    return (
                      <Button key={a} variant={meta.variant || "default"} size="sm" onClick={() => onAction(a, charge)}>
                        <Icon className="w-4 h-4" /> {meta.label}
                      </Button>
                    );
                  })
                )}
              </div>
            </div>
          </>
        )}
      </SheetContent>
    </Sheet>

    <CancelarCobrancaDialog
      open={repairOpen}
      title="Reparar alocação da intermediação"
      description="Use quando as primeiras cobranças foram baixadas antes de o plano de intermediação existir: a comissão do corretor não foi financiada e o repasse ao proprietário saiu cheio. A ação re-deriva as alocações, estorna e regrava o lançamento do recebimento e recalcula o repasse. É idempotente — se já estiver correto, nada muda."
      contextLine={charge ? `${charge.description} · ${charge.reference_month}` : undefined}
      confirmLabel="Reparar alocações"
      loadingText="Reparando..."
      minReasonLength={10}
      wide
      onCancel={() => setRepairOpen(false)}
      onConfirm={async (reason) => {
        if (!charge) return;
        try {
          await repairMutation.mutateAsync({ chargeId: charge.id, reason });
          setRepairOpen(false);
        } catch {
          // toast já tratado no hook; mantém o diálogo aberto para nova tentativa
        }
      }}
    />
    </>
  );
}

function FactCell({
  icon: Icon,
  label,
  value,
  accent,
}: {
  icon: typeof DollarSign;
  label: string;
  value: React.ReactNode;
  accent?: "ok" | "warning" | "critical";
}) {
  const color =
    accent === "ok"
      ? "text-status-ok"
      : accent === "warning"
        ? "text-status-warning"
        : accent === "critical"
          ? "text-status-critical"
          : "text-foreground";
  return (
    <div>
      <p className="text-xs font-bold text-muted-foreground mb-1">{label}</p>
      <div className={clsx("flex items-center gap-2 text-sm font-bold", color)}>
        <Icon className={clsx("w-4 h-4 shrink-0", accent ? color : "text-primary-600")} />
        <span className="truncate" title={typeof value === "string" ? value : undefined}>
          {value || "—"}
        </span>
      </div>
    </div>
  );
}

function RowKV({
  label,
  value,
  accent,
  totalStyle,
}: {
  label: string;
  value: React.ReactNode;
  accent?: "ok" | "warning";
  totalStyle?: boolean;
}) {
  const color = accent === "ok" ? "text-status-ok" : accent === "warning" ? "text-status-warning" : "text-foreground";
  return (
    <div className={clsx("flex justify-between items-center", totalStyle ? "text-base" : "text-sm")}>
      <span className={clsx(totalStyle ? "font-bold text-foreground" : "text-muted-foreground")}>{label}</span>
      <span className={clsx("font-bold", totalStyle ? "text-primary-600 text-xl" : color)}>{value}</span>
    </div>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div>
      <h3 className="text-xs font-bold uppercase tracking-wider text-muted-foreground mb-2">{title}</h3>
      <div className="grid grid-cols-2 gap-4">{children}</div>
    </div>
  );
}

function BreakdownRow({
  label,
  value,
  negative,
  prefix,
  tone,
  bold,
}: {
  label: string;
  value: number;
  negative?: boolean;
  prefix?: string;
  tone?: "ok" | "primary" | "warning" | "muted";
  bold?: boolean;
}) {
  const color =
    tone === "ok"
      ? "text-status-ok"
      : tone === "primary"
        ? "text-primary-700"
        : tone === "warning"
          ? "text-status-warning"
          : tone === "muted"
            ? "text-muted-foreground"
            : "text-foreground";
  const abs = Math.abs(value);
  const sign = negative ? "− " : prefix ? `${prefix} ` : "";
  return (
    <div className={clsx("flex items-start justify-between gap-3 px-3 py-2 text-sm border-t border-border first:border-t-0")}>
      <span className="min-w-0 text-muted-foreground">{label}</span>
      <span className={clsx("shrink-0 font-mono font-bold tabular-nums", color, bold && "text-base")}>
        {sign}
        {abs.toLocaleString("pt-BR", { style: "currency", currency: "BRL" })}
      </span>
    </div>
  );
}
