import { contractLabel } from "../../contracts/contractLabel";
import { RentalInspectionsPanel } from "../../components/ContractRentalTab";
import { useConfirmAction } from "../../hooks/useConfirmAction";
import type { Dispatch, ReactNode, SetStateAction } from "react";
import { Link } from "react-router";
import { toast } from "sonner";
import {
 AlertCircle,
 ArrowLeft,
 Banknote,
 BarChart3,
 Bell,
 Building2,
 Calendar,
 CheckCircle2,
 Clock,
 CreditCard,
 Crown,
 DollarSign,
 Droplet,
 Download,
 FileText,
 History,
 Home,
 Mail,
 MapPin,
 Percent,
 Phone,
 Plus,
 Receipt,
 Search,
 Send,
 Settings,
 ShieldCheck,
 Trash2,
 User,
 Wallet,
 X,
} from "lucide-react";
import { clsx } from "clsx";
import { motion } from "motion/react";
import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, Legend } from "recharts";
import { EmptyState } from "../../ui/EmptyState";
import { Button } from "../../ui/button";
import { ContractStatus } from "../../contracts/types";
import { DocumentsTab } from "../../components/DocumentsTab";
import type { UtilityAccount, UtilityAccountType } from "../../services/utilityAccount.service";
import { formatISODateBR } from "../../utils/format";
import { formatPropertyAddress } from "../../data";
import {
 DETAIL_TABS,
 RC,
 type DetailTab,
 type ModalState,
 type ViewState,
} from "./presentation";

type MutationState = {
 isPending: boolean;
 mutate: (...args: any[]) => void;
};

type RentalPermissions = {
 chargesWrite: boolean;
 chargesRead: boolean;
 financeRead: boolean;
 financeWrite: boolean;
 financeApproveTransfer: boolean;
 contractsWrite: boolean;
};

type ChargeSort = "date_desc" | "date_asc" | "value_desc" | "value_asc";
type ChargePeriod = "all" | "this_month" | "last_3" | "last_6";
type RepasseStatus = "Todos" | "Pendente" | "Aprovado" | "Processado" | "Cancelado" | "A apurar" | "Sem saldo" | "Liquidado sem saldo";
type RepasseSort = "month_desc" | "month_asc" | "value_desc" | "value_asc";
type RepassePeriod = "all" | "last_3" | "last_6" | "last_12";

const chargeStatusTone = (status: string) => {
 if (status === "Pago") return "bg-status-ok-soft text-status-ok border-status-ok/30";
 if (status === "A Vencer") return "bg-status-info-soft text-status-info border-status-info/30";
 if (status === "Em Atraso") return "bg-status-critical-soft text-status-critical border-status-critical/30";
 if (status === "Parcial") return "bg-status-warning-soft text-status-warning border-status-warning/30";
 return "bg-muted text-muted-foreground border-border";
};

const repasseStatusTone = (status: string) => {
 if (status === "Liquidado sem saldo") return "bg-status-ok-soft text-status-ok border-status-ok/30";
 if (status === "Processado") return "bg-status-ok-soft text-status-ok border-status-ok/30";
 if (status === "Aprovado") return "bg-status-info-soft text-status-info border-status-info/30";
 if (status === "Sem saldo") return "bg-status-info-soft text-status-info border-status-info/30";
 if (status === "Pendente") return "bg-status-warning-soft text-status-warning border-status-warning/30";
 return "bg-muted text-muted-foreground border-border";
};

export interface RentalDetailViewProps {
 selectedContract: any;
 perms: RentalPermissions;
 detailTab: DetailTab;
 setDetailTab: Dispatch<SetStateAction<DetailTab>>;
 setView: Dispatch<SetStateAction<ViewState>>;
 setActiveModal: Dispatch<SetStateAction<ModalState>>;
 setAuditContractId: Dispatch<SetStateAction<string | null>>;
 setAuditLabel: Dispatch<SetStateAction<string>>;
 openModal: (type: ModalState, contract: any) => void;
 handleTerminateContract: () => void;
 terminateContractMutation: MutationState;
 formatCurrency: (value: number) => string;
 tenantLead?: any;
 propertyForContract?: any;
 ownerLead?: any;
 chargeStats: { total: number; paid: number; overdue: number; pending: number };
 charges: any[];
 financialHistory: Array<{ month: string; receita: number; despesa: number; repasse: number }>;
 utilityAccounts: UtilityAccount[];
 utilityStats: {
  tenantTotalCents: number;
  tenantCount: number;
  ownerTotalCents: number;
  ownerCount: number;
  inactiveCount: number;
 };
 utilityTypeLabel: (type: UtilityAccountType) => string;
 utilityTypeIcon: (type: UtilityAccountType) => ReactNode;
 handleRemoveUtilityAccount: (account: UtilityAccount) => void;
 onLaunchUtilityCharge: (account: UtilityAccount) => void;
 deleteUtilityAccountMutation: MutationState;
 adminFeePct: number;
 filteredCharges: any[];
 hasActiveChargeFilter: boolean;
 clearChargeFilters: () => void;
 selectedChargeIds: Set<string>;
 visibleSelectableIds: string[];
 allVisibleSelected: boolean;
 toggleChargeSelected: (id: string) => void;
 toggleSelectAllVisible: () => void;
 clearChargeSelection: () => void;
 handleBulkMarkPaid: () => void | Promise<void>;
 handleBulkNotify: () => void | Promise<void>;
 chargeFilter: string;
 setChargeFilter: Dispatch<SetStateAction<string>>;
 chargeSearch: string;
 setChargeSearch: Dispatch<SetStateAction<string>>;
 chargeSortBy: ChargeSort;
 setChargeSortBy: Dispatch<SetStateAction<ChargeSort>>;
 chargePeriod: ChargePeriod;
 setChargePeriod: Dispatch<SetStateAction<ChargePeriod>>;
 setAuditChargeId: Dispatch<SetStateAction<string | null>>;
 handleMarkAsPaid: (chargeId: string) => void | Promise<void>;
 handleViewChargeReceipt: (chargeId: string) => void;
 handleSendChargeReminder: (chargeId: string, options?: { skipConfirm?: boolean }) => void | Promise<unknown>;
 realTransfers: any[];
 repasses: any[];
 filteredRepasses: any[];
 handleDownloadRepasseReceipt: (repasseId: string) => void | Promise<void>;
 approveTransferMutation: MutationState;
 processTransferMutation: MutationState;
 setAuditTransferId: Dispatch<SetStateAction<string | null>>;
 resolveActorName: (rawActor?: string | null) => string;
 repasseStatusFilter: RepasseStatus;
 setRepasseStatusFilter: Dispatch<SetStateAction<RepasseStatus>>;
 repasseSearch: string;
 setRepasseSearch: Dispatch<SetStateAction<string>>;
 repasseSortBy: RepasseSort;
 setRepasseSortBy: Dispatch<SetStateAction<RepasseSort>>;
 repassePeriod: RepassePeriod;
 setRepassePeriod: Dispatch<SetStateAction<RepassePeriod>>;
 hasActiveRepasseFilter: boolean;
 clearRepasseFilters: () => void;
 selectedTransferIds: Set<string>;
 selectableTransferIds: string[];
 allVisibleTransfersSelected: boolean;
 toggleTransferSelected: (id: string) => void;
 toggleSelectAllTransfers: () => void;
 clearTransferSelection: () => void;
 handleBulkApproveRepasses: () => void;
 handleApproveRepasse: (repasseId: string) => void | Promise<boolean>;
 handleProcessRepasse: (repasseId: string) => void;
 handleSettleRepasseWithoutBalance: (repasseId: string) => void;
}

export function RentalDetailView({
 selectedContract,
 perms,
 detailTab,
 setDetailTab,
 setView,
 setActiveModal,
 setAuditContractId,
 setAuditLabel,
 openModal,
 handleTerminateContract,
 terminateContractMutation,
 formatCurrency,
 tenantLead,
 propertyForContract,
 ownerLead,
 chargeStats,
 charges,
 financialHistory,
 utilityAccounts,
 utilityStats,
 utilityTypeLabel,
 utilityTypeIcon,
 handleRemoveUtilityAccount,
 onLaunchUtilityCharge,
 deleteUtilityAccountMutation,
 adminFeePct,
 filteredCharges,
 hasActiveChargeFilter,
 clearChargeFilters,
 selectedChargeIds,
 visibleSelectableIds,
 allVisibleSelected,
 toggleChargeSelected,
 toggleSelectAllVisible,
 clearChargeSelection,
 handleBulkMarkPaid,
 handleBulkNotify,
 chargeFilter,
 setChargeFilter,
 chargeSearch,
 setChargeSearch,
 chargeSortBy,
 setChargeSortBy,
 chargePeriod,
 setChargePeriod,
 setAuditChargeId,
 handleMarkAsPaid,
 handleViewChargeReceipt,
 handleSendChargeReminder,
 realTransfers,
 repasses,
 filteredRepasses,
 handleDownloadRepasseReceipt,
 approveTransferMutation,
 processTransferMutation,
 setAuditTransferId,
 resolveActorName,
 repasseStatusFilter,
 setRepasseStatusFilter,
 repasseSearch,
 setRepasseSearch,
 repasseSortBy,
 setRepasseSortBy,
 repassePeriod,
 setRepassePeriod,
 hasActiveRepasseFilter,
 clearRepasseFilters,
 selectedTransferIds,
 selectableTransferIds,
 allVisibleTransfersSelected,
 toggleTransferSelected,
 toggleSelectAllTransfers,
 clearTransferSelection,
 handleBulkApproveRepasses,
 handleApproveRepasse,
 handleProcessRepasse,
 handleSettleRepasseWithoutBalance,
}: RentalDetailViewProps) {
 const { confirmAction, confirmationDialog } = useConfirmAction();
 const renderTabGeral = () => (
 <motion.div initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.25 }}>
 <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
 <div className="bg-card p-6 rounded-2xl border border-border shadow-sm">
 <h3 className="text-base font-bold text-foreground mb-6 flex items-center gap-2 pb-4 border-b border-border"><Building2 className="w-5 h-5 text-primary-600" /> Informações do Imóvel</h3>
 <div className="space-y-4">
 <div className="flex items-start gap-3">
   <MapPin className="w-4 h-4 text-muted-foreground mt-0.5" />
   <div>
     <p className="text-xs text-muted-foreground mb-1">Endereço</p>
     <p className="text-sm font-bold text-foreground">
       {propertyForContract
         ? (propertyForContract.title || formatPropertyAddress(propertyForContract.address))
         : (selectedContract.property_title || 'Não informado')}
     </p>
     {propertyForContract?.id && (
       <Link to={`/imoveis/${propertyForContract.id}`} className="text-xs text-primary-700 hover:underline mt-1 inline-block">Ver imóvel</Link>
     )}
   </div>
 </div>
 <div className="flex items-start gap-3"><DollarSign className="w-4 h-4 text-muted-foreground mt-0.5" /><div><p className="text-xs text-muted-foreground mb-1">Valor Mensal</p><p className="text-sm font-bold text-foreground">{formatCurrency(selectedContract.value || 0)}</p></div></div>
 </div>
 </div>
 <div className="bg-card p-6 rounded-2xl border border-border shadow-sm">
 <h3 className="text-base font-bold text-foreground mb-6 flex items-center gap-2 pb-4 border-b border-border"><Calendar className="w-5 h-5 text-primary-600" /> Período do Contrato</h3>
 <div className="grid grid-cols-2 gap-6 mb-6">
 <div><p className="text-xs text-muted-foreground mb-1">Início</p><p className="text-sm font-bold text-foreground">{formatISODateBR(selectedContract.start_date) || 'N/A'}</p></div>
 <div><p className="text-xs text-muted-foreground mb-1">Término</p><p className="text-sm font-bold text-foreground">{formatISODateBR(selectedContract.end_date) || 'N/A'}</p></div>
 </div>
 <div className="grid grid-cols-2 gap-6">
 <div><p className="text-xs text-muted-foreground mb-1">Próximo Reajuste</p><p className="text-sm font-bold text-foreground">{formatISODateBR(selectedContract.next_adjustment ?? selectedContract.nextAdjustment) || 'N/A'}</p></div>
 <div><p className="text-xs text-muted-foreground mb-1">Índice de Reajuste</p><p className="text-sm font-bold text-foreground">{selectedContract.adjust_index || selectedContract.index || 'IGPM'}</p></div>
 </div>
 </div>
 <div className="bg-card p-6 rounded-2xl border border-border shadow-sm">
 <h3 className="text-base font-bold text-foreground mb-6 flex items-center gap-2 pb-4 border-b border-border"><User className="w-5 h-5 text-primary-600" /> Inquilino</h3>
 <div className="space-y-4">
 <div><p className="text-xs text-muted-foreground mb-1">Nome</p><p className="text-sm font-bold text-foreground">{tenantLead?.name || selectedContract.client || 'Não informado'}</p></div>
 <div className="flex items-center gap-2 text-sm text-muted-foreground"><Mail className="w-4 h-4 text-muted-foreground" /> {tenantLead?.email || 'Não informado'}</div>
 <div className="flex items-center gap-2 text-sm text-muted-foreground"><Phone className="w-4 h-4 text-muted-foreground" /> {tenantLead?.phone || 'Não informado'}</div>
 <div className="flex items-center gap-2 text-sm text-muted-foreground">
   <FileText className="w-4 h-4 text-muted-foreground" />
   {tenantLead ? (
     <Link to={`/leads/${tenantLead.id}`} className="hover:text-primary-700 hover:underline">Ver lead do inquilino</Link>
   ) : (
     <span>Lead não vinculado</span>
   )}
 </div>
 </div>
 </div>
 <div className="bg-card p-6 rounded-2xl border border-border shadow-sm">
 <h3 className="text-base font-bold text-foreground mb-6 flex items-center gap-2 pb-4 border-b border-border"><Crown className="w-5 h-5 text-primary-600" /> Proprietário</h3>
 <div className="space-y-4">
 <div><p className="text-xs text-muted-foreground mb-1">Nome</p><p className="text-sm font-bold text-foreground">{ownerLead?.name || 'Não informado'}</p></div>
 <div className="flex items-center gap-2 text-sm text-muted-foreground"><Mail className="w-4 h-4 text-muted-foreground" /> {ownerLead?.email || 'Não informado'}</div>
 <div className="flex items-center gap-2 text-sm text-muted-foreground"><Phone className="w-4 h-4 text-muted-foreground" /> {ownerLead?.phone || 'Não informado'}</div>
 <div className="flex items-center gap-2 text-sm text-muted-foreground">
   <CreditCard className="w-4 h-4 text-muted-foreground" />
   {ownerLead ? (
     <Link to={`/leads/${ownerLead.id}`} className="hover:text-primary-700 hover:underline">Ver lead do proprietário</Link>
   ) : (
     <span>Lead não vinculado</span>
   )}
 </div>
 </div>
 </div>
 {/* Garantia */}
 <div className="md:col-span-2 bg-card p-6 rounded-2xl border border-border shadow-sm">
 <h3 className="text-base font-bold text-foreground mb-6 flex items-center gap-2 pb-4 border-b border-border"><ShieldCheck className="w-5 h-5 text-primary-600" /> Garantia Locatícia</h3>
 <div className="grid grid-cols-1 sm:grid-cols-3 gap-6">
 <div><p className="text-xs text-muted-foreground mb-1">Tipo de Garantia</p><p className="text-sm font-bold text-foreground">{selectedContract.guarantee_type || 'Não informada'}</p></div>
 <div><p className="text-xs text-muted-foreground mb-1">Valor Depositado</p><p className="text-sm font-bold text-foreground">{selectedContract.guarantee_value ? formatCurrency(selectedContract.guarantee_value) : 'N/A'}</p></div>
 <div><p className="text-xs text-muted-foreground mb-1">Status</p>
 {selectedContract.guarantee_type ? (
 <span className="px-2.5 py-1 bg-status-ok-soft text-status-ok text-[11px] font-bold uppercase tracking-wider rounded-md border border-status-ok/20 inline-flex items-center gap-1"><CheckCircle2 className="w-3 h-3" /> Ativa</span>
 ) : (
 <span className="px-2.5 py-1 bg-muted text-muted-foreground text-[11px] font-bold uppercase tracking-wider rounded-md border border-border inline-flex items-center gap-1"><AlertCircle className="w-3 h-3" /> Pendente</span>
 )}
 </div>
 </div>
 </div>
 </div>
 </motion.div>
 );

 // ====== TAB: FINANCEIRO ======
 const renderTabFinanceiro = () => (
 <motion.div initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.25 }} className="space-y-6">
 {/* KPI Cards */}
 <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
 <div className="bg-card rounded-2xl border border-border p-5 shadow-sm">
 <div className="flex items-center gap-3 mb-2">
 <div className="p-2 rounded-lg bg-primary-50 text-primary-600"><DollarSign className="w-5 h-5" /></div>
 <h3 className="text-sm font-medium text-muted-foreground">Aluguel Vigente</h3>
 </div>
 <p className="text-2xl font-bold text-foreground">{formatCurrency(selectedContract.value)}</p>
 </div>
 <div className="bg-card rounded-2xl border border-border p-5 shadow-sm">
 <div className="flex items-center gap-3 mb-2">
 <div className="p-2 rounded-lg bg-status-ok-soft text-status-ok"><CheckCircle2 className="w-5 h-5" /></div>
 <h3 className="text-sm font-medium text-muted-foreground">Cobranças recebidas</h3>
 </div>
 <p className="text-2xl font-bold text-foreground">{formatCurrency(chargeStats.paid)}</p>
 </div>
 <div className="bg-card rounded-2xl border border-border p-5 shadow-sm">
 <div className="flex items-center gap-3 mb-2">
 <div className="p-2 rounded-lg bg-status-critical-soft text-status-critical"><AlertCircle className="w-5 h-5" /></div>
 <h3 className="text-sm font-medium text-muted-foreground">Em Atraso</h3>
 </div>
 <p className="text-2xl font-bold text-foreground">{formatCurrency(chargeStats.overdue)}</p>
 </div>
 <div className="bg-card rounded-2xl border border-border p-5 shadow-sm">
 <div className="flex items-center gap-3 mb-2">
 <div className="p-2 rounded-lg bg-stage-contract/10 text-stage-contract"><Percent className="w-5 h-5" /></div>
 <h3 className="text-sm font-medium text-muted-foreground">Receita de administração ({adminFeePct.toLocaleString('pt-BR')}%)</h3>
 </div>
 <p className="text-2xl font-bold text-foreground">{formatCurrency((selectedContract.value || 0) * (adminFeePct / 100))}</p>
 <p className="mt-1 text-caption text-muted-foreground">Receita da imobiliária, não aluguel bruto.</p>
 </div>
 </div>

 {/* Chart */}
 <div className="bg-card rounded-2xl border border-border p-6 shadow-sm">
 <div className="flex items-center justify-between mb-6">
 <h3 className="text-base font-bold text-foreground flex items-center gap-2"><BarChart3 className="w-5 h-5 text-primary-600" /> Fluxo Financeiro - Últimos 6 meses</h3>
 </div>
 <RC height={280}>
 <BarChart data={financialHistory} barGap={4}>
 <CartesianGrid strokeDasharray="3 3" stroke="#e2e8f0" />
 <XAxis dataKey="month" tick={{ fontSize: 12, fill: '#94a3b8' }} />
 <YAxis tick={{ fontSize: 12, fill: '#94a3b8' }} tickFormatter={(v) => `${(v / 1000).toFixed(0)}k`} />
 <Tooltip formatter={(value) => formatCurrency(Number(value))} contentStyle={{ borderRadius: 12, border: '1px solid #e2e8f0', fontSize: 13 }} />
 <Legend wrapperStyle={{ fontSize: 12 }} />
 <Bar dataKey="receita" name="Recebimentos brutos" fill="#6366f1" radius={[4, 4, 0, 0]} />
 <Bar dataKey="despesa" name="Taxas e deduções" fill="#f43f5e" radius={[4, 4, 0, 0]} />
 <Bar dataKey="repasse" name="Repasse" fill="#10b981" radius={[4, 4, 0, 0]} />
 </BarChart>
 </RC>
 </div>

 {/* Recent Transactions */}
 <div className="bg-card rounded-2xl border border-border shadow-sm overflow-hidden">
 <div className="p-6 border-b border-border bg-muted/50">
 <h3 className="text-base font-bold text-foreground flex items-center gap-2"><Wallet className="w-5 h-5 text-primary-600" /> Resumo de Pagamentos Recentes</h3>
 </div>
 <div className="divide-y divide-border">
 {charges.filter(c => c.status === 'Pago').slice(0, 5).map(charge => (
 <div key={charge.id} className="flex items-center justify-between px-6 py-4 hover:bg-muted/50 transition-colors">
 <div className="flex items-center gap-4">
 <div className="w-9 h-9 rounded-lg bg-status-ok-soft text-status-ok flex items-center justify-center"><CheckCircle2 className="w-4 h-4" /></div>
 <div>
 <p className="text-sm font-medium text-foreground">{charge.description}</p>
 <p className="text-xs text-muted-foreground">Pago em {charge.paymentDate} via {charge.method}</p>
 </div>
 </div>
 <span className="text-sm font-bold text-foreground">{formatCurrency(charge.value)}</span>
 </div>
 ))}
 </div>
 </div>
 </motion.div>
 );

 // ====== TAB: CONTAS DE CONSUMO ======
 const renderTabContas = () => (
 <motion.div initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.25 }} className="space-y-6">
 {/* Header */}
 <div className="flex items-center justify-between">
 <div>
 <h3 className="text-base font-bold text-foreground">Contas de Consumo Vinculadas</h3>
 <p className="text-sm text-muted-foreground mt-1">Gerencie todas as contas de consumo associadas a este contrato</p>
 </div>
 <Button
   onClick={() => setActiveModal('vincularConta')}
 ><Plus className="w-4 h-4" /> Vincular Conta</Button>
 </div>

 {/* Summary Cards */}
 <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
 <div className="bg-card rounded-xl border border-border p-4 shadow-sm">
 <p className="text-xs text-muted-foreground mb-1">Total Contas Inquilino</p>
 <p className="text-xl font-bold text-foreground">{formatCurrency(utilityStats.tenantTotalCents / 100)}</p>
 <p className="text-caption text-muted-foreground mt-1">{utilityStats.tenantCount} contas vinculadas</p>
 </div>
 <div className="bg-card rounded-xl border border-border p-4 shadow-sm">
 <p className="text-xs text-muted-foreground mb-1">Total Contas Proprietário</p>
 <p className="text-xl font-bold text-foreground">{formatCurrency(utilityStats.ownerTotalCents / 100)}</p>
 <p className="text-caption text-muted-foreground mt-1">{utilityStats.ownerCount} contas vinculadas</p>
 </div>
 <div className="bg-card rounded-xl border border-border p-4 shadow-sm">
 <p className="text-xs text-muted-foreground mb-1">Inativas</p>
 <p className="text-xl font-bold text-status-critical">{utilityStats.inactiveCount}</p>
 <p className="text-caption text-muted-foreground mt-1">contas marcadas como inativas</p>
 </div>
 </div>

 {/* Account Cards */}
 {utilityAccounts.length === 0 ? (
 <EmptyState
 icon={Droplet}
 title="Nenhuma conta vinculada"
 message="Vincule contas de água, energia, gás, condomínio, IPTU ou outras para acompanhar consumo e responsabilidade de pagamento."
 />
 ) : (
 <div className="space-y-3">
 {utilityAccounts.map((account) => (
 <div key={account.id} className="bg-card rounded-xl border border-border p-5 shadow-sm hover:border-primary-200 transition-colors">
 <div className="flex items-start justify-between gap-4">
 <div className="flex items-start gap-4">
 <div className={clsx(
 "w-11 h-11 rounded-xl flex items-center justify-center shrink-0",
 account.status === 'INACTIVE' ? "bg-muted text-muted-foreground" : "bg-primary-50 text-primary-600"
 )}>
 {utilityTypeIcon(account.type)}
 </div>
 <div>
 <div className="flex items-center gap-2 mb-1">
 <h4 className="text-sm font-bold text-foreground">{utilityTypeLabel(account.type)}</h4>
 <span className={clsx(
 "text-caption font-bold uppercase tracking-wider px-2 py-0.5 rounded-md border",
 account.status === 'ACTIVE' ? "bg-status-ok-soft text-status-ok border-status-ok/20" : "bg-muted text-muted-foreground border-border"
 )}>{account.status === 'ACTIVE' ? 'Ativa' : 'Inativa'}</span>
 {account.auto_pay_enabled && (
 <span className="text-caption font-bold uppercase tracking-wider px-2 py-0.5 rounded-md border bg-status-info-soft text-status-info border-status-info/20">Forma recorrente conferida</span>
 )}
 <span className={clsx(
 "text-caption font-bold uppercase tracking-wider px-2 py-0.5 rounded-md border",
 account.billing_mode === 'VARIABLE_SEPARATE' ? "bg-status-warning-soft text-status-warning border-status-warning/20" :
 account.billing_mode === 'BUNDLED' ? "bg-primary-50 text-primary-700 border-primary-100" :
 "bg-muted text-muted-foreground border-border",
 )}>
 {account.billing_mode === 'VARIABLE_SEPARATE' ? 'Variável por mês' : account.billing_mode === 'BUNDLED' ? 'Inclusa no aluguel' : 'Somente acompanhamento'}
 </span>
 </div>
 <p className="text-xs text-muted-foreground">{account.provider}{account.account_number ? ` — Conta: ${account.account_number}` : ''}</p>
 <div className="flex flex-wrap items-center gap-x-4 gap-y-1 mt-2 text-xs text-muted-foreground">
 <span>Responsável: <span className="font-medium text-foreground">{account.responsible_party === 'TENANT' ? 'Inquilino' : account.responsible_party === 'OWNER' ? 'Proprietário' : 'Imobiliária'}</span></span>
 {account.payment_method && (
 <span>Pagamento: <span className="font-medium text-foreground">{account.payment_method}</span></span>
 )}
 {account.last_payment_date && (
 <span>Último Pgto: <span className="font-medium text-foreground">{formatISODateBR(account.last_payment_date)}</span></span>
 )}
 </div>
 </div>
 </div>
 <div className="text-right shrink-0">
 <p className="text-lg font-bold text-foreground">{formatCurrency((account.estimated_amount_cents || 0) / 100)}</p>
 {account.next_due_date && (
 <p className="text-caption text-muted-foreground mt-1">Próx. Venc: {formatISODateBR(account.next_due_date)}</p>
 )}
 {account.due_day ? (
 <p className="text-caption text-muted-foreground mt-1">Dia {account.due_day} de cada mês</p>
 ) : null}
 <div className="flex items-center gap-1.5 mt-2 justify-end">
 {account.billing_mode === 'VARIABLE_SEPARATE' && (
 <Button
 variant="outline"
 size="sm"
 onClick={() => onLaunchUtilityCharge(account)}
 ><Receipt className="size-3.5" /> Lançar mês</Button>
 )}
 <Button
 variant="ghost"
 size="icon"
 onClick={() => handleRemoveUtilityAccount(account)}
 disabled={deleteUtilityAccountMutation.isPending}
 className="size-8 text-muted-foreground hover:text-status-critical hover:bg-status-critical-soft"
 title="Remover conta"
 ><Trash2 className="w-3.5 h-3.5" /></Button>
 </div>
 </div>
 </div>
 </div>
 ))}
 </div>
 )}
 </motion.div>
 );

 // ====== TAB: COBRANCAS ======
 const renderTabCobrancas = () => (
 <motion.div initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.25 }} className="space-y-6">
 {/* Header Actions */}
 <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
 <div>
 <h3 className="text-base font-bold text-foreground">Histórico de Cobranças</h3>
 <p className="text-sm text-muted-foreground mt-1">Gerencie todas as cobranças emitidas para este contrato</p>
 </div>
 <div className="flex items-center gap-3">
 {perms.chargesWrite && (
 <>
 <Button
 variant="outline"
 onClick={() => {
   const overdue = charges.filter((c) => c.status === 'Em Atraso' && c._source === 'charge');
   if (overdue.length === 0) {
     toast.message("Nenhuma cobrança em atraso elegível para lembrete.");
     return;
   }
   const remaining = new Set(overdue.map(charge => charge.id));
   confirmAction(`Disparar lembrete para ${overdue.length} cobrança(s) em atraso?`, async () => {
     const results = await Promise.allSettled([...remaining].map(async id => {
       await handleSendChargeReminder(id, { skipConfirm: true });
       remaining.delete(id);
     }));
     if (results.some(result => result.status === "rejected")) throw new Error("Alguns lembretes falharam. Tente novamente para reenviar apenas os pendentes.");
   }, { title: "Enviar lembretes", confirmLabel: "Enviar" });
 }}
 ><Send className="w-4 h-4" /> Enviar Lembrete</Button>
 <Button onClick={() => setActiveModal('novaCobranca')}><Plus className="w-4 h-4" /> Nova Cobrança</Button>
 </>
 )}
 </div>
 </div>

 {charges.some((charge) => charge._includedInTotals === false) && (
 <div className="flex items-start gap-2 rounded-xl border border-status-info/30 bg-status-info-soft/50 px-4 py-3 text-sm text-muted-foreground" role="status">
 <AlertCircle className="mt-0.5 size-4 shrink-0 text-status-info" />
 <p>Documentos sem vínculo canônico aparecem como <strong className="text-foreground">A apurar</strong> e ficam fora dos totais até a composição ser conferida.</p>
 </div>
 )}

 {/* Stats Bar */}
 <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
 <div className="rounded-xl p-3 border bg-muted border-border">
 <p className="text-caption font-medium text-muted-foreground">Total Cobrado</p>
 <p className="text-lg font-bold text-foreground">{formatCurrency(chargeStats.total)}</p>
 </div>
 <div className="rounded-xl p-3 border bg-status-ok-soft border-status-ok/20">
 <p className="text-caption font-medium text-status-ok">Recebido</p>
 <p className="text-lg font-bold text-status-ok">{formatCurrency(chargeStats.paid)}</p>
 </div>
 <div className="rounded-xl p-3 border bg-status-critical-soft border-status-critical/20">
 <p className="text-caption font-medium text-status-critical">Em Atraso</p>
 <p className="text-lg font-bold text-status-critical">{formatCurrency(chargeStats.overdue)}</p>
 </div>
 <div className="rounded-xl p-3 border bg-status-info-soft border-status-info/20">
 <p className="text-caption font-medium text-status-info">A Vencer</p>
 <p className="text-lg font-bold text-status-info">{formatCurrency(chargeStats.pending)}</p>
 </div>
 </div>

 {/* Filter Pills + Search + Period + Sort */}
 <div className="flex flex-wrap items-center gap-2">
 {['Todos', 'Pago', 'Parcial', 'A Vencer', 'Em Atraso', 'Encerrada', 'A apurar'].map(filter => (
 <button key={filter} onClick={() => setChargeFilter(filter)} className={clsx(
 "px-3 py-1.5 text-xs font-bold rounded-lg transition-all border",
 chargeFilter === filter ? "bg-primary text-primary-foreground border-primary" : "bg-card text-muted-foreground border-border hover:bg-muted"
 )}>
 {filter} {filter !== 'Todos' && `(${charges.filter(c => c.filterStatus === filter).length})`}
 </button>
 ))}
 <div className="flex-1" />
 <div className="relative">
 <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-muted-foreground" />
 <input
 type="text"
 value={chargeSearch}
 onChange={(e) => setChargeSearch(e.target.value)}
 placeholder="Buscar descrição..."
 className="w-48 pl-8 pr-3 py-1.5 bg-card border border-border rounded-lg text-xs focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none transition-all"
 />
 </div>
 <select
 value={chargePeriod}
 onChange={(e) => setChargePeriod(e.target.value as any)}
 className="bg-card border border-border text-foreground text-xs font-medium rounded-lg px-2.5 py-1.5 outline-none"
 title="Período"
 >
 <option value="all">Todos os períodos</option>
 <option value="this_month">Este mês</option>
 <option value="last_3">Últimos 3 meses</option>
 <option value="last_6">Últimos 6 meses</option>
 </select>
 <select
 value={chargeSortBy}
 onChange={(e) => setChargeSortBy(e.target.value as any)}
 className="bg-card border border-border text-foreground text-xs font-medium rounded-lg px-2.5 py-1.5 outline-none"
 title="Ordenar por"
 >
 <option value="date_desc">Vencimento (mais novo)</option>
 <option value="date_asc">Vencimento (mais antigo)</option>
 <option value="value_desc">Valor (maior)</option>
 <option value="value_asc">Valor (menor)</option>
 </select>
 <div className="text-[11px] text-muted-foreground font-medium">
 {filteredCharges.length} de {charges.length}
 </div>
 </div>

 {/* Bulk Action Bar — appears only when selection is non-empty.
     Sits above the table so the operator sees the action affordances
     near the checkboxes that drive them. */}
 {selectedChargeIds.size > 0 && (
 <div className="flex items-center justify-between gap-3 px-4 py-3 bg-primary-50 border border-primary-200 rounded-xl">
 <div className="flex items-center gap-2 text-xs font-bold text-primary-700">
 <CheckCircle2 className="w-4 h-4" />
 {selectedChargeIds.size} cobrança(s) selecionada(s)
 </div>
 <div className="flex items-center gap-2">
 {perms.financeWrite && (
 <Button
 variant="outline"
 size="sm"
 onClick={handleBulkMarkPaid}
 disabled={selectedChargeIds.size !== 1}
 title={selectedChargeIds.size !== 1 ? "A baixa exige conferir uma cobrança por vez" : "Registrar pagamento da cobrança selecionada"}
 className="text-status-ok hover:text-status-ok bg-status-ok-soft hover:bg-status-ok/20 border-status-ok/30"
 ><CheckCircle2 className="w-3.5 h-3.5" /> Baixar selecionada</Button>
 )}
 {perms.chargesWrite && (
 <Button
 variant="outline"
 size="sm"
 onClick={handleBulkNotify}
 ><Send className="w-3.5 h-3.5" /> Enviar Lembrete</Button>
 )}
 <Button
 variant="ghost"
 size="sm"
 onClick={clearChargeSelection}
 className="text-muted-foreground hover:text-foreground"
 >Limpar</Button>
 </div>
 </div>
 )}

 {/* Charges Table */}
 <div className="bg-card rounded-2xl border border-border shadow-sm overflow-hidden">
 <div className="overflow-x-auto">
 <table className="w-full text-left text-sm whitespace-nowrap">
 <thead className="bg-muted text-muted-foreground border-b border-border">
 <tr>
 <th className="px-3 py-3.5 font-medium w-10">
 <input
 type="checkbox"
 checked={allVisibleSelected}
 onChange={toggleSelectAllVisible}
 disabled={visibleSelectableIds.length === 0}
 aria-label="Selecionar todas visíveis"
 className="rounded border-border focus:ring-2 focus:ring-primary-600/20 disabled:opacity-40"
 />
 </th>
 <th className="px-6 py-3.5 font-medium">ID</th>
 <th className="px-6 py-3.5 font-medium">Descrição</th>
 <th className="px-6 py-3.5 font-medium">Tipo</th>
 <th className="px-6 py-3.5 font-medium">Vencimento</th>
 <th className="px-6 py-3.5 font-medium">Valor</th>
 <th className="px-6 py-3.5 font-medium">Status</th>
 <th className="px-6 py-3.5 font-medium text-right">Ações</th>
 </tr>
 </thead>
 <tbody className="divide-y divide-border">
 {filteredCharges.map(charge => (
 <tr key={charge.id} className={clsx("hover:bg-muted/50 transition-colors", selectedChargeIds.has(charge.id) && "bg-primary-50/40")}>
 <td className="px-3 py-4">
 <input
 type="checkbox"
 checked={selectedChargeIds.has(charge.id)}
 onChange={() => toggleChargeSelected(charge.id)}
 disabled={
   charge._source !== 'charge' ||
   (!charge._canMarkPaid && !charge._canNotify) ||
   (!perms.financeWrite && !perms.chargesWrite)
 }
 aria-label={`Selecionar cobrança ${charge.id}`}
 className="rounded border-border focus:ring-2 focus:ring-primary-600/20 disabled:opacity-40"
 />
 </td>
 <td className="px-6 py-4 font-mono text-xs text-muted-foreground">{charge.id}</td>
 <td className="px-6 py-4 font-medium text-foreground">
 <div className="flex items-center gap-2">
 <span>{charge.description}</span>
 {charge.remindersCount > 0 && (
 <button
 onClick={() => setAuditChargeId(charge.id)}
 title="Ver histórico de lembretes"
 className="text-caption font-bold uppercase tracking-wider px-2 py-0.5 rounded-md border bg-status-info-soft text-status-info border-status-info/20 inline-flex items-center gap-1 hover:bg-status-info/20 transition-colors"
 >
 <Bell className="w-3 h-3" />
 {charge.remindersCount}
 </button>
 )}
 </div>
 </td>
 <td className="px-6 py-4"><span className="text-xs font-medium text-muted-foreground bg-muted px-2 py-0.5 rounded">{charge.type}</span></td>
 <td className="px-6 py-4 text-muted-foreground">{formatISODateBR(charge.dueDate) || charge.dueDate}</td>
 <td className="px-6 py-4 font-bold text-foreground">{formatCurrency(charge.value)}</td>
 <td className="px-6 py-4">
 <span className={clsx(
 "inline-flex items-center gap-1.5 py-1 px-2.5 rounded-full text-[11px] font-bold border uppercase tracking-wider",
 chargeStatusTone(charge.status)
 )}>
 {charge.status === 'Pago' && <CheckCircle2 className="w-3 h-3" />}
 {(charge.status === 'A Vencer' || charge.status === 'Parcial') && <Clock className="w-3 h-3" />}
 {charge.status === 'Em Atraso' && <AlertCircle className="w-3 h-3" />}
 {charge.status}
 </span>
 </td>
 <td className="px-6 py-4 text-right">
 <div className="flex items-center justify-end gap-1.5">
 {charge._source === 'charge' && (charge.status === 'Pago' || charge.status === 'Parcial') && (
 <Button variant="link" size="sm" className="h-auto p-0 text-xs" onClick={() => handleViewChargeReceipt(charge.id)}>Ver recibo</Button>
 )}
 {perms.financeWrite && charge._canMarkPaid && (
 <Button variant="ghost" size="sm" onClick={() => handleMarkAsPaid(charge.id)} className="text-status-ok hover:text-status-ok hover:bg-status-ok-soft"><CheckCircle2 className="w-3 h-3" /> Dar baixa</Button>
 )}
 {perms.chargesWrite && charge._canNotify && (
 <Button variant="ghost" size="sm" onClick={() => handleSendChargeReminder(charge.id)} className="text-muted-foreground hover:text-foreground"><Send className="w-3 h-3" /> Notificar</Button>
 )}
 {charge._source !== 'charge' && (
 <Link to="/financeiro/lancamentos" className="text-xs font-bold text-primary-600 hover:text-primary-800">Revisar vínculo</Link>
 )}
 </div>
 </td>
 </tr>
 ))}
 </tbody>
 </table>
 </div>
 {filteredCharges.length === 0 && (
 <EmptyState
 icon={Receipt}
 title={hasActiveChargeFilter ? "Nenhum resultado" : "Nenhuma cobrança ainda"}
 message={
 hasActiveChargeFilter
 ? "Nenhuma cobrança bate com os filtros aplicados."
 : "As cobranças aparecerão aqui quando forem geradas pela régua ou criadas manualmente."
 }
 action={
 hasActiveChargeFilter ? (
 <Button
 variant="outline"
 size="sm"
 onClick={clearChargeFilters}
 >
 Limpar filtros
 </Button>
 ) : undefined
 }
 className="border-none bg-transparent my-6"
 />
 )}
 </div>
 </motion.div>
 );

 // ====== TAB: REPASSES ======
 const renderTabRepasses = () => {
 const nextRepasse = repasses.find(r => r.action === 'process') ?? repasses.find(r => r.action === 'approve') ?? repasses.find(r => r.action === 'settle');
 return (
 <motion.div initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.25 }} className="space-y-6">
 {/* Header */}
 <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
 <div>
 <h3 className="text-base font-bold text-foreground">Repasses ao Proprietário</h3>
 <p className="text-sm text-muted-foreground mt-1">O repasse normal nasce após o recebimento e considera administração, contas e demais deduções.</p>
 </div>
 <div className="flex items-center gap-3">
 <Button variant="outline" onClick={() => setDetailTab('cobrancas')}><Receipt className="w-4 h-4" /> Ver recebimentos</Button>
 </div>
 </div>

 {repasses.some((repasse) => repasse.status === 'A apurar') && (
 <div className="flex items-start gap-2 rounded-xl border border-status-info/30 bg-status-info-soft/50 px-4 py-3 text-sm text-muted-foreground" role="status">
 <AlertCircle className="mt-0.5 size-4 shrink-0 text-status-info" />
 <p>Repasses sem origem e saldo econômico compatíveis ficam <strong className="text-foreground">A apurar</strong>. Eles não podem ser aprovados ou processados por atalho.</p>
 </div>
 )}

 {/* Next Repasse Card */}
 {nextRepasse && (
 <div className="bg-linear-to-r from-primary-600 to-primary-700 rounded-2xl p-6 text-white shadow-lg">
 <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
 <div>
 <p className="text-primary-200 text-xs font-medium mb-1">Próximo passo - {nextRepasse.month}</p>
 <p className="text-3xl font-bold">{formatCurrency(nextRepasse.netRepasse)}</p>
 <div className="flex flex-wrap gap-4 mt-3 text-xs text-primary-200">
 <span>Aluguel Bruto: {formatCurrency(nextRepasse.grossRent)}</span>
 <span>Taxa Admin: -{formatCurrency(nextRepasse.adminFee)}</span>
 <span>Deduções: -{formatCurrency(nextRepasse.utilityDeductions + nextRepasse.maintenanceDeductions)}</span>
 </div>
 </div>
 <div className="flex items-center gap-2">
 <Button
 variant="ghost"
 onClick={() => handleDownloadRepasseReceipt(nextRepasse.id)}
 className="bg-primary-700/40 text-white hover:bg-primary-700/60 hover:text-white"
 ><Download className="w-4 h-4" /> Baixar PDF</Button>
 {perms.financeApproveTransfer && nextRepasse.action === 'approve' && (
 <Button variant="secondary" onClick={() => handleApproveRepasse(nextRepasse.id)} disabled={approveTransferMutation.isPending}>
 <CheckCircle2 className="w-5 h-5" /> Aprovar repasse
 </Button>
 )}
 {perms.financeApproveTransfer && nextRepasse.action === 'process' && (
 <Button variant="secondary" onClick={() => handleProcessRepasse(nextRepasse.id)} disabled={processTransferMutation.isPending}>
 <Banknote className="w-5 h-5" /> Processar com comprovante
 </Button>
 )}
 {perms.financeApproveTransfer && nextRepasse.action === 'settle' && (
 <Button variant="secondary" onClick={() => handleSettleRepasseWithoutBalance(nextRepasse.id)} disabled={processTransferMutation.isPending}>
 <CheckCircle2 className="w-5 h-5" /> Concluir sem saldo
 </Button>
 )}
 </div>
 </div>
 </div>
 )}

 {/* Repasse Breakdown (Demonstrativo) */}
 <div className="bg-card rounded-2xl border border-border p-6 shadow-sm">
 <h4 className="text-sm font-bold text-foreground mb-4 flex items-center gap-2"><Receipt className="w-4 h-4 text-primary-600" /> Demonstrativo de Repasse - {nextRepasse?.month || 'Mensal'}</h4>
 <div className="space-y-3">
 <div className="flex justify-between items-center py-2"><span className="text-sm text-muted-foreground">Aluguel Bruto</span><span className="text-sm font-bold text-foreground">{formatCurrency(nextRepasse?.grossRent || selectedContract?.value || 0)}</span></div>
 <div className="flex justify-between items-center py-2 text-status-critical"><span className="text-sm">(-) Receita de administração ({adminFeePct.toLocaleString('pt-BR')}%)</span><span className="text-sm font-bold">-{formatCurrency(nextRepasse?.adminFee || ((selectedContract?.value || 0) * adminFeePct / 100))}</span></div>
 <div className="flex justify-between items-center py-2 text-status-critical"><span className="text-sm">(-) Deduções de contas</span><span className="text-sm font-bold">-{formatCurrency(nextRepasse?.utilityDeductions || 0)}</span></div>
 <div className="flex justify-between items-center py-2 text-status-critical"><span className="text-sm">(-) Manutenções pendentes</span><span className="text-sm font-bold">-{formatCurrency(nextRepasse?.maintenanceDeductions || 0)}</span></div>
 <div className="border-t-2 border-border pt-3 flex justify-between items-center">
 <span className="text-base font-bold text-foreground">Valor Líquido do Repasse</span>
 <span className="text-xl font-bold text-status-ok">{formatCurrency(nextRepasse?.netRepasse || Math.max((selectedContract?.value || 0) * (1 - adminFeePct / 100), 0))}</span>
 </div>
 </div>
 </div>

 {/* History Table */}
 <div className="bg-card rounded-2xl border border-border shadow-sm overflow-hidden">
 <div className="p-4 border-b border-border bg-muted/50 space-y-3">
 <div className="flex items-center justify-between gap-3 flex-wrap">
 <h4 className="text-sm font-bold text-foreground flex items-center gap-2"><Wallet className="w-4 h-4 text-primary-600" /> Histórico de Repasses</h4>
 <div className="text-[11px] text-muted-foreground font-medium">{filteredRepasses.length} de {repasses.length}</div>
 </div>
 <div className="flex flex-wrap items-center gap-2">
 {(['Todos', 'Pendente', 'Aprovado', 'Processado', 'Sem saldo', 'Liquidado sem saldo', 'Cancelado', 'A apurar'] as const).map((opt) => (
 <button
 key={opt}
 onClick={() => setRepasseStatusFilter(opt)}
 className={clsx(
 "px-3 py-1.5 text-xs font-bold rounded-lg transition-all border",
 repasseStatusFilter === opt ? "bg-primary text-primary-foreground border-primary" : "bg-card text-muted-foreground border-border hover:bg-muted"
 )}
 >
 {opt} {opt !== 'Todos' && `(${repasses.filter(r => r.status === opt).length})`}
 </button>
 ))}
 <div className="flex-1" />
 <div className="relative">
 <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-muted-foreground" />
 <input
 type="text"
 value={repasseSearch}
 onChange={(e) => setRepasseSearch(e.target.value)}
 placeholder="Buscar competência..."
 className="w-48 pl-8 pr-3 py-1.5 bg-card border border-border rounded-lg text-xs focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none transition-all"
 />
 </div>
 <select
 value={repassePeriod}
 onChange={(e) => setRepassePeriod(e.target.value as any)}
 className="bg-card border border-border text-foreground text-xs font-medium rounded-lg px-2.5 py-1.5 outline-none"
 title="Período"
 >
 <option value="all">Todas as competências</option>
 <option value="last_3">Últimos 3 meses</option>
 <option value="last_6">Últimos 6 meses</option>
 <option value="last_12">Últimos 12 meses</option>
 </select>
 <select
 value={repasseSortBy}
 onChange={(e) => setRepasseSortBy(e.target.value as any)}
 className="bg-card border border-border text-foreground text-xs font-medium rounded-lg px-2.5 py-1.5 outline-none"
 title="Ordenar por"
 >
 <option value="month_desc">Competência (mais recente)</option>
 <option value="month_asc">Competência (mais antiga)</option>
 <option value="value_desc">Valor líquido (maior)</option>
 <option value="value_asc">Valor líquido (menor)</option>
 </select>
 </div>
 </div>

 {/* Bulk action bar for repasses */}
 {perms.financeApproveTransfer && selectedTransferIds.size > 0 && (
 <div className="flex items-center justify-between gap-3 px-4 py-3 bg-primary-50 border-y border-primary-200">
 <div className="flex items-center gap-2 text-xs font-bold text-primary-700">
 <Banknote className="w-4 h-4" />
 {selectedTransferIds.size} repasse(s) selecionado(s)
 </div>
 <div className="flex items-center gap-2">
 <Button
 variant="outline"
 size="sm"
 onClick={handleBulkApproveRepasses}
 disabled={approveTransferMutation.isPending}
 className="text-status-ok hover:text-status-ok bg-status-ok-soft hover:bg-status-ok/20 border-status-ok/30"
 ><CheckCircle2 className="w-3.5 h-3.5" /> Aprovar selecionados</Button>
 <Button
 variant="ghost"
 size="sm"
 onClick={clearTransferSelection}
 className="text-muted-foreground hover:text-foreground"
 >Limpar</Button>
 </div>
 </div>
 )}

 <div className="overflow-x-auto">
 <table className="w-full text-left text-sm whitespace-nowrap">
 <thead className="bg-card text-muted-foreground border-b border-border">
 <tr>
 <th className="px-3 py-3.5 font-medium w-10">
 <input
 type="checkbox"
 checked={allVisibleTransfersSelected}
 onChange={toggleSelectAllTransfers}
 disabled={selectableTransferIds.length === 0 || !perms.financeApproveTransfer}
 aria-label="Selecionar todos os repasses prontos para aprovação"
 className="rounded border-border focus:ring-2 focus:ring-primary-600/20 disabled:opacity-40"
 />
 </th>
 <th className="px-6 py-3.5 font-medium">Competência</th>
 <th className="px-6 py-3.5 font-medium">Aluguel Bruto</th>
 <th className="px-6 py-3.5 font-medium">Taxa Admin.</th>
 <th className="px-6 py-3.5 font-medium">Deduções</th>
 <th className="px-6 py-3.5 font-medium">Líquido</th>
 <th className="px-6 py-3.5 font-medium">Status</th>
 <th className="px-6 py-3.5 font-medium text-right">Ações</th>
 </tr>
 </thead>
 <tbody className="divide-y divide-border">
 {filteredRepasses.map(rep => {
 const isSelectable = selectableTransferIds.includes(rep.id);
 const isApiTransfer = realTransfers.some((t) => t.id === rep.id);
 return (
 <tr key={rep.id} className={clsx("hover:bg-muted/50 transition-colors", selectedTransferIds.has(rep.id) && "bg-primary-50/40")}>
 <td className="px-3 py-4">
 <input
 type="checkbox"
 checked={selectedTransferIds.has(rep.id)}
 onChange={() => toggleTransferSelected(rep.id)}
 disabled={!isSelectable || !perms.financeApproveTransfer}
 aria-label={`Selecionar repasse ${rep.month}`}
 className="rounded border-border focus:ring-2 focus:ring-primary-600/20 disabled:opacity-40"
 />
 </td>
 <td className="px-6 py-4 font-medium text-foreground">
 <div className="flex items-center gap-2">
 <span>{rep.month}</span>
 {isApiTransfer && rep.history && rep.history.length > 1 && (
 <button
 onClick={() => setAuditTransferId(rep.id)}
 title="Ver histórico do repasse"
 className="text-caption font-bold uppercase tracking-wider px-2 py-0.5 rounded-md border bg-status-info-soft text-status-info border-status-info/20 inline-flex items-center gap-1 hover:bg-status-info/20 transition-colors"
 ><History className="w-3 h-3" />{rep.history.length}</button>
 )}
 </div>
 {(rep.approvedBy || rep.processedBy) && (
 <div className="text-caption text-muted-foreground mt-0.5 space-y-0.5">
 {rep.approvedBy && (
 <div>Aprovado por <span className="font-medium text-foreground">{rep.approvedLabel || resolveActorName(rep.approvedBy)}</span>{rep.approvedAt ? ` em ${formatISODateBR(rep.approvedAt)}` : ''}</div>
 )}
 {rep.processedBy && (
 <div>Processado por <span className="font-medium text-foreground">{rep.processedLabel || resolveActorName(rep.processedBy)}</span>{rep.processedAt ? ` em ${formatISODateBR(rep.processedAt)}` : ''}</div>
 )}
 </div>
 )}
 </td>
 <td className="px-6 py-4 text-muted-foreground">{formatCurrency(rep.grossRent)}</td>
 <td className="px-6 py-4 text-status-critical">-{formatCurrency(rep.adminFee)}</td>
 <td className="px-6 py-4 text-status-critical">-{formatCurrency(rep.utilityDeductions + rep.maintenanceDeductions)}</td>
 <td className="px-6 py-4 font-bold text-status-ok">{formatCurrency(rep.netRepasse)}</td>
 <td className="px-6 py-4">
 <span className={clsx(
 "inline-flex items-center gap-1.5 py-1 px-2.5 rounded-full text-[11px] font-bold border uppercase tracking-wider",
 repasseStatusTone(rep.status)
 )}>
 {rep.status === 'Processado' ? <CheckCircle2 className="w-3 h-3" /> : <Clock className="w-3 h-3" />}
 {rep.status}
 </span>
 </td>
 <td className="px-6 py-4 text-right">
 {rep.status === 'Processado' ? (
 <div className="flex items-center justify-end gap-1.5">
 <Button
 variant="ghost"
 size="sm"
 onClick={() => handleDownloadRepasseReceipt(rep.id)}
 className="text-primary-600 hover:text-primary-800 hover:bg-primary-50"
 ><FileText className="w-3 h-3" /> Comprovante PDF</Button>
 </div>
 ) : rep.action ? (
 <div className="flex items-center justify-end gap-1.5">
 <Button
 variant="ghost"
 size="sm"
 onClick={() => handleDownloadRepasseReceipt(rep.id)}
 className="text-muted-foreground hover:text-foreground"
 title="Demonstrativo prévio"
 ><Download className="w-3 h-3" /> Demonstrativo</Button>
 {perms.financeApproveTransfer && rep.action === 'approve' && (
 <Button variant="ghost" size="sm" onClick={() => handleApproveRepasse(rep.id)} disabled={approveTransferMutation.isPending} className="text-status-ok hover:text-status-ok hover:bg-status-ok-soft"><CheckCircle2 className="w-3 h-3" /> Aprovar</Button>
 )}
 {perms.financeApproveTransfer && rep.action === 'process' && (
 <Button variant="ghost" size="sm" onClick={() => handleProcessRepasse(rep.id)} disabled={processTransferMutation.isPending} className="text-status-ok hover:text-status-ok hover:bg-status-ok-soft"><Banknote className="w-3 h-3" /> Processar</Button>
 )}
 {perms.financeApproveTransfer && rep.action === 'settle' && (
 <Button variant="ghost" size="sm" onClick={() => handleSettleRepasseWithoutBalance(rep.id)} disabled={processTransferMutation.isPending} className="text-status-ok hover:text-status-ok hover:bg-status-ok-soft"><CheckCircle2 className="w-3 h-3" /> Concluir sem saldo</Button>
 )}
 </div>
 ) : rep.status === 'A apurar' ? (
 <Link to="/financeiro/repasses" className="text-xs font-bold text-primary-600 hover:text-primary-800">Revisar composição</Link>
 ) : (
 <span className="text-xs text-muted-foreground">Sem ações</span>
 )}
 </td>
 </tr>
 );
 })}
 </tbody>
 </table>
 {filteredRepasses.length === 0 && (
 <EmptyState
 icon={Wallet}
 title={hasActiveRepasseFilter ? "Nenhum repasse encontrado" : "Sem repasses ainda"}
 message={
 hasActiveRepasseFilter
 ? "Nenhum repasse bate com os filtros aplicados."
 : "Os repasses aparecerão aqui após serem gerados a partir das cobranças do contrato."
 }
 action={
 hasActiveRepasseFilter ? (
 <Button
 variant="outline"
 size="sm"
 onClick={clearRepasseFilters}
 >
 Limpar filtros
 </Button>
 ) : undefined
 }
 className="border-none bg-transparent my-6"
 />
 )}
 </div>
 </div>
 </motion.div>
 );
 };

 // ====== DETAIL VIEW (with tabs) ======
 return (
 <>
 {confirmationDialog}
 <div className="flex-1 overflow-y-auto bg-muted p-6 lg:p-8">
 <div className="max-w-6xl mx-auto space-y-6">
 {/* Breadcrumb */}
 <div className="flex items-center gap-2 text-sm text-muted-foreground mb-4">
 <Button variant="link" size="sm" onClick={() => setView('list')} className="h-auto p-0 gap-1"><Home className="w-3.5 h-3.5" /> Início</Button>
 <span>/</span>
 <Button variant="link" size="sm" onClick={() => setView('list')} className="h-auto p-0">Contratos Ativos</Button>
 <span>/</span>
 <span className="text-foreground font-medium">{contractLabel(selectedContract)}</span>
 </div>

 {/* Header */}
 <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4 bg-card p-6 rounded-2xl border border-border shadow-sm">
 <div>
 <Button variant="ghost" size="sm" onClick={() => setView('list')} className="mb-2 text-muted-foreground hover:text-foreground"><ArrowLeft className="w-4 h-4" /> Voltar</Button>
 <div className="flex items-center gap-3">
 <h1 className="text-2xl font-bold text-foreground">{selectedContract.display_id || "Contrato de locação"}</h1>
 <span className="px-2.5 py-1 bg-status-ok-soft text-status-ok text-[11px] font-bold uppercase tracking-wider rounded-md border border-status-ok/20 flex items-center gap-1"><CheckCircle2 className="w-3 h-3" /> {selectedContract.status === ContractStatus.ACTIVE ? 'Ativo' : 'Encerrado'}</span>
 </div>
 <p className="text-sm text-muted-foreground mt-1">{selectedContract.client} - {selectedContract.property_title}</p>
 </div>
 <div className="flex flex-wrap items-center gap-2 sm:gap-3 w-full sm:w-auto">
 <Button
 variant="outline"
 onClick={() => { setAuditContractId(selectedContract.id); setAuditLabel(`${selectedContract.id} - ${selectedContract.client}`); }}
 className="flex-1 sm:flex-none"
 ><History className="w-4 h-4" /> <span className="hidden sm:inline">Histórico</span></Button>
 {perms.contractsWrite && (
 <Button
 variant="outline"
 onClick={handleTerminateContract}
 loading={terminateContractMutation.isPending}
 loadingText="Encerrando…"
 disabled={selectedContract.status === ContractStatus.TERMINATED}
 className="flex-1 sm:flex-none border-status-critical/30 text-status-critical bg-status-critical-soft hover:bg-status-critical-soft hover:text-status-critical"
 ><X className="w-4 h-4" /> <span className="hidden sm:inline">Encerrar</span></Button>
 )}
 {perms.chargesWrite && (
 <Button onClick={() => openModal('cobrancas', selectedContract)} className="flex-1 sm:flex-none"><Settings className="w-4 h-4" /> <span className="hidden sm:inline">Editar</span></Button>
 )}
 </div>
 </div>

 <Button asChild variant="outline"><Link to={`/manutencao?contrato=${encodeURIComponent(selectedContract.id)}`}>Nova manutenção deste contrato</Link></Button>

 {/* Tabs */}
 <div className="flex bg-card border border-border rounded-xl p-1 shadow-sm overflow-x-auto">
 {DETAIL_TABS.map(tab => (
 <button
 key={tab.id}
 onClick={() => setDetailTab(tab.id)}
 className={clsx(
 "flex-1 py-2.5 text-sm font-bold rounded-lg transition-all flex items-center justify-center gap-2 min-w-[120px]",
 detailTab === tab.id
 ? "bg-primary text-primary-foreground shadow-sm"
 : "text-muted-foreground hover:text-foreground hover:bg-muted"
 )}
 >
 {tab.icon}
 <span className="hidden sm:inline">{tab.label}</span>
 </button>
 ))}
 </div>

 {/* Tab Content */}
 {detailTab === 'geral' && renderTabGeral()}
 {detailTab === 'financeiro' && renderTabFinanceiro()}
 {detailTab === 'contas' && renderTabContas()}
 {detailTab === 'cobrancas' && renderTabCobrancas()}
 {detailTab === 'repasses' && renderTabRepasses()}
 {detailTab === 'vistorias' && <RentalInspectionsPanel contract={selectedContract} />}
 {detailTab === 'documentos' && <DocumentsTab contract={selectedContract} />}
 </div>
 </div>
 </>
 );
}
