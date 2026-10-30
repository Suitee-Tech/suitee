import type { Dispatch, SetStateAction } from "react";
import { Link } from "react-router";
import {
 Calendar,
 Clock,
 DollarSign,
 Droplet,
 ExternalLink,
 Eye,
 FileText,
 Home,
 MapPin,
 Plus,
 Search,
 Settings,
 TrendingUp,
 User,
 Wallet,
} from "lucide-react";
import { clsx } from "clsx";
import { AnimatePresence, motion } from "motion/react";
import { EmptyState } from "../../ui/EmptyState";
import { Button } from "../../ui/button";
import { PageHeader } from "../../ui/page-header";
import { SignatureStatusBadge } from "../../components/signature/SignatureStatusBadge";
import type { Contract } from "../../contracts/types";
import { formatISODateBR } from "../../utils/format";
import type { ModalState } from "./presentation";

type ContractTypeFilter = "Todos" | "locacao";
type ContractStatusFilter = "Todos" | "Ativos";
type ContractSort = "Inquilino" | "Valor";

type RentalListPermissions = {
 chargesWrite: boolean;
 financeWrite: boolean;
 contractsWrite: boolean;
};

export interface RentalListViewProps {
 perms: RentalListPermissions;
 handleHeaderConfigurarPlano: () => void;
 handleHeaderRegistrarConta: () => void;
 handleHeaderGerarRepasse: () => void;
 handleHeaderNovoContrato: () => void;
 focusMode: boolean;
 activeContracts: Contract[];
 managedRentTotal: number;
 agencyAdminRevenue: number;
 upcomingAdjustments: number;
 formatCurrency: (value: number) => string;
 searchInput: string;
 setSearchInput: Dispatch<SetStateAction<string>>;
 contractTypeFilter: ContractTypeFilter;
 setContractTypeFilter: Dispatch<SetStateAction<ContractTypeFilter>>;
 statusFilter: ContractStatusFilter;
 setStatusFilter: Dispatch<SetStateAction<ContractStatusFilter>>;
 sortBy: ContractSort;
 setSortBy: Dispatch<SetStateAction<ContractSort>>;
 filteredContracts: Contract[];
 contractsLoading: boolean;
 isMobile: boolean;
 openModal: (type: ModalState, contract: Contract) => void;
 viewDetail: (contract: Contract) => void;
}

export function RentalListView({
  perms,
  handleHeaderConfigurarPlano,
  handleHeaderRegistrarConta,
 handleHeaderGerarRepasse,
 handleHeaderNovoContrato,
 focusMode,
 activeContracts,
 managedRentTotal,
 agencyAdminRevenue,
 upcomingAdjustments,
 formatCurrency,
 searchInput,
 setSearchInput,
 contractTypeFilter,
 setContractTypeFilter,
 statusFilter,
 setStatusFilter,
 sortBy,
 setSortBy,
 filteredContracts,
 contractsLoading,
 isMobile,
 openModal,
 viewDetail,
}: RentalListViewProps) {
 return (
 <>
 {/* Sticky Header */}
 <div className="flex-none px-6 lg:px-8 py-6 border-b border-border bg-card sticky top-0 z-10">
 <PageHeader
 title="Contratos de locação"
 description="Gerencie todos os contratos de locação e configurações"
 >
 <div className="flex flex-wrap items-center gap-2 sm:gap-3 w-full sm:w-auto">
 {perms.chargesWrite && (
 <Button variant="outline" onClick={handleHeaderConfigurarPlano} className="flex-1 sm:flex-none"><Settings className="w-4 h-4" /> <span className="hidden sm:inline">Plano financeiro</span></Button>
 )}
 {perms.chargesWrite && (
 <Button variant="outline" onClick={handleHeaderRegistrarConta} className="flex-1 sm:flex-none"><Droplet className="w-4 h-4" /> <span className="hidden sm:inline">Vincular conta</span></Button>
 )}
 {perms.financeWrite && (
 <Button variant="outline" onClick={handleHeaderGerarRepasse} className="flex-1 sm:flex-none"><FileText className="w-4 h-4" /> <span className="hidden sm:inline">Preparar repasse</span></Button>
 )}
 <Button onClick={handleHeaderNovoContrato} className="flex-1 sm:flex-none"><Plus className="w-4 h-4" /> <span className="hidden sm:inline">Novo Contrato</span></Button>
 </div>
 </PageHeader>
 </div>

 <div className="flex-1 overflow-y-auto p-6 lg:p-8 space-y-8">
 {/* KPIs */}
 <AnimatePresence>
 {!focusMode && (
 <motion.div
 initial={{ opacity: 0, height: 0 }}
 animate={{ opacity: 1, height: "auto" }}
 exit={{ opacity: 0, height: 0 }}
 transition={{ duration: 0.3, ease: "easeInOut" }}
 >
 <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
 <div className="bg-card p-6 rounded-2xl border border-border shadow-sm flex items-center gap-5 hover:border-primary-200 transition-colors">
 <div className="w-12 h-12 rounded-xl bg-muted flex items-center justify-center border border-border"><Calendar className="w-6 h-6 text-muted-foreground" /></div>
 <div><p className="text-sm font-medium text-muted-foreground mb-1">Contratos Ativos</p><h2 className="text-3xl font-bold text-foreground">{activeContracts.length}</h2></div>
 </div>
 <div className="bg-card p-6 rounded-2xl border border-border shadow-sm flex items-center gap-5 hover:border-primary-200 transition-colors">
 <div className="w-12 h-12 rounded-xl bg-muted flex items-center justify-center border border-border"><DollarSign className="w-6 h-6 text-muted-foreground" /></div>
 <div><p className="text-sm font-medium text-muted-foreground mb-1">Aluguel administrado</p><div className="flex items-baseline gap-2"><h2 className="text-2xl font-bold text-foreground">{formatCurrency(managedRentTotal)}</h2></div><p className="mt-1 text-caption text-muted-foreground">Valor bruto sob gestão</p></div>
 </div>
 <div className="bg-card p-6 rounded-2xl border border-border shadow-sm flex items-center gap-5 hover:border-primary-200 transition-colors">
 <div className="w-12 h-12 rounded-xl bg-primary-50 flex items-center justify-center border border-primary-100"><Wallet className="w-6 h-6 text-primary-700" /></div>
 <div><p className="text-sm font-medium text-muted-foreground mb-1">Receita de administração</p><h2 className="text-2xl font-bold text-foreground">{formatCurrency(agencyAdminRevenue)}</h2><p className="mt-1 text-caption text-muted-foreground">Estimativa mensal da imobiliária</p></div>
 </div>
 <div className="bg-card p-6 rounded-2xl border border-border shadow-sm flex items-center gap-5 hover:border-primary-200 transition-colors">
 <div className="w-12 h-12 rounded-xl bg-muted flex items-center justify-center border border-border"><Clock className="w-6 h-6 text-muted-foreground" /></div>
 <div><p className="text-sm font-medium text-muted-foreground mb-1">Renovações Próximas</p><h2 className="text-3xl font-bold text-foreground">{upcomingAdjustments}</h2></div>
 </div>
 </div>
 </motion.div>
 )}
 </AnimatePresence>

 {/* List */}
 <div className="space-y-4">
 <h2 className="text-xl font-bold text-foreground">Lista de Contratos</h2>
 <div className="bg-card rounded-2xl border border-border shadow-sm overflow-hidden flex flex-col">
 {/* Filters */}
 <div className="p-4 border-b border-border flex flex-wrap gap-4 items-center justify-between bg-muted/50">
 <div className="relative flex-1 min-w-[280px] max-w-md">
 <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
 <input
   type="text"
   value={searchInput}
   onChange={(e) => setSearchInput(e.target.value)}
   placeholder="Buscar por inquilino ou imóvel..."
   className="w-full pl-9 pr-4 py-2 bg-muted border-transparent rounded-xl text-sm focus:bg-card focus:border-primary-600 focus:ring-2 focus:ring-primary-600/20 outline-none transition-all"
 />
 </div>
 <div className="flex items-center gap-3">
 <div className="flex items-center gap-2">
 <span className="text-xs font-medium text-muted-foreground">Tipo:</span>
 <select
 value={contractTypeFilter}
 onChange={(e) => setContractTypeFilter(e.target.value as any)}
 className="bg-card border border-border text-foreground text-sm font-medium rounded-lg px-3 py-2 outline-none"
 >
 <option value="Todos">Todos</option>
 <option value="locacao">Locação</option>
 </select>
 </div>
 <div className="flex items-center gap-2">
 <span className="text-xs font-medium text-muted-foreground">Status:</span>
 <select
   value={statusFilter}
   onChange={(e) => setStatusFilter(e.target.value as any)}
   className="bg-card border border-border text-foreground text-sm font-medium rounded-lg px-3 py-2 outline-none"
 >
   <option value="Todos">Todos</option>
   <option value="Ativos">Ativos</option>
 </select>
 </div>
 <div className="flex items-center gap-2">
 <span className="text-xs font-medium text-muted-foreground">Ordenar por:</span>
 <select
   value={sortBy}
   onChange={(e) => setSortBy(e.target.value as any)}
   className="bg-card border border-border text-foreground text-sm font-medium rounded-lg px-3 py-2 outline-none"
 >
   <option value="Inquilino">Inquilino</option>
   <option value="Valor">Valor</option>
 </select>
 </div>
 </div>
 <div className="text-xs text-muted-foreground font-medium ml-auto hidden xl:block">{filteredContracts.length} contratos</div>
 </div>

 {/* Table */}
 <div className="overflow-x-auto">
 {contractsLoading ? (
   <div className="divide-y divide-border">
     {[1, 2, 3, 4].map((i) => (
       <div key={i} className="p-4 animate-pulse">
         <div className="flex items-center gap-3">
           <div className="w-9 h-9 rounded-xl bg-muted" />
           <div className="flex-1 space-y-2">
             <div className="h-3 w-1/3 bg-muted rounded" />
             <div className="h-3 w-1/2 bg-muted rounded" />
           </div>
           <div className="h-6 w-20 bg-muted rounded-md" />
         </div>
       </div>
     ))}
   </div>
 ) : filteredContracts.length === 0 ? (
   <div className="p-10">
     <EmptyState
       icon={FileText}
       title="Nenhum contrato encontrado"
       message={searchInput ? "Tente ajustar a busca ou remover filtros." : "Crie o primeiro contrato a partir de um negócio."}
     />
   </div>
 ) : isMobile ? (
 <div className="divide-y divide-border">
 {filteredContracts.map(contract => (
 <div key={contract.id} className="p-4">
 <div className="flex items-start gap-3">
 <div className={clsx(
 "w-9 h-9 rounded-xl flex items-center justify-center shrink-0 border border-border",
 contract.type === 'venda' ? "bg-status-ok-soft text-status-ok" : "bg-primary-50 text-primary-700"
 )}>
 {contract.type === 'venda' ? <DollarSign className="w-4 h-4" /> : <Home className="w-4 h-4" />}
 </div>
 <div className="min-w-0 flex-1">
 <div className="flex items-center gap-2 min-w-0">
 <span className="text-xs font-bold text-foreground">
 {contract.display_id || contract.id.substring(0, 8)}
 </span>
 {(contract as any).isDeal && (
 <span className="px-1.5 py-0.5 rounded bg-status-warning-soft text-status-warning text-caption uppercase tracking-wider whitespace-nowrap">
 Negócio
 </span>
 )}
 <SignatureStatusBadge kind="contract" status={contract.status} size="sm" className="ml-auto whitespace-nowrap" />
 </div>
 <div className="mt-1 text-sm font-medium text-foreground truncate" title={contract.client}>{contract.client}</div>
 <div className="mt-0.5 text-xs text-muted-foreground truncate" title={contract.property_title}>{contract.property_title}</div>
 <div className="mt-2 flex items-center justify-between gap-3">
 <span className="text-sm font-bold text-foreground">{formatCurrency(contract.value || 0)}</span>
 <span className="text-xs text-muted-foreground whitespace-nowrap">Reajuste: {formatISODateBR(contract.next_adjustment ?? contract.nextAdjustment) || '-'}</span>
 </div>
 </div>
 </div>
 <div className="mt-3 flex flex-wrap gap-2">
 {perms.chargesWrite && (
 <Button variant="ghost" size="sm" onClick={() => openModal('cobrancas', contract)} className="flex-1 min-w-[140px] text-muted-foreground hover:text-primary-700 hover:bg-primary-50">
 <Settings className="w-3.5 h-3.5" /> Plano financeiro
 </Button>
 )}
 {contract.type === 'locacao' && perms.contractsWrite && (
 <Button variant="ghost" size="sm" onClick={() => openModal('reajuste', contract)} className="flex-1 min-w-[140px] text-muted-foreground hover:text-primary-700 hover:bg-primary-50">
 <TrendingUp className="w-3.5 h-3.5" /> Reajustar
 </Button>
 )}
 <Button variant="ghost" size="sm" onClick={() => viewDetail(contract)} className="flex-1 min-w-[140px] text-muted-foreground hover:text-primary-700 hover:bg-primary-50">
 <Eye className="w-3.5 h-3.5" /> Ver
 </Button>
 </div>
 </div>
 ))}
 </div>
 ) : (
 <table className="w-full text-left border-collapse min-w-[1000px]">
 <thead>
 <tr className="border-b border-border text-muted-foreground text-xs font-medium tracking-wide">
 <th className="px-6 py-4">Tipo</th>
 <th className="px-6 py-4 min-w-[140px]">Número</th>
 <th className="px-6 py-4">Cliente</th>
 <th className="px-6 py-4">Imóvel</th>
 <th className="px-6 py-4">Valor</th>
 <th className="px-6 py-4">Índice/Cond.</th>
 <th className="px-6 py-4">Prox. Reajuste</th>
 <th className="px-6 py-4">Status</th>
 <th className="px-6 py-4 text-right">Ações</th>
 </tr>
 </thead>
 <tbody className="divide-y divide-border">
 {filteredContracts.map(contract => (
 <tr key={contract.id} className="hover:bg-muted/80 transition-colors group">
 <td className="px-6 py-4">
 <div className={clsx(
 "w-8 h-8 rounded-lg flex items-center justify-center",
 contract.type === 'venda' ? "bg-status-ok-soft text-status-ok" : "bg-primary-50 text-primary-700"
 )}>
 {contract.type === 'venda' ? <DollarSign className="w-4 h-4" /> : <Home className="w-4 h-4" />}
 </div>
 </td>
 <td className="px-6 py-4">
 <span className="text-xs font-bold text-foreground flex items-center gap-1.5">
 {contract.display_id || contract.id.substring(0, 8)}
 {(contract as any).isDeal && <span className="px-1.5 py-0.5 rounded bg-status-warning-soft text-status-warning text-caption uppercase tracking-wider">Negócio</span>}
 </span>
 </td>
 <td className="px-6 py-4">
 <div className="flex items-center gap-2">
 <div className="w-6 h-6 rounded-full bg-muted flex items-center justify-center text-muted-foreground"><User className="w-3 h-3" /></div>
 <span className="text-sm font-medium text-foreground">{contract.client}</span>
 </div>
 </td>
 <td className="px-6 py-4">
 <div className="flex items-center gap-2 text-sm text-muted-foreground"><MapPin className="w-3.5 h-3.5 text-muted-foreground" /><span className="truncate max-w-[200px]" title={contract.property_title}>{contract.property_title}</span></div>
 </td>
 <td className="px-6 py-4"><span className="text-sm font-bold text-foreground">{formatCurrency(contract.value || 0)}</span></td>
 <td className="px-6 py-4">
 <div className="flex items-center gap-1.5">
 <span className="text-xs font-bold text-foreground">{contract.adjust_index || contract.index || (contract.type === 'venda' ? 'À Vista' : 'IGPM')}</span>
 {(contract.auto_adjust || contract.autoIndex) && <span className="text-caption font-bold text-primary-700 bg-primary-50 px-1.5 py-0.5 rounded border border-primary-100">Auto</span>}
 </div>
 </td>
 <td className="px-6 py-4"><span className="text-sm text-muted-foreground">{formatISODateBR(contract.next_adjustment ?? contract.nextAdjustment) || '-'}</span></td>
 <td className="px-6 py-4"><SignatureStatusBadge kind="contract" status={contract.status} /></td>
 <td className="px-6 py-4">
 <div className="flex items-center justify-end gap-2">
 {perms.chargesWrite && (
 <Button variant="ghost" size="sm" onClick={() => openModal('cobrancas', contract)} className="text-muted-foreground hover:text-primary-700 hover:bg-primary-50"><Settings className="w-3.5 h-3.5" /> Plano financeiro</Button>
 )}
 {contract.type === 'locacao' && perms.contractsWrite && (
 <Button variant="ghost" size="sm" onClick={() => openModal('reajuste', contract)} className="text-muted-foreground hover:text-primary-700 hover:bg-primary-50"><TrendingUp className="w-3.5 h-3.5" /> Reajustar</Button>
 )}
 <Button variant="ghost" size="sm" onClick={() => viewDetail(contract)} className="text-muted-foreground hover:text-primary-700 hover:bg-primary-50"><Eye className="w-3.5 h-3.5" /> Ver</Button>
 {/* Phase 4.6 — direct link to consolidated RentalDetails page when the
     contract is a locação. Falls back gracefully (RentalDetails shows
     a friendly empty state) for legacy contracts that didn't go
     through the new contract.signed → Rental aggregate path. */}
 {contract.type === 'locacao' && (
   <Link to={`/locacao/${contract.id}`} className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-bold text-muted-foreground hover:text-primary-700 hover:bg-primary-50 rounded-lg transition-colors" title="Visão consolidada da locação"><ExternalLink className="w-3.5 h-3.5" /> Locação</Link>
 )}
 </div>
 </td>
 </tr>
 ))}
 </tbody>
 </table>
 )}
 </div>
 </div>
 </div>
 </div>
 </>
 );
}
