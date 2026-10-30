import { useEffect, useState, useMemo } from "react";
import { useLocation, useNavigate } from "react-router";
import { ArrowLeft, QrCode, Receipt, Barcode } from "lucide-react";
import { clsx } from "clsx";
import type { PortalCharge } from "../services/portal.service";
import { format, differenceInCalendarDays } from "date-fns";
import { EmptyState } from "../../../shared/ui/EmptyState";
import { PortalError, PortalLoading } from "../components/PortalState";
import { usePortalDeals } from "../context/PortalDealsContext";
import { usePortalPersona } from "../hooks/usePortalPersona";
import { RelationshipSwitcher } from "../components/RelationshipSwitcher";
import { usePortalDealCharges } from "../hooks/useDealResources";
import { Button } from "../../../shared/ui/button";
import { PortalRepassesView } from "../components/PortalRepassesView";
import { PortalChargeBreakdown, civilDayDiff, shouldShowChargeBreakdown } from "../components/PortalChargeBreakdown";
import { PDFDownloadButton } from "../components/PDFDownloadButton";
import { PortalPage } from "../shell/PortalPage";
import { PortalPageHeader } from "../shell/PortalPageHeader";

function formatCurrencyBRL(value: number) {
  if (!Number.isFinite(value)) return "—";
  return value.toLocaleString("pt-BR", { style: "currency", currency: "BRL" });
}

function openPayment(paymentUrl: string) {
  window.open(paymentUrl, "_blank", "noopener,noreferrer");
}

export function PortalFinance() {
  const navigate = useNavigate();
  const location = useLocation();
  // /portal/finance?tab=pagos (menu "Extrato de pagamentos") pré-seleciona o filtro.
  const filterFromUrl = useMemo<"ALL" | "OPEN" | "PAID">(() => {
    try {
      const tab = new URLSearchParams(location.search).get("tab");
      if (tab === "pagos") return "PAID";
      if (tab === "abertos") return "OPEN";
      return "ALL";
    } catch {
      return "ALL";
    }
  }, [location.search]);
  const [filter, setFilter] = useState<"ALL" | "OPEN" | "PAID">(filterFromUrl);

  useEffect(() => {
    setFilter(filterFromUrl);
  }, [filterFromUrl]);

  const selectFilter = (next: "ALL" | "OPEN" | "PAID") => {
    setFilter(next);
    const params = new URLSearchParams(location.search);
    if (next === "PAID") params.set("tab", "pagos");
    else if (next === "OPEN") params.set("tab", "abertos");
    else params.delete("tab");
    navigate({ pathname: location.pathname, search: params.toString() ? `?${params.toString()}` : "" }, { replace: true });
  };
  const { deals, activeDeal, activeDealId, isLoading, error, retry, setActiveDealId } = usePortalDeals();
  const chargesQuery = usePortalDealCharges(activeDeal?.id);
  const charges: PortalCharge[] = useMemo(() => chargesQuery.data ?? [], [chargesQuery.data]);
  const isChargesLoading = chargesQuery.isPending && Boolean(activeDeal?.id);
  const chargesError = chargesQuery.error;

  // Quem anuncia recebe repasses/recebimentos; quem paga vê boletos. A decisão
  // vem do dicionário por persona (papel × tipo do negócio), não de "é owner?".
  const { persona, copy } = usePortalPersona();
  const isOwnerView = Boolean(activeDeal) && persona.isOwnerSide;



  const filteredBills = charges.filter((bill) => {
    if (filter === "OPEN") return bill.status === "PENDING" || bill.status === "OVERDUE";
    if (filter === "PAID") return bill.status === "PAID" || bill.status === "PARTIALLY_PAID";
    return true;
  });

  const nextDue = useMemo(() => {
    const open = charges.filter((c) => c.status === "PENDING" || c.status === "OVERDUE");
    if (open.length === 0) return null;
    return open.reduce((prev, curr) => (new Date(prev.due_date) < new Date(curr.due_date) ? prev : curr));
  }, [charges]);

  if (isOwnerView) {
    return <PortalRepassesView title={copy.repassesTitle} />;
  }

  // Dias civis entre o vencimento e hoje, na MESMA régua do backend (a cobrança
  // vence no dia e só atrasa no seguinte): `differenceInCalendarDays` sobre a
  // data em UTC marcava "vencido" na véspera, porque a meia-noite UTC cai às 21h
  // de Brasília do dia anterior.
  const nextDueDays = nextDue ? -civilDayDiff(new Date(), new Date(nextDue.due_date)) : null;
  const nextDueLabel =
    nextDueDays === null
      ? ""
      : nextDueDays < 0
        ? `Venceu há ${Math.abs(nextDueDays)} ${Math.abs(nextDueDays) === 1 ? "dia" : "dias"}`
        : nextDueDays === 0
          ? "Vence hoje"
          : nextDueDays === 1
            ? "Vence amanhã"
            : `Vence em ${nextDueDays} dias`;

  if (isLoading || isChargesLoading) {
    return (
      <PortalPage>
        <PortalLoading rows={4} />
      </PortalPage>
    );
  }

  if (error && deals.length === 0) {
    return (
      <PortalPage className="px-5 pt-8">
        <PortalError onRetry={retry} />
      </PortalPage>
    );
  }

  if (chargesError) {
    return (
      <PortalPage className="px-5 pt-8">
        <PortalError
          title="Não foi possível carregar"
          description="Erro ao carregar dados financeiros. Tente novamente."
          onRetry={() => chargesQuery.refetch()}
        />
      </PortalPage>
    );
  }

  return (
    <PortalPage>
      <PortalPageHeader title={copy.financeTitle} />

      {deals.length > 1 && (
        <div className="px-5 mt-2">
          <RelationshipSwitcher
            deals={deals}
            leads={[]}
            activeDealId={activeDealId}
            onSelectDeal={setActiveDealId}
            onSelectLead={() => undefined}
            title="Selecionar transação (financeiro)"
          />
        </div>
      )}

      {nextDue ? (
        <section className="px-5 mt-4">
          <div className="bg-[#1f232b] rounded-3xl p-6 shadow-md text-white">
            <p className="text-label text-muted-foreground/70 font-medium mb-1">Próximo vencimento</p>
            <h2 className="text-[36px] font-bold tracking-tight leading-tight mb-1">
              {formatCurrencyBRL(nextDue.total_amount || nextDue.amount)}
            </h2>
            <p className="text-sm text-muted-foreground/70 mb-4">
              {nextDueLabel} —{" "}
              {format(new Date(nextDue.due_date), "dd/MM/yyyy")}
            </p>

            {shouldShowChargeBreakdown(nextDue) && <PortalChargeBreakdown charge={nextDue} tone="dark" className="mb-4" />}

            <p className="text-xs text-muted-foreground/70 leading-relaxed">
              Realize o pagamento conforme as instruções enviadas pela sua imobiliária. O recibo fica disponível aqui assim
              que o pagamento for confirmado.
            </p>

            {nextDue.payment_url && (
              <div className="mt-5 grid grid-cols-2 gap-3">
                <Button
                  onClick={() => openPayment(nextDue.payment_url!)}
                  className="h-11 rounded-xl bg-status-ok hover:bg-status-ok/90 text-white font-semibold text-sm gap-1.5"
                >
                  <QrCode className="w-4 h-4" />
                  Pagar via PIX
                </Button>
                <Button
                  onClick={() => openPayment(nextDue.payment_url!)}
                  variant="outline"
                  className="h-11 rounded-xl bg-white/10 hover:bg-white/15 border border-white/20 text-white font-semibold text-sm gap-1.5"
                >
                  <Barcode className="w-4 h-4" />
                  Ver boleto
                </Button>
              </div>
            )}
          </div>
        </section>
      ) : (
        <section className="px-5 mt-4">
          <div className="bg-muted rounded-3xl p-6 text-center text-muted-foreground">
            <p>Você não tem cobranças pendentes.</p>
          </div>
        </section>
      )}

      <div className="flex items-center gap-2 mt-8 px-5">
        <button
          onClick={() => selectFilter("OPEN")}
          className={clsx(
            "px-5 py-2 rounded-full text-sm font-medium transition-colors",
            filter === "OPEN" ? "bg-[#1f232b] text-white" : "bg-card border border-border text-muted-foreground",
          )}
        >
          Em aberto
        </button>
        <button
          onClick={() => selectFilter("PAID")}
          className={clsx(
            "px-5 py-2 rounded-full text-sm font-medium transition-colors",
            filter === "PAID" ? "bg-[#1f232b] text-white" : "bg-card border border-border text-muted-foreground",
          )}
        >
          Pagos
        </button>
        <button
          onClick={() => selectFilter("ALL")}
          className={clsx(
            "px-5 py-2 rounded-full text-sm font-medium transition-colors",
            filter === "ALL" ? "bg-[#1f232b] text-white" : "bg-card border border-border text-muted-foreground",
          )}
        >
          Todos
        </button>
      </div>

      <section className="px-5 mt-6 space-y-4">
        {filteredBills.map((bill) => (
          <div key={bill.id} className="bg-card border border-border rounded-3xl p-5 shadow-sm">
            <div className="flex items-start justify-between mb-3">
              <div>
                <h3 className="text-title font-bold text-foreground">{bill.description || "Cobrança"}</h3>
                <p className="text-label text-muted-foreground mt-0.5">
                  Vencimento: {format(new Date(bill.due_date), "dd/MM/yyyy")}
                </p>
              </div>
              <span
                className={clsx(
                  "px-2.5 py-1 rounded-md text-caption font-semibold tracking-wide",
                  bill.status === "PENDING" || bill.status === "OVERDUE"
                    ? "bg-muted text-muted-foreground"
                    : "bg-status-ok-soft text-status-ok",
                )}
              >
                {bill.status === "PENDING" || bill.status === "OVERDUE" ? "Em aberto" : "Pago"}
              </span>
            </div>

            <p className="text-[18px] font-bold text-foreground mb-4">{formatCurrencyBRL(bill.total_amount || bill.amount)}</p>

            {shouldShowChargeBreakdown(bill) && (
              <PortalChargeBreakdown charge={bill} showDaysOverdue className="mb-4" />
            )}

            <div className="flex items-center gap-4 flex-wrap">
              {(bill.status === "PENDING" || bill.status === "OVERDUE") &&
                (bill.payment_url ? (
                  <>
                    <Button
                      onClick={() => openPayment(bill.payment_url!)}
                      className="h-9 rounded-xl bg-status-ok hover:bg-status-ok/90 text-white font-semibold text-xs gap-1.5"
                    >
                      <QrCode className="w-3.5 h-3.5" />
                      Pagar via PIX
                    </Button>
                    <Button
                      onClick={() => openPayment(bill.payment_url!)}
                      variant="outline"
                      size="sm"
                      className="h-9 rounded-xl gap-1.5 text-xs font-semibold"
                    >
                      <Barcode className="w-3.5 h-3.5" />
                      2ª via
                    </Button>
                  </>
                ) : (
                  <span className="text-label text-muted-foreground">Pague conforme as instruções da imobiliária.</span>
                ))}
              {bill.status === "PAID" && (
                <PDFDownloadButton
                  url={`/client-portal/charges/${bill.id}/receipt`}
                  filename={`recibo-${bill.id}.pdf`}
                  label="Baixar recibo"
                  variant="ghost"
                  size="sm"
                />
              )}
            </div>
          </div>
        ))}
        {filteredBills.length === 0 && (
          <div className="mt-8">
            <EmptyState icon={Receipt} title="Nenhuma cobrança" message="Nenhuma cobrança encontrada para este filtro." />
          </div>
        )}
      </section>

      <div className="h-4" />
    </PortalPage>
  );
}
