import { RentalInspectionsPanel } from "../components/ContractRentalTab";
import { useConfirmAction } from "../hooks/useConfirmAction";
import { useState, type ReactNode } from "react";
import { useParams, useNavigate, Link } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  ArrowLeft,
  Banknote,
  Bell,
  Building2,
  Calendar,
  CheckCircle2,
  ChevronRight,
  DollarSign,
  Download,
  Droplet,
  FileCheck,
  FileText,
  Flame,
  Handshake,
  Home,
  Loader2,
  Play,
  Plus,
  Receipt,
  ShieldCheck,
  Trash2,
  User,
  UserCheck,
  Wallet,
  Wrench,
  Zap,
} from "lucide-react";
import { clsx } from "clsx";
import { fetchRental, rentalQueryKey } from "../rentals/queries";
import { chargeService, type Charge } from "../services/charge.service";
import { maintenanceService, type MaintenanceTicket } from "../services/maintenance.service";
import { transferService } from "../services/transfer.service";
import type { UtilityAccount, UtilityAccountType } from "../services/utilityAccount.service";
import type { ReminderRule } from "../services/reminderRule.service";
import { useLeadsContext } from "../context/LeadsContext";
import { usePropertiesContext } from "../context/PropertiesContext";
import {
  useUtilityAccountsQuery,
  useDeleteUtilityAccountMutation,
  useContractTransfersQuery,
  useMarkChargePaidMutation,
  useApproveTransferMutation,
  useProcessTransferMutation,
  useReminderRulesQuery,
  useDeleteReminderRuleMutation,
  useRunReminderRulesNowMutation,
} from "../hooks/useContracts";
import { usePermissionsContext } from "../context/PermissionsContext";
import { UtilityAccountModal } from "../components/UtilityAccountModal";
import { ReminderRuleModal } from "../components/ReminderRuleModal";
import { MarkPaidModal } from "../components/MarkPaidModal";
import { ProcessRepasseModal } from "../components/ProcessRepasseModal";
import { ChargeReceiptModal } from "../components/ChargeReceiptModal";
import { EmptyState } from "../ui/EmptyState";
import { Button } from "../ui/button";
import type { Contract } from "../contracts/types";
import { formatCurrencyBRL, formatCurrencyBRL0, formatCompetencia, formatISODateBR } from "../utils/format";
import { availableChargeActions, mapChargeStatus } from "../utils/charge";
import { chargeReceivableAmounts } from "../lib/finance/economicRecords";
import { getOwnerTransferPresentation, type OwnerTransferDisplayStatus } from "../lib/finance/ownerTransferFlow";
import { notifyApiError } from "../services/errorToast";

export function RentalDetails() {
  const { confirmAction, confirmationDialog } = useConfirmAction();
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { leads } = useLeadsContext();
  const { properties } = usePropertiesContext();
  const { can } = usePermissionsContext();

  // P1/A7: as escritas desta tela (contas de consumo, régua de lembretes,
  // configuração de cobrança) são guardadas por `charges:write` no
  // toca-financial. `rentals:write` foi removida do catálogo: não existe rota de
  // escrita de locação e a permissão fazia `operacoes` ver botões que o servidor
  // recusava.
  const canWrite = can("charges:write");
  const canSettleCharges = can("finance:write");
  const canApproveTransfer = can("finance:approve_transfer");

  const [utilityModalOpen, setUtilityModalOpen] = useState(false);
  const [reminderModalOpen, setReminderModalOpen] = useState(false);
  const [editingRule, setEditingRule] = useState<ReminderRule | null>(null);
  const [markingPaidChargeId, setMarkingPaidChargeId] = useState<string | null>(null);
  const [receiptChargeId, setReceiptChargeId] = useState<string | null>(null);
  const [processingTransferId, setProcessingTransferId] = useState<string | null>(null);

  const rentalQuery = useQuery({
    queryKey: rentalQueryKey(id ?? ""),
    queryFn: () => fetchRental(id!),
    enabled: Boolean(id),
    staleTime: 30_000,
    // A 404 means "no consolidated rental for this contract yet" (e.g. a
    // contract signed before the Rental aggregate existed). That's an
    // expected empty state, not a transient failure — don't retry it.
    retry: (failureCount, error) =>
      (error as { response?: { status?: number } })?.response?.status === 404
        ? false
        : failureCount < 3,
  });

  const rental = rentalQuery.data ?? null;

  const chargesQuery = useQuery({
    queryKey: ["rentals", id, "charges"],
    queryFn: async (): Promise<Charge[]> => {
      if (!rental) return [];
      const res = rental.deal_id
        ? await chargeService.getAll({ deal_id: rental.deal_id, limit: 200 })
        : rental.contract_id
          ? await chargeService.getAll({ contract_id: rental.contract_id, limit: 200 })
          : null;
      return res?.data ?? [];
    },
    enabled: Boolean(rental?.id),
    staleTime: 30_000,
  });

  const ticketsQuery = useQuery({
    queryKey: ["rentals", id, "maintenance"],
    queryFn: async (): Promise<MaintenanceTicket[]> => {
      if (!id) return [];
      const res = await maintenanceService.getAll({ rental_id: id });
      return res.data ?? [];
    },
    enabled: Boolean(id),
    staleTime: 30_000,
  });

  const utilityQuery = useUtilityAccountsQuery(rental?.contract_id ?? null);
  const transfersQuery = useContractTransfersQuery(rental?.contract_id ?? null);
  const reminderRulesQuery = useReminderRulesQuery();

  const deleteUtility = useDeleteUtilityAccountMutation(rental?.contract_id ?? null);
  const markChargePaid = useMarkChargePaidMutation(rental?.contract_id ?? null);
  const approveTransfer = useApproveTransferMutation(rental?.contract_id ?? null);
  const processTransfer = useProcessTransferMutation(rental?.contract_id ?? null);
  const deleteRule = useDeleteReminderRuleMutation();
  const runReminders = useRunReminderRulesNowMutation();

  if (rentalQuery.isLoading) {
    return (
      <div className="p-8 flex items-center justify-center text-muted-foreground gap-2">
        <Loader2 className="w-4 h-4 animate-spin" />
        Carregando detalhes da locação...
      </div>
    );
  }

  // A 404 (or a successful-but-empty response) is not an error: the contract
  // simply has no consolidated rental yet. Show a friendly empty state and
  // reserve the error UI for genuine failures (network, 5xx).
  const rentalErrorStatus = (rentalQuery.error as { response?: { status?: number } } | null)?.response?.status;
  const rentalNotFound = rentalErrorStatus === 404 || (!rentalQuery.isError && !rental);

  if (rentalQuery.isError && !rentalNotFound) {
    return (
      <div className="p-8 text-center text-muted-foreground">
        <p>Falha ao carregar a locação.</p>
        <Button onClick={() => navigate(-1)} className="mt-4">
          Voltar
        </Button>
      </div>
    );
  }

  if (rentalNotFound) {
    return (
      <div className="p-8">
        <EmptyState
          icon={Home}
          title="Locação não disponível"
          message="Não encontramos uma visão consolidada para esta locação. Ela é criada automaticamente quando o contrato é assinado."
          action={<Button onClick={() => navigate(-1)}>Voltar</Button>}
        />
      </div>
    );
  }

  // The guards above cover every null path (loading, error, not-found); this
  // final check narrows `rental` to non-null for the rest of the render.
  if (!rental) return null;

  const tenantLead = leads.find((l) => l.id === rental.tenant_lead_id);
  const ownerLead = leads.find((l) => l.id === rental.owner_lead_id);
  const property = properties.find((p) => p.id === rental.property_id);

  const charges = chargesQuery.data ?? [];
  const tickets = ticketsQuery.data ?? [];
  const utilityAccounts = utilityQuery.data ?? [];
  const transfers = transfersQuery.data ?? [];
  const reminderRules = reminderRulesQuery.data ?? [];
  const markingPaidCharge = markingPaidChargeId ? charges.find((charge) => charge.id === markingPaidChargeId) ?? null : null;
  const receiptCharge = receiptChargeId ? charges.find((charge) => charge.id === receiptChargeId) ?? null : null;
  const processingTransfer = processingTransferId
    ? transfers.find((transfer) => transfer.id === processingTransferId) ?? null
    : null;

  const monthlyValue = (rental.monthly_value_cents || 0) / 100;
  const startFmt = rental.start_date ? new Date(rental.start_date).toLocaleDateString("pt-BR") : "—";
  const endFmt = rental.end_date ? new Date(rental.end_date).toLocaleDateString("pt-BR") : "—";

  const handleRemoveUtility = (account: UtilityAccount) => {
    confirmAction(`Remover ${utilityTypeLabel(account.type)} (${account.provider})?`, () => deleteUtility.mutateAsync(account.id), { title: "Remover conta", confirmLabel: "Remover", variant: "danger" });
  };

  const handleRemoveRule = (rule: ReminderRule) => {
    confirmAction(`Remover a regra "${rule.name}"?`, () => deleteRule.mutateAsync(rule.id), { title: "Remover regra", confirmLabel: "Remover", variant: "danger" });
  };

  const handleDownloadReceipt = async (transferId: string) => {
    try {
      const blob = await transferService.downloadReceipt(transferId);
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = `repasse-${transferId.slice(0, 8)}.pdf`;
      document.body.appendChild(link);
      link.click();
      link.remove();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
    } catch (error: any) {
      notifyApiError(error, { context: "Baixar comprovante" });
    }
  };

  const handleApproveTransfer = async (transferId: string) => {
    const transfer = transfers.find((item) => item.id === transferId);
    if (!transfer || getOwnerTransferPresentation(transfer).action !== "approve") {
      toast.warning("Este repasse não está pronto para aprovação. Revise a composição financeira.");
      return;
    }
    try {
      await approveTransfer.mutateAsync(transferId);
    } catch {
      // O hook apresenta o erro e mantém a ação disponível para nova tentativa.
    }
  };

  const handleOpenProcessTransfer = (transferId: string) => {
    const transfer = transfers.find((item) => item.id === transferId);
    if (!transfer || getOwnerTransferPresentation(transfer).action !== "process") {
      toast.warning("Apenas repasses aprovados e com cobertura canônica podem ser processados.");
      return;
    }
    setProcessingTransferId(transferId);
  };

  // Repasse sem saldo a repassar: nada é transferido nem lançado no caixa, e o
  // backend não exige comprovante. Confirmação simples, sem o painel de
  // comprovante usado no processamento normal.
  const handleSettleTransferWithoutBalance = (transferId: string) => {
    const transfer = transfers.find((item) => item.id === transferId);
    if (!transfer || getOwnerTransferPresentation(transfer).action !== "settle") {
      toast.warning("Este repasse não está mais sem saldo a repassar. Revise a composição financeira.");
      return;
    }
    confirmAction(
      `Não há valor a transferir na competência ${formatCompetencia(transfer.reference_month)}. O repasse será encerrado sem comprovante e nenhuma movimentação de caixa será registrada. Esta ação não pode ser desfeita.`,
      async () => {
        await processTransfer.mutateAsync({ transferId, evidence: {} });
      },
      { title: "Concluir repasse sem saldo", confirmLabel: "Concluir sem saldo", variant: "warning" },
    );
  };

  return (
    <div className="flex flex-col h-full bg-muted overflow-auto">
      {confirmationDialog}
      <div className="flex-none px-6 py-4 lg:px-8 border-b border-border bg-card sticky top-0 z-20">
        <div className="flex items-center justify-between gap-4">
          <div className="flex items-center gap-3 min-w-0">
            <Button variant="ghost" size="icon" onClick={() => navigate(-1)} className="shrink-0" aria-label="Voltar">
              <ArrowLeft className="w-5 h-5" />
            </Button>
            <div className="min-w-0">
              <h1 className="text-xl font-bold text-foreground truncate">Locação #{rental.id.slice(0, 8)}</h1>
              <p className="text-xs text-muted-foreground truncate">
                {property?.title || "Imóvel"} · {formatCurrencyBRL0(monthlyValue)}/mês
              </p>
            </div>
          </div>
          <span
            className={clsx(
              "px-3 py-1 text-xs font-bold rounded-full shrink-0",
              rental.status === "ACTIVE"
                ? "bg-status-ok-soft text-status-ok"
                : rental.status === "ENDED"
                  ? "bg-muted text-muted-foreground"
                  : "bg-status-critical-soft text-status-critical",
            )}
          >
            {rental.status === "ACTIVE" ? "Ativa" : rental.status === "ENDED" ? "Encerrada" : "Rescindida"}
          </span>
        </div>
      </div>

      <div className="flex-1 p-6 lg:p-8">
        <div className="max-w-6xl mx-auto space-y-6">
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            <ParticipantCard
              icon={UserCheck}
              accent="text-status-info"
              accentBg="bg-status-info-soft"
              title="Inquilino"
              name={tenantLead?.name || rental.tenant_lead_id}
              subtitle={tenantLead?.email || tenantLead?.phone}
              to={`/leads/${rental.tenant_lead_id}`}
            />
            <ParticipantCard
              icon={User}
              accent="text-status-warning"
              accentBg="bg-status-warning-soft"
              title="Proprietário"
              name={ownerLead?.name || rental.owner_lead_id}
              subtitle={ownerLead?.email || ownerLead?.phone}
              to={`/leads/${rental.owner_lead_id}`}
            />
            <ParticipantCard
              icon={Building2}
              accent="text-primary-600"
              accentBg="bg-primary-50"
              title="Imóvel"
              name={property?.title || rental.property_id}
              subtitle={property?.address ? formatPropertyAddressShort(property) : undefined}
              to={`/imoveis/${rental.property_id}`}
            />
          </div>

          <div className="bg-card rounded-2xl border border-border p-5 grid grid-cols-2 md:grid-cols-4 gap-4">
            <Fact icon={Calendar} label="Início" value={startFmt} />
            <Fact icon={Calendar} label="Fim previsto" value={endFmt} />
            <Fact icon={DollarSign} label="Aluguel mensal" value={formatCurrencyBRL0(monthlyValue)} />
            {rental.deal_id && (
              <Fact icon={Handshake} label="Negócio" value={`#${rental.deal_id.slice(0, 8)}`} to={`/esteira/deals/${rental.deal_id}`} />
            )}
          </div>

          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
            <div className="bg-card rounded-2xl border border-border p-5">
              <h3 className="text-base font-bold text-foreground flex items-center gap-2 mb-4">
                <Wallet className="w-4 h-4 text-primary" /> Cobranças
              </h3>
              {chargesQuery.isLoading ? (
                <p className="text-sm text-muted-foreground">Carregando…</p>
              ) : charges.length === 0 ? (
                <EmptyState icon={Wallet} message="Sem cobranças geradas ainda." />
              ) : (
                <ul className="space-y-2 max-h-[420px] overflow-y-auto">
                  {charges.map((charge) => {
                    const status = mapChargeStatus(charge.status);
                    const actions = availableChargeActions(charge.status);
                    return (
                      <li key={charge.id} className="flex items-center justify-between gap-3 p-3 rounded-xl border border-border bg-background/50">
                        <div className="min-w-0 flex-1">
                          <p className="text-sm font-semibold text-foreground truncate">{charge.description || charge.type}</p>
                          <p className="text-xs text-muted-foreground">
                            {/* Vencimento é dia do calendário (gravado em UTC): formatar
                                no fuso local mostrava 09/10 para a cobrança de 10/10. */}
                            Vence: {formatISODateBR(charge.due_date) || "—"}
                          </p>
                        </div>
                        <div className="flex items-center gap-2 shrink-0">
                          <span
                            className={clsx(
                              "px-2 py-0.5 text-xs font-bold rounded-full",
                              status === "Pago"
                                ? "bg-status-ok-soft text-status-ok"
                                : status === "Em Atraso"
                                  ? "bg-status-critical-soft text-status-critical"
                                  : status === "Cancelada" || status === "Reversada"
                                    ? "bg-muted text-muted-foreground"
                                    : "bg-status-warning-soft text-status-warning",
                            )}
                          >
                            {status}
                          </span>
                          {actions.includes("receipt") && (
                            <Button variant="ghost" size="sm" onClick={() => setReceiptChargeId(charge.id)}>
                              <Receipt className="w-3 h-3" /> Recibo
                            </Button>
                          )}
                          {canSettleCharges && actions.includes("pay") && (
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => setMarkingPaidChargeId(charge.id)}
                              className="text-status-ok hover:text-status-ok hover:bg-status-ok-soft"
                            >
                              <CheckCircle2 className="w-3 h-3" /> Dar baixa
                            </Button>
                          )}
                        </div>
                      </li>
                    );
                  })}
                </ul>
              )}
            </div>

            <div className="bg-card rounded-2xl border border-border p-5">
              <h3 className="text-base font-bold text-foreground flex items-center gap-2 mb-4">
                <Wrench className="w-4 h-4 text-primary" /> Manutenção
                <Link className="ml-auto text-label text-primary" to={`/manutencao?contrato=${encodeURIComponent(rental.contract_id)}`}>Novo chamado</Link>
              </h3>
              {ticketsQuery.isLoading ? (
                <p className="text-sm text-muted-foreground">Carregando…</p>
              ) : tickets.length === 0 ? (
                <EmptyState icon={Wrench} message="Sem chamados de manutenção" />
              ) : (
                <ul className="space-y-2 max-h-[420px] overflow-y-auto">
                  {tickets.map((t) => (
                    <li key={t.id} className="flex items-center justify-between gap-3 p-3 rounded-xl border border-border bg-background/50">
                      <div className="min-w-0 flex-1">
                        <p className="text-sm font-semibold text-foreground truncate">{t.category || "Manutenção"}</p>
                        <p className="text-xs text-muted-foreground line-clamp-1">{t.description || "—"}</p>
                      </div>
                      <span
                        className={clsx(
                          "px-2 py-0.5 text-xs font-bold rounded-full shrink-0",
                          t.status === "Concluído"
                            ? "bg-status-ok-soft text-status-ok"
                            : t.status === "Em Andamento"
                              ? "bg-status-info-soft text-status-info"
                              : t.status === "Cancelado"
                                ? "bg-muted text-muted-foreground"
                                : "bg-status-warning-soft text-status-warning",
                        )}
                      >
                        {t.status}
                      </span>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          </div>

          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
            <div className="bg-card rounded-2xl border border-border p-5">
              <div className="flex items-center justify-between gap-3 mb-4">
                <h3 className="text-base font-bold text-foreground flex items-center gap-2">
                  <Droplet className="w-4 h-4 text-primary" /> Contas de Consumo
                </h3>
                {canWrite && (
                  <Button size="sm" onClick={() => setUtilityModalOpen(true)}>
                    <Plus className="w-4 h-4" /> Adicionar Conta
                  </Button>
                )}
              </div>
              {utilityQuery.isLoading ? (
                <p className="text-sm text-muted-foreground">Carregando…</p>
              ) : utilityAccounts.length === 0 ? (
                <EmptyState icon={Droplet} message="Nenhuma conta de consumo vinculada." />
              ) : (
                <ul className="space-y-2 max-h-[420px] overflow-y-auto">
                  {utilityAccounts.map((account) => (
                    <li key={account.id} className="flex items-center justify-between gap-3 p-3 rounded-xl border border-border bg-background/50">
                      <div className="flex items-center gap-3 min-w-0 flex-1">
                        <div className="w-9 h-9 rounded-lg bg-primary-50 text-primary-600 flex items-center justify-center shrink-0">
                          {utilityTypeIcon(account.type)}
                        </div>
                        <div className="min-w-0">
                          <p className="text-sm font-semibold text-foreground truncate">{utilityTypeLabel(account.type)}</p>
                          <p className="text-xs text-muted-foreground truncate">
                            {account.provider} · {account.responsible_party === "TENANT" ? "Inquilino" : "Proprietário"}
                          </p>
                        </div>
                      </div>
                      <div className="flex items-center gap-2 shrink-0">
                        <span className="text-sm font-bold text-foreground">
                          {formatCurrencyBRL((account.estimated_amount_cents || 0) / 100)}
                        </span>
                        {canWrite && (
                          <Button
                            variant="ghost"
                            size="icon"
                            className="size-8 text-muted-foreground hover:text-status-critical hover:bg-status-critical-soft"
                            onClick={() => handleRemoveUtility(account)}
                            disabled={deleteUtility.isPending}
                            aria-label="Remover conta"
                          >
                            <Trash2 className="w-3.5 h-3.5" />
                          </Button>
                        )}
                      </div>
                    </li>
                  ))}
                </ul>
              )}
            </div>

            <div className="bg-card rounded-2xl border border-border p-5">
              <h3 className="text-base font-bold text-foreground flex items-center gap-2 mb-4">
                <Banknote className="w-4 h-4 text-primary" /> Repasses ao Proprietário
              </h3>
              {transfersQuery.isLoading ? (
                <p className="text-sm text-muted-foreground">Carregando…</p>
              ) : transfers.length === 0 ? (
                <EmptyState icon={Banknote} message="Nenhum repasse registrado." />
              ) : (
                <ul className="space-y-2 max-h-[420px] overflow-y-auto">
                  {transfers.map((transfer) => {
                    const presentation = getOwnerTransferPresentation(transfer);
                    const badge = transferStatusBadge(presentation.status);
                    return (
                      <li key={transfer.id} className="flex items-center justify-between gap-3 p-3 rounded-xl border border-border bg-background/50">
                        <div className="min-w-0 flex-1">
                          <p className="text-sm font-semibold text-foreground truncate">{formatCompetencia(transfer.reference_month)}</p>
                          <p className="text-xs text-muted-foreground">Líquido: {formatCurrencyBRL(transfer.net_amount || 0)}</p>
                        </div>
                        <div className="flex items-center gap-2 shrink-0">
                          <span className={clsx("px-2 py-0.5 text-xs font-bold rounded-full", badge.cls)}>{badge.label}</span>
                          {presentation.action === "approve" && canApproveTransfer && (
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => handleApproveTransfer(transfer.id)}
                              disabled={approveTransfer.isPending}
                              className="text-status-ok hover:text-status-ok hover:bg-status-ok-soft"
                            >
                              <CheckCircle2 className="w-3 h-3" /> Aprovar
                            </Button>
                          )}
                          {presentation.action === "process" && canApproveTransfer && (
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => handleOpenProcessTransfer(transfer.id)}
                              disabled={processTransfer.isPending}
                            >
                              <Banknote className="w-3 h-3" /> Processar com comprovante
                            </Button>
                          )}
                          {presentation.action === "settle" && canApproveTransfer && (
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => handleSettleTransferWithoutBalance(transfer.id)}
                              disabled={processTransfer.isPending}
                              className="text-status-ok hover:text-status-ok hover:bg-status-ok-soft"
                            >
                              <CheckCircle2 className="w-3 h-3" /> Concluir sem saldo
                            </Button>
                          )}
                          {presentation.status === "Processado" && (
                            <Button variant="ghost" size="sm" onClick={() => handleDownloadReceipt(transfer.id)}>
                              <Download className="w-3 h-3" /> Comprovante
                            </Button>
                          )}
                          {presentation.status === "A apurar" && (
                            <Link to="/financeiro/repasses" className="text-xs font-bold text-primary-600 hover:text-primary-800">
                              Revisar composição
                            </Link>
                          )}
                        </div>
                      </li>
                    );
                  })}
                </ul>
              )}
            </div>
          </div>

          <div className="bg-card rounded-2xl border border-border p-5">
            <div className="flex items-center justify-between gap-3 mb-4">
              <h3 className="text-base font-bold text-foreground flex items-center gap-2">
                <Bell className="w-4 h-4 text-primary" /> Régua de Lembretes
              </h3>
              {canWrite && (
                <div className="flex items-center gap-2">
                  <Button variant="outline" size="sm" onClick={() => runReminders.mutate()} disabled={runReminders.isPending}>
                    <Play className="w-3.5 h-3.5" /> Executar lembretes
                  </Button>
                  <Button size="sm" onClick={() => { setEditingRule(null); setReminderModalOpen(true); }}>
                    <Plus className="w-4 h-4" /> Nova Regra
                  </Button>
                </div>
              )}
            </div>
            {reminderRulesQuery.isLoading ? (
              <p className="text-sm text-muted-foreground">Carregando…</p>
            ) : reminderRulesQuery.isError ? (
              <p className="text-sm text-muted-foreground">Régua de lembretes indisponível no momento.</p>
            ) : reminderRules.length === 0 ? (
              <EmptyState icon={Bell} message="Nenhuma régua de lembrete configurada." />
            ) : (
              <ul className="space-y-2">
                {reminderRules.map((rule) => (
                  <li key={rule.id} className="flex items-center justify-between gap-3 p-3 rounded-xl border border-border bg-background/50">
                    <div className="min-w-0 flex-1">
                      <p className="text-sm font-semibold text-foreground truncate">{rule.name}</p>
                      <p className="text-xs text-muted-foreground truncate">
                        {reminderTriggerLabel(rule)} · {rule.channels.join(", ")}
                      </p>
                    </div>
                    <div className="flex items-center gap-2 shrink-0">
                      <span
                        className={clsx(
                          "px-2 py-0.5 text-xs font-bold rounded-full",
                          rule.enabled ? "bg-status-ok-soft text-status-ok" : "bg-muted text-muted-foreground",
                        )}
                      >
                        {rule.enabled ? "Ativa" : "Inativa"}
                      </span>
                      {canWrite && (
                        <>
                          <Button variant="ghost" size="sm" onClick={() => { setEditingRule(rule); setReminderModalOpen(true); }}>
                            Editar
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="size-8 text-muted-foreground hover:text-status-critical hover:bg-status-critical-soft"
                            onClick={() => handleRemoveRule(rule)}
                            disabled={deleteRule.isPending}
                            aria-label="Remover regra"
                          >
                            <Trash2 className="w-3.5 h-3.5" />
                          </Button>
                        </>
                      )}
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </div>

          {rental.contract_id && (
            <Link
              to={`/contratos/${rental.contract_id}`}
              className="bg-card rounded-2xl border border-border p-5 flex items-center gap-4 hover:border-primary-300 hover:shadow-md transition-all group"
            >
              <div className="w-12 h-12 rounded-xl bg-status-ok-soft flex items-center justify-center shrink-0">
                <FileCheck className="w-6 h-6 text-status-ok" />
              </div>
              <div className="min-w-0 flex-1">
                <p className="text-sm font-bold text-foreground group-hover:text-primary-700 transition-colors">
                  Ver contrato vinculado
                </p>
                <p className="text-xs text-muted-foreground">
                  Abrir o contrato desta locação — minuta, assinaturas e PDF assinado.
                </p>
              </div>
              <ChevronRight className="w-5 h-5 text-muted-foreground shrink-0" />
            </Link>
          )}
        </div>
      </div>

      {rental.contract_id && <RentalInspectionsPanel contract={{ id: rental.contract_id, property_id: rental.property_id } as Contract} />}

      {utilityModalOpen && (
        <UtilityAccountModal
          contract={{
            id: rental.contract_id,
            property_id: rental.property_id,
            client: tenantLead?.name ?? "",
            property_title: property?.title ?? "",
          } as Contract}
          onClose={() => setUtilityModalOpen(false)}
        />
      )}

      {reminderModalOpen && (
        <ReminderRuleModal rule={editingRule} onClose={() => { setReminderModalOpen(false); setEditingRule(null); }} />
      )}

      {markingPaidCharge && (
        <MarkPaidModal
          chargeId={markingPaidCharge.id}
          defaultAmount={
            chargeReceivableAmounts(markingPaidCharge).pending +
            chargeReceivableAmounts(markingPaidCharge).overdue
          }
          charge={markingPaidCharge}
          onClose={() => setMarkingPaidChargeId(null)}
          onSave={async (chargeId, data) => {
            await markChargePaid.mutateAsync({ chargeId, data });
            await chargesQuery.refetch();
          }}
        />
      )}

      {receiptCharge && (
        <ChargeReceiptModal
          charge={receiptCharge}
          payerName={tenantLead?.name}
          contractDisplayID={rental.contract_id}
          onClose={() => setReceiptChargeId(null)}
        />
      )}

      {processingTransfer && (
        <ProcessRepasseModal
          transferId={processingTransfer.id}
          transfer={processingTransfer}
          onClose={() => setProcessingTransferId(null)}
          onProcess={async (transferId, evidence) => {
            await processTransfer.mutateAsync({ transferId, evidence });
          }}
        />
      )}
    </div>
  );
}

type ParticipantCardProps = {
  icon: typeof Home;
  accent: string;
  accentBg: string;
  title: string;
  name: string;
  subtitle?: string;
  to: string;
};

function ParticipantCard({ icon: Icon, accent, accentBg, title, name, subtitle, to }: ParticipantCardProps) {
  return (
    <Link to={to} className="bg-card rounded-2xl border border-border p-5 hover:border-primary-300 hover:shadow-md transition-all group block">
      <div className="flex items-center gap-3">
        <div className={clsx("w-10 h-10 rounded-full flex items-center justify-center shrink-0", accentBg)}>
          <Icon className={clsx("w-5 h-5", accent)} />
        </div>
        <div className="min-w-0 flex-1">
          <p className="text-xs uppercase tracking-wide text-muted-foreground font-semibold">{title}</p>
          <p className="text-sm font-bold text-foreground truncate group-hover:text-primary-700 transition-colors">{name}</p>
          {subtitle && <p className="text-xs text-muted-foreground truncate mt-0.5">{subtitle}</p>}
        </div>
      </div>
    </Link>
  );
}

type FactProps = {
  icon: typeof Home;
  label: string;
  value: string;
  to?: string;
};

function Fact({ icon: Icon, label, value, to }: FactProps) {
  const content = (
    <>
      <p className="text-caption uppercase tracking-wider text-muted-foreground font-bold mb-1 flex items-center gap-1.5">
        <Icon className="w-3 h-3" /> {label}
      </p>
      <p className="text-sm font-bold text-foreground">{value}</p>
    </>
  );
  if (to) {
    return (
      <Link to={to} className="block hover:text-primary-700 transition-colors">
        {content}
      </Link>
    );
  }
  return <div>{content}</div>;
}

function formatPropertyAddressShort(property: { address?: { street?: string; neighborhood?: string; city?: string } }): string {
  if (!property.address) return "";
  const parts = [property.address.street, property.address.neighborhood, property.address.city].filter(Boolean);
  return parts.join(", ");
}

function utilityTypeLabel(type: UtilityAccountType): string {
  return (
    ({
      AGUA: "Água",
      ENERGIA: "Energia",
      GAS: "Gás",
      CONDOMINIO: "Condomínio",
      IPTU: "IPTU",
      INTERNET: "Internet",
      SEGURO: "Seguro",
      OUTROS: "Outros",
    } as Record<UtilityAccountType, string>)[type] ?? type
  );
}

function utilityTypeIcon(type: UtilityAccountType): ReactNode {
  switch (type) {
    case "AGUA":
      return <Droplet className="w-4 h-4" />;
    case "ENERGIA":
      return <Zap className="w-4 h-4" />;
    case "GAS":
      return <Flame className="w-4 h-4" />;
    case "CONDOMINIO":
      return <Building2 className="w-4 h-4" />;
    case "IPTU":
      return <Receipt className="w-4 h-4" />;
    case "SEGURO":
      return <ShieldCheck className="w-4 h-4" />;
    default:
      return <FileText className="w-4 h-4" />;
  }
}

function transferStatusBadge(status: OwnerTransferDisplayStatus): { label: string; cls: string } {
  if (status === "Liquidado sem saldo") return { label: "Liquidado sem saldo", cls: "bg-status-ok-soft text-status-ok" };
  if (status === "Processado") return { label: "Processado", cls: "bg-status-ok-soft text-status-ok" };
  if (status === "Aprovado") return { label: "Aprovado", cls: "bg-status-info-soft text-status-info" };
  if (status === "Sem saldo") return { label: "Sem saldo", cls: "bg-status-info-soft text-status-info" };
  if (status === "Cancelado") return { label: "Cancelado", cls: "bg-muted text-muted-foreground" };
  if (status === "A apurar") return { label: "A apurar", cls: "bg-muted text-muted-foreground" };
  return { label: "Pendente", cls: "bg-status-warning-soft text-status-warning" };
}

function reminderTriggerLabel(rule: ReminderRule): string {
  const days = rule.offset_days;
  switch (rule.trigger_type) {
    case "BEFORE_DUE":
      return `${days} dia(s) antes do vencimento`;
    case "AFTER_DUE":
      return `${days} dia(s) após o vencimento`;
    default:
      return "No vencimento";
  }
}
