import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router";
import { ArrowLeft, Download, FileBarChart, Loader2 } from "lucide-react";
import { clsx } from "clsx";
import { EmptyState } from "../../../shared/ui/EmptyState";
import { PortalError, PortalLoading } from "../components/PortalState";
import { portalService, type PortalInforme } from "../services/portal.service";
import { PortalPage } from "../shell/PortalPage";
import { PortalPageHeader } from "../shell/PortalPageHeader";

function formatCurrencyBRL(value: number): string {
  if (!Number.isFinite(value)) return "—";
  return value.toLocaleString("pt-BR", { style: "currency", currency: "BRL" });
}

const STATUS_LABEL: Record<string, { label: string; tone: "warn" | "ok" | "fail" }> = {
  READY: { label: "Disponível", tone: "ok" },
  PENDING: { label: "Em processamento", tone: "warn" },
  GENERATING: { label: "Em processamento", tone: "warn" },
  ERROR: { label: "Erro", tone: "fail" },
};

export function PortalInformeRendimentos() {
  const navigate = useNavigate();
  const [informes, setInformes] = useState<PortalInforme[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [yearFilter, setYearFilter] = useState<number | "">("");
  const [downloadingId, setDownloadingId] = useState<string | null>(null);

  const availableYears = useMemo(() => {
    const set = new Set<number>();
    set.add(new Date().getFullYear() - 1);
    for (const inf of informes) {
      if (Number.isFinite(inf.year)) set.add(inf.year);
    }
    return Array.from(set).sort((a, b) => b - a);
  }, [informes]);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    portalService
      .getMyInformes(typeof yearFilter === "number" ? yearFilter : undefined)
      .then((data) => {
        if (cancelled) return;
        setInformes(Array.isArray(data) ? data : []);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(err?.message || "Erro ao carregar informes.");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [yearFilter]);

  const handleDownload = async (informe: PortalInforme) => {
    setDownloadingId(informe.id);
    try {
      const url = portalService.getMyInformePdfUrl(informe.id);
      const res = await fetch(url, { credentials: "include" });
      if (!res.ok) {
        throw new Error(`HTTP ${res.status}`);
      }
      const blob = await res.blob();
      const blobUrl = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = blobUrl;
      a.download = `informe-rendimentos-${informe.year}.pdf`;
      document.body.appendChild(a);
      a.click();
      a.remove();
      URL.revokeObjectURL(blobUrl);
    } catch {
      setError("Erro ao baixar o informe.");
    } finally {
      setDownloadingId(null);
    }
  };

  if (loading && informes.length === 0) {
    return (
      <PortalPage>
        <PortalLoading rows={4} />
      </PortalPage>
    );
  }

  if (error && informes.length === 0) {
    return (
      <PortalPage className="px-5 pt-8">
        <PortalError
          title="Não foi possível carregar"
          description={error}
          onRetry={() => {
            setError(null);
            setLoading(true);
            setYearFilter((y) => y);
          }}
        />
      </PortalPage>
    );
  }

  return (
    <PortalPage>
      <PortalPageHeader title="Declaração para IR" backTo="/portal/menu" className="-mx-5 px-5" />

      <p className="px-5 text-label text-muted-foreground">
        Informe de Rendimentos anual dos seus imóveis administrados — use na sua declaração de Imposto de Renda.
      </p>

      <div className="px-5 mt-4">
        <select
          value={yearFilter}
          onChange={(e) => setYearFilter(e.target.value ? Number(e.target.value) : "")}
          className="px-3 py-2 rounded-full bg-card border border-border text-sm font-medium text-muted-foreground"
          aria-label="Filtrar por ano"
        >
          <option value="">Todos os anos</option>
          {availableYears.map((y) => (
            <option key={y} value={y}>
              {y}
            </option>
          ))}
        </select>
      </div>

      <section className="px-5 mt-6 space-y-4">
        {loading && informes.length > 0 && (
          <div className="text-center py-2 text-caption text-muted-foreground inline-flex items-center gap-2 justify-center w-full">
            <Loader2 className="w-3.5 h-3.5 animate-spin" /> Atualizando…
          </div>
        )}

        {!loading && informes.length === 0 && (
          <div className="mt-8">
            <EmptyState
              icon={FileBarChart}
              title="Nenhum informe disponível"
              message={
                yearFilter
                  ? "Nenhum informe para o ano selecionado."
                  : "Seus informes de rendimentos aparecerão aqui após serem gerados pela administradora."
              }
            />
          </div>
        )}

        {informes.map((inf) => (
          <InformeCard
            key={inf.id}
            informe={inf}
            downloading={downloadingId === inf.id}
            onDownload={() => handleDownload(inf)}
          />
        ))}
      </section>

      <div className="h-4" />
    </PortalPage>
  );
}

/** Informes gerados antes da separação por fluxo (T-06) caem nos totais legados. */
function hasFlowBreakdown(informe: PortalInforme): boolean {
  return (informe.rental_count ?? 0) > 0 || (informe.sale_count ?? 0) > 0;
}

interface InformeCardProps {
  informe: PortalInforme;
  downloading: boolean;
  onDownload: () => void;
}

function InformeCard({ informe, downloading, onDownload }: InformeCardProps) {
  const statusCfg = STATUS_LABEL[informe.status] || { label: informe.status, tone: "warn" as const };
  const statusClass = {
    ok: "bg-status-ok-soft text-status-ok",
    warn: "bg-status-warning-soft text-status-warning",
    fail: "bg-status-critical-soft text-status-critical",
  }[statusCfg.tone];

  return (
    <div className="bg-card border border-border rounded-3xl p-5 shadow-sm">
      <div className="flex items-start justify-between mb-3">
        <div>
          <h3 className="text-title font-bold text-foreground">Informe {informe.year}</h3>
          {informe.generated_at && (
            <p className="text-label text-muted-foreground mt-0.5">
              Gerado em {new Date(informe.generated_at).toLocaleDateString("pt-BR")}
            </p>
          )}
        </div>
        <span className={clsx("px-2.5 py-1 rounded-md text-caption font-semibold tracking-wide", statusClass)}>{statusCfg.label}</span>
      </div>

      <p className="text-[24px] font-bold text-foreground">{formatCurrencyBRL(informe.total_net)}</p>
      <p className="text-caption text-muted-foreground mb-3">Valor líquido repassado no ano</p>

      {/* T-06: locação (renda tributável) e venda (repasse de capital) são
          apresentadas separadas — somar as duas num "aluguel bruto" fazia um
          repasse de venda de milhões parecer rendimento de aluguel. */}
      {hasFlowBreakdown(informe) ? (
        <dl className="grid grid-cols-2 gap-2 text-caption mb-4">
          <dt className="col-span-2 font-semibold text-foreground">Locação</dt>
          <dt className="text-muted-foreground">Aluguel bruto</dt>
          <dd className="text-right font-medium tabular-nums text-foreground">
            {formatCurrencyBRL(informe.rental_gross ?? 0)}
          </dd>
          <dt className="text-muted-foreground">Taxa de administração</dt>
          <dd className="text-right font-medium tabular-nums text-foreground">
            {formatCurrencyBRL(informe.rental_admin_fee ?? 0)}
          </dd>
          <dt className="text-muted-foreground">Intermediação de locação</dt>
          <dd className="text-right font-medium tabular-nums text-foreground">
            {formatCurrencyBRL(informe.rental_intermediation ?? 0)}
          </dd>
          {(informe.sale_count ?? 0) > 0 && (
            <>
              <dt className="col-span-2 mt-1 font-semibold text-foreground">Vendas do ano</dt>
              <dt className="text-muted-foreground">Valor das vendas</dt>
              <dd className="text-right font-medium tabular-nums text-foreground">
                {formatCurrencyBRL(informe.sale_gross ?? 0)}
              </dd>
              <dt className="text-muted-foreground">Comissão de intermediação</dt>
              <dd className="text-right font-medium tabular-nums text-foreground">
                {formatCurrencyBRL(informe.sale_intermediation ?? 0)}
              </dd>
            </>
          )}
        </dl>
      ) : (
        <dl className="grid grid-cols-2 gap-2 text-caption mb-4">
          <dt className="text-muted-foreground">Aluguel bruto</dt>
          <dd className="text-right font-medium tabular-nums text-foreground">{formatCurrencyBRL(informe.total_gross)}</dd>
          <dt className="text-muted-foreground">Taxa de administração</dt>
          <dd className="text-right font-medium tabular-nums text-foreground">{formatCurrencyBRL(informe.total_admin_fee)}</dd>
          {/* T-05: linha de intermediação imobiliária — explica o repasse menor
              que o bruto junto com a taxa de administração. */}
          <dt className="text-muted-foreground">Intermediação imobiliária</dt>
          <dd className="text-right font-medium tabular-nums text-foreground">{formatCurrencyBRL(informe.total_intermediation)}</dd>
        </dl>
      )}

      <button
        onClick={onDownload}
        disabled={downloading}
        className="w-full inline-flex items-center justify-center gap-2 py-2.5 rounded-xl bg-card border border-border hover:bg-muted text-sm font-bold text-foreground disabled:opacity-50 transition-colors"
      >
        {downloading ? <Loader2 className="w-4 h-4 animate-spin" /> : <Download className="w-4 h-4" />}
        Baixar informe PDF
      </button>
    </div>
  );
}
