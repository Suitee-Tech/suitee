import { useConfirmAction } from "../../hooks/useConfirmAction";
import type { Dispatch, ReactNode, SetStateAction } from "react";
import {
 AlertCircle,
 Bell,
 Calendar,
 CheckCircle2,
 ChevronDown,
 Clock,
 DollarSign,
 Mail,
 Percent,
 Plus,
 Receipt,
 Settings,
 Layers3,
 Trash2,
 TrendingUp,
 Zap,
} from "lucide-react";
import { clsx } from "clsx";
import { EmptyState } from "../../ui/EmptyState";
import { Button } from "../../ui/button";
import { Skeleton } from "../../ui/skeleton";
import { ExpandableSidePanel } from "../../components/ExpandableSidePanel";
import type { UtilityAccount, UtilityAccountType } from "../../services/utilityAccount.service";
import type { ReminderRule } from "../../services/reminderRule.service";
import type { ModalState } from "./presentation";
import type { RentalChargeConfig } from "../../rentals/types";
import { parseMoneyBR } from "../../utils/format";

type SettingsTab = "plano" | "contas" | "regua";

type MutationState = {
 isPending: boolean;
 mutate: (...args: any[]) => void;
};

type ChargeConfigForm = {
 monthlyAmount: string;
 defaultDueDay: string;
 firstChargeDate: string;
 firstChargeType: "same_month_pro_rata" | "next_month_accumulated";
 adminFeePct: string;
 lateFeePct: string;
 monthlyInterestPct: string;
};

type ReajusteForm = {
 index: string;
 percent: string;
 baseDate: string;
};

type SelectedRentalContract = {
 id: string;
 client?: string;
 property_title?: string;
 display_id?: string;
 value?: number;
 adjust_index?: string;
 index?: string;
};

export interface RentalSettingsPanelProps {
 selectedContract: SelectedRentalContract | null;
 activeModal: ModalState;
 setActiveModal: Dispatch<SetStateAction<ModalState>>;
 settingsTab: SettingsTab;
 setSettingsTab: Dispatch<SetStateAction<SettingsTab>>;
 formatCurrency: (value: number) => string;
 adjustContractMutation: MutationState;
 utilityAccounts: UtilityAccount[];
 utilityAccountsLoading: boolean;
 updateUtilityAccountMutation: MutationState;
 reminderRules: ReminderRule[];
 reminderRulesLoading: boolean;
 updateReminderRuleMutation: MutationState;
 deleteReminderRuleMutation: MutationState & { mutateAsync: (id: string) => Promise<unknown> };
 runRemindersNowMutation: MutationState & { mutateAsync: () => Promise<unknown> };
 setEditingRule: Dispatch<SetStateAction<ReminderRule | null>>;
 setReminderRuleModalOpen: Dispatch<SetStateAction<boolean>>;
 editingPaymentForAccount: string | null;
 setEditingPaymentForAccount: Dispatch<SetStateAction<string | null>>;
 paymentMethodDraft: string;
 setPaymentMethodDraft: Dispatch<SetStateAction<string>>;
 utilityTypeLabel: (type: UtilityAccountType) => string;
 utilityTypeIcon: (type: UtilityAccountType) => ReactNode;
 rentalChargeConfig?: RentalChargeConfig | null;
 rentalChargeConfigLoading: boolean;
 chargesCount: number;
 defaultAdminFeePct: number;
 defaultLateFeePct: number;
 defaultMonthlyInterestPct: number;
 onOpenComposition: () => void;
 onLaunchUtilityCharge: (account: UtilityAccount) => void;
 createRentalChargeConfigMutation: MutationState;
 updateRentalChargeConfigMutation: MutationState;
 generateRentalChargesMutation: MutationState;
 chargeConfigForm: ChargeConfigForm;
 setChargeConfigForm: Dispatch<SetStateAction<ChargeConfigForm>>;
 updateContractRatesMutation: MutationState;
 handleSaveContractRates: () => void;
 handleSaveChargeConfig: () => void | Promise<void>;
 handleGenerateCharges: () => void;
 chargeConfigLocked: boolean;
 reajusteForm: ReajusteForm;
 setReajusteForm: Dispatch<SetStateAction<ReajusteForm>>;
 reajustePreviewValue: number;
 handleApplyReajuste: () => void;
}

function RentalSettingsSkeleton({ label }: { label: string }) {
 return (
 <div className="space-y-4" role="status" aria-label={label}>
 <Skeleton className="h-20 w-full rounded-xl" />
 <div className="grid grid-cols-2 gap-4">
 <Skeleton className="h-16 rounded-xl" />
 <Skeleton className="h-16 rounded-xl" />
 </div>
 <Skeleton className="h-28 w-full rounded-xl" />
 <span className="sr-only">{label}</span>
 </div>
 );
}

export function RentalSettingsPanel({
 selectedContract,
 activeModal,
 setActiveModal,
 settingsTab,
 setSettingsTab,
 formatCurrency,
 adjustContractMutation,
 utilityAccounts,
 utilityAccountsLoading,
 updateUtilityAccountMutation,
 reminderRules,
 reminderRulesLoading,
 updateReminderRuleMutation,
 deleteReminderRuleMutation,
 runRemindersNowMutation,
 setEditingRule,
 setReminderRuleModalOpen,
 editingPaymentForAccount,
 setEditingPaymentForAccount,
 paymentMethodDraft,
 setPaymentMethodDraft,
 utilityTypeLabel,
 utilityTypeIcon,
 rentalChargeConfig,
 rentalChargeConfigLoading,
 chargesCount,
 defaultAdminFeePct,
 defaultLateFeePct,
 defaultMonthlyInterestPct,
 onOpenComposition,
 onLaunchUtilityCharge,
 createRentalChargeConfigMutation,
 updateRentalChargeConfigMutation,
 generateRentalChargesMutation,
 chargeConfigForm,
 setChargeConfigForm,
 updateContractRatesMutation,
 handleSaveContractRates,
 handleSaveChargeConfig,
 handleGenerateCharges,
 chargeConfigLocked,
 reajusteForm,
 setReajusteForm,
 reajustePreviewValue,
 handleApplyReajuste,
}: RentalSettingsPanelProps) {
 const { confirmAction, confirmationDialog } = useConfirmAction();
 if (!selectedContract || (activeModal !== 'cobrancas' && activeModal !== 'reajuste')) return null;
 const monthlyBase = rentalChargeConfig
   ? rentalChargeConfig.monthly_amount / 100
   : (parseMoneyBR(chargeConfigForm.monthlyAmount) ?? selectedContract.value ?? 0);
 const configuredAdminFee = parseMoneyBR(chargeConfigForm.adminFeePct);
 const effectiveAdminFeePct = configuredAdminFee && configuredAdminFee > 0
   ? configuredAdminFee
   : defaultAdminFeePct;
 const bundledTotal = (rentalChargeConfig?.bundled_items ?? [])
   .filter((item) => item.enabled)
   .reduce((total, item) => total + item.amount_cents / 100, 0);
 const agencyRevenue = monthlyBase * (effectiveAdminFeePct / 100);
 const ownerBaseEstimate = Math.max(monthlyBase - agencyRevenue, 0);
 const modeLabel = (account: UtilityAccount) => {
   if (account.billing_mode === 'VARIABLE_SEPARATE') return 'Variável por mês';
   if (account.billing_mode === 'BUNDLED') return 'Inclusa no aluguel';
   return 'Somente acompanhamento';
 };
 return (
 <>
 {confirmationDialog}
 <ExpandableSidePanel
 open={true}
 onClose={() => setActiveModal('none')}
 defaultSize="3xl"
 expandedSize="full"
 bgClass="bg-muted"
 renderHeader={({ expandButton, closeButton }) => (
 <div className="bg-card px-6 py-4 border-b border-border flex items-center justify-between z-10 shrink-0">
 <div className="flex items-center gap-3">
 <div className="p-2 bg-primary-50 text-primary-700 rounded-lg">
 {activeModal === 'cobrancas' ? <Settings className="w-5 h-5" /> : <TrendingUp className="w-5 h-5" />}
 </div>
 <div>
 <h2 className="text-lg font-bold text-foreground">
 {activeModal === 'cobrancas' ? 'Plano financeiro da locação' : 'Aplicar Reajuste Contratual'}
 </h2>
 <p className="text-sm text-muted-foreground">
 {activeModal === 'cobrancas'
   ? `${selectedContract.client || 'Inquilino não informado'} · ${selectedContract.property_title || 'Imóvel não informado'} · ${selectedContract.display_id ?? selectedContract.id.slice(0, 8)}`
   : `${selectedContract.client} - ${selectedContract.property_title}`}
 </p>
 </div>
 </div>
 <div className="flex items-center gap-2">
 {expandButton}
 {closeButton}
 </div>
 </div>
 )}
 >
 <div className="flex-1 overflow-y-auto">
 {activeModal === 'cobrancas' ? (
 <div className="p-6">
 <div className="flex p-1 bg-muted rounded-xl mb-6">
 <button onClick={() => setSettingsTab('plano')} className={clsx("flex-1 py-2 text-sm font-medium rounded-lg transition-all flex items-center justify-center gap-2", settingsTab === 'plano' ? "bg-card text-foreground shadow-sm" : "text-muted-foreground hover:text-foreground")}>
 <DollarSign className="w-4 h-4" /> Plano e composição
 </button>
 <button onClick={() => setSettingsTab('contas')} className={clsx("flex-1 py-2 text-sm font-medium rounded-lg transition-all flex items-center justify-center gap-2", settingsTab === 'contas' ? "bg-card text-foreground shadow-sm" : "text-muted-foreground hover:text-foreground")}>
 <Settings className="w-4 h-4" /> Contas e pagamentos
 </button>
 <button onClick={() => setSettingsTab('regua')} className={clsx("flex-1 py-2 text-sm font-medium rounded-lg transition-all flex items-center justify-center gap-2", settingsTab === 'regua' ? "bg-card text-foreground shadow-sm" : "text-muted-foreground hover:text-foreground")}>
 <Bell className="w-4 h-4" /> Régua de Comunicação
 </button>
 </div>
 {settingsTab === 'plano' && (rentalChargeConfigLoading ? (
 <RentalSettingsSkeleton label="Carregando configuração de cobranças" />
 ) : (
 <div className="space-y-8">
 <div className="grid grid-cols-2 gap-6 bg-card p-5 rounded-xl border border-border">
 <div><p className="text-xs text-muted-foreground mb-1">Inquilino:</p><p className="text-sm font-semibold text-foreground">{selectedContract.client}</p></div>
 <div><p className="text-xs text-muted-foreground mb-1">Imóvel:</p><p className="text-sm font-semibold text-foreground">{selectedContract.property_title}</p></div>
 </div>
 <div className="rounded-xl border border-border bg-card p-5">
 <div className="mb-4 flex items-center justify-between gap-3">
 <div>
 <h3 className="text-sm font-bold text-foreground">Jornada da cobrança mensal</h3>
 <p className="mt-1 text-xs text-muted-foreground">Configure na ordem e emita somente depois de revisar a prévia.</p>
 </div>
 <span className={clsx(
 "rounded-full border px-2.5 py-1 text-caption font-bold uppercase tracking-wide",
 chargesCount > 0 ? "border-status-ok/20 bg-status-ok-soft text-status-ok" : "border-status-warning/20 bg-status-warning-soft text-status-warning",
 )}>{chargesCount > 0 ? `${chargesCount} emitida${chargesCount === 1 ? '' : 's'}` : 'Ainda não emitido'}</span>
 </div>
 <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
 {[
 { label: '1. Plano-base', done: Boolean(rentalChargeConfig), helper: rentalChargeConfig ? 'Calendário definido' : 'Pendente' },
 { label: '2. Composição', done: Boolean(rentalChargeConfig), helper: bundledTotal > 0 ? formatCurrency(bundledTotal) : 'Sem itens extras' },
 { label: '3. Contas', done: utilityAccounts.length > 0, helper: utilityAccounts.length > 0 ? `${utilityAccounts.length} vinculada${utilityAccounts.length === 1 ? '' : 's'}` : 'Opcional' },
 { label: '4. Emissão', done: chargesCount > 0, helper: chargesCount > 0 ? 'Calendário materializado' : 'Aguardando revisão' },
 ].map((step) => (
 <div key={step.label} className={clsx("rounded-xl border p-3", step.done ? "border-status-ok/20 bg-status-ok-soft/60" : "border-border bg-muted/50")}>
 <div className="flex items-center gap-2">
 {step.done ? <CheckCircle2 className="size-4 text-status-ok" /> : <span className="size-4 rounded-full border-2 border-muted-foreground/40" />}
 <p className="text-xs font-bold text-foreground">{step.label}</p>
 </div>
 <p className="mt-1 pl-6 text-caption text-muted-foreground">{step.helper}</p>
 </div>
 ))}
 </div>
 </div>
 <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4">
 <div className="rounded-xl border border-border bg-card p-4">
 <p className="text-caption font-bold uppercase tracking-wide text-muted-foreground">Aluguel base</p>
 <p className="mt-1 text-xl font-bold text-foreground">{formatCurrency(monthlyBase)}</p>
 </div>
 <div className="rounded-xl border border-border bg-card p-4">
 <p className="text-caption font-bold uppercase tracking-wide text-muted-foreground">Itens fixos inclusos</p>
 <p className="mt-1 text-xl font-bold text-foreground">{formatCurrency(bundledTotal)}</p>
 </div>
 <div className="rounded-xl border border-primary-100 bg-primary-50 p-4">
 <p className="text-caption font-bold uppercase tracking-wide text-primary-700">Receita da imobiliária · {effectiveAdminFeePct.toLocaleString('pt-BR')}%</p>
 <p className="mt-1 text-xl font-bold text-primary-800">{formatCurrency(agencyRevenue)}</p>
 </div>
 <div className="rounded-xl border border-status-ok/20 bg-status-ok-soft p-4">
 <p className="text-caption font-bold uppercase tracking-wide text-status-ok">Repasse-base ao proprietário</p>
 <p className="mt-1 text-xl font-bold text-status-ok">{formatCurrency(ownerBaseEstimate)}</p>
 <p className="mt-1 text-caption text-muted-foreground">Antes de contas e outras deduções.</p>
 </div>
 </div>
 {chargeConfigLocked && (
 <div className="bg-status-warning-soft border border-status-warning/30 text-status-warning text-xs font-medium px-4 py-3 rounded-lg flex items-start gap-2">
 <AlertCircle className="w-4 h-4 mt-0.5 shrink-0" />
 <span>A primeira cobrança deste contrato já foi paga. Valores e datas estão travados — apenas operações específicas (gerar próximas, ajustar) seguem disponíveis.</span>
 </div>
 )}
 <section>
 <h3 className="text-sm font-bold text-foreground mb-1">Plano-base e primeiro aluguel</h3>
 <p className="mb-4 text-xs text-muted-foreground">Define o calendário recorrente e como tratar o período entre o início do contrato e a primeira competência.</p>
 <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
 <div>
 <label className="block text-xs font-medium text-foreground mb-1">Valor do Aluguel (R$)</label>
 <input
 type="text"
 value={chargeConfigForm.monthlyAmount}
 onChange={(e) => setChargeConfigForm((f) => ({ ...f, monthlyAmount: e.target.value }))}
 disabled={Boolean(rentalChargeConfig) || chargeConfigLocked}
 className="w-full px-3 py-2 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none transition-all disabled:opacity-60 disabled:cursor-not-allowed"
 />
 <p className="text-caption text-muted-foreground mt-1">{rentalChargeConfig ? 'Para alterar o valor, aplique um Reajuste.' : 'Definido na criação da configuração.'}</p>
 </div>
 <div>
 <label className="block text-xs font-medium text-foreground mb-1">Dia de Vencimento</label>
 <input
 type="number"
 min={1}
 max={31}
 value={chargeConfigForm.defaultDueDay}
 onChange={(e) => setChargeConfigForm((f) => ({ ...f, defaultDueDay: e.target.value }))}
 disabled={chargeConfigLocked}
 className="w-full px-3 py-2 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none transition-all disabled:opacity-60 disabled:cursor-not-allowed"
 />
 </div>
 <div>
 <label className="block text-xs font-medium text-foreground mb-1">Data da primeira cobrança</label>
 <input
 type="date"
 value={chargeConfigForm.firstChargeDate}
 onChange={(e) => setChargeConfigForm((f) => ({ ...f, firstChargeDate: e.target.value }))}
 disabled={chargeConfigLocked}
 className="w-full px-3 py-2 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none transition-all disabled:opacity-60 disabled:cursor-not-allowed"
 />
 </div>
 <div className="sm:col-span-2">
 <label className="block text-xs font-medium text-foreground mb-2">Tratamento do primeiro aluguel</label>
 <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
 <button
 type="button"
 onClick={() => setChargeConfigForm((form) => ({ ...form, firstChargeType: 'same_month_pro_rata' }))}
 disabled={Boolean(rentalChargeConfig) || chargeConfigLocked}
 className={clsx(
 "rounded-xl border p-4 text-left transition disabled:cursor-not-allowed disabled:opacity-60",
 chargeConfigForm.firstChargeType === 'same_month_pro_rata' ? "border-primary-300 bg-primary-50 ring-2 ring-primary-600/20" : "border-border bg-card hover:bg-muted",
 )}
 >
 <span className="block text-sm font-bold text-foreground">Cobrar pro-rata no início</span>
 <span className="mt-1 block text-xs leading-relaxed text-muted-foreground">Emite apenas os dias ocupados na primeira competência. Comissão de intermediação usa o valor efetivamente recebido.</span>
 </button>
 <button
 type="button"
 onClick={() => setChargeConfigForm((form) => ({ ...form, firstChargeType: 'next_month_accumulated' }))}
 disabled={Boolean(rentalChargeConfig) || chargeConfigLocked}
 className={clsx(
 "rounded-xl border p-4 text-left transition disabled:cursor-not-allowed disabled:opacity-60",
 chargeConfigForm.firstChargeType === 'next_month_accumulated' ? "border-primary-300 bg-primary-50 ring-2 ring-primary-600/20" : "border-border bg-card hover:bg-muted",
 )}
 >
 <span className="block text-sm font-bold text-foreground">Acumular no próximo mês</span>
 <span className="mt-1 block text-xs leading-relaxed text-muted-foreground">Soma o pro-rata ao próximo aluguel. Se a corretagem não couber, o saldo continua nas competências seguintes.</span>
 </button>
 </div>
 </div>
 </div>
 </section>
 <section>
 <h3 className="text-sm font-bold text-foreground mb-1">Receita e encargos do contrato</h3>
 <p className="text-xs text-muted-foreground mb-3">Se o campo ficar vazio, a locação herda o padrão da imobiliária: {defaultAdminFeePct.toLocaleString('pt-BR')}% administração, {defaultLateFeePct.toLocaleString('pt-BR')}% multa e {defaultMonthlyInterestPct.toLocaleString('pt-BR')}% juros ao mês.</p>
 <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
 <div>
 <label className="block text-xs font-medium text-foreground mb-1">Taxa de Administração (%)</label>
 <input
 type="text"
 inputMode="decimal"
 value={chargeConfigForm.adminFeePct}
 onChange={(e) => setChargeConfigForm((f) => ({ ...f, adminFeePct: e.target.value }))}
 placeholder={String(defaultAdminFeePct)}
 className="w-full px-3 py-2 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none transition-all"
 />
 <p className="text-caption text-muted-foreground mt-1">Aplicada na hora de gerar repasses</p>
 </div>
 <div>
 <label className="block text-xs font-medium text-foreground mb-1">Multa por Atraso (%)</label>
 <input
 type="text"
 inputMode="decimal"
 value={chargeConfigForm.lateFeePct}
 onChange={(e) => setChargeConfigForm((f) => ({ ...f, lateFeePct: e.target.value }))}
 placeholder={String(defaultLateFeePct)}
 className="w-full px-3 py-2 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none transition-all"
 />
 <p className="text-caption text-muted-foreground mt-1">Percentual flat aplicado quando a cobrança vence</p>
 </div>
 <div className="col-span-2">
 <label className="block text-xs font-medium text-foreground mb-1">Juros por Atraso (% ao mês)</label>
 <input
 type="text"
 inputMode="decimal"
 value={chargeConfigForm.monthlyInterestPct}
 onChange={(e) => setChargeConfigForm((f) => ({ ...f, monthlyInterestPct: e.target.value }))}
 placeholder={String(defaultMonthlyInterestPct)}
 className="w-full px-3 py-2 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none transition-all"
 />
 <p className="text-caption text-muted-foreground mt-1">Convertido em juros pro-rata diários sobre o valor em aberto</p>
 </div>
 </div>
 <div className="flex justify-end pt-3">
 <Button
 variant="outline"
 onClick={handleSaveContractRates}
 loading={updateContractRatesMutation.isPending}
 loadingText="Salvando taxas…"
 >
 <Percent className="w-4 h-4" /> Salvar Taxas no Contrato
 </Button>
 </div>
 </section>
 <section className="rounded-xl border border-border bg-card p-5">
 <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
 <div className="flex items-start gap-3">
 <div className="rounded-lg bg-primary-50 p-2 text-primary-700"><Layers3 className="size-4" /></div>
 <div>
 <h3 className="text-sm font-bold text-foreground">Composição mensal</h3>
 <p className="mt-1 text-xs leading-relaxed text-muted-foreground">Condomínio, IPTU, seguro e outros valores fixos cobrados junto do aluguel. Contas variáveis ficam na aba Contas.</p>
 <p className="mt-2 text-xs font-bold text-foreground">Total previsto ao inquilino: {formatCurrency(monthlyBase + bundledTotal)}</p>
 </div>
 </div>
 <Button variant="outline" onClick={onOpenComposition} disabled={!rentalChargeConfig} className="shrink-0">
 <Layers3 className="size-4" /> {bundledTotal > 0 ? 'Editar composição' : 'Configurar composição'}
 </Button>
 </div>
 {!rentalChargeConfig && <p className="mt-3 text-[11px] text-status-warning">Salve o plano-base para liberar a composição mensal.</p>}
 </section>
 <div className="flex justify-between items-center pt-4 gap-3">
 <Button
 variant="outline"
 onClick={handleGenerateCharges}
 loading={generateRentalChargesMutation.isPending}
 loadingText="Emitindo…"
 disabled={!rentalChargeConfig}
 >
 <Receipt className="w-4 h-4" /> Emitir cobranças previstas
 </Button>
 <Button
 onClick={handleSaveChargeConfig}
 loading={createRentalChargeConfigMutation.isPending || updateRentalChargeConfigMutation.isPending}
 loadingText="Salvando…"
 disabled={chargeConfigLocked}
 >
 {rentalChargeConfig ? 'Salvar alterações do plano' : 'Criar plano-base'}
 </Button>
 </div>
 </div>
 ))}
 {settingsTab === 'contas' && (utilityAccountsLoading ? (
 <RentalSettingsSkeleton label="Carregando contas vinculadas" />
 ) : (
 <div className="space-y-4">
 <div className="flex items-start justify-between mb-2 gap-3">
 <div>
 <h3 className="text-base font-bold text-foreground">Contas, responsabilidades e lançamentos</h3>
 <p className="text-sm text-muted-foreground">Veja quem absorve cada valor e se ele acompanha o aluguel, gera lançamento mensal ou é apenas informativo.</p>
 </div>
 <Button
 variant="outline"
 onClick={() => setActiveModal('vincularConta')}
 className="shrink-0"
 ><Plus className="w-4 h-4" /> Nova Conta</Button>
 </div>

 {utilityAccounts.length === 0 ? (
 <EmptyState
 icon={Zap}
 title="Nenhuma conta vinculada ainda"
 message="Vincule condomínio, IPTU, consumo ou outra obrigação e defina seu efeito financeiro."
 />
 ) : (
 <>
 {utilityAccounts.map((account) => {
 const isEditing = editingPaymentForAccount === account.id;
 return (
 <div key={account.id} className="bg-card border border-border p-4 rounded-xl shadow-sm">
 <div className="flex items-center justify-between gap-4">
 <div className="flex items-center gap-4 min-w-0">
 <div className={clsx(
 "w-10 h-10 rounded-full flex items-center justify-center shrink-0",
 account.auto_pay_enabled ? "bg-status-ok-soft text-status-ok" : "bg-muted text-muted-foreground"
 )}>
 {utilityTypeIcon(account.type)}
 </div>
 <div className="min-w-0">
 <div className="flex flex-wrap items-center gap-2">
 <h4 className="text-sm font-bold text-foreground">{utilityTypeLabel(account.type)}</h4>
 <span className={clsx(
 "text-caption font-bold uppercase tracking-wider px-2 py-0.5 rounded-md border",
 account.billing_mode === 'VARIABLE_SEPARATE'
 ? "bg-status-warning-soft text-status-warning border-status-warning/20"
 : account.billing_mode === 'BUNDLED'
 ? "bg-primary-50 text-primary-700 border-primary-100"
 : "bg-muted text-muted-foreground border-border"
 )}>{modeLabel(account)}</span>
 {account.auto_pay_enabled && <span className="text-caption font-bold uppercase tracking-wider px-2 py-0.5 rounded-md border bg-status-ok-soft text-status-ok border-status-ok/20">Forma recorrente conferida</span>}
 </div>
 <p className="text-xs text-muted-foreground mt-0.5">
 {account.provider} — {formatCurrency((account.estimated_amount_cents || 0) / 100)} · Responsável: {account.responsible_party === 'TENANT' ? 'Inquilino' : account.responsible_party === 'OWNER' ? 'Proprietário' : 'Imobiliária'}
 </p>
 {account.billing_mode === 'BUNDLED' && account.bundled_item_key && <p className="mt-1 text-[11px] text-muted-foreground">Item da composição: <span className="font-medium text-foreground">{account.bundled_item_key}</span></p>}
 {account.payment_method && !isEditing && (
 <p className="text-[11px] text-muted-foreground mt-1">Forma de pagamento: <span className="font-medium text-foreground">{account.payment_method}</span></p>
 )}
 </div>
 </div>

 <div className="flex items-center gap-2 shrink-0">
 {account.billing_mode === 'VARIABLE_SEPARATE' && (
 <Button variant="outline" size="sm" onClick={() => onLaunchUtilityCharge(account)}>
 <Receipt className="size-3.5" /> Lançar mês
 </Button>
 )}
 <button
 onClick={() => {
 updateUtilityAccountMutation.mutate({
 id: account.id,
 data: { auto_pay_enabled: !account.auto_pay_enabled },
 });
 }}
 disabled={updateUtilityAccountMutation.isPending}
 className={clsx(
 "relative inline-flex h-6 w-11 items-center rounded-full transition-colors shadow-inner shrink-0",
 account.auto_pay_enabled ? "bg-primary-600" : "bg-muted",
 updateUtilityAccountMutation.isPending && "opacity-60 cursor-not-allowed"
 )}
 aria-pressed={account.auto_pay_enabled}
 aria-label="Alternar conferência da forma recorrente"
 >
 <span className={clsx(
 "inline-block h-4 w-4 transform rounded-full bg-card shadow transition-transform",
 account.auto_pay_enabled ? "translate-x-6" : "translate-x-1"
 )} />
 </button>
 <Button
 variant="outline"
 size="sm"
 onClick={() => {
 setEditingPaymentForAccount(account.id);
 setPaymentMethodDraft(account.payment_method ?? "");
 }}
 ><Settings className="w-3.5 h-3.5" /> Forma de pagamento</Button>
 </div>
 </div>

 {isEditing && (
 <div className="mt-4 pt-4 border-t border-border space-y-3">
 <label className="block text-xs font-medium text-foreground">Forma recorrente cadastrada</label>
 <input
 type="text"
 value={paymentMethodDraft}
 onChange={(e) => setPaymentMethodDraft(e.target.value)}
 placeholder="Débito automático, cartão recorrente, boleto..."
 className="w-full px-3 py-2 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none transition-all"
 />
 <div className="flex justify-end gap-2">
 <Button
 variant="ghost"
 size="sm"
 onClick={() => {
 setEditingPaymentForAccount(null);
 setPaymentMethodDraft("");
 }}
 >Cancelar</Button>
 <Button
 size="sm"
 loading={updateUtilityAccountMutation.isPending}
 loadingText="Salvando…"
 onClick={() => {
 updateUtilityAccountMutation.mutate(
 {
 id: account.id,
 data: { payment_method: paymentMethodDraft.trim() },
 },
 {
 onSuccess: () => {
 setEditingPaymentForAccount(null);
 setPaymentMethodDraft("");
 },
 }
 );
 }}
 >Salvar</Button>
 </div>
 </div>
 )}
 </div>
 );
 })}
 </>
 )}

 <div className="bg-primary-50 border border-primary-100 p-4 rounded-xl mt-6">
 <h4 className="text-sm font-bold text-primary-600 flex items-center gap-2 mb-1"><Zap className="w-4 h-4" /> O que esta configuração faz?</h4>
 <p className="text-xs text-primary-700 leading-relaxed">
 A forma de pagamento e a conferência são registros operacionais. Elas não debitam uma conta bancária nem confirmam pagamento sem integração. O efeito contábil vem do modo de cobrança e do responsável definidos em cada conta.
 </p>
 </div>
 </div>
 ))}
 {settingsTab === 'regua' && (reminderRulesLoading ? (
 <RentalSettingsSkeleton label="Carregando regras de comunicação" />
 ) : (
 <div className="space-y-4">
 <div className="flex items-start justify-between gap-3 mb-2">
 <div>
 <h3 className="text-base font-bold text-foreground">Régua de Comunicação</h3>
 <p className="text-sm text-muted-foreground">Regras que disparam lembretes automaticamente todo dia conforme o vencimento das cobranças. Compartilhadas entre todos os contratos da organização.</p>
 </div>
 <div className="flex items-center gap-2 shrink-0">
 <Button
 variant="outline"
 onClick={() => {
   confirmAction("Executar a régua agora? Vai avaliar todas as regras ativas e disparar lembretes para as cobranças elegíveis hoje.", () => runRemindersNowMutation.mutateAsync(), { title: "Executar régua", confirmLabel: "Executar" });
 }}
 loading={runRemindersNowMutation.isPending}
 loadingText="Executando…"
 ><TrendingUp className="w-4 h-4" /> Rodar Agora</Button>
 <Button
 onClick={() => {
   setEditingRule(null);
   setReminderRuleModalOpen(true);
 }}
 ><Plus className="w-4 h-4" /> Nova Regra</Button>
 </div>
 </div>

 {reminderRules.length === 0 ? (
 <EmptyState
 icon={Bell}
 title="Nenhuma regra configurada"
 message="Crie regras para lembrar inquilinos antes, no dia ou depois do vencimento. As regras rodam diariamente em todos os contratos ativos."
 />
 ) : (
 reminderRules.map((rule) => {
 const triggerLabel =
   rule.trigger_type === 'BEFORE_DUE' ? 'Antes do vencimento'
   : rule.trigger_type === 'ON_DUE' ? 'No dia do vencimento'
   : 'Após o vencimento';
 const offsetLabel =
   rule.trigger_type === 'ON_DUE'
     ? ''
     : `${rule.offset_days} ${rule.offset_days === 1 ? 'dia' : 'dias'}`;
 return (
 <div key={rule.id} className="bg-card border border-border p-5 rounded-xl shadow-sm flex gap-4">
 <div className={clsx("mt-1", rule.enabled ? "text-primary-600" : "text-muted-foreground")}>
 <Mail className="w-5 h-5" />
 </div>
 <div className="flex-1 min-w-0">
 <div className="flex items-center gap-3 mb-2 flex-wrap">
 <h4 className="text-sm font-bold text-foreground">{rule.name}</h4>
 <span className="text-caption font-semibold text-muted-foreground bg-muted px-2 py-0.5 rounded-full border border-border">{triggerLabel}</span>
 {offsetLabel && <span className="text-caption font-semibold text-muted-foreground flex items-center gap-1"><Clock className="w-3 h-3" /> {offsetLabel}</span>}
 {rule.channels.map((c) => (
 <span key={c} className="text-caption font-semibold text-primary-700 bg-primary-50 px-2 py-0.5 rounded-full border border-primary-200">{c}</span>
 ))}
 {!rule.enabled && <span className="text-caption font-semibold text-status-critical bg-status-critical-soft px-2 py-0.5 rounded-full border border-status-critical/20">Pausada</span>}
 </div>
 {rule.description && (
 <p className="text-[11px] text-muted-foreground italic mb-1">{rule.description}</p>
 )}
 <p className="text-xs text-muted-foreground leading-relaxed pr-4 whitespace-pre-wrap">{rule.template}</p>
 </div>
 <div className="flex flex-col items-end justify-between border-l border-border pl-4 shrink-0">
 <button
 onClick={() => updateReminderRuleMutation.mutate({ id: rule.id, data: { enabled: !rule.enabled } })}
 disabled={updateReminderRuleMutation.isPending}
 aria-pressed={rule.enabled}
 className={clsx(
 "relative inline-flex h-6 w-11 items-center rounded-full transition-colors shadow-inner",
 rule.enabled ? "bg-primary-600" : "bg-muted",
 updateReminderRuleMutation.isPending && "opacity-60 cursor-not-allowed"
 )}
 >
 <span className={clsx(
 "inline-block h-4 w-4 transform rounded-full bg-card shadow transition-transform",
 rule.enabled ? "translate-x-6" : "translate-x-1"
 )} />
 </button>
 <div className="flex items-center gap-2 mt-4">
 <Button
 variant="ghost"
 size="icon"
 onClick={() => {
   setEditingRule(rule);
   setReminderRuleModalOpen(true);
 }}
 className="size-8 text-muted-foreground hover:text-primary-700"
 title="Editar"
 ><Settings className="w-4 h-4" /></Button>
 <Button
 variant="ghost"
 size="icon"
 onClick={() => {
   confirmAction(`Remover a regra "${rule.name}"?`, () => deleteReminderRuleMutation.mutateAsync(rule.id), { title: "Remover regra", confirmLabel: "Remover", variant: "danger" });
 }}
 disabled={deleteReminderRuleMutation.isPending}
 className="size-8 text-status-critical hover:text-status-critical"
 title="Remover"
 ><Trash2 className="w-4 h-4" /></Button>
 </div>
 </div>
 </div>
 );
 })
 )}
 </div>
 ))}
 </div>
 ) : (
 <div className="p-6 space-y-6">
 <div className="grid grid-cols-2 gap-6 bg-card p-5 rounded-xl border border-border shadow-sm">
 <div><p className="text-xs text-muted-foreground mb-1">Valor Atual:</p><p className="text-lg font-bold text-foreground">{formatCurrency(selectedContract.value || 0)}</p></div>
 <div className="text-right"><p className="text-xs text-muted-foreground mb-1">Índice:</p><p className="text-lg font-bold text-foreground">{selectedContract.adjust_index || selectedContract.index || 'IGPM'}</p></div>
 <div className="col-span-2 pt-4 border-t border-border flex justify-between items-center"><span className="font-bold text-primary-600">Novo Valor:</span><span className="text-2xl font-bold text-primary-600">{formatCurrency(reajustePreviewValue)}</span></div>
 </div>
 <div className="space-y-4">
 <div>
 <label className="block text-sm font-medium text-foreground mb-1">Índice de Reajuste *</label>
 <div className="relative">
 <select
 value={reajusteForm.index}
 onChange={(e) => setReajusteForm((f) => ({ ...f, index: e.target.value }))}
 className="w-full px-4 py-2.5 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none appearance-none transition-all"
 >
 <option value="IGPM">IGPM</option>
 <option value="IPCA">IPCA</option>
 <option value="INPC">INPC</option>
 <option value="MANUAL">Manual</option>
 </select>
 <ChevronDown className="w-4 h-4 text-muted-foreground absolute right-3 top-1/2 -translate-y-1/2 pointer-events-none" />
 </div>
 </div>
 <div className="grid grid-cols-2 gap-4">
 <div>
 <label className="block text-sm font-medium text-foreground mb-1">Percentual do Índice *</label>
 <input
 type="text"
 value={reajusteForm.percent}
 onChange={(e) => setReajusteForm((f) => ({ ...f, percent: e.target.value }))}
 placeholder="0,00"
 className="w-full px-4 py-2.5 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none transition-all"
 />
 </div>
 <div>
 <label className="block text-sm font-medium text-foreground mb-1">Próximo Reajuste *</label>
 <div className="relative">
 <input
 type="date"
 value={reajusteForm.baseDate}
 onChange={(e) => setReajusteForm((f) => ({ ...f, baseDate: e.target.value }))}
 className="w-full px-4 py-2.5 bg-card border border-border rounded-xl text-sm focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 outline-none transition-all"
 />
 <Calendar className="w-4 h-4 text-muted-foreground absolute right-3 top-1/2 -translate-y-1/2 pointer-events-none" />
 </div>
 </div>
 </div>
 </div>
 <div className="flex gap-4 pt-4">
 <Button variant="outline" onClick={() => setActiveModal('none')} className="flex-1">Cancelar</Button>
 <Button
 onClick={handleApplyReajuste}
 loading={adjustContractMutation.isPending}
 loadingText="Aplicando…"
 className="flex-1"
 >
 Aplicar Reajuste
 </Button>
 </div>
 </div>
 )}
 </div>
 </ExpandableSidePanel>
 </>
 );
}
