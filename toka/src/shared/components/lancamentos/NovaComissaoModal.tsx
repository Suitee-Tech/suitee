import { useMemo, useEffect, useId, useRef, useState, type FormEvent } from "react";
import { Briefcase, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";

import { useAgentsContext } from "../../context/AgentsContext";
import { useContractsContext } from "../../context/ContractsContext";
import { useEsteiraDeals } from "../../hooks/useEsteiraDeals";
import { commissionService, type CommissionSplitInput } from "../../services/commission.service";
import { commissionRulesService } from "../../services/commissionRules.service";
import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetTitle } from "../../ui/sheet";
import { Button } from "../../ui/button";
import { MoneyInput } from "../../ui/money-input";
import { SearchableSelect } from "../../ui/searchable-select";
import { formatMoneyBR } from "../../utils/format";

interface NovaComissaoModalProps {
  onClose: () => void;
  onCreated: () => void;
}

interface SplitRow {
  broker_id: string;
  broker_mode?: "catalog" | "manual";
  role: string;
  percentage?: number;
  amount?: number;
}

const ROLE_OPTIONS: { value: string; label: string }[] = [
  { value: "captador", label: "Captador" },
  { value: "co_captador", label: "Co-captador" },
  { value: "visitador", label: "Visitador" },
  { value: "atendente", label: "Atendente" },
  { value: "fechamento", label: "Fechamento" },
  { value: "imobiliaria", label: "Imobiliária" },
];

const MANUAL_OPTION = "__manual__";
const NO_SELECTION_OPTION = "__none__";
const emptyRow = (): SplitRow => ({ broker_id: "", broker_mode: "catalog", role: "fechamento", percentage: undefined, amount: undefined });

const newIdempotencyKey = () => {
  const suffix =
    typeof crypto !== "undefined" && typeof crypto.randomUUID === "function"
      ? crypto.randomUUID()
      : `${Date.now()}-${Math.random().toString(36).slice(2)}`;
  return `manual-commission-batch:${suffix}`;
};

export function NovaComissaoModal({ onClose, onCreated }: NovaComissaoModalProps) {
  const { deals } = useEsteiraDeals();
  const { contracts } = useContractsContext();
  const { agents } = useAgentsContext();
  const hasDeals = deals.length > 0;

  const fieldId = useId();
  const [dealId, setDealId] = useState("");
  const [contractSelection, setContractSelection] = useState("");
  const [splits, setSplits] = useState<SplitRow[]>(() => [emptyRow()]);
  const [roleDefaults, setRoleDefaults] = useState<Record<string, number>>({});
  const [saving, setSaving] = useState(false);
  const pendingAttemptRef = useRef<{ fingerprint: string; idempotencyKey: string } | null>(null);
  const requestInFlightRef = useRef(false);

  // Integra as regras configuradas (commissionRulesService) para pré-preencher
  // o % default por papel — mesma fonte da esteira (§14 do compêndio).
  useEffect(() => {
    let cancelled = false;
    commissionRulesService
      .get()
      .catch(() => [])
      .then((rules) => {
        if (cancelled) return;
        const map: Record<string, number> = {};
        (rules || []).forEach((r) => {
          map[r.role] = r.default_percentage;
        });
        setRoleDefaults(map);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const resolvedContractId = useMemo(() => {
    if (!dealId) return undefined;
    const found = contracts.find((c) => c.deal_id === dealId || (c as { dealId?: string }).dealId === dealId);
    return found?.id;
  }, [dealId, contracts]);

  const selectedDeal = useMemo(() => deals.find((d) => d.id === dealId), [deals, dealId]);
  const linkedContracts = useMemo(
    () => contracts.filter((contract) => contract.deal_id === dealId || (contract as { dealId?: string }).dealId === dealId),
    [contracts, dealId],
  );
  const effectiveContractId = contractSelection === MANUAL_OPTION
    ? undefined
    : contractSelection || resolvedContractId;
  const effectiveContract = useMemo(
    () => contracts.find((contract) => contract.id === effectiveContractId),
    [contracts, effectiveContractId],
  );
  const commissionPool = effectiveContract?.value && effectiveContract.commission_pct
    ? (effectiveContract.value * effectiveContract.commission_pct) / 100
    : undefined;

  useEffect(() => {
    setContractSelection("");
  }, [dealId]);

  const addRow = () => setSplits((prev) => [...prev, { ...emptyRow(), percentage: roleDefaults["fechamento"] ?? undefined }]);
  const removeRow = (idx: number) => setSplits((prev) => (prev.length === 1 ? prev : prev.filter((_, i) => i !== idx)));
  const updateRow = (idx: number, patch: Partial<SplitRow>) => setSplits((prev) => prev.map((r, i) => (i === idx ? { ...r, ...patch } : r)));
  // Ao trocar o papel, pré-preenche o % default configurado se ainda vazio.
  const updateRowRole = (idx: number, role: string) =>
    setSplits((prev) =>
      prev.map((r, i) =>
        i === idx
          ? { ...r, role, percentage: typeof r.percentage === "number" ? r.percentage : roleDefaults[role] ?? undefined }
          : r,
      ),
    );

  // Residual explícito da imobiliária: quando a soma dos % dos corretores é
  // menor que 100, o restante fica retido pela imobiliária (sem beneficiário
  // sintético) — mesmo contrato do split editor (§13 do compêndio).
  const brokerSum = splits.reduce((acc, s) => acc + (typeof s.percentage === "number" ? s.percentage : 0), 0);
  const imobiliariaResidual = brokerSum > 0 && brokerSum < 100 ? 100 - brokerSum : 0;
  const splitsTotal = splits.reduce((total, split) => total + (split.amount ?? 0), 0);
  const imobiliariaAmountResidual = commissionPool === undefined ? undefined : Math.max(commissionPool - splitsTotal, 0);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (requestInFlightRef.current) return;
    if (!dealId.trim()) {
      toast.error("Informe o negócio (deal) da comissão.");
      return;
    }
    const validSplits = splits.filter((s) => s.broker_id.trim() && s.amount && s.amount > 0);
    if (validSplits.length === 0) {
      toast.error("Adicione ao menos um participante com corretor e valor maior que zero.");
      return;
    }
    if (splits.some((split) => split.percentage !== undefined && (split.percentage <= 0 || split.percentage > 100))) {
      toast.error("Cada percentual deve ser maior que zero e menor ou igual a 100%.");
      return;
    }
    if (brokerSum > 100.0001) {
      toast.error("A soma dos percentuais não pode ultrapassar 100% da comissão contratual.");
      return;
    }
    const duplicateSplit = validSplits.some((split, index) =>
      validSplits.findIndex((candidate) => candidate.broker_id.trim() === split.broker_id.trim() && candidate.role === split.role) !== index,
    );
    if (duplicateSplit) {
      toast.error("O mesmo corretor e papel aparece mais de uma vez. Consolide os valores em uma única participação.");
      return;
    }
    if (commissionPool !== undefined && splitsTotal > commissionPool + 0.01) {
      toast.error("A distribuição ultrapassa a comissão total prevista no contrato.");
      return;
    }
    const payloadSplits: CommissionSplitInput[] = validSplits.map((s) => ({
      broker_id: s.broker_id.trim(),
      role: s.role || undefined,
      percentage: typeof s.percentage === "number" ? s.percentage : undefined,
      amount: s.amount as number,
    }));

    const payload = {
      deal_id: dealId.trim(),
      contract_id: effectiveContractId,
      splits: payloadSplits,
    };
    const fingerprint = JSON.stringify(payload);
    const pendingAttempt = pendingAttemptRef.current;
    const idempotencyKey =
      pendingAttempt?.fingerprint === fingerprint ? pendingAttempt.idempotencyKey : newIdempotencyKey();
    pendingAttemptRef.current = { fingerprint, idempotencyKey };

    requestInFlightRef.current = true;
    setSaving(true);
    try {
      const created = await commissionService.createBatch(payload, idempotencyKey);
      pendingAttemptRef.current = null;
      toast.success(created.length === 1 ? "Comissão criada." : `${created.length} comissões criadas.`);
      onCreated();
      onClose();
    } catch (err) {
      console.error(err);
      toast.error("Não foi possível criar as comissões.");
    } finally {
      requestInFlightRef.current = false;
      setSaving(false);
    }
  };

  return (
    <Sheet open onOpenChange={(next) => { if (!next) onClose(); }}>
      <SheetContent
        side="right"
        expandable
        expandedWidth="sm:max-w-3xl lg:max-w-5xl"
        className="flex h-full w-full flex-col gap-0 overflow-hidden p-0 sm:max-w-lg"
      >
        <div className="flex items-center gap-3 border-b border-border bg-muted/50 px-6 py-4 pr-12">
          <div className="p-2 bg-primary-100 rounded-xl">
            <Briefcase className="w-5 h-5 text-primary-600" />
          </div>
          <div>
            <SheetTitle className="text-lg font-bold text-foreground">Nova Comissão</SheetTitle>
            <SheetDescription className="text-xs">Defina a distribuição da comissão entre os participantes do negócio.</SheetDescription>
          </div>
        </div>

        <form onSubmit={handleSubmit} className="flex min-h-0 flex-1 flex-col">
          <div className="min-h-0 flex-1 space-y-6 overflow-y-auto p-6">
          <div className="space-y-1.5">
            <label className="block text-sm font-bold text-foreground">Negócio *</label>
            {hasDeals ? (
              <SearchableSelect
                value={dealId || NO_SELECTION_OPTION}
                onValueChange={(next) => setDealId(next === NO_SELECTION_OPTION ? "" : next)}
                options={[
                  { value: NO_SELECTION_OPTION, label: "Selecione um negócio..." },
                  ...deals.map((deal) => ({
                    value: deal.id,
                    label: `${deal.client || "Cliente"} — ${deal.property || "Imóvel"} (${deal.type})`,
                    searchText: `${deal.client || ""} ${deal.property || ""} ${deal.type} ${deal.id}`,
                  })),
                ]}
                placeholder="Selecione um negócio..."
                searchPlaceholder="Buscar cliente, imóvel ou negócio..."
                emptyMessage="Nenhum negócio encontrado."
                aria-label="Negócio"
                aria-required="true"
              />
            ) : (
              <input
                id={`${fieldId}-deal`}
                type="text"
                aria-label="Negócio"
                aria-required="true"
                value={dealId}
                onChange={(e) => setDealId(e.target.value)}
                placeholder="ID do negócio (deal_id)"
                className="w-full px-4 py-2.5 rounded-xl border border-border bg-muted focus:bg-card focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500 transition-all outline-none"
              />
            )}
            <p className="mt-1 text-xs text-muted-foreground">
              {effectiveContractId
                ? "Contrato relacionado ao negócio — a comissão ficará rastreável no financeiro."
                : selectedDeal
                  ? "Sem contrato vinculado — trate como exceção somente se o negócio ainda não tiver contrato."
                  : "Selecione o negócio que originou a comissão."}
            </p>
          </div>

          {selectedDeal && (
            <div className="rounded-xl border border-border bg-muted/30 p-3 space-y-3">
              <div>
                <p className="text-sm font-bold text-foreground">Relação contratual</p>
                <p className="text-xs text-muted-foreground">A comissão herda cliente, imóvel e contrato do negócio sempre que a relação existir.</p>
              </div>
              <div className="space-y-1.5">
                <label className="block text-xs font-bold text-foreground">Contrato</label>
                <SearchableSelect
                  value={contractSelection || (resolvedContractId ? "__auto__" : MANUAL_OPTION)}
                  onValueChange={(next) => setContractSelection(next === "__auto__" ? "" : next)}
                  options={[
                    ...(resolvedContractId
                      ? [{ value: "__auto__", label: "Usar contrato detectado automaticamente" }]
                      : []),
                    ...linkedContracts.map((contract) => ({
                      value: contract.id,
                      label: `${contract.property_title || "Imóvel"} — ${contract.client || "Cliente"}`,
                      searchText: `${contract.property_title || ""} ${contract.client || ""} ${contract.id}`,
                    })),
                    { value: MANUAL_OPTION, label: "Sem contrato (exceção coerente)" },
                  ]}
                  placeholder="Selecione um contrato..."
                  searchPlaceholder="Buscar contrato, cliente ou imóvel..."
                  emptyMessage="Nenhum contrato encontrado."
                  aria-label="Contrato relacionado"
                />
              </div>
              <div className="grid grid-cols-1 items-start gap-2 text-xs text-muted-foreground sm:grid-cols-2">
                <span>Cliente: <strong className="text-foreground">{selectedDeal.client || "—"}</strong></span>
                <span>Imóvel: <strong className="text-foreground">{selectedDeal.property || "—"}</strong></span>
              </div>
            </div>
          )}

          {effectiveContract && (
            <div className="rounded-xl border border-primary-200 bg-primary-50/40 p-3">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <p className="text-sm font-bold text-foreground">Composição da comissão contratual</p>
                  <p className="text-xs text-muted-foreground">
                    {commissionPool !== undefined
                      ? `${effectiveContract.commission_pct}% sobre ${formatMoneyBR(effectiveContract.value, { withSymbol: true })}`
                      : "O contrato não informa valor e percentual suficientes para conferir automaticamente."}
                  </p>
                </div>
                {commissionPool !== undefined && (
                  <p className="text-base font-bold text-foreground">{formatMoneyBR(commissionPool, { withSymbol: true })}</p>
                )}
              </div>
              {commissionPool !== undefined && (
                <div className="mt-3 grid grid-cols-2 gap-3 text-xs sm:grid-cols-3">
                  <span>Distribuído: <strong className="block text-sm text-foreground">{formatMoneyBR(splitsTotal, { withSymbol: true })}</strong></span>
                  <span>Receita da imobiliária: <strong className="block text-sm text-status-ok">{formatMoneyBR(imobiliariaAmountResidual, { withSymbol: true })}</strong></span>
                  <span>Percentual distribuído: <strong className={brokerSum > 100 ? "block text-sm text-status-critical" : "block text-sm text-foreground"}>{brokerSum.toFixed(1)}%</strong></span>
                </div>
              )}
            </div>
          )}

          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <p className="text-sm font-bold text-foreground">Distribuição</p>
              <Button type="button" variant="outline" size="sm" onClick={addRow}>
                <Plus className="w-3.5 h-3.5" /> Participante
              </Button>
            </div>

            {splits.map((row, idx) => (
              <div key={idx} className="rounded-xl border border-border bg-muted/30 p-3 space-y-2">
                <div className="flex items-center justify-between gap-2">
                  <span className="text-xs font-bold uppercase tracking-wide text-muted-foreground">Participante {idx + 1}</span>
                  {splits.length > 1 && (
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      className="h-7 w-7 text-status-critical"
                      onClick={() => removeRow(idx)}
                      title="Remover participante"
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                    </Button>
                  )}
                </div>

                <div className="min-h-[76px] space-y-1.5">
                  <label className="block text-xs font-bold text-foreground">Corretor *</label>
                  {agents.length > 0 && row.broker_mode !== "manual" ? (
                    <SearchableSelect
                      value={row.broker_id || NO_SELECTION_OPTION}
                      onValueChange={(next) => updateRow(idx, next === MANUAL_OPTION
                        ? { broker_id: "", broker_mode: "manual" }
                        : { broker_id: next === NO_SELECTION_OPTION ? "" : next, broker_mode: "catalog" })}
                      options={[
                        { value: NO_SELECTION_OPTION, label: "Selecione um corretor..." },
                        ...agents.map((agent) => ({
                          value: agent.id,
                          label: agent.name,
                          searchText: `${agent.name} ${agent.id}`,
                        })),
                        { value: MANUAL_OPTION, label: "Outro corretor/entidade (exceção)" },
                      ]}
                      placeholder="Selecione um corretor..."
                      searchPlaceholder="Buscar corretor..."
                      emptyMessage="Nenhum corretor encontrado."
                      aria-label={`Corretor do participante ${idx + 1}`}
                    />
                  ) : (
                    <div className="space-y-1">
                      <input
                        id={`${fieldId}-broker-${idx}`}
                        type="text"
                        aria-label={`Corretor do participante ${idx + 1}`}
                        value={row.broker_id}
                        onChange={(e) => updateRow(idx, { broker_id: e.target.value, broker_mode: "manual" })}
                        placeholder="ID do corretor (broker_id)"
                        className="w-full px-3 py-2 rounded-lg border border-border bg-card focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500 transition-all outline-none text-sm"
                      />
                      <p className="text-[11px] text-muted-foreground">
                        {agents.length > 0 ? "Exceção: informe um ID que não está no cadastro de corretores." : "Cadastro de corretores indisponível; informe o ID para continuar."}
                      </p>
                    </div>
                  )}
                </div>

                <div className="grid grid-cols-1 items-start gap-2 sm:grid-cols-3">
                  <div className="min-h-[64px]">
                    <label htmlFor={`${fieldId}-role-${idx}`} className="block text-xs font-bold text-foreground mb-1">Papel</label>
                    <select
                      id={`${fieldId}-role-${idx}`}
                      value={row.role}
                      onChange={(e) => updateRowRole(idx, e.target.value)}
                      className="w-full px-3 py-2 rounded-lg border border-border bg-card focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500 transition-all outline-none text-sm"
                    >
                      {ROLE_OPTIONS.map((o) => (
                        <option key={o.value} value={o.value}>
                          {o.label}
                        </option>
                      ))}
                    </select>
                  </div>
                  <div>
                    <label htmlFor={`${fieldId}-percent-${idx}`} className="block text-xs font-bold text-foreground mb-1">% do total (opc.)</label>
                    <input
                      id={`${fieldId}-percent-${idx}`}
                      type="text"
                      inputMode="decimal"
                      value={typeof row.percentage === "number" ? String(row.percentage) : ""}
                      onChange={(e) => {
                        const raw = e.target.value.replace(",", ".").trim();
                        const num = raw === "" ? undefined : Number(raw);
                        const percentage = num !== undefined && Number.isFinite(num) ? num : undefined;
                        const calculatedAmount = commissionPool !== undefined && percentage !== undefined && row.amount === undefined
                          ? (commissionPool * percentage) / 100
                          : row.amount;
                        updateRow(idx, { percentage, amount: calculatedAmount });
                      }}
                      placeholder="0"
                      className="w-full px-3 py-2 rounded-lg border border-border bg-card focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500 transition-all outline-none text-sm text-right"
                    />
                    {commissionPool !== undefined && row.percentage !== undefined && row.amount !== undefined && Math.abs(row.amount - (commissionPool * row.percentage) / 100) > 0.01 && (
                      <p className="mt-1 text-[11px] text-status-warning">Esperado pelo percentual: {formatMoneyBR((commissionPool * row.percentage) / 100, { withSymbol: true })}</p>
                    )}
                  </div>
                  <div>
                    <label className="block text-xs font-bold text-foreground mb-1">Valor (R$) *</label>
                    <MoneyInput
                      value={row.amount}
                      onChange={(v) => updateRow(idx, { amount: v })}
                      placeholder="0,00"
                      className="w-full px-3 py-2 rounded-lg border border-border bg-card focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500 transition-all outline-none text-sm text-right"
                    />
                  </div>
                </div>
              </div>
            ))}

            {imobiliariaResidual > 0 && (
              <p className="text-xs text-muted-foreground">
                A soma dos percentuais é {brokerSum.toFixed(1)}% — a imobiliária retém{' '}
                <strong className="text-foreground">{imobiliariaResidual.toFixed(1)}%</strong> como residual
                explícito (sem beneficiário sintético).
              </p>
            )}
          </div>

          </div>
          <SheetFooter className="mt-auto flex-row justify-end gap-2 border-t border-border bg-background px-6 py-4">
            <Button type="button" variant="outline" onClick={onClose}>
              Cancelar
            </Button>
            <Button type="submit" loading={saving} loadingText="Criando...">
              Criar Comissão
            </Button>
          </SheetFooter>
        </form>
      </SheetContent>
    </Sheet>
  );
}
