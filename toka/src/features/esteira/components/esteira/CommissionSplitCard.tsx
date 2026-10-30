"use client";

/**
 * CommissionSplitCard + CommissionSplitEditor
 *
 * Card de superfície exibido na aba "Proposta" da Esteira Digital (apenas SALE).
 * Permite ao admin/gestor editar a distribuição de comissão entre corretores
 * (captador, co-captador, visitador, atendente, fechamento, imobiliária) e
 * ajustar o percentual / valor total da comissão.
 */

import { useEffect, useMemo, useState } from "react";
import { Percent, Users, AlertTriangle, Info, Loader2 } from "lucide-react";
import { toast } from "sonner";

import { Button } from "../../../../shared/ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "../../../../shared/ui/dialog";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "../../../../shared/ui/select";
import { MoneyInput } from "../../../../shared/ui/money-input";
import { PercentInput } from "../../../../shared/ui/percent-input";
import { ConfirmDialog } from "../../../../shared/components/ConfirmDialog";
import { brokerService, type BrokerDTO } from "../../../../shared/services/broker.service";
import { type BrokerParticipation, type BrokerParticipationRole, type DealDTO } from "../../../../shared/services/deal.service";
import { commissionRulesService } from "../../../../shared/services/commissionRules.service";
import { tenantFinancialSettingsService } from "../../../../shared/services/tenantFinancialSettings.service";
import { leadService } from "../../../../shared/services/lead.service";
import { propertyService, type PropertyDTO } from "../../../../shared/services/property.service";
import { unwrapApiData } from "../../../../shared/services/api";
import { useDealsContext } from "../../../../shared/context/DealsContext";
import { useCan } from "../../../../shared/permissions/usePermission";
import { formatCurrencyBRL0 } from "../../../../shared/utils/format";
import {
  buildCommissionSplitEditorRows,
  buildCommissionSplitAutofillParticipants,
  evaluateCommissionSplitRows,
  serializeCommissionSplitRows,
  type CommissionRolePercentages,
  type CommissionSplitAutofillParticipant,
  type CommissionSplitEditorRow,
} from "./commissionSplitEditor";

// ── Labels & defaults ──────────────────────────────────────────────

const ROLE_LABEL: Record<BrokerParticipationRole, string> = {
  captador: "Captador",
  co_captador: "Co-captador",
  visitador: "Visitador",
  atendente: "Atendente",
  fechamento: "Fechamento",
  imobiliaria: "Imobiliária",
};

const FALLBACK_COMMISSION_DEFAULT_SALE = 6;
const FALLBACK_COMMISSION_DEFAULT_RENT = 100;

// ── Props ───────────────────────────────────────────────────────────

interface CommissionSplitCardProps {
  deal: DealDTO;
  dealValue: number; // deal.value — base de cálculo da comissão
  locked?: boolean; // deal CONVERTED ou CANCELLED
  onUpdated?: () => void;
}

// ── Card ────────────────────────────────────────────────────────────

export function CommissionSplitCard({ deal, dealValue, locked, onUpdated }: CommissionSplitCardProps) {
  const can = useCan();
  // A comissão do negócio é decisão de gestão: admin, gestor, operações e
  // financeiro (`commissions:write`). Antes o gate era `deals:write`, que o
  // corretor tem — ele conseguia editar o próprio percentual na esteira.
  // O backend exige a MESMA permissão no PUT/POST das participações.
  const canEdit = can("commissions:write") && !locked;
  const { updateBrokerParticipations } = useDealsContext();

  // ── Parametrização unificada do backend ──────────────────────────
  // Substitui os percentuais globais de fallback e
  // FALLBACK_COMMISSION_DEFAULT_* pelos valores reais configurados
  // no tenant (tenantFinancialSettings + commissionRulesService).
  // A resolução espelha a ordem do backend:
  //   (1) deal.commission (campo explícito no negócio)
  //   (2) tenantFinancialSettings (default por tipo SALE/RENT)
  //   (3) fallback local (6% venda / 100% locação)
  const [commissionDefaults, setCommissionDefaults] = useState<{
    rolePercentages: Record<string, number>;
    salePct: number;
    rentPct: number;
    loaded: boolean;
  }>({
    rolePercentages: {},
    salePct: FALLBACK_COMMISSION_DEFAULT_SALE,
    rentPct: FALLBACK_COMMISSION_DEFAULT_RENT,
    loaded: false,
  });

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const [rules, settings] = await Promise.all([
          commissionRulesService.get().catch(() => []),
          tenantFinancialSettingsService.get().catch(() => null),
        ]);
        if (cancelled) return;
        const roleMap: Record<string, number> = {};
        (rules || []).forEach((r: any) => {
          roleMap[r.role] = r.default_percentage;
        });
        setCommissionDefaults({
          rolePercentages: roleMap,
          salePct: settings?.default_commission_pct_sale ?? FALLBACK_COMMISSION_DEFAULT_SALE,
          rentPct: settings?.default_commission_pct_rent ?? FALLBACK_COMMISSION_DEFAULT_RENT,
          loaded: true,
        });
      } catch {
        if (!cancelled) {
          setCommissionDefaults(prev => ({ ...prev, loaded: true }));
        }
      }
    })();
    return () => { cancelled = true; };
  }, []);

  const [autofillParticipants, setAutofillParticipants] = useState<{
    value: CommissionSplitAutofillParticipant[];
    loaded: boolean;
  }>({ value: [], loaded: false });

  useEffect(() => {
    let cancelled = false;
    setAutofillParticipants({ value: [], loaded: false });

    const primaryCustomerRole = deal.type === "RENT" ? "tenant" : "buyer";
    const primaryPartyLeadId = deal.parties?.find(
      (party) => party.role === primaryCustomerRole && party.is_primary,
    )?.lead_id;
    const leadId = deal.lead_id || primaryPartyLeadId || deal.customer_id;

    Promise.all([
      deal.property_id
        ? propertyService
            .getById(deal.property_id)
            .then((response) => unwrapApiData<PropertyDTO>(response))
            .catch(() => null)
        : Promise.resolve(null),
      leadId ? leadService.getById(leadId).catch(() => null) : Promise.resolve(null),
    ]).then(([property, lead]) => {
      if (cancelled) return;
      setAutofillParticipants({
        value: buildCommissionSplitAutofillParticipants(
          property?.capturing_brokers ?? [],
          lead?.assigned_broker_id,
        ),
        loaded: true,
      });
    });

    return () => {
      cancelled = true;
    };
  }, [deal.customer_id, deal.lead_id, deal.parties, deal.property_id, deal.type]);

  const participations = useMemo(
    () => deal.broker_participations ?? [],
    [deal.broker_participations],
  );

  // Resolve o percentual de comissão seguindo a hierarquia do backend:
  // 1. deal.commission (explícito) → 2. tenant default por tipo → 3. fallback
  const commissionPct = deal.commission > 0
    ? deal.commission
    : (deal.type === 'RENT' ? commissionDefaults.rentPct : commissionDefaults.salePct);
  const totalCommission = dealValue * (commissionPct / 100);

  // Cache de nomes dos brokers para exibição
  const [brokerNames, setBrokerNames] = useState<Record<string, string>>({});
  useEffect(() => {
    const ids = new Set(participations.map((p) => p.broker_id).filter(Boolean));
    const missing = [...ids].filter((id) => !brokerNames[id]);
    if (missing.length === 0) return;
    Promise.all(missing.map((id) => brokerService.getById(id).catch(() => null))).then(
      (results) => {
        const map: Record<string, string> = {};
        for (const r of results) {
          if (r?.data?.data) {
            const b = r.data.data as BrokerDTO;
            map[b.id] = b.name;
          }
        }
        setBrokerNames((prev) => ({ ...prev, ...map }));
      },
    );
  }, [participations]);

  const [editing, setEditing] = useState(false);

  return (
    <div className="rounded-xl border border-border bg-card overflow-hidden">
      {/* Cabeçalho */}
      <div className="px-4 py-3 flex items-center justify-between gap-2 border-b border-border">
        <div className="flex items-center gap-2 min-w-0">
          <Users className="w-4 h-4 text-muted-foreground shrink-0" />
          <h4 className="text-caption font-bold text-foreground">Comissão</h4>
          {locked && (
            <span className="inline-flex items-center gap-1 text-caption font-bold px-1.5 py-0.5 rounded bg-muted text-muted-foreground">
              Travado
            </span>
          )}
        </div>
        {canEdit && (
          <Button
            size="sm"
            variant="outline"
            onClick={() => setEditing(true)}
            disabled={!commissionDefaults.loaded || !autofillParticipants.loaded}
          >
            {commissionDefaults.loaded && autofillParticipants.loaded
              ? "Editar"
              : <Loader2 className="w-4 h-4 animate-spin" />}
          </Button>
        )}
      </div>

      {/* Resumo */}
      <div className="px-4 py-3 space-y-2">
        <div className="flex items-center justify-between gap-2">
          <span className="text-micro text-muted-foreground">Valor do negócio</span>
          <span className="text-caption font-semibold text-foreground tabular-nums">
            {formatCurrencyBRL0(dealValue)}
          </span>
        </div>
        <div className="flex items-center justify-between gap-2">
          <span className="text-micro text-muted-foreground">Comissão total ({commissionPct}%)</span>
          <span className="text-caption font-semibold text-foreground tabular-nums">
            {formatCurrencyBRL0(totalCommission)}
          </span>
        </div>
      </div>

      {/* Validação cross-module: alerta quando splits não somam 100% */}
      {participations.length > 0 && (() => {
        const sumPct = participations.reduce((s, p) => s + (p.percentage ?? 0), 0);
        if (Math.abs(sumPct - 100) >= 0.02) {
          return (
            <div className="px-4 py-2 bg-status-warning-soft border-b border-status-warning/20">
              <p className="text-caption text-status-warning flex items-center gap-1.5">
                <AlertTriangle className="w-3.5 h-3.5 shrink-0" />
                A soma dos splits é {sumPct.toFixed(1)}% — deveria ser 100%.
                Edite a distribuição para corrigir.
              </p>
            </div>
          );
        }
        return null;
      })()}

      {/* Indicador de remainder retido pela imobiliária */}
      {participations.length > 0 && (() => {
        const imob = participations.find(p => p.role === 'imobiliaria');
        const retainedPct = imob?.percentage ?? 0;
        if (imob && retainedPct > 0) {
          return (
            <div className="px-4 py-1.5 bg-blue-50 dark:bg-blue-950/30 border-b border-blue-200 dark:border-blue-800">
              <p className="text-caption text-blue-700 dark:text-blue-300 flex items-center gap-1.5">
                <Info className="w-3.5 h-3.5 shrink-0" />
                Receita retida pela imobiliária: {retainedPct.toFixed(1)}% —{' '}
                {formatCurrencyBRL0(totalCommission * (retainedPct / 100))}
              </p>
            </div>
          );
        }
        return null;
      })()}

      {/* Tabela de splits */}
      {participations.length > 0 && (
        <div className="border-t border-border">
          <div className="divide-y divide-border">
            {participations.map((p, participationIndex) => {
              const name = p.role === "imobiliaria"
                ? "Receita retida pela imobiliária"
                : brokerNames[p.broker_id] || p.broker_id;
              const sharePct = p.percentage ?? 0;
              const shareAmount = totalCommission * (sharePct / 100);
              return (
                <div
                  key={`${p.broker_id}-${p.role}-${participationIndex}`}
                  className="px-4 py-2 flex items-center justify-between gap-2"
                >
                  <div className="min-w-0">
                    <p className="text-caption font-medium text-foreground truncate">{name}</p>
                    <p className="text-micro text-muted-foreground">{ROLE_LABEL[p.role as BrokerParticipationRole] ?? p.role}</p>
                  </div>
                  <div className="text-right shrink-0">
                    <p className="text-caption font-semibold text-foreground tabular-nums">
                      {formatCurrencyBRL0(shareAmount)}
                    </p>
                    <p className="text-micro text-muted-foreground tabular-nums">
                      {sharePct.toFixed(1)}%
                    </p>
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}

      {/* Modal de edição */}
      {editing && (
        <CommissionSplitEditor
          deal={deal}
          dealValue={dealValue}
          commissionPct={commissionPct}
          existingParticipations={participations}
          rolePercentages={commissionDefaults.rolePercentages}
          autofillParticipants={autofillParticipants.value}
          open={editing}
          onOpenChange={setEditing}
          updateBrokerParticipations={updateBrokerParticipations}
          onSaved={() => {
            setEditing(false);
            onUpdated?.();
          }}
        />
      )}
    </div>
  );
}

// ── Editor Modal ────────────────────────────────────────────────────

interface EditorProps {
  deal: DealDTO;
  dealValue: number;
  commissionPct: number;
  existingParticipations: BrokerParticipation[];
  /** Percentuais por role carregados do backend (commissionRulesService) */
  rolePercentages: CommissionRolePercentages;
  autofillParticipants: CommissionSplitAutofillParticipant[];
  open: boolean;
  onOpenChange: (v: boolean) => void;
  updateBrokerParticipations: (
    dealId: string,
    participations: BrokerParticipation[],
    commissionPct?: number,
  ) => Promise<DealDTO>;
  onSaved: () => void;
}

function CommissionSplitEditor({
  deal,
  dealValue,
  commissionPct: initialCommissionPct,
  existingParticipations,
  rolePercentages,
  autofillParticipants,
  open,
  onOpenChange,
  updateBrokerParticipations,
  onSaved,
}: EditorProps) {
  // ── Estado ──
  const [commissionPct, setCommissionPct] = useState(initialCommissionPct);
  const [commissionAmount, setCommissionAmount] = useState(dealValue * (initialCommissionPct / 100));
  const [rows, setRows] = useState<CommissionSplitEditorRow[]>([]);
  const [brokers, setBrokers] = useState<BrokerDTO[]>([]);
  const [brokersLoading, setBrokersLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);

  // ── Inicialização ──
  useEffect(() => {
    if (!open) return;
    setBrokersLoading(true);
    // Carrega lista de brokers
    brokerService.getAll().then((res) => {
      const list = (res.data?.data ?? []) as BrokerDTO[];
      setBrokers(list.filter((b) => b.active));
    }).catch(() => {
      toast.error("Não foi possível carregar a lista de corretores.");
    }).finally(() => {
      setBrokersLoading(false);
    });

    setRows(buildCommissionSplitEditorRows(
      existingParticipations,
      rolePercentages,
      autofillParticipants,
    ));
  }, [open, existingParticipations, rolePercentages, autofillParticipants]);

  // ── Sincronização percentage ↔ amount ──

  const updatePct = (pct: number | undefined) => {
    const safe = pct ?? 0;
    setCommissionPct(safe);
    setCommissionAmount(safe > 0 ? dealValue * (safe / 100) : 0);
  };

  const updateAmount = (amt: number | undefined) => {
    const safe = amt ?? 0;
    setCommissionAmount(safe);
    if (dealValue > 0) {
      setCommissionPct((safe / dealValue) * 100);
    } else {
      setCommissionPct(0);
    }
  };

  // ── Manipulação de linhas ──
  const updateRowPercentage = (idx: number, pct: number) => {
    setRows((prev) => prev.map((r, i) => (i === idx ? { ...r, percentage: Math.max(0, Math.min(100, pct)) } : r)));
  };

  const updateRowBroker = (idx: number, brokerId: string) => {
    setRows((prev) => prev.map((r, i) => (i === idx ? { ...r, brokerId } : r)));
  };

  // ── Validação ──
  const {
    sum,
    sumOk,
    hasEmptyBroker,
    hasDuplicateParticipant,
    hasRowsChanged,
  } = evaluateCommissionSplitRows(rows, existingParticipations, rolePercentages);
  const hasAnyChanged =
    Math.abs(commissionPct - initialCommissionPct) > 0.005 ||
    hasRowsChanged;

  const canSave = sumOk && !hasEmptyBroker && !hasDuplicateParticipant && hasAnyChanged;

  // ── Save ──
  const handleSave = async () => {
    setSaving(true);
    try {
      const participations = serializeCommissionSplitRows(rows);

      await updateBrokerParticipations(
        deal.id,
        participations,
        commissionPct !== initialCommissionPct ? commissionPct : undefined,
      );
      toast.success("Distribuição de comissão atualizada");
      setConfirmOpen(false);
      onSaved();
    } catch (err: any) {
      const msg = err?.response?.data?.message ?? err?.message ?? "Erro ao salvar";
      toast.error(msg);
    } finally {
      setSaving(false);
    }
  };

  // ── Totais calculados ──
  const totalCommissionAmount = dealValue * (commissionPct / 100);

  // Negócio assinado tem o plano de comissão congelado nas cobranças; editar o
  // rateio depois disso não altera as comissões já materializadas.
  const planFrozen = deal.status === "CONTRACT" || deal.status === "CONVERTED";

  // ── Render ──
  return (
    <>
      <Dialog open={open} onOpenChange={(v) => { if (!saving) onOpenChange(v); }}>
        <DialogContent className="max-w-xl w-full max-h-[90vh] overflow-hidden flex flex-col p-0 gap-0">
          {/* Header */}
          <DialogHeader className="px-5 pt-5 pb-2 shrink-0">
            <DialogTitle className="flex items-center gap-2">
              <Percent className="w-5 h-5 text-primary" />
              Editar comissão
            </DialogTitle>
            <DialogDescription>
              Ajuste o percentual total da comissão e a distribuição entre os corretores.
            </DialogDescription>
          </DialogHeader>

          {/* Body com scroll */}
          <div className="px-5 py-3 space-y-5 overflow-y-auto flex-1">
            {planFrozen && (
              // O plano de comissão é congelado na assinatura: as cobranças de
              // venda nascem com o pool daquele rateio e as comissões/payouts
              // são materializados a partir dele. Alterar aqui depois disso não
              // recalcula o que já existe — sem este aviso o operador mudava o
              // rateio achando que corrigia a comissão já gerada.
              <div className="flex items-start gap-2 rounded-xl border border-status-warning/30 bg-status-warning-soft p-3 text-xs text-status-warning">
                <AlertTriangle className="mt-0.5 size-4 shrink-0" />
                <div className="min-w-0 leading-relaxed">
                  <p className="font-bold">Rateio já congelado na assinatura</p>
                  <p className="mt-1">
                    Este negócio já foi assinado e as comissões foram geradas com o rateio vigente naquele
                    momento. Salvar aqui vale para os próximos negócios; para corrigir este, estorne as
                    comissões geradas e refaça a distribuição.
                  </p>
                </div>
              </div>
            )}
            {/* ── Comissão total: percentage + amount ── */}
            <section className="rounded-lg border border-border bg-muted/30 p-4 space-y-3">
              <p className="text-micro font-bold text-muted-foreground uppercase tracking-wide">
                Comissão total
              </p>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                <div className="space-y-1.5">
                  <label className="text-caption text-muted-foreground">Percentual (%)</label>
                  <PercentInput
                    value={commissionPct}
                    onChange={updatePct}
                    className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm tabular-nums focus:outline-none focus:ring-2 focus:ring-primary"
                  />
                </div>
                <div className="space-y-1.5">
                  <label className="text-caption text-muted-foreground">Valor (R$)</label>
                  <MoneyInput
                    value={commissionAmount}
                    onChange={updateAmount}
                    fractionDigits={0}
                    withSymbol={false}
                    className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm tabular-nums focus:outline-none focus:ring-2 focus:ring-primary"
                  />
                </div>
              </div>
              <p className="text-micro text-muted-foreground">
                Valor do negócio: <span className="font-semibold text-foreground">{formatCurrencyBRL0(dealValue)}</span>
                {" "}&middot;{" "}
                Total: <span className="font-semibold text-foreground">{formatCurrencyBRL0(totalCommissionAmount)}</span>
              </p>
            </section>

            {/* ── Per-role rows ── */}
            <section>
              <p className="text-micro font-bold text-muted-foreground uppercase tracking-wide mb-3">
                Distribuição por função
              </p>
              <div className="space-y-2">
                {rows.map((row, i) => {
                  const amount = totalCommissionAmount * (row.percentage / 100);
                  return (
                    <div
                      key={row.rowKey}
                      className="grid grid-cols-1 sm:grid-cols-[1fr_140px_100px] gap-2 items-center rounded-lg border border-border bg-card px-3 py-2"
                    >
                      {/* Role + Broker select */}
                      <div className="min-w-0">
                        <p className="text-caption font-medium text-foreground truncate">
                          {ROLE_LABEL[row.role]}
                        </p>
                        {row.role === "imobiliaria" ? (
                          <p className="mt-1 text-xs text-muted-foreground">
                            Sem favorecido: valor retido internamente.
                          </p>
                        ) : (
                          <Select
                            value={row.brokerId || "__none__"}
                            onValueChange={(v) => updateRowBroker(i, v === "__none__" ? "" : v)}
                            disabled={row.percentage === 0 || brokersLoading}
                          >
                            <SelectTrigger className="mt-1 w-full h-8 text-xs">
                              <SelectValue
                                placeholder={brokersLoading ? "Carregando corretores…" : "Selecionar corretor…"}
                              />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem value="__none__" className="text-xs">
                                Nenhum
                              </SelectItem>
                              {brokers.map((b) => (
                                <SelectItem key={b.id} value={b.id} className="text-xs">
                                  {b.name}
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                        )}
                      </div>

                      {/* Percentage */}
                      <div className="flex items-center gap-1">
                        <PercentInput
                          value={row.percentage}
                          onChange={(v) => updateRowPercentage(i, v ?? 0)}
                          className="w-full rounded-lg border border-border bg-background px-2 py-1 text-sm tabular-nums text-right focus:outline-none focus:ring-2 focus:ring-primary"
                        />
                      </div>

                      {/* Amount (read-only) */}
                      <div className="text-right text-sm tabular-nums text-muted-foreground">
                        {formatCurrencyBRL0(amount)}
                      </div>
                    </div>
                  );
                })}
              </div>

              {/* Sum bar */}
              <div className="mt-3 flex items-center justify-between">
                <span className="text-micro text-muted-foreground">Soma dos percentuais</span>
                <span
                  className={`text-caption font-bold tabular-nums ${
                    sumOk ? "text-status-success" : "text-status-destructive"
                  }`}
                >
                  {sum.toFixed(1)}% de 100%
                </span>
              </div>
              {!sumOk && (
                <p className="text-micro text-status-destructive mt-1">
                  A soma deve ser exatamente 100%. Ajuste os percentuais.
                </p>
              )}
              {hasEmptyBroker && (
                <p className="text-micro text-status-destructive mt-0.5">
                  Corretor é obrigatório para toda função com percentual acima de 0%.
                </p>
              )}
              {hasDuplicateParticipant && (
                <p className="text-micro text-status-destructive mt-0.5">
                  O mesmo corretor não pode aparecer duas vezes na mesma função.
                </p>
              )}
            </section>
          </div>

          {/* Footer */}
          <div className="px-5 py-4 border-t border-border flex justify-end gap-2 shrink-0 bg-card">
            <Button variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
              Cancelar
            </Button>
            <Button
              onClick={() => setConfirmOpen(true)}
              disabled={!canSave || saving}
              loading={saving}
            >
              Salvar
            </Button>
          </div>
        </DialogContent>
      </Dialog>

      {/* Modal de confirmação institucional */}
      <ConfirmDialog
        open={confirmOpen}
        onCancel={() => setConfirmOpen(false)}
        onConfirm={handleSave}
        title="Confirmar alterações"
        description={
          <div className="text-left space-y-2">
            <p>Você está prestes a atualizar a distribuição de comissão deste negócio.</p>
            <ul className="list-disc pl-4 text-sm space-y-0.5">
              <li>Comissão total: <strong>{commissionPct.toFixed(1)}%</strong> ({formatCurrencyBRL0(totalCommissionAmount)})</li>
              {rows.filter((r) => r.percentage > 0).map((r) => (
                <li key={r.rowKey}>
                  {ROLE_LABEL[r.role]}: <strong>{r.percentage.toFixed(1)}%</strong>
                  {" "}({formatCurrencyBRL0(totalCommissionAmount * (r.percentage / 100))})
                </li>
              ))}
            </ul>
            {commissionPct !== initialCommissionPct && (
              <p className="text-xs text-muted-foreground">
                O percentual da comissão foi alterado de {initialCommissionPct.toFixed(1)}% para {commissionPct.toFixed(1)}%.
              </p>
            )}
          </div>
        }
        confirmLabel="Salvar alterações"
      />
    </>
  );
}
