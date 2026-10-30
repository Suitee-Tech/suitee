import { useConfirmAction } from "../hooks/useConfirmAction";
import { usePermissionsContext } from "../context/PermissionsContext";
import { useQuery } from "@tanstack/react-query";
import { Link, useSearchParams } from "react-router";
import { InspectionScheduleForm } from "./inspection/InspectionScheduleForm";
import { useState, useEffect, useId, useMemo, useCallback } from "react";
import { motion } from "motion/react";
import {
  Calendar,
  ClipboardList,
  Wrench,
  Receipt,
  ListChecks,
  RefreshCw,
  Plus,
  ShieldCheck,
  Loader2,
  ChevronRight,
  CheckCircle2,
  AlertTriangle,
  Lightbulb,
  Droplet,
  Building2,
  Flame,
  Wifi,
  ShieldAlert,
} from "lucide-react";
import { clsx } from "clsx";
import { toast } from "sonner";
import type { Contract } from "../contracts/types";
import { chargeService, type Charge } from "../services/charge.service";
import {
  utilityAccountService,
  type UtilityAccount,
  type UtilityAccountType,
  type CreateUtilityAccountDTO,
} from "../services/utilityAccount.service";
import { inspectionService, type Inspection } from "../services/inspection.service";
import { InspectionChecklistDialog } from "./inspection/InspectionChecklistDialog";
import { InspectionDiffDialog } from "./inspection/InspectionDiffDialog";
import { maintenanceService, type MaintenanceTicket } from "../services/maintenance.service";
import { EmptyState } from "../ui/EmptyState";
import { Button } from "../ui/button";
import { formatCompetencia, formatISODateBR } from "../utils/format";
import { chargeDueLabel, CHARGE_DUE_TONE_CLASS } from "../utils/chargeDueLabel";
import { MoneyInput } from "../ui/money-input";

interface ContractRentalTabProps {
  contract: Contract;
}

const formatCurrency = (val: number) =>
  new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL" }).format(val || 0);

// Timestamps de verdade (ex.: data de um reajuste) são exibidos no fuso de quem
// olha; DATA DE VENCIMENTO não: ela é gravada como meia-noite UTC e significa um
// dia do calendário, então quem formata é `formatISODateBR` — sem isso o Brasil
// (UTC-3) via o vencimento de 10/10 como 09/10.
const formatDate = (s?: string) => (s ? new Date(s).toLocaleDateString("pt-BR") : "—");

const UTILITY_ICONS: Record<UtilityAccountType, React.ComponentType<{ className?: string }>> = {
  AGUA: Droplet,
  ENERGIA: Lightbulb,
  GAS: Flame,
  CONDOMINIO: Building2,
  IPTU: ClipboardList,
  INTERNET: Wifi,
  SEGURO: ShieldCheck,
  OUTROS: Receipt,
};

const UTILITY_LABELS: Record<UtilityAccountType, string> = {
  AGUA: "Água",
  ENERGIA: "Energia",
  GAS: "Gás",
  CONDOMINIO: "Condomínio",
  IPTU: "IPTU",
  INTERNET: "Internet",
  SEGURO: "Seguro",
  OUTROS: "Outros",
};


const INSPECTION_TYPE_LABELS: Record<string, string> = {
  ENTRY: "Vistoria de entrada",
  EXIT: "Vistoria de saída",
  PERIODIC: "Vistoria periódica",
};

type Section = "previsibilidade" | "recorrentes" | "vistorias" | "manutencao";

/**
 * Deriva as "Próximas cobranças" (até 6, em ordem de vencimento) com o rótulo e o
 * texto de vencimento já resolvidos.
 *
 * Existe separada do componente para poder ser testada sozinha — e porque a versão
 * anterior fazia as duas contas erradas aqui dentro:
 *
 *  - `new Date(due_date).toLocaleDateString("pt-BR")` formatava no fuso do
 *    navegador uma data gravada como meia-noite UTC: no Brasil (UTC-3) a cobrança
 *    de 10/10 aparecia como **09/10**;
 *  - `new Date(due_date) < new Date()` comparava INSTANTES, então a cobrança que
 *    vence no próprio dia já aparecia como **"Em Atraso"** — a mesma cobrança que a
 *    tabela do plano mostrava como Pendente.
 *
 * Agora a data sai de `formatISODateBR` (dia do calendário, UTC) e o estado de
 * `chargeDueLabel` (a régua única do produto, por dia civil no fuso do tenant).
 */
export function upcomingChargeRows(charges: Charge[], limite = 6): Array<Charge & {
  label: string;
  badgeClass: string;
  dueDateLabel: string;
}> {
  return charges
    .filter((c) => c && (c.status === "PENDING" || c.status === "OVERDUE"))
    .slice()
    .sort((a, b) => new Date(a.due_date).getTime() - new Date(b.due_date).getTime())
    .slice(0, limite)
    .map((c) => {
      const due = chargeDueLabel(c);
      return {
        ...c,
        label: due.label,
        badgeClass: CHARGE_DUE_TONE_CLASS[due.tone],
        dueDateLabel: formatISODateBR(c.due_date) || "—",
      };
    });
}

export function ContractRentalTab({ contract }: ContractRentalTabProps) {
  const [searchParams] = useSearchParams();
  const [section, setSection] = useState<Section>(() => searchParams.get("secao") === "vistorias" ? "vistorias" : "previsibilidade");
  const [charges, setCharges] = useState<Charge[]>([]);
  const [utilityAccounts, setUtilityAccounts] = useState<UtilityAccount[]>([]);
  const [inspections, setInspections] = useState<Inspection[]>([]);
  const [maintenance, setMaintenance] = useState<MaintenanceTicket[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);

  const reload = useCallback(async () => {
    setLoading(true);
    try {
      const [chargesRes, utils, insps, maints] = await Promise.allSettled([
        chargeService.getAll({ contract_id: contract.id, limit: 200 }),
        utilityAccountService.list(contract.id),
        inspectionService.getAll({ contract_id: contract.id, limit: 50 }),
        contract.property_id
          ? maintenanceService.getAll({ propertyId: contract.property_id })
          : Promise.resolve({ data: [] as MaintenanceTicket[], meta: {} }),
      ]);
      setCharges(chargesRes.status === "fulfilled" ? (((chargesRes.value as any)?.data || []) as Charge[]) : []);
      setUtilityAccounts(utils.status === "fulfilled" ? utils.value : []);
      setInspections(insps.status === "fulfilled" ? ((insps.value as any)?.data || []) : []);
      setMaintenance(
        maints.status === "fulfilled"
          ? ((maints.value as any)?.data || []).filter(
              (m: MaintenanceTicket) => !m.contract_id || m.contract_id === contract.id,
            )
          : [],
      );
      const anyFailed = [chargesRes, utils, insps, maints].some((r) => r.status === "rejected");
      setLoadError(
        anyFailed
          ? "Não foi possível carregar todos os dados da locação. Algumas informações podem estar ausentes."
          : null,
      );
    } catch (err) {
      console.error(err);
      setLoadError("Não foi possível carregar os dados da locação.");
      toast.error("Erro ao carregar dados da locação.");
    } finally {
      setLoading(false);
    }
  }, [contract.id, contract.property_id]);

  useEffect(() => {
    reload();
  }, [reload]);

  const monthlySummary = useMemo(() => {
    const groups = new Map<string, { month: string; total: number; charges: Charge[] }>();
    for (const ch of charges) {
      const key = ch.reference_month || ch.due_date?.slice(0, 7) || "?";
      if (!groups.has(key)) groups.set(key, { month: key, total: 0, charges: [] });
      const entry = groups.get(key)!;
      entry.total += ch.total_amount || ch.amount || 0;
      entry.charges.push(ch);
    }
    return Array.from(groups.values()).sort((a, b) => a.month.localeCompare(b.month));
  }, [charges]);

  const upcomingCharges = useMemo(() => upcomingChargeRows(charges), [charges]);

  const sections: Array<{
    id: Section;
    label: string;
    icon: React.ComponentType<{ className?: string }>;
    count?: number;
  }> = [
    { id: "previsibilidade", label: "Previsibilidade", icon: Calendar, count: charges.length },
    { id: "recorrentes", label: "Cobranças recorrentes", icon: Receipt, count: utilityAccounts.length },
    { id: "vistorias", label: "Vistorias", icon: ClipboardList, count: inspections.length },
    { id: "manutencao", label: "Manutenção", icon: Wrench, count: maintenance.length },
  ];

  return (
    <motion.div initial={{ opacity: 0, y: 10 }} animate={{ opacity: 1, y: 0 }} className="space-y-6">
      {loadError && (
        <div className="flex items-center justify-between gap-3 rounded-xl border border-status-critical/30 bg-status-critical-soft px-4 py-3 text-sm text-status-critical">
          <span className="flex items-center gap-2">{loadError}</span>
          <Button variant="outline" size="sm" onClick={reload}>Tentar novamente</Button>
        </div>
      )}
      <div className="flex flex-wrap items-center gap-2">
        {sections.map((s) => {
          const active = section === s.id;
          return (
            <button
              key={s.id}
              onClick={() => setSection(s.id)}
              className={clsx(
                "px-4 py-2 rounded-xl text-sm font-medium border transition-colors flex items-center gap-2",
                active
                  ? "bg-primary border-primary text-primary-foreground"
                  : "bg-card border-border text-foreground hover:bg-muted",
              )}
            >
              <s.icon className="w-4 h-4" />
              {s.label}
              {typeof s.count === "number" && (
                <span className={clsx("text-xs px-1.5 py-0.5 rounded", active ? "bg-white/20" : "bg-muted text-muted-foreground")}>
                  {s.count}
                </span>
              )}
            </button>
          );
        })}
        <Button
          variant="ghost"
          size="icon"
          onClick={reload}
          loading={loading}
          className="ml-auto size-9 text-muted-foreground hover:text-primary-600"
          title="Atualizar"
        >
          <RefreshCw className="w-4 h-4" />
        </Button>
      </div>

      {section === "previsibilidade" && (
        <PrevisibilidadeSection
          contract={contract}
          charges={charges}
          monthlySummary={monthlySummary}
          upcomingCharges={upcomingCharges}
          loading={loading}
        />
      )}
      {section === "recorrentes" && (
        <RecorrentesSection contract={contract} utilityAccounts={utilityAccounts} onChange={reload} loading={loading} />
      )}
      {section === "vistorias" && <VistoriasSection contract={contract} inspections={inspections} onChange={reload} loading={loading} />}
      {section === "manutencao" && <ManutencaoSection contractId={contract.id} tickets={maintenance} loading={loading} />}
    </motion.div>
  );
}

function PrevisibilidadeSection({
  contract,
  charges,
  monthlySummary,
  upcomingCharges,
  loading,
}: {
  contract: Contract;
  charges: Charge[];
  monthlySummary: Array<{ month: string; total: number; charges: Charge[] }>;
  upcomingCharges: ReturnType<typeof upcomingChargeRows>;
  loading: boolean;
}) {
  const totalPending = charges
    .filter((c) => c.status === "PENDING" || c.status === "OVERDUE")
    .reduce((acc, c) => acc + (c.total_amount || c.amount || 0), 0);
  const totalPaid = charges.filter((c) => c.status === "PAID").reduce((acc, c) => acc + (c.paid_amount || c.total_amount || 0), 0);
  const overdueCount = charges.filter((c) => c.status === "OVERDUE").length;

  return (
    <div className="space-y-6">

      <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
        <SummaryCard
          label="Valor mensal"
          value={formatCurrency(contract.value || 0)}
          subtitle={contract.adjust_index ? `Reajuste: ${contract.adjust_index}` : "Sem índice configurado"}
          tone="primary"
        />
        <SummaryCard
          label="A receber"
          value={formatCurrency(totalPending)}
          subtitle={`${charges.filter((c) => c.status === "PENDING" || c.status === "OVERDUE").length} cobrança(s) em aberto`}
          tone="warning"
        />
        <SummaryCard
          label="Recebido"
          value={formatCurrency(totalPaid)}
          subtitle={`${charges.filter((c) => c.status === "PAID").length} cobrança(s) paga(s)`}
          tone="ok"
        />
        <SummaryCard
          label="Em atraso"
          value={overdueCount ? `${overdueCount}` : "0"}
          subtitle="Cobranças vencidas"
          tone={overdueCount > 0 ? "critical" : "muted"}
        />
      </div>

      <div className="bg-card rounded-xl border border-border shadow-sm overflow-hidden">
        <div className="p-5 border-b border-border bg-muted/40 flex items-center justify-between">
          <h3 className="text-base font-bold text-foreground flex items-center gap-2">
            <Calendar className="w-5 h-5 text-primary-600" />
            Calendário de cobranças
          </h3>
          <span className="text-xs text-muted-foreground">{monthlySummary.length} mês(es) projetado(s)</span>
        </div>
        {loading ? (
          <div className="p-8 flex items-center justify-center text-muted-foreground">
            <Loader2 className="w-5 h-5 animate-spin mr-2" /> Carregando...
          </div>
        ) : monthlySummary.length === 0 ? (
          <EmptyState
            icon={Calendar}
            title="Sem previsibilidade"
            message="Nenhuma cobrança gerada para este contrato. Quando o RentalChargeConfig é criado, as parcelas mensais aparecem aqui."
            className="border-none bg-transparent my-6"
          />
        ) : (
          <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-3 p-5">
            {monthlySummary.map((m) => {
              const allPaid = m.charges.every((c) => c.status === "PAID");
              const anyOverdue = m.charges.some((c) => c.status === "OVERDUE");
              return (
                <div
                  key={m.month}
                  className={clsx(
                    "rounded-xl border p-3",
                    allPaid ? "bg-green-50 border-green-200" : anyOverdue ? "bg-status-critical-soft border-status-critical/30" : "bg-muted border-border",
                  )}
                >
                  <p className="text-xs text-muted-foreground">{formatCompetencia(m.month)}</p>
                  <p className="text-sm font-bold text-foreground">{formatCurrency(m.total)}</p>
                  <p className="text-xs text-muted-foreground mt-1">{m.charges.length} cobrança(s)</p>
                </div>
              );
            })}
          </div>
        )}
      </div>

      <div className="bg-card rounded-xl border border-border shadow-sm overflow-hidden">
        <div className="p-5 border-b border-border bg-muted/40">
          <h3 className="text-base font-bold text-foreground flex items-center gap-2">
            <ListChecks className="w-5 h-5 text-primary-600" />
            Próximas cobranças
          </h3>
        </div>
        {loading ? (
          <div className="p-8 flex items-center justify-center text-muted-foreground">
            <Loader2 className="w-5 h-5 animate-spin mr-2" /> Carregando...
          </div>
        ) : upcomingCharges.length === 0 ? (
          <EmptyState
            icon={CheckCircle2}
            title="Sem cobranças pendentes"
            message="Todas as cobranças foram pagas ou ainda não há cobranças geradas."
            className="border-none bg-transparent my-6"
          />
        ) : (
          <div className="divide-y divide-border">
            {upcomingCharges.map((c) => (
              <div key={c.id} className="px-5 py-3 flex items-center justify-between gap-3 hover:bg-muted/40">
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium text-foreground truncate">{c.description || c.type}</p>
                  <p className="text-xs text-muted-foreground">Vencimento: {c.dueDateLabel} · Ref.: {formatCompetencia(c.reference_month)}</p>
                </div>
                <p className="text-sm font-bold text-foreground">{formatCurrency(c.total_amount || c.amount)}</p>
                <span className={clsx("px-2 py-0.5 text-xs font-semibold rounded-full border whitespace-nowrap", c.badgeClass)}>
                  {c.label}
                </span>
              </div>
            ))}
          </div>
        )}
      </div>

      {contract.adjustments && contract.adjustments.length > 0 && (
        <div className="bg-card rounded-xl border border-border shadow-sm overflow-hidden">
          <div className="p-5 border-b border-border bg-muted/40">
            <h3 className="text-base font-bold text-foreground flex items-center gap-2">
              <RefreshCw className="w-5 h-5 text-primary-600" />
              Histórico de reajustes
            </h3>
          </div>
          <div className="divide-y divide-border">
            {[...(contract.adjustments || [])].reverse().map((a, idx) => (
              <div key={idx} className="px-5 py-3 flex items-center justify-between gap-3">
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium text-foreground">
                    {formatDate(a.applied_at)} · {a.mode === "auto" ? "Automático" : "Manual"}
                    {a.index_name ? ` · ${a.index_name}` : ""}
                  </p>
                  {a.note && <p className="text-xs text-muted-foreground">{a.note}</p>}
                </div>
                <p className="text-sm text-muted-foreground">
                  {formatCurrency(a.previous_value)} → <span className="font-bold text-foreground">{formatCurrency(a.new_value)}</span>
                </p>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

function SummaryCard({
  label,
  value,
  subtitle,
  tone,
}: {
  label: string;
  value: string;
  subtitle: string;
  tone: "primary" | "ok" | "warning" | "critical" | "muted";
}) {
  const palettes: Record<typeof tone, string> = {
    primary: "bg-primary-50 text-primary-600 border-primary-100",
    ok: "bg-status-ok-soft text-status-ok border-status-ok/20",
    warning: "bg-status-warning-soft text-status-warning border-status-warning/30",
    critical: "bg-status-critical-soft text-status-critical border-status-critical/30",
    muted: "bg-muted text-muted-foreground border-border",
  };
  return (
    <div className="bg-card rounded-xl border border-border p-4 shadow-sm">
      <div className="flex items-center gap-2">
        <div className={clsx("p-1.5 rounded-lg border text-xs", palettes[tone])}>•</div>
        <p className="text-xs uppercase tracking-wider text-muted-foreground">{label}</p>
      </div>
      <p className="text-xl font-bold text-foreground mt-2">{value}</p>
      <p className="text-xs text-muted-foreground">{subtitle}</p>
    </div>
  );
}

function RecorrentesSection({
  contract,
  utilityAccounts,
  onChange,
  loading,
}: {
  contract: Contract;
  utilityAccounts: UtilityAccount[];
  onChange: () => void;
  loading: boolean;
}) {
  const [showForm, setShowForm] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const formId = useId();
  const [form, setForm] = useState<Partial<CreateUtilityAccountDTO>>({
    type: "CONDOMINIO",
    responsible_party: "TENANT",
    estimated_amount: 0,
    auto_pay_enabled: false,
    due_day: 10,
    provider: "",
  });

  const handleAdd = async () => {
    if (!form.type || !form.provider || !form.estimated_amount) {
      toast.error("Preencha tipo, fornecedor e valor.");
      return;
    }
    setSubmitting(true);
    try {
      await utilityAccountService.create({
        contract_id: contract.id,
        property_id: contract.property_id || "",
        type: form.type as UtilityAccountType,
        provider: form.provider!,
        account_number: form.account_number,
        responsible_party: (form.responsible_party as any) || "TENANT",
        estimated_amount: Number(form.estimated_amount) || 0,
        auto_pay_enabled: !!form.auto_pay_enabled,
        due_day: form.due_day,
        notes: form.notes,
      });
      toast.success("Cobrança recorrente adicionada.");
      setShowForm(false);
      setForm({
        type: "CONDOMINIO",
        responsible_party: "TENANT",
        estimated_amount: 0,
        auto_pay_enabled: false,
        due_day: 10,
        provider: "",
      });
      onChange();
    } catch {
      toast.error("Erro ao adicionar cobrança recorrente.");
    } finally {
      setSubmitting(false);
    }
  };

  const handleDelete = async (id: string) => {
    try {
      await utilityAccountService.remove(id);
      toast.success("Cobrança removida.");
      onChange();
    } catch {
      toast.error("Erro ao remover.");
    }
  };

  return (
    <div className="bg-card rounded-xl border border-border shadow-sm overflow-hidden">
      <div className="p-5 border-b border-border bg-muted/40 flex items-center justify-between">
        <div>
          <h3 className="text-base font-bold text-foreground flex items-center gap-2">
            <Receipt className="w-5 h-5 text-primary-600" />
            Cobranças recorrentes (IPTU, condomínio, utilidades)
          </h3>
          <p className="text-xs text-muted-foreground mt-1">
            Configure os custos mensais que devem ser cobrados ou repassados durante o contrato.
          </p>
        </div>
        <Button onClick={() => setShowForm((v) => !v)}>
          <Plus className="w-4 h-4" />
          Adicionar
        </Button>
      </div>

      {showForm && (
        <div className="p-5 border-b border-border bg-primary-50/40 dark:bg-primary-950/10 space-y-3">
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label htmlFor={`${formId}-type`} className="block text-xs font-bold text-foreground uppercase tracking-wider mb-1">Tipo</label>
              <select
                id={`${formId}-type`}
                value={form.type}
                onChange={(e) => setForm({ ...form, type: e.target.value as UtilityAccountType })}
                className="w-full px-3 py-2 bg-card border border-border rounded-lg text-sm"
              >
                {(Object.keys(UTILITY_LABELS) as UtilityAccountType[]).map((k) => (
                  <option key={k} value={k}>
                    {UTILITY_LABELS[k]}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label htmlFor={`${formId}-responsible`} className="block text-xs font-bold text-foreground uppercase tracking-wider mb-1">Responsável</label>
              <select
                id={`${formId}-responsible`}
                value={form.responsible_party}
                onChange={(e) => setForm({ ...form, responsible_party: e.target.value as any })}
                className="w-full px-3 py-2 bg-card border border-border rounded-lg text-sm"
              >
                <option value="TENANT">Locatário</option>
                <option value="OWNER">Proprietário</option>
              </select>
            </div>
            <div>
              <label htmlFor={`${formId}-provider`} className="block text-xs font-bold text-foreground uppercase tracking-wider mb-1">Fornecedor / órgão <span aria-hidden="true" className="text-status-critical">*</span></label>
              <input
                id={`${formId}-provider`}
                type="text"
                aria-required="true"
                value={form.provider || ""}
                onChange={(e) => setForm({ ...form, provider: e.target.value })}
                placeholder="Ex.: Sabesp, Prefeitura, Condomínio Vila Olímpia"
                className="w-full px-3 py-2 bg-card border border-border rounded-lg text-sm"
              />
            </div>
            <div>
              <label htmlFor={`${formId}-account`} className="block text-xs font-bold text-foreground uppercase tracking-wider mb-1">Nº conta / inscrição</label>
              <input
                id={`${formId}-account`}
                type="text"
                value={form.account_number || ""}
                onChange={(e) => setForm({ ...form, account_number: e.target.value })}
                className="w-full px-3 py-2 bg-card border border-border rounded-lg text-sm"
              />
            </div>
            <div>
              <label htmlFor={`${formId}-estimated`} className="block text-xs font-bold text-foreground uppercase tracking-wider mb-1">Valor estimado (R$) <span aria-hidden="true" className="text-status-critical">*</span></label>
              <MoneyInput
                id={`${formId}-estimated`}
                aria-required="true"
                withSymbol
                placeholder="R$ 0,00"
                value={typeof form.estimated_amount === 'number' ? form.estimated_amount : undefined}
                onChange={(v) => setForm({ ...form, estimated_amount: v ?? 0 })}
                className="w-full px-3 py-2 bg-card border border-border rounded-lg text-sm"
              />
            </div>
            <div>
              <label htmlFor={`${formId}-due-day`} className="block text-xs font-bold text-foreground uppercase tracking-wider mb-1">Dia de vencimento</label>
              <input
                id={`${formId}-due-day`}
                type="number"
                min="1"
                max="31"
                value={form.due_day || 1}
                onChange={(e) => setForm({ ...form, due_day: parseInt(e.target.value) || 1 })}
                className="w-full px-3 py-2 bg-card border border-border rounded-lg text-sm"
              />
            </div>
          </div>
          <label className="flex items-center gap-2 text-sm text-foreground">
            <input
              type="checkbox"
              checked={!!form.auto_pay_enabled}
              onChange={(e) => setForm({ ...form, auto_pay_enabled: e.target.checked })}
            />
            Débito automático
          </label>
          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setShowForm(false)}>
              Cancelar
            </Button>
            <Button onClick={handleAdd} loading={submitting}>
              Salvar
            </Button>
          </div>
        </div>
      )}

      {loading ? (
        <div className="p-8 flex items-center justify-center text-muted-foreground">
          <Loader2 className="w-5 h-5 animate-spin mr-2" /> Carregando...
        </div>
      ) : utilityAccounts.length === 0 ? (
        <EmptyState
          icon={Receipt}
          title="Nenhuma cobrança recorrente configurada"
          message="Adicione IPTU, condomínio, água, energia e demais utilidades que devem aparecer no mês a mês."
          className="border-none bg-transparent my-6"
        />
      ) : (
        <div className="divide-y divide-border">
          {utilityAccounts.map((u) => {
            const Icon = UTILITY_ICONS[u.type] || Receipt;
            return (
              <div key={u.id} className="px-5 py-3 flex items-center gap-4">
                <div className="p-2 rounded-lg bg-primary-50 text-primary-600">
                  <Icon className="w-5 h-5" />
                </div>
                <div className="flex-1 min-w-0">
                  <p className="font-medium text-foreground">
                    {UTILITY_LABELS[u.type]} · {u.provider}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {u.responsible_party === "TENANT" ? "Locatário" : "Proprietário"}
                    {u.account_number ? ` · Nº ${u.account_number}` : ""}
                    {u.due_day ? ` · Vence dia ${u.due_day}` : ""}
                    {u.auto_pay_enabled ? " · Débito automático" : ""}
                  </p>
                </div>
                <p className="text-sm font-bold text-foreground">{formatCurrency((u.estimated_amount_cents || 0) / 100)}</p>
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={() => handleDelete(u.id)}
                  className="size-9 text-base text-muted-foreground hover:text-status-critical"
                  title="Remover"
                >
                  ×
                </Button>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

function VistoriasSection({
  contract,
  inspections,
  onChange,
  loading,
}: {
  contract: Contract;
  inspections: Inspection[];
  onChange: () => void;
  loading: boolean;
}) {
  const { confirmAction, confirmationDialog } = useConfirmAction();
  const [editingInspection, setEditingInspection] = useState<Inspection>();
  const [showForm, setShowForm] = useState(false);
  const [selectedInspection, setSelectedInspection] = useState<Inspection | null>(null);
  const [diffOpen, setDiffOpen] = useState<{ entryId: string; exitId: string } | null>(null);
  const hasEntry = inspections.some((i) => i.type === "ENTRY" && i.status === "Concluída");
  const hasExit = inspections.some((i) => i.type === "EXIT" && i.status === "Concluída");

  return (
    <div className="space-y-4">
      {confirmationDialog}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
        <ChecklistItem done={hasEntry} label="Vistoria de entrada concluída" hint="Registra o estado do imóvel quando o locatário recebe as chaves." />
        <ChecklistItem done={hasExit} label="Vistoria de saída concluída" hint="Pré-requisito para encerrar o contrato (quando exigido)." />
      </div>

      <div className="bg-card rounded-xl border border-border shadow-sm overflow-hidden">
        <div className="p-5 border-b border-border bg-muted/40 flex items-center justify-between">
          <h3 className="text-base font-bold text-foreground flex items-center gap-2">
            <ClipboardList className="w-5 h-5 text-primary-600" />
            Vistorias agendadas e concluídas
          </h3>
          <div className="flex items-center gap-2">
            {(() => {
              const latestEntry = [...inspections].reverse().find((i) => i.type === "ENTRY");
              const latestExit = [...inspections].reverse().find((i) => i.type === "EXIT");
              if (!latestEntry || !latestExit) return null;
              return (
                <Button
                  variant="outline"
                  onClick={() => setDiffOpen({ entryId: latestEntry.id, exitId: latestExit.id })}
                  title="Comparar checklist e fotos entre vistoria de entrada e saída"
                >
                  <ClipboardList className="w-4 h-4" />
                  Comparar Entrada × Saída
                </Button>
              );
            })()}
            <Button onClick={() => { setEditingInspection(undefined); setShowForm((v) => !v); }}>
              <Plus className="w-4 h-4" />
              Agendar
            </Button>
          </div>
        </div>

        {showForm && <InspectionScheduleForm key={editingInspection?.id ?? "new"} inspection={editingInspection} contract={contract} onCancel={() => setShowForm(false)} onSaved={() => { setShowForm(false); onChange(); }} />}

        {loading ? (
          <div className="p-8 flex items-center justify-center text-muted-foreground">
            <Loader2 className="w-5 h-5 animate-spin mr-2" /> Carregando...
          </div>
        ) : inspections.length === 0 ? (
          <EmptyState
            icon={ClipboardList}
            title="Nenhuma vistoria registrada"
            message="Agende a vistoria de entrada agora e a de saída antes do encerramento."
            className="border-none bg-transparent my-6"
          />
        ) : (
          <div className="divide-y divide-border">
            {inspections.map((i) => {
              const itemCount = i.findings?.length || 0;
              const photoCount = (i.findings || []).reduce((acc, f) => acc + (f.photos?.length || 0), 0);
              return (
                <div key={i.id}>
                <button
                  type="button"
                  onClick={() => setSelectedInspection(i)}
                  className="w-full px-5 py-3 flex items-center gap-3 text-left hover:bg-muted/40 transition-colors"
                >
                  <div
                    className={clsx(
                      "p-2 rounded-lg",
                      i.status === "Concluída"
                        ? "bg-green-50 text-green-700"
                        : i.status === "Cancelada"
                          ? "bg-muted text-muted-foreground"
                          : "bg-yellow-50 text-yellow-700",
                    )}
                  >
                    <ClipboardList className="w-5 h-5" />
                  </div>
                  <div className="flex-1 min-w-0">
                    <p className="font-medium text-foreground">{INSPECTION_TYPE_LABELS[i.type] || i.type}</p>
                    <p className="text-xs text-muted-foreground">
                      {new Date(i.scheduled_date).toLocaleString("pt-BR", { dateStyle: "short", timeStyle: "short" })} · {i.inspector_name} · {i.status}
                      {itemCount > 0 && ` · ${itemCount} item${itemCount === 1 ? "" : "s"}`}
                      {photoCount > 0 && ` · ${photoCount} foto${photoCount === 1 ? "" : "s"}`}
                    </p>
                  </div>
                  <ChevronRight className="w-4 h-4 text-muted-foreground" />
                </button>
                {!['Concluída', 'Cancelada'].includes(i.status) && <div className="px-5 pb-3 flex flex-wrap gap-2">
                  <Button variant="outline" size="sm" onClick={() => { setEditingInspection(i); setShowForm(true); }}>Reagendar vistoria</Button>
                  <Button variant="outline" size="sm" onClick={() => confirmAction("Cancelar esta vistoria e atualizar seu compromisso na agenda?", async () => { await inspectionService.update(i.id, { status: "Cancelada" }); onChange(); }, { title: "Cancelar vistoria", confirmLabel: "Cancelar vistoria" })}>Cancelar vistoria</Button>
                </div>}
                </div>
              );
            })}
          </div>
        )}
      </div>

      {selectedInspection && (
        <InspectionChecklistDialog
          inspection={selectedInspection}
          open={true}
          onClose={() => setSelectedInspection(null)}
          onSaved={() => {
            setSelectedInspection(null);
            onChange();
          }}
        />
      )}

      {diffOpen && <InspectionDiffDialog entryId={diffOpen.entryId} exitId={diffOpen.exitId} open={true} onClose={() => setDiffOpen(null)} />}
    </div>
  );
}

function ChecklistItem({ done, label, hint }: { done: boolean; label: string; hint: string }) {
  return (
    <div className={clsx("rounded-xl border p-4 flex items-start gap-3", done ? "bg-green-50 border-green-200" : "bg-muted border-border")}>
      {done ? (
        <CheckCircle2 className="w-5 h-5 text-green-700 mt-0.5 flex-shrink-0" />
      ) : (
        <AlertTriangle className="w-5 h-5 text-status-warning mt-0.5 flex-shrink-0" />
      )}
      <div>
        <p className={clsx("text-sm font-medium", done ? "text-green-900" : "text-foreground")}>{label}</p>
        <p className="text-xs text-muted-foreground mt-0.5">{hint}</p>
      </div>
    </div>
  );
}

function ManutencaoSection({ tickets, loading, contractId }: { tickets: MaintenanceTicket[]; loading: boolean; contractId: string }) {
  return (
    <div className="bg-card rounded-xl border border-border shadow-sm overflow-hidden">
      <div className="p-5 border-b border-border bg-muted/40 flex items-center justify-between">
        <h3 className="text-base font-bold text-foreground flex items-center gap-2">
          <Wrench className="w-5 h-5 text-primary-600" />
          Manutenções
        </h3>
        <Link to={`/manutencao?contrato=${encodeURIComponent(contractId)}`} className="text-sm text-primary-600 hover:text-primary-700 font-medium flex items-center gap-1">
          Abrir tela completa <ChevronRight className="w-4 h-4" />
        </Link>
      </div>
      {loading ? (
        <div className="p-8 flex items-center justify-center text-muted-foreground">
          <Loader2 className="w-5 h-5 animate-spin mr-2" /> Carregando...
        </div>
      ) : tickets.length === 0 ? (
        <EmptyState
          icon={Wrench}
          title="Sem chamados de manutenção"
          message="Os chamados criados pelo locatário ou administradora aparecem aqui automaticamente."
          className="border-none bg-transparent my-6"
        />
      ) : (
        <div className="divide-y divide-border">
          {tickets.map((t) => (
            <div key={t.id} className="px-5 py-3 flex items-center gap-3">
              <div
                className={clsx(
                  "p-2 rounded-lg",
                  t.status === "Concluído"
                    ? "bg-green-50 text-green-700"
                    : t.priority === "Alta" || t.priority === "Urgente"
                      ? "bg-status-critical-soft text-status-critical"
                      : "bg-yellow-50 text-yellow-700",
                )}
              >
                <Wrench className="w-5 h-5" />
              </div>
              <div className="flex-1 min-w-0">
                <p className="font-medium text-foreground truncate">{t.description || t.category}</p>
                <p className="text-xs text-muted-foreground">
                  {t.category} · {t.status} · {formatDate(t.created_at)}
                  {t.estimated_cost ? ` · Estimativa ${formatCurrency(t.estimated_cost)}` : ""}
                </p>
              </div>
              {t.priority === "Urgente" && <ShieldAlert className="w-4 h-4 text-status-critical" />}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

/** Mesmo fluxo de vistoria nas duas apresentações de gestão do aluguel. */
export function RentalInspectionsPanel({ contract }: { contract: Contract }) {
  const { can } = usePermissionsContext();
  const allowed = can("rentals:inspect");
  const query = useQuery({ enabled: allowed, queryKey: ["rental-inspections", contract.id], queryFn: async () =>
    (await inspectionService.getAll({ contract_id: contract.id, limit: 100 })).data });
  if (!allowed) return null;
  if (query.isError) return <div role="alert" className="p-4"><p>Não foi possível carregar as vistorias.</p><Button onClick={() => void query.refetch()}>Tentar novamente</Button></div>;
  return <VistoriasSection contract={contract} inspections={query.data ?? []} loading={query.isLoading} onChange={() => void query.refetch()} />;
}
