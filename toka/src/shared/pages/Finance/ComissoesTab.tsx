import { listAllCommissions } from "../../services/financialLists";
import { FinanceLoadError } from "../../components/finance/FinanceLoadError";
import { useEffect, useMemo, useState, useCallback } from "react";
import {
  Search,
  DollarSign,
  AlertTriangle,
  CheckCircle2,
  XCircle,
  Briefcase,
  Eye,
  MoreHorizontal,
  Calendar,
  Loader2,
} from "lucide-react";
import { toast } from "sonner";
import { commissionService, Commission, CommissionStatus } from "../../services/commission.service";
import { formatMoneyBR, formatISODateBR } from "../../utils/format";
import { CommissionDetailDrawer, CommissionDrawerAction } from "../../components/CommissionDetailDrawer";
import { MarkPaidCommissionModal } from "../../components/MarkPaidCommissionModal";
import { CancelarCobrancaDialog } from "../../components/CancelarCobrancaDialog";
import { Button } from "../../ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "../../ui/dropdown-menu";
import { clsx } from "clsx";
import { useDealsContext } from "../../context/DealsContext";
import { useAgentsContext } from "../../context/AgentsContext";

const STATUS_OPTIONS: { value: CommissionStatus | "ALL"; label: string }[] = [
  { value: "ALL", label: "Todos status" },
  { value: "PENDING", label: "Pendentes" },
  { value: "APPROVED", label: "Aprovadas" },
  { value: "PAID", label: "Pagas" },
  { value: "REVERSED", label: "Estornadas" },
  { value: "CANCELLED", label: "Canceladas" },
];

const ROLE_LABEL: Record<string, string> = {
  captador: "Captador",
  co_captador: "Co-captador",
  visitador: "Visitador",
  atendente: "Atendente",
  fechamento: "Fechamento",
  imobiliaria: "Imobiliária",
};

const STATUS_PILL: Record<CommissionStatus, string> = {
  PENDING: "bg-status-warning-soft text-status-warning border-status-warning/30",
  APPROVED: "bg-status-info-soft text-status-info border-status-info/30",
  PAID: "bg-status-ok-soft text-status-ok border-status-ok/30",
  REVERSED: "bg-status-critical-soft text-status-critical border-status-critical/30",
  CANCELLED: "bg-muted text-muted-foreground border-border",
};

const STATUS_LABEL: Record<CommissionStatus, string> = {
  PENDING: "Pendente",
  APPROVED: "Aprovada",
  PAID: "Paga",
  REVERSED: "Estornada",
  CANCELLED: "Cancelada",
};

const formatBRL = (v?: number) => {
  if (v === undefined || v === null || isNaN(v)) return "—";
  return formatMoneyBR(v);
};

const formatDate = (iso?: string) => {
  if (!iso) return "—";
  return formatISODateBR(iso) || "—";
};

const isRentalDealType = (value?: string) => {
  const normalized = (value || "").trim().toLowerCase();
  return normalized === "rent" || normalized === "rental" || normalized === "locacao" || normalized === "locação";
};

export function ComissoesTab() {
  const { dealDTOs } = useDealsContext();
  const { agents } = useAgentsContext();
  const [items, setItems] = useState<Commission[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState(false);
  const [brokerOptions, setBrokerOptions] = useState<Array<{ id: string; label: string }>>([]);

  // Filtros — controlados no client. O backend `list` ainda não aceita
  // filtros tipados via query (apenas limit/offset). Filtrar no client é
  // OK para o volume típico de uma imobiliária (centenas, não milhares);
  // se virar problema, migrar para filtros via API e paginar real.
  const [statusFilter, setStatusFilter] = useState<CommissionStatus | "ALL">("ALL");
  const [search, setSearch] = useState("");
  const [brokerFilter, setBrokerFilter] = useState("ALL");
  const [contractFilter, setContractFilter] = useState("ALL");
  const [dateFrom, setDateFrom] = useState("");
  const [dateTo, setDateTo] = useState("");

  // Selected for detail / actions
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [payingId, setPayingId] = useState<string | null>(null);
  const [cancellingId, setCancellingId] = useState<string | null>(null);
  const [reversingId, setReversingId] = useState<string | null>(null);

  const refetch = useCallback(async () => {
    setLoading(true);
    setLoadError(false);
    setItems([]);
    try {
      // Publica os indicadores somente depois de carregar todas as páginas.
      setItems(await listAllCommissions());
    } catch {
      setLoadError(true);
      toast.error("Não foi possível carregar as comissões.");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    refetch();
  }, [refetch]);

  // Resolve broker_id → nome do agente (corretor) para exibir rótulo legível.
  const brokerName = useCallback(
    (brokerId?: string) => {
      if (!brokerId) return "—";
      const agent = agents.find((x) => x.id === brokerId);
      return agent?.name ?? brokerId;
    },
    [agents],
  );

  // Derive broker options from the loaded set. Mostra apenas brokers que
  // de fato têm comissões — evita poluir o dropdown com nomes sem registro.
  useEffect(() => {
    const ids = Array.from(new Set(items.map((c) => c.broker_id))).filter(Boolean);
    setBrokerOptions(ids.map((id) => ({ id, label: brokerName(id) })));
  }, [items, brokerName]);

  // Map deal_id → contract label para mostrar algo legível na coluna Contrato.
  const dealLabel = useCallback(
    (dealId?: string) => {
      if (!dealId) return "—";
      const d = (dealDTOs as any[]).find((x) => x.id === dealId);
      return d?.title || d?.property_title || dealId.slice(0, 8);
    },
    [dealDTOs]
  );

  const isRentalCommission = useCallback(
    (commission?: Commission | null) => {
      if (!commission?.deal_id) return false;
      const deal = (dealDTOs as any[]).find((item) => item.id === commission.deal_id);
      return isRentalDealType(deal?.type);
    },
    [dealDTOs],
  );

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return items.filter((c) => {
      if (statusFilter !== "ALL" && c.status !== statusFilter) return false;
      if (brokerFilter !== "ALL" && c.broker_id !== brokerFilter) return false;
      if (contractFilter !== "ALL" && c.deal_id !== contractFilter) return false;
      if (dateFrom) {
        const cd = new Date(c.created_at);
        if (cd < new Date(`${dateFrom}T00:00:00`)) return false;
      }
      if (dateTo) {
        const cd = new Date(c.created_at);
        if (cd > new Date(`${dateTo}T23:59:59.999`)) return false;
      }
      if (q) {
        const hay = `${c.id} ${brokerName(c.broker_id)} ${dealLabel(c.deal_id)} ${c.broker_id} ${c.deal_id} ${c.contract_id || ""} ${c.role || ""}`.toLowerCase();
        if (!hay.includes(q)) return false;
      }
      return true;
    });
  }, [items, search, statusFilter, brokerFilter, contractFilter, dateFrom, dateTo, brokerName, dealLabel]);

  const kpis = useMemo(() => {
    const pendingGross = filtered
      .filter((c) => c.status === "PENDING")
      .reduce((sum, c) => sum + (c.gross_amount ?? c.amount), 0);
    const approvedNet = filtered
      .filter((c) => c.status === "APPROVED")
      .reduce((sum, c) => sum + (c.net_amount ?? c.gross_amount ?? c.amount), 0);
    const paidNet = filtered
      .filter((c) => c.status === "PAID")
      .reduce((sum, c) => sum + (c.net_amount ?? c.gross_amount ?? c.amount), 0);
    const byStatus = filtered.reduce(
      (acc, c) => {
        acc[c.status] = (acc[c.status] || 0) + 1;
        return acc;
      },
      {} as Record<string, number>
    );
    return { pendingGross, approvedNet, paidNet, byStatus };
  }, [filtered]);

  // Update local item após ação (evita refetch full a cada toggle).
  const applyUpdate = (updated: Commission) => {
    setItems((prev) => prev.map((c) => (c.id === updated.id ? updated : c)));
  };

  const handleAction = useCallback(
    async (action: CommissionDrawerAction, commission: Commission) => {
      try {
        if (action === "approve") {
          const updated = await commissionService.approve(commission.id);
          applyUpdate(updated);
          toast.success("Comissão aprovada");
        } else if (action === "pay") {
          setPayingId(commission.id);
        } else if (action === "cancel") {
          setCancellingId(commission.id);
        } else if (action === "reverse") {
          setReversingId(commission.id);
        }
      } catch (err) {
        console.error(err);
        // A13: mesmo motivo do inbox — a recusa de segregação de funções chega
        // como 409 com a causa; mostrar o genérico esconde o que fazer.
        toast.error(
          (err as { response?: { data?: { message?: string } } })?.response?.data?.message ||
            "Não foi possível aplicar a ação."
        );
      }
    },
    []
  );

  const selectedCommission = selectedId ? items.find((c) => c.id === selectedId) || null : null;
  const payingCommission = payingId ? items.find((c) => c.id === payingId) : null;
  const cancellingCommission = cancellingId ? items.find((c) => c.id === cancellingId) : null;
  const reversingCommission = reversingId ? items.find((c) => c.id === reversingId) : null;

  if (loadError) return <FinanceLoadError message="Não foi possível carregar todas as comissões." onRetry={() => void refetch()} />;
  if (loading) return <p role="status">Carregando comissões…</p>;

  return (
    <div className="space-y-6">
      <p className="text-xs text-muted-foreground">
        Totais da seleção atual. O período filtra a data de criação da comissão. O desconto de IRRF/INSS/ISS é lançado
        em <span className="font-bold">Descontos</span> e só é aceito enquanto a comissão está Pendente — depois da
        aprovação da ordem de pagamento o líquido já está comprometido. Cada desconto vira um tributo retido a recolher
        em Financeiro → Impostos retidos.
      </p>
      {/* KPIs */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <KpiCard
          icon={AlertTriangle}
          label="Pendentes (bruto)"
          value={formatBRL(kpis.pendingGross)}
          tone="warning"
          sub={`${kpis.byStatus["PENDING"] || 0} comissões`}
        />
        <KpiCard
          icon={Briefcase}
          label="Aprovadas a pagar (líquido)"
          value={formatBRL(kpis.approvedNet)}
          tone="info"
          sub={`${kpis.byStatus["APPROVED"] || 0} comissões`}
        />
        <KpiCard
          icon={CheckCircle2}
          label="Pagas (líquido)"
          value={formatBRL(kpis.paidNet)}
          tone="ok"
          sub={`${kpis.byStatus["PAID"] || 0} comissões`}
        />
      </div>

      {/* Filtros */}
      <div className="bg-card rounded-xl border border-border p-4 space-y-3">
        <div className="flex flex-wrap gap-3 items-end">
          <div className="flex-1 min-w-[200px]">
            <label className="block text-xs font-bold text-muted-foreground mb-1">Buscar</label>
            <div className="relative">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
              <input
                type="search"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder="ID, corretor, deal, contrato..."
                className="w-full pl-9 pr-3 py-2 rounded-lg border border-border bg-muted focus:bg-card focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500 outline-none text-sm"
              />
            </div>
          </div>
          <div>
            <label className="block text-xs font-bold text-muted-foreground mb-1">Status</label>
            <select
              value={statusFilter}
              onChange={(e) => setStatusFilter(e.target.value as any)}
              className="px-3 py-2 rounded-lg border border-border bg-muted focus:bg-card focus:ring-2 focus:ring-primary-500/20 outline-none text-sm"
            >
              {STATUS_OPTIONS.map((s) => (
                <option key={s.value} value={s.value}>
                  {s.label}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-xs font-bold text-muted-foreground mb-1">Corretor</label>
            <select
              value={brokerFilter}
              onChange={(e) => setBrokerFilter(e.target.value)}
              className="px-3 py-2 rounded-lg border border-border bg-muted focus:bg-card focus:ring-2 focus:ring-primary-500/20 outline-none text-sm max-w-[200px]"
            >
              <option value="ALL">Todos</option>
              {brokerOptions.map((b) => (
                <option key={b.id} value={b.id}>
                  {b.label}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-xs font-bold text-muted-foreground mb-1">De</label>
            <input
              type="date"
              value={dateFrom}
              onChange={(e) => setDateFrom(e.target.value)}
              className="px-3 py-2 rounded-lg border border-border bg-muted focus:bg-card focus:ring-2 focus:ring-primary-500/20 outline-none text-sm"
            />
          </div>
          <div>
            <label className="block text-xs font-bold text-muted-foreground mb-1">Até</label>
            <input
              type="date"
              value={dateTo}
              onChange={(e) => setDateTo(e.target.value)}
              className="px-3 py-2 rounded-lg border border-border bg-muted focus:bg-card focus:ring-2 focus:ring-primary-500/20 outline-none text-sm"
            />
          </div>
        </div>
        <p className="text-xs text-muted-foreground">
          Mostrando {filtered.length} de {items.length} comissões
        </p>
      </div>

      {/* Tabela */}
      <div className="bg-card rounded-xl border border-border overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead className="bg-muted/50 border-b border-border">
              <tr>
                <th className="px-4 py-3 text-left font-bold text-xs text-muted-foreground uppercase tracking-wider">Contrato / Deal</th>
                <th className="px-4 py-3 text-left font-bold text-xs text-muted-foreground uppercase tracking-wider">Corretor</th>
                <th className="px-4 py-3 text-left font-bold text-xs text-muted-foreground uppercase tracking-wider">Role</th>
                <th className="px-4 py-3 text-right font-bold text-xs text-muted-foreground uppercase tracking-wider">Bruto</th>
                <th className="px-4 py-3 text-right font-bold text-xs text-muted-foreground uppercase tracking-wider">Descontos</th>
                <th className="px-4 py-3 text-right font-bold text-xs text-muted-foreground uppercase tracking-wider">Líquido</th>
                <th className="px-4 py-3 text-left font-bold text-xs text-muted-foreground uppercase tracking-wider">Criada</th>
                <th className="px-4 py-3 text-left font-bold text-xs text-muted-foreground uppercase tracking-wider">Status</th>
                <th className="px-4 py-3 text-right font-bold text-xs text-muted-foreground uppercase tracking-wider">Ações</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {loading ? (
                <tr>
                  <td colSpan={9} className="px-4 py-10 text-center text-muted-foreground">
                    <Loader2 className="inline w-5 h-5 animate-spin mr-2" /> Carregando…
                  </td>
                </tr>
              ) : filtered.length === 0 ? (
                <tr>
                  <td colSpan={9} className="px-4 py-10 text-center text-muted-foreground">
                    Nenhuma comissão atende aos filtros.
                  </td>
                </tr>
              ) : (
                filtered.map((c) => {
                  const meta = STATUS_PILL[c.status];
                  const net = c.net_amount ?? c.gross_amount ?? c.amount;
                  const gross = c.gross_amount ?? c.amount;
                  // Desconto é o que separa o bruto do líquido. A lista de
                  // `deductions` é a origem; o total vem do próprio dinheiro
                  // (bruto − líquido) para não divergir do valor que será pago.
                  const deductionsTotal = Math.max(0, gross - net);
                  const deductionsCount = c.deductions?.length ?? 0;
                  const canLaunchDeduction = c.status === "PENDING" && deductionsCount === 0;
                  const requiresRentalReconciliation = isRentalCommission(c) && (c.funding_required_cents ?? 0) <= 0;
                  return (
                    <tr key={c.id} className="hover:bg-muted/30 transition-colors">
                      <td className="px-4 py-3 font-medium text-foreground">{dealLabel(c.deal_id)}</td>
                      <td className="px-4 py-3 text-foreground">{brokerName(c.broker_id)}</td>
                      <td className="px-4 py-3 text-muted-foreground">{c.role ? ROLE_LABEL[c.role] || c.role : "—"}</td>
                      <td className="px-4 py-3 text-right font-mono text-foreground">{formatBRL(gross)}</td>
                      <td className="px-4 py-3 text-right font-mono">
                        <span className={deductionsTotal > 0 ? "font-bold text-status-critical" : "text-muted-foreground"}>
                          {deductionsTotal > 0 ? `−${formatBRL(deductionsTotal)}` : formatBRL(0)}
                        </span>
                        {deductionsCount > 0 && (
                          <span className="block text-xs text-muted-foreground">
                            {deductionsCount} {deductionsCount === 1 ? "lançamento" : "lançamentos"}
                          </span>
                        )}
                      </td>
                      <td className="px-4 py-3 text-right font-mono font-bold text-status-ok">{formatBRL(net)}</td>
                      <td className="px-4 py-3 text-muted-foreground flex items-center gap-1.5">
                        <Calendar className="w-3.5 h-3.5" /> {formatDate(c.created_at)}
                      </td>
                      <td className="px-4 py-3">
                        <span className={clsx("inline-flex px-2 py-0.5 rounded-full text-xs font-bold border", meta)}>
                          {STATUS_LABEL[c.status] || c.status}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-right">
                        <div className="flex items-center justify-end gap-1">
                          {/* Caminho explícito para o desconto: o editor vive no
                              drawer de detalhe e, sem esta ação na linha, lançar
                              IRRF/INSS/ISS dependia de adivinhar o ícone do olho. */}
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => setSelectedId(c.id)}
                            aria-label={
                              canLaunchDeduction
                                ? `Lançar desconto na comissão ${c.id}`
                                : `Ver descontos da comissão ${c.id}`
                            }
                          >
                            {canLaunchDeduction ? "Lançar desconto" : "Descontos"}
                          </Button>
                          {/* Nome acessível próprio: o `title` não é lido por
                              leitor de tela e não aparece no foco do teclado. */}
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => setSelectedId(c.id)}
                            aria-label={`Ver detalhe da comissão ${c.id}`}
                          >
                            <Eye className="w-4 h-4" aria-hidden="true" />
                          </Button>
                          {!c.payout_order_id && (c.status === "PENDING" || c.status === "APPROVED") && (
                            <DropdownMenu>
                              <DropdownMenuTrigger
                                type="button"
                                className="inline-flex items-center justify-center h-8 w-8 rounded-md text-muted-foreground hover:bg-muted hover:text-foreground transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
                                aria-label="Mais ações"
                              >
                                <MoreHorizontal className="w-4 h-4" />
                              </DropdownMenuTrigger>
                              <DropdownMenuContent align="end" className="w-48">
                                {c.status === "PENDING" && !requiresRentalReconciliation && (
                                  <DropdownMenuItem onSelect={() => handleAction("approve", c)}>
                                    <CheckCircle2 className="w-4 h-4 mr-2" /> Aprovar
                                  </DropdownMenuItem>
                                )}
                                {!requiresRentalReconciliation && (
                                  <DropdownMenuItem onSelect={() => handleAction("pay", c)}>
                                    <DollarSign className="w-4 h-4 mr-2" /> Marcar Paga
                                  </DropdownMenuItem>
                                )}
                                {!requiresRentalReconciliation && <DropdownMenuSeparator />}
                                <DropdownMenuItem
                                  onSelect={() => handleAction("cancel", c)}
                                  className="text-status-critical focus:bg-status-critical-soft focus:text-status-critical"
                                >
                                  <XCircle className="w-4 h-4 mr-2" /> Cancelar
                                </DropdownMenuItem>
                              </DropdownMenuContent>
                            </DropdownMenu>
                          )}
                        </div>
                      </td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Drawer detalhe */}
      <CommissionDetailDrawer
        commission={selectedCommission}
        brokerLabel={brokerName(selectedCommission?.broker_id)}
        contractLabel={selectedCommission ? dealLabel(selectedCommission.deal_id) : undefined}
        rentalOrigin={isRentalCommission(selectedCommission)}
        onClose={() => setSelectedId(null)}
        onCommissionChange={(updated) => applyUpdate(updated)}
        onAction={(action, commission) => {
          if (action !== "approve") setSelectedId(null);
          handleAction(action, commission);
        }}
      />

      {/* Mark paid modal */}
      {payingCommission && (
        <MarkPaidCommissionModal
          commissionId={payingCommission.id}
          brokerLabel={brokerName(payingCommission.broker_id)}
          netAmount={payingCommission.net_amount ?? payingCommission.gross_amount ?? payingCommission.amount}
          onClose={() => setPayingId(null)}
          onSave={async (id, data) => {
            try {
              const updated = await commissionService.pay(id, data);
              applyUpdate(updated);
              toast.success("Comissão marcada como paga");
            } catch (err) {
              console.error(err);
              toast.error("Não foi possível marcar como paga.");
              throw err;
            }
          }}
        />
      )}

      {/* Cancel dialog */}
      {cancellingCommission && (
        <CancelarCobrancaDialog
          open={!!cancellingId}
          title="Cancelar Comissão"
          description="A comissão será marcada como CANCELADA. O motivo entra no audit trail."
          contextLine={`${brokerName(cancellingCommission.broker_id)} · ${dealLabel(cancellingCommission.deal_id)}`}
          onCancel={() => setCancellingId(null)}
          onConfirm={async (reason) => {
            try {
              const updated = await commissionService.cancel(cancellingCommission.id, reason);
              applyUpdate(updated);
              setCancellingId(null);
              toast.success("Comissão cancelada");
            } catch (err) {
              console.error(err);
              toast.error("Não foi possível cancelar.");
            }
          }}
        />
      )}

      {/* Reverse dialog */}
      {reversingCommission && (
        <CancelarCobrancaDialog
          open={!!reversingId}
          title="Estornar comissão"
          description="O estorno preserva a comissão original no histórico e reverte o lançamento de caixa vinculado. Depois, recrie a comissão com a composição correta."
          contextLine={`${brokerName(reversingCommission.broker_id)} · ${dealLabel(reversingCommission.deal_id)}`}
          confirmLabel="Confirmar estorno"
          loadingText="Estornando..."
          onCancel={() => setReversingId(null)}
          onConfirm={async (reason) => {
            try {
              const updated = await commissionService.reverse(reversingCommission.id, reason);
              applyUpdate(updated);
              setReversingId(null);
              toast.success("Comissão estornada");
            } catch (err) {
              console.error(err);
              toast.error("Não foi possível estornar.");
            }
          }}
        />
      )}
    </div>
  );
}

function KpiCard({ icon: Icon, label, value, tone, sub }: { icon: typeof DollarSign; label: string; value: string; tone: "warning" | "info" | "ok"; sub?: string }) {
  const cls =
    tone === "warning"
      ? "bg-status-warning-soft text-status-warning border-status-warning/30"
      : tone === "info"
      ? "bg-status-info-soft text-status-info border-status-info/30"
      : "bg-status-ok-soft text-status-ok border-status-ok/30";
  return (
    <div className="rounded-xl border border-border bg-card p-4">
      <div className="flex items-start gap-3">
        <div className={clsx("p-2 rounded-lg border", cls)}>
          <Icon className="w-5 h-5" />
        </div>
        <div className="min-w-0">
          <p className="text-xs font-bold text-muted-foreground uppercase tracking-wider">{label}</p>
          <p className="text-2xl font-bold text-foreground mt-1 truncate">{value}</p>
          {sub && <p className="text-xs text-muted-foreground mt-0.5">{sub}</p>}
        </div>
      </div>
    </div>
  );
}
