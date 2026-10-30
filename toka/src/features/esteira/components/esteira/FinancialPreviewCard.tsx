import { useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertCircle, BookOpenCheck, Calendar, ChevronDown, ChevronUp, Coins, Loader2, Receipt, RefreshCw, ShieldAlert, TrendingUp, Users, Wallet } from "lucide-react";
import { financialPreviewService, FinancialPreview } from "../../../../shared/services/financialPreview.service";
import { formatMoneyBR, formatCompetencia } from "../../../../shared/utils/format";
import { allocationTypeLabel } from "../../../../shared/lib/finance/allocationLabels";
import { Button } from "../../../../shared/ui/button";
import {
  type CachedRentalPreContractPlan,
  rentalPlanCacheKey,
} from "../../../../shared/lib/finance/preContractPlanCache";
import { useSessionRecovery } from "../../../../shared/context/SessionRecoveryContext";

interface FinancialPreviewCardProps {
 dealId: string;
 // Quando contractId existe, prefere a query por contrato (datas finais).
 contractId?: string;
}

/**
 * FinancialPreviewCard renderiza a projeção do ciclo financeiro de um
 * deal/contrato ANTES da assinatura: charges previstas, commissions
 * por broker, transfers mensais ao proprietário e totais.
 *
 * Não persiste nada — só consome /financial-preview/deals/:id (ou
 * /contracts/:id). Mesmo input → mesma saída (idempotente em leitura).
 * O backend reusa as funções puras buildCharges +
 * ResolveBrokerCommissionShares, então o que aparece aqui é
 * literalmente o que será materializado no contract.signed.
 */
export function FinancialAccessRestricted() {
 return (
 <section
 aria-labelledby="financial-access-restricted-title"
 className="rounded-2xl border border-border bg-card p-6"
 >
 <div className="flex items-start gap-3">
 <div className="rounded-xl bg-status-warning-soft p-2 text-status-warning" aria-hidden="true">
 <ShieldAlert className="size-5" />
 </div>
 <div className="max-w-xl">
 <h3 id="financial-access-restricted-title" className="font-bold text-foreground">
 Área financeira restrita
 </h3>
 <p className="mt-1 text-sm leading-relaxed text-muted-foreground">
 Seu perfil não possui acesso às projeções, recebíveis, comissões e repasses desta negociação.
 </p>
 <p className="mt-3 text-xs leading-relaxed text-muted-foreground">
 Se precisar consultar esses dados, solicite acesso a um gestor, administrador ou responsável financeiro da imobiliária.
 </p>
 </div>
 </div>
 </section>
 );
}

function getHttpStatus(error: unknown) {
 return (error as { response?: { status?: number } })?.response?.status;
}

export function FinancialPreviewCard({ dealId, contractId }: FinancialPreviewCardProps) {
 const [showAllCharges, setShowAllCharges] = useState(false);
 const { isRecovering } = useSessionRecovery();
 const queryClient = useQueryClient();
 // Antes de existir contrato, as datas vêm do rascunho do plano pré-contrato
 // (início/prazo configurados na aba Assinatura). Sem elas o backend assumiria
 // o prazo padrão e a projeção mostraria outro calendário — com o residual do
 // último mês calculado sobre uma data que o operador não escolheu.
 const plannedDates = useMemo(() => {
   if (contractId || !dealId) return undefined;
   const cached = queryClient.getQueryData<CachedRentalPreContractPlan>(rentalPlanCacheKey(dealId));
   if (!cached?.startDate || !cached?.endDate) return undefined;
   return { startDate: cached.startDate, endDate: cached.endDate };
 }, [contractId, dealId, queryClient]);
 const { data, isLoading, isError, error, isFetching, refetch } = useQuery<FinancialPreview>({
 queryKey: ["financial-preview", contractId ? { contractId } : { dealId, ...plannedDates }],
 queryFn: () => (contractId
 ? financialPreviewService.getByContract(contractId)
 : financialPreviewService.getByDeal(dealId, plannedDates)),
 staleTime: 30_000,
 // Don't retry a gateway timeout/overload — surface the error fast instead
 // of leaving "Calculando…" spinning while we re-hammer a slow backend.
 retry: (count: number, err: unknown) => {
 const status = getHttpStatus(err);
 // 401 = access token vencido (a sessão é renovada e a consulta repetida);
 // 403/404/409 são erros reais e 503/504 seguem sem retry de propósito — a
 // projeção não fica martelando um gateway lento com "Calculando…" na tela.
 if (status === 401) return count < 3;
 if (status === 403 || status === 404 || status === 409 || status === 503 || status === 504) return false;
 return count < 1;
 },
 });

 const fmt = (cents: number) => formatMoneyBR(cents / 100);

 const dateFmt = useMemo(() => new Intl.DateTimeFormat("pt-BR", { day: "2-digit", month: "2-digit", year: "numeric", timeZone: "UTC" }), []);

 if (isLoading) {
 return (
 <div className="bg-card rounded-2xl border border-border p-6 text-sm text-muted-foreground">
 Calculando projeção financeira...
 </div>
 );
 }
 if (isRecovering && !data) {
 return (
 <div className="bg-card rounded-2xl border border-border p-6 flex items-center gap-2 text-sm text-muted-foreground" role="status">
 <Loader2 className="size-4 animate-spin" aria-hidden="true" />
 Restaurando a sessão...
 </div>
 );
 }
 if (isError || !data) {
 const status = getHttpStatus(error);

 if (status === 403) {
 return <FinancialAccessRestricted />;
 }
 if (status === 409) {
 return <div role="alert" className="rounded-2xl border border-status-warning/30 bg-card p-6 text-sm"><p className="font-bold text-foreground">Condições da parceria a revisar</p><p className="mt-2 text-muted-foreground">Solicite ao responsável financeiro a conferência do acordo aceito e do histórico deste negócio. Os valores ficam a apurar até que as condições registradas sejam confirmadas.</p></div>;
 }

 return (
 <div role="alert" className="bg-card rounded-2xl border border-status-critical/30 p-6 flex items-start gap-3 text-sm">
 <AlertCircle className="w-4 h-4 text-status-critical mt-0.5" />
 <div className="min-w-0">
 <p className="font-bold text-status-critical">Não foi possível calcular a projeção</p>
 <p className="text-muted-foreground mt-1">
 Houve uma falha ao carregar os dados financeiros. Tente novamente em instantes.
 </p>
 <Button
 type="button"
 variant="outline"
 size="sm"
 className="mt-4 gap-2"
 disabled={isFetching}
 onClick={() => void refetch()}
 >
 {isFetching ? <Loader2 className="size-3.5 animate-spin" /> : <RefreshCw className="size-3.5" />}
 {isFetching ? "Tentando novamente..." : "Tentar novamente"}
 </Button>
 </div>
 </div>
 );
 }

 // The API serializes empty projection lists as null (a nil Go slice marshals
 // to JSON null), so coalesce every list to [] before reading .length / .map.
 const charges = data.charges ?? [];
 const commissions = data.commissions ?? [];
 const transfers = data.transfers ?? [];
 const notes = data.notes ?? [];
 const composition = data.composition;
 const visibleCharges = showAllCharges ? charges : charges.slice(0, 6);
 const accountRows = composition ? buildAccountRows(data, composition) : [];
 const saleComposition = data.deal_type === "SALE" && composition?.flow === "sale" ? composition : undefined;
 const receivableSummary = saleComposition
 ? {
 label: "Valor da venda",
 amountCents: saleComposition.gross_cents,
 detail: charges.length > 0 ? formatItemCount(charges.length, "cobrança prevista", "cobranças previstas") : "Cronograma pendente",
 }
 : {
 label: "A receber",
 amountCents: data.total_charges_cents,
 detail: formatItemCount(charges.length),
 };
 const ownerSummary = saleComposition
 ? {
 label: "Ao vendedor",
 amountCents: saleComposition.owner_payable_cents,
 detail: "Obrigação prevista",
 }
 : {
 label: "Ao proprietário",
 amountCents: data.total_transfers_cents,
 detail: formatItemCount(transfers.length),
 };

 return (
 <div className="space-y-4">
 {/* Header com totais */}
 <div className="bg-card rounded-2xl border border-border p-5">
 <div className="flex items-start justify-between gap-4">
 <div>
 <h3 className="text-base font-bold text-foreground">Projeção financeira</h3>
 <p className="text-xs text-muted-foreground mt-1">
 Totais estimados para {data.deal_type === "RENT" ? "toda a vigência da locação" : "a operação de venda"}. Nada é contabilizado nesta tela.
 </p>
 </div>
 </div>
 <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 mt-4">
 <SummaryStat icon={Receipt} label={receivableSummary.label} value={fmt(receivableSummary.amountCents)} detail={receivableSummary.detail} />
 <SummaryStat icon={Users} label="Pool de comissão" value={fmt(data.total_commission_cents)} detail={formatItemCount(commissions.length)} />
 <SummaryStat icon={TrendingUp} label={ownerSummary.label} value={fmt(ownerSummary.amountCents)} detail={ownerSummary.detail} />
 </div>
 {saleComposition && charges.length === 0 && (
 <p className="mt-3 text-[11px] leading-relaxed text-muted-foreground">
 O valor da venda e a obrigação ao vendedor são projeções econômicas. Os recebíveis contábeis serão distribuídos somente após a definição do plano de pagamento.
 </p>
 )}
 {notes && notes.length > 0 && (
 <div className="mt-4 space-y-2">
 {notes.map((note, i) => (
 <div key={i} className="flex items-start gap-2 text-xs text-status-warning bg-status-warning-soft border border-status-warning/30 rounded-lg px-3 py-2">
 <AlertCircle className="w-3.5 h-3.5 flex-shrink-0 mt-0.5" />
 <span>{note}</span>
 </div>
 ))}
 </div>
 )}
 </div>

 {composition && (
 <Section icon={Wallet} title={composition.flow === "sale" ? "Destino do valor da venda" : "Destino dos recebimentos na vigência"}>
 <div className="p-4 space-y-3">
 {composition.flow === "sale" ? (
 <>
 <CompositionRow label="Recebimento previsto do comprador" value={fmt(composition.gross_cents)} />
 <CompositionRow label="Obrigação líquida ao proprietário" value={fmt(composition.owner_payable_cents)} tone="strong" />
 <div className="rounded-xl border border-border bg-muted/30 p-3 space-y-2">
 <CompositionRow label="Comissão total da venda" value={fmt(composition.commission_pool_cents)} />
 <div className="border-t border-border pt-2 space-y-2">
 <CompositionRow label="Comissões a corretores" value={fmt(composition.commission_payable_cents)} tone="negative" />
 {composition.partner_payable_cents > 0 && <CompositionRow label="Parceiros, entidades e royalties" value={fmt(composition.partner_payable_cents)} tone="negative" />}
 <CompositionRow label="Receita efetiva da imobiliária" value={fmt(composition.agency_retained_cents)} tone="positive" />
 </div>
 </div>
 <p className="pt-2 text-xs text-muted-foreground border-t border-border">
 O valor do proprietário e a comissão total fecham o recebimento da venda; dentro da comissão, cada obrigação permanece separada da receita da imobiliária.
 </p>
 </>
 ) : (
 <>
 <CompositionRow label="Aluguel recebido" value={fmt(composition.gross_cents)} />
 <CompositionRow label="Receita recorrente · taxa de administração" value={fmt(composition.admin_fee_cents ?? composition.agency_revenue_cents)} tone="positive" />
 {(composition.intermediation_cents ?? 0) > 0 && <CompositionRow label="Intermediação apropriada" value={fmt(composition.intermediation_cents ?? 0)} />}
 {(composition.commission_funded_cents ?? 0) > 0 && <CompositionRow label="Comissão a corretores coberta" value={fmt(composition.commission_funded_cents ?? 0)} tone="negative" />}
 <CompositionRow label="Receita própria líquida da imobiliária" value={fmt(composition.agency_retained_cents)} tone="positive" />
 <CompositionRow label="Repasse líquido ao proprietário" value={fmt(composition.available_cents)} tone="strong" />
 <p className="pt-2 text-xs text-muted-foreground border-t border-border">
 A cada competência, a taxa de administração é apropriada primeiro; depois o sistema cobre a intermediação pendente e libera o saldo do proprietário.
 </p>
 </>
 )}
 </div>
 </Section>
 )}

 {accountRows.length > 0 && (
 <Section icon={BookOpenCheck} title="Classificação gerencial prevista">
 <div className="divide-y divide-border">
 {accountRows.map((row) => (
 <div key={row.code} className="grid grid-cols-[minmax(0,1fr)_auto] gap-3 px-4 py-3 text-sm">
 <div className="min-w-0">
 <div className="flex flex-wrap items-center gap-2">
 <p className="font-medium text-foreground">{row.label}</p>
 <code className="rounded bg-muted px-1.5 py-0.5 text-caption font-semibold text-primary-700">{allocationTypeLabel(row.code)}</code>
 </div>
 <p className="mt-1 text-xs leading-relaxed text-muted-foreground">{row.description}</p>
 </div>
 <div className="text-right">
 <span className="block whitespace-nowrap font-bold tabular-nums text-foreground">{fmt(row.amountCents)}</span>
 <span className="text-caption font-semibold uppercase tracking-wide text-muted-foreground">{row.group}</span>
 </div>
 </div>
 ))}
 </div>
 <p className="border-t border-border bg-muted/20 px-4 py-2.5 text-[11px] leading-relaxed text-muted-foreground">
 As contas preservam a natureza econômica do valor. A classificação não altera contrato, cobrança ou beneficiário.
 </p>
 </Section>
 )}

 {/* Charges */}
 {charges.length > 0 && (
 <Section icon={Receipt} title={`Recebíveis previstos (${charges.length})`}>
 <div className="divide-y divide-border">
 {visibleCharges.map((c) => (
 <div key={c.source_event_id} className="flex items-center justify-between gap-3 py-2 px-3 text-sm">
 <div className="min-w-0 flex-1">
 <p className="font-medium text-foreground truncate">{c.description}</p>
 <p className="text-xs text-muted-foreground">
 <Calendar className="inline w-3 h-3 mr-1" />
 {dateFmt.format(new Date(c.due_date))} · ref {formatCompetencia(c.reference_month)}
 </p>
 </div>
 <span className="font-bold text-foreground whitespace-nowrap">{fmt(c.amount_cents)}</span>
 </div>
 ))}
 {charges.length > 6 && (
 <div className="flex justify-center px-3 py-2.5">
 <button
 type="button"
 onClick={() => setShowAllCharges((current) => !current)}
 className="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-bold text-primary-700 hover:bg-primary/10"
 >
 {showAllCharges ? <ChevronUp className="size-3.5" /> : <ChevronDown className="size-3.5" />}
 {showAllCharges ? "Mostrar resumo" : `Ver todos os ${charges.length} recebíveis`}
 </button>
 </div>
 )}
 </div>
 </Section>
 )}

 {data.partnership_fees && data.partnership_fees.length>0 && <Section icon={Users} title="Taxas do acordo de parceria"><p className="px-3 text-sm text-muted-foreground">Valores fixos por negócio, provisionados na assinatura e liquidados separadamente. Não reduzem a comissão nem o repasse do proprietário.</p>{data.partnership_fees.map(fee=><div key={fee.kind} className="flex items-center justify-between gap-3 px-3 py-2 text-sm"><div><p className="font-medium">{fee.label}</p><p className="text-muted-foreground">{fee.direction==="receivable"?"A receber da parceira de origem":"A pagar à parceira de origem"}</p></div><strong>{fmt(fee.amount_cents)}</strong></div>)}</Section>}
 {/* Commissions */}
 {commissions.length > 0 && (
 <Section icon={Users} title={`Comissões (${commissions.length})`}>
 <div className="divide-y divide-border">
 {commissions.map((c, i) => (
 <div key={`${c.broker_id || "broker"}-${i}`} className="flex items-center justify-between gap-3 py-2 px-3 text-sm">
 <div className="min-w-0 flex-1">
 <p className="font-medium text-foreground">
 {c.retained ? "Receita retida pela imobiliária" : (c.role === "empresa_captadora" ? "Parceira de origem" : c.role) || c.broker_id || "Corretor"}
 </p>
 <p className="text-xs text-muted-foreground">
 {c.percentage.toFixed(1)}% · {c.retained ? "resultado próprio" : "obrigação a pagar"}
 </p>
 </div>
 <span className={`font-bold whitespace-nowrap ${c.retained ? "text-status-ok" : "text-foreground"}`}>{fmt(c.amount_cents)}</span>
 </div>
 ))}
 </div>
 </Section>
 )}

 {/* Transfers (RENT) */}
 {transfers && transfers.length > 0 && (
 <Section icon={TrendingUp} title={`Repasses mensais ao proprietário (${transfers.length})`}>
 <div className="divide-y divide-border">
 {transfers.slice(0, 6).map((t) => (
 <div key={t.reference_month} className="flex items-center justify-between gap-3 py-2 px-3 text-sm">
 <div className="min-w-0 flex-1">
 <p className="font-medium text-foreground">Mês {formatCompetencia(t.reference_month)}</p>
 <p className="text-xs text-muted-foreground">
 Recebido {fmt(t.gross_cents)} · adm. {fmt(t.admin_fee_cents)}
 {t.intermediation_cents > 0 && ` · intermediação ${fmt(t.intermediation_cents)}`}
 </p>
 {t.intermediation_carry_cents > 0 && (
 <p className="mt-1 text-xs font-medium text-status-warning">Saldo de intermediação para o próximo mês: {fmt(t.intermediation_carry_cents)}</p>
 )}
 </div>
 <div className="text-right">
 <span className="block font-bold text-status-ok whitespace-nowrap">{fmt(t.net_cents)}</span>
 {t.net_cents === 0 && <span className="text-[11px] text-muted-foreground">Sem repasse nesta competência</span>}
 </div>
 </div>
 ))}
 {transfers.length > 6 && (
 <p className="text-xs text-muted-foreground px-3 py-2">
 + {transfers.length - 6} repasses adicionais até o fim do contrato.
 </p>
 )}
 </div>
 </Section>
 )}

 {data.gateway_disabled && (
 <p className="text-xs text-muted-foreground text-center">
 Cobranças não receberão checkout do gateway — modo manual ativo.
 </p>
 )}
 </div>
 );
}

interface AccountPreviewRow {
 code: string;
 label: string;
 group: string;
 description: string;
 amountCents: number;
}

function buildAccountRows(data: FinancialPreview, composition: NonNullable<FinancialPreview["composition"]>): AccountPreviewRow[] {
 const isSale = composition.flow === "sale";
 const rows: AccountPreviewRow[] = [
 {
 code: "accounts_receivable",
 label: isSale ? "Recebível do comprador" : "Recebíveis do locatário",
 group: "Ativo",
 description: isSale
 ? "Somente as parcelas em dinheiro do plano; dação e troca permanecem registros não financeiros."
 : "Soma das competências projetadas para toda a vigência.",
 amountCents: data.total_charges_cents,
 },
 {
 code: "owner_payable",
 label: isSale ? "Vendedor a repassar" : "Proprietário a repassar",
 group: "Passivo de terceiros",
 description: "Valor de terceiro separado da receita e da disponibilidade da imobiliária.",
 amountCents: composition.owner_payable_cents,
 },
 ];

 if (composition.commission_payable_cents > 0) {
 rows.push({
 code: "commission_payable",
 label: "Corretores a pagar",
 group: "Passivo de terceiros",
 description: isSale
 ? "Fatia de comissão devida aos corretores da operação."
 : "Intermediação coberta pelos recebimentos sem consumir caixa livre da imobiliária.",
 amountCents: composition.commission_payable_cents,
 });
 }
 if (composition.partner_payable_cents > 0) {
 rows.push({
 code: "partner_payable",
 label: "Parceiros e entidades a pagar",
 group: "Passivo de terceiros",
 description: "Obrigações de parceria e royalties mantidas fora da receita própria.",
 amountCents: composition.partner_payable_cents,
 });
 }
 rows.push({
 code: "revenue",
 label: isSale ? "Receita líquida da imobiliária" : "Receita de administração e intermediação",
 group: "Resultado",
 description: isSale
 ? "Saldo econômico da comissão após obrigações com corretores, parceiros e royalties."
 : "Taxa de administração e parcela própria da intermediação apropriadas pela imobiliária.",
 amountCents: composition.agency_retained_cents,
 });
 return rows;
}

interface SummaryStatProps {
 icon: typeof Receipt;
 label: string;
 value: string;
 detail: string;
}
function SummaryStat({ icon: Icon, label, value, detail }: SummaryStatProps) {
 return (
 <div className="bg-muted/40 rounded-xl border border-border p-3 flex items-center gap-3">
 <div className="w-9 h-9 rounded-lg bg-card border border-border flex items-center justify-center">
 <Icon className="w-4 h-4 text-primary-600" />
 </div>
 <div className="min-w-0">
 <p className="text-caption font-bold uppercase tracking-wider text-muted-foreground">{label}</p>
 <p className="text-sm font-bold text-foreground">{value}</p>
 <p className="text-xs text-muted-foreground">{detail}</p>
 </div>
 </div>
 );
}

function formatItemCount(count: number, singular = "item", plural = "itens") {
 return `${count} ${count === 1 ? singular : plural}`;
}

function CompositionRow({ label, value, tone }: { label: string; value: string; tone?: "positive" | "negative" | "strong" }) {
 const color = tone === "positive" ? "text-status-ok" : tone === "negative" ? "text-status-critical" : tone === "strong" ? "text-primary-700" : "text-foreground";
 return (
 <div className="flex items-center justify-between gap-4 text-sm">
 <span className={tone === "strong" ? "font-bold text-foreground" : "text-muted-foreground"}>{label}</span>
 <span className={`shrink-0 font-bold tabular-nums ${color}`}>{value}</span>
 </div>
 );
}

interface SectionProps {
 icon: typeof Coins;
 title: string;
 children: React.ReactNode;
}
function Section({ icon: Icon, title, children }: SectionProps) {
 return (
 <div className="bg-card rounded-2xl border border-border overflow-hidden">
 <div className="flex items-center gap-2 px-4 py-3 border-b border-border bg-muted/40">
 <Icon className="w-4 h-4 text-muted-foreground" />
 <h4 className="text-sm font-bold text-foreground">{title}</h4>
 </div>
 {children}
 </div>
 );
}
