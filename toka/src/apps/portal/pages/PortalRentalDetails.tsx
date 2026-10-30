import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { ArrowLeft, Building2, Calendar, DollarSign, Home, Loader2, Wallet, Wrench } from "lucide-react";
import { clsx } from "clsx";
import { Button } from "../../../shared/ui/button";
import { PortalEmpty, PortalError } from "../components/PortalState";
import { portalService, type PortalRental, type PortalRentalDTO } from "../services/portal.service";
import { formatCurrencyBRL0 } from "../../../shared/utils/format";
import { PortalPage } from "../shell/PortalPage";
import { PortalPageHeader } from "../shell/PortalPageHeader";
import { rentalRoleLabel } from "../lib/portalPersona";

export function PortalRentalDetails() {
  const navigate = useNavigate();
  const { id } = useParams<{ id: string }>();
  const [item, setItem] = useState<PortalRentalDTO | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!id) return;
    let cancelled = false;
    setLoading(true);
    portalService
      .getMyRentals()
      .then((data) => {
        if (cancelled) return;
        const found = (data ?? []).find((d) => d.rental.id === id);
        setItem(found ?? null);
        setLoading(false);
      })
      .catch((e) => {
        if (!cancelled) {
          setError(e instanceof Error ? e.message : "Erro ao carregar locação");
          setLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [id]);

  if (loading) {
    return (
      <PortalPage className="flex items-center justify-center">
        <Loader2 className="w-5 h-5 animate-spin text-muted-foreground" />
      </PortalPage>
    );
  }

  if (error) {
    return (
      <PortalPage className="px-5 pt-8">
        <PortalError description={error} />
      </PortalPage>
    );
  }

  if (!item) {
    return (
      <PortalPage className="px-5 pt-8">
        <PortalEmpty icon={Home} title="Locação não encontrada" description="Esta locação não está vinculada à sua conta." />
      </PortalPage>
    );
  }

  const { rental: r, my_role } = item;

  return (
    <PortalPage>
      <PortalPageHeader
        title={`Locação #${r.id.slice(0, 8)}`}
        subtitle={rentalRoleLabel(my_role)}
      />
      <div className="px-5 pt-4 space-y-4">
        <RentalSummaryCard rental={r} />

        <Link to={`/portal/finance?rental_id=${r.id}`} className="block bg-card rounded-2xl border border-border p-4 hover:border-primary-300 transition-all">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-xl bg-primary-50 flex items-center justify-center">
              <Wallet className="w-5 h-5 text-primary-600" />
            </div>
            <div className="flex-1 min-w-0">
              <p className="text-sm font-bold text-foreground">Cobranças</p>
              <p className="text-xs text-muted-foreground">Ver boletos, vencimentos e pagamentos</p>
            </div>
            <span className="text-muted-foreground">›</span>
          </div>
        </Link>

        <Link to={`/portal/chamados?rental_id=${r.id}`} className="block bg-card rounded-2xl border border-border p-4 hover:border-primary-300 transition-all">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-xl bg-primary-50 flex items-center justify-center">
              <Wrench className="w-5 h-5 text-primary-600" />
            </div>
            <div className="flex-1 min-w-0">
              <p className="text-sm font-bold text-foreground">Manutenção</p>
              <p className="text-xs text-muted-foreground">Solicitações e tickets do imóvel</p>
            </div>
            <span className="text-muted-foreground">›</span>
          </div>
        </Link>

        {r.deal_id && (
          <Link to={`/portal/deals/${r.deal_id}`} className="block bg-card rounded-2xl border border-border p-4 hover:border-primary-300 transition-all">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 rounded-xl bg-primary-50 flex items-center justify-center">
                <Building2 className="w-5 h-5 text-primary-600" />
              </div>
              <div className="flex-1 min-w-0">
                <p className="text-sm font-bold text-foreground">Negócio</p>
                <p className="text-xs text-muted-foreground">Detalhes da negociação que originou esta locação</p>
              </div>
              <span className="text-muted-foreground">›</span>
            </div>
          </Link>
        )}
      </div>
    </PortalPage>
  );
}

function RentalSummaryCard({ rental: r }: { rental: PortalRental }) {
  const monthly = (r.monthly_value_cents || 0) / 100;
  return (
    <div className="bg-card rounded-2xl border border-border p-5">
      <div className="flex items-center justify-between mb-4">
        <p className="text-sm font-bold text-foreground uppercase tracking-wide">Resumo</p>
        <span
          className={clsx(
            "px-2.5 py-1 text-xs font-bold rounded-full",
            r.status === "ACTIVE"
              ? "bg-status-ok-soft text-status-ok"
              : r.status === "ENDED"
                ? "bg-muted text-muted-foreground"
                : "bg-status-critical-soft text-status-critical",
          )}
        >
          {r.status === "ACTIVE" ? "Ativa" : r.status === "ENDED" ? "Encerrada" : "Rescindida"}
        </span>
      </div>
      <div className="grid grid-cols-2 gap-4">
        <Field icon={DollarSign} label="Aluguel" value={formatCurrencyBRL0(monthly)} />
        <Field icon={Calendar} label="Início" value={r.start_date ? new Date(r.start_date).toLocaleDateString("pt-BR") : "—"} />
        <Field icon={Calendar} label="Fim previsto" value={r.end_date ? new Date(r.end_date).toLocaleDateString("pt-BR") : "—"} />
        <Field icon={Building2} label="Imóvel" value={`#${r.property_id.slice(0, 8)}`} />
      </div>
    </div>
  );
}

function Field({ icon: Icon, label, value }: { icon: typeof Home; label: string; value: string }) {
  return (
    <div>
      <p className="text-caption uppercase tracking-wider text-muted-foreground font-bold mb-0.5 flex items-center gap-1">
        <Icon className="w-3 h-3" /> {label}
      </p>
      <p className="text-sm font-bold text-foreground">{value}</p>
    </div>
  );
}
