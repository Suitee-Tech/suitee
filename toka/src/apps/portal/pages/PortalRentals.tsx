import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router";
import { ArrowLeft, Calendar, DollarSign, Home, Loader2 } from "lucide-react";
import { clsx } from "clsx";
import { Button } from "../../../shared/ui/button";
import { PortalEmpty, PortalError } from "../components/PortalState";
import { portalService, type PortalRentalDTO } from "../services/portal.service";
import { formatCurrencyBRL0 } from "../../../shared/utils/format";
import { PortalPage } from "../shell/PortalPage";
import { PortalPageHeader } from "../shell/PortalPageHeader";
import { AgencyIdentity } from "../components/AgencyIdentity";

export function PortalRentals() {
  const navigate = useNavigate();
  const [items, setItems] = useState<PortalRentalDTO[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    portalService
      .getMyRentals()
      .then((data) => {
        if (!cancelled) {
          setItems(data ?? []);
          setLoading(false);
        }
      })
      .catch((e) => {
        if (!cancelled) {
          setError(e instanceof Error ? e.message : "Erro ao carregar locações");
          setLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const tenantOf = items.filter((i) => i.my_role === "tenant" || i.my_role === "both");
  const ownerOf = items.filter((i) => i.my_role === "owner" || i.my_role === "both");

  return (
    <PortalPage>
      <PortalPageHeader title="Minhas Locações" />

      <div className="px-5 pt-4 space-y-6">
        {loading && (
          <div className="flex items-center justify-center py-10 text-muted-foreground">
            <Loader2 className="w-4 h-4 animate-spin mr-2" /> Carregando…
          </div>
        )}

        {error && <PortalError description={error} />}

        {!loading && !error && items.length === 0 && (
          <PortalEmpty icon={Home} title="Sem locações" description="Você ainda não tem locações vinculadas à sua conta." />
        )}

        {tenantOf.length > 0 && (
          <RentalSection
            title="Como locatário"
            subtitle="Locações em que você mora ou usa o imóvel"
            items={tenantOf}
            roleLabel="Locatário"
          />
        )}

        {ownerOf.length > 0 && (
          <RentalSection
            title="Como locador"
            subtitle="Imóveis seus que estão alugados"
            items={ownerOf}
            roleLabel="Locador"
          />
        )}
      </div>
    </PortalPage>
  );
}

function RentalSection({
  title,
  subtitle,
  items,
  roleLabel,
}: {
  title: string;
  subtitle: string;
  items: PortalRentalDTO[];
  roleLabel: string;
}) {
  return (
    <section>
      <div className="mb-3">
        <h2 className="text-sm font-bold text-foreground uppercase tracking-wide">{title}</h2>
        <p className="text-xs text-muted-foreground mt-0.5">{subtitle}</p>
      </div>
      <div className="space-y-3">
        {items.map(({ rental: r }) => (
          <Link
            key={r.id}
            to={`/portal/locacao/${r.id}`}
            className="block bg-card rounded-2xl border border-border p-4 hover:border-primary-300 hover:shadow-md transition-all"
          >
            <div className="flex items-center justify-between gap-3 mb-3">
              <div className="min-w-0 flex-1">
                <p className="text-base font-bold text-foreground truncate">Locação #{r.id.slice(0, 8)}</p>
                <p className="text-[11px] text-muted-foreground uppercase tracking-wide font-bold mt-0.5">{roleLabel}</p>
                <div className="mt-2"><AgencyIdentity tenantId={r.tenant_id} compact /></div>
              </div>
              <span
                className={clsx(
                  "px-2.5 py-1 text-xs font-bold rounded-full shrink-0",
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
            <div className="grid grid-cols-2 gap-3 text-sm">
              <Fact icon={DollarSign} label="Aluguel mensal" value={formatCurrencyBRL0((r.monthly_value_cents || 0) / 100)} />
              <Fact icon={Calendar} label="Início" value={r.start_date ? new Date(r.start_date).toLocaleDateString("pt-BR") : "—"} />
            </div>
          </Link>
        ))}
      </div>
    </section>
  );
}

function Fact({ icon: Icon, label, value }: { icon: typeof Home; label: string; value: string }) {
  return (
    <div>
      <p className="text-caption uppercase tracking-wider text-muted-foreground font-bold mb-0.5 flex items-center gap-1">
        <Icon className="w-3 h-3" /> {label}
      </p>
      <p className="text-sm font-bold text-foreground truncate">{value}</p>
    </div>
  );
}
