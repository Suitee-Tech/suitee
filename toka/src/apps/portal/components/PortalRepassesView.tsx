import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router";
import { ArrowLeft, Download, Loader2, Receipt as ReceiptIcon } from "lucide-react";
import { clsx } from "clsx";
import { EmptyState } from "../../../shared/ui/EmptyState";
import { PortalError, PortalLoading } from "./PortalState";
import { portalService, type PortalTransfer, type PortalTransferStatus } from "../services/portal.service";
import { PDFDownloadButton } from "./PDFDownloadButton";
import { PortalPage } from "../shell/PortalPage";
import { PortalPageHeader } from "../shell/PortalPageHeader";

function formatCurrencyBRL(value: number): string {
  if (!Number.isFinite(value)) return "—";
  return value.toLocaleString("pt-BR", { style: "currency", currency: "BRL" });
}

function formatReferenceMonth(yyyymm: string): string {
  const [y, m] = yyyymm.split("-");
  const months = ["Jan", "Fev", "Mar", "Abr", "Mai", "Jun", "Jul", "Ago", "Set", "Out", "Nov", "Dez"];
  const idx = parseInt(m, 10) - 1;
  if (idx >= 0 && idx < 12 && y) return `${months[idx]}/${y}`;
  return yyyymm;
}

const STATUS_LABEL: Record<PortalTransferStatus, { label: string; tone: "warn" | "ok" | "fail" }> = {
  PENDING: { label: "Pendente", tone: "warn" },
  APPROVED: { label: "Aprovado", tone: "warn" },
  PROCESSED: { label: "Recebido", tone: "ok" },
  FAILED: { label: "Falhou", tone: "fail" },
  CANCELLED: { label: "Cancelado", tone: "fail" },
};

export function PortalRepassesView({ title = "Repasses recebidos" }: { title?: string } = {}) {
  const navigate = useNavigate();
  const [transfers, setTransfers] = useState<PortalTransfer[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [statusFilter, setStatusFilter] = useState<"" | PortalTransferStatus>("");
  const [yearFilter, setYearFilter] = useState<number | "">("");
  const [downloadingId, setDownloadingId] = useState<string | null>(null);

  const availableYears = useMemo(() => {
    const set = new Set<number>();
    set.add(new Date().getFullYear());
    for (const t of transfers) {
      const y = parseInt((t.reference_month || "").slice(0, 4), 10);
      if (Number.isFinite(y)) set.add(y);
    }
    return Array.from(set).sort((a, b) => b - a);
  }, [transfers]);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    portalService
      .getMyTransfers({
        status: statusFilter || undefined,
        year: typeof yearFilter === "number" ? yearFilter : undefined,
        limit: 100,
      })
      .then((data) => {
        if (cancelled) return;
        setTransfers(Array.isArray(data) ? data : []);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(err?.message || "Erro ao carregar repasses.");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [statusFilter, yearFilter]);

  const handleDownload = async (transfer: PortalTransfer) => {
    setDownloadingId(transfer.id);
    try {
      const url = portalService.getMyTransferReceiptUrl(transfer.id);
      const res = await fetch(url, { credentials: "include" });
      if (!res.ok) {
        throw new Error(`HTTP ${res.status}`);
      }
      const blob = await res.blob();
      const blobUrl = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = blobUrl;
      a.download = `repasse-${transfer.reference_month}-${transfer.id.slice(0, 8)}.pdf`;
      document.body.appendChild(a);
      a.click();
      a.remove();
      URL.revokeObjectURL(blobUrl);
    } catch {
      setError("Erro ao baixar comprovante.");
    } finally {
      setDownloadingId(null);
    }
  };

  if (loading && transfers.length === 0) {
    return (
      <PortalPage>
        <PortalLoading rows={4} />
      </PortalPage>
    );
  }

  if (error && transfers.length === 0) {
    return (
      <PortalPage className="px-5 pt-8">
        <PortalError
          title="Não foi possível carregar"
          description={error}
          onRetry={() => {
            setError(null);
            setLoading(true);
            setStatusFilter((s) => s);
          }}
        />
      </PortalPage>
    );
  }

  return (
    <PortalPage>
      <PortalPageHeader title={title} backTo="/portal/finance" />

      <div className="px-5 mt-2 flex flex-wrap items-center gap-2">
        <button
          onClick={() => setStatusFilter("")}
          className={clsx(
            "px-5 py-2 rounded-full text-sm font-medium transition-colors",
            statusFilter === "" ? "bg-[#1f232b] text-white" : "bg-card border border-border text-muted-foreground",
          )}
        >
          Todos
        </button>
        <button
          onClick={() => setStatusFilter("PROCESSED")}
          className={clsx(
            "px-5 py-2 rounded-full text-sm font-medium transition-colors",
            statusFilter === "PROCESSED"
              ? "bg-[#1f232b] text-white"
              : "bg-card border border-border text-muted-foreground",
          )}
        >
          Recebidos
        </button>
        <button
          onClick={() => setStatusFilter("PENDING")}
          className={clsx(
            "px-5 py-2 rounded-full text-sm font-medium transition-colors",
            statusFilter === "PENDING" ? "bg-[#1f232b] text-white" : "bg-card border border-border text-muted-foreground",
          )}
        >
          Pendentes
        </button>
        <select
          value={yearFilter}
          onChange={(e) => setYearFilter(e.target.value ? Number(e.target.value) : "")}
          className="ml-auto px-3 py-2 rounded-full bg-card border border-border text-sm font-medium text-muted-foreground"
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

      {transfers.length > 0 && (
        <section className="px-5 mt-4">
          <SummaryCard transfers={transfers} />
        </section>
      )}

      <section className="px-5 mt-6 space-y-4">
        {loading && transfers.length > 0 && (
          <div className="text-center py-2 text-caption text-muted-foreground inline-flex items-center gap-2 justify-center w-full">
            <Loader2 className="w-3.5 h-3.5 animate-spin" /> Atualizando…
          </div>
        )}

        {!loading && transfers.length === 0 && (
          <div className="mt-8">
            <EmptyState
              icon={ReceiptIcon}
              title="Nenhum repasse encontrado"
              message={
                statusFilter || yearFilter
                  ? "Nenhum repasse para os filtros selecionados."
                  : "Você ainda não recebeu repasses. Eles aparecerão aqui após o primeiro processamento."
              }
            />
          </div>
        )}

        {transfers.map((t) => (
          <TransferCard
            key={t.id}
            transfer={t}
            downloading={downloadingId === t.id}
            onDownload={() => handleDownload(t)}
          />
        ))}
      </section>

      <div className="h-4" />
    </PortalPage>
  );
}

function SummaryCard({ transfers }: { transfers: PortalTransfer[] }) {
  const totals = useMemo(() => {
    let processed = 0;
    let processedCount = 0;
    let pending = 0;
    let pendingCount = 0;
    for (const t of transfers) {
      if (t.status === "PROCESSED") {
        processed += t.net_amount;
        processedCount++;
      } else if (t.status === "PENDING" || t.status === "APPROVED") {
        pending += t.net_amount;
        pendingCount++;
      }
    }
    return { processed, processedCount, pending, pendingCount };
  }, [transfers]);

  return (
    <div className="bg-[#1f232b] rounded-3xl p-6 shadow-md text-white">
      <p className="text-label text-muted-foreground/70 font-medium mb-1">Total recebido (filtro atual)</p>
      <h2 className="text-[36px] font-bold tracking-tight leading-tight mb-1">{formatCurrencyBRL(totals.processed)}</h2>
      <p className="text-sm text-muted-foreground/70">
        {totals.processedCount} repasse{totals.processedCount === 1 ? "" : "s"} processado
        {totals.processedCount === 1 ? "" : "s"}
        {totals.pendingCount > 0 && (
          <>
            {" · "}
            <span className="text-status-warning">
              {formatCurrencyBRL(totals.pending)} pendente em {totals.pendingCount} repasse
              {totals.pendingCount === 1 ? "" : "s"}
            </span>
          </>
        )}
      </p>
    </div>
  );
}

interface TransferCardProps {
  transfer: PortalTransfer;
  downloading: boolean;
  onDownload: () => void;
}

function TransferCard({ transfer, downloading, onDownload }: TransferCardProps) {
  const statusCfg = STATUS_LABEL[transfer.status] || { label: transfer.status, tone: "warn" as const };
  const statusClass = {
    ok: "bg-status-ok-soft text-status-ok",
    warn: "bg-status-warning-soft text-status-warning",
    fail: "bg-status-critical-soft text-status-critical",
  }[statusCfg.tone];

  return (
    <div className="bg-card border border-border rounded-3xl p-5 shadow-sm">
      <div className="flex items-start justify-between mb-3">
        <div>
          <h3 className="text-title font-bold text-foreground">{formatReferenceMonth(transfer.reference_month)}</h3>
          {transfer.processed_at && (
            <p className="text-label text-muted-foreground mt-0.5">
              Processado em {new Date(transfer.processed_at).toLocaleDateString("pt-BR")}
            </p>
          )}
        </div>
        <span className={clsx("px-2.5 py-1 rounded-md text-caption font-semibold tracking-wide", statusClass)}>
          {statusCfg.label}
        </span>
      </div>

      <p className="text-[24px] font-bold text-foreground">{formatCurrencyBRL(transfer.net_amount)}</p>
      <p className="text-caption text-muted-foreground mb-3">Valor líquido</p>

      <dl className="grid grid-cols-2 gap-2 text-caption mb-4">
        <BreakdownRow label="Aluguel bruto" value={transfer.gross_rent} positive />
        <BreakdownRow label={`Taxa adm (${transfer.admin_fee_percent.toFixed(1)}%)`} value={-transfer.admin_fee_amount} />
        {intermediationAmount(transfer) > 0 && (
          <BreakdownRow label="Intermediação (comissão do 1º aluguel)" value={-intermediationAmount(transfer)} />
        )}
        {transfer.maintenance_deductions > 0 && <BreakdownRow label="Manutenções" value={-transfer.maintenance_deductions} />}
        {transfer.other_deductions > 0 && <BreakdownRow label="Outras deduções" value={-transfer.other_deductions} />}
      </dl>

      {/* Sem esta nota o proprietário via "aluguel − taxa adm" e não fechava com
          o valor líquido: a comissão do primeiro aluguel é diluída nos
          primeiros meses e sai do próprio aluguel. */}
      {intermediationAmount(transfer) > 0 && (
        <p className="text-caption text-muted-foreground mb-4">
          A intermediação é descontada do próprio aluguel nos primeiros meses da locação
          {intermediationCommissionFunded(transfer) > 0
            ? `; ${formatCurrencyBRL(intermediationCommissionFunded(transfer))} foram para comissões de corretores.`
            : "."}
        </p>
      )}

      <div className="space-y-2">
        <button
          onClick={onDownload}
          disabled={downloading}
          className="w-full inline-flex items-center justify-center gap-2 py-2.5 rounded-xl bg-card border border-border hover:bg-muted text-sm font-bold text-foreground disabled:opacity-50 transition-colors"
        >
          {downloading ? <Loader2 className="w-4 h-4 animate-spin" /> : <Download className="w-4 h-4" />}
          Baixar comprovante PDF
        </button>
        {transfer.reference_month &&
          (() => {
            const [y, m] = transfer.reference_month.split("-");
            if (!y || !m) return null;
            return (
              <PDFDownloadButton
                url={`/client-portal/owner-statements?year=${y}&month=${parseInt(m, 10)}&format=pdf`}
                filename={`demonstrativo-${transfer.reference_month}.pdf`}
                label={`Demonstrativo de ${formatReferenceMonth(transfer.reference_month)}`}
                variant="outline"
                size="sm"
                className="w-full"
              />
            );
          })()}
      </div>
    </div>
  );
}

/** Intermediação descontada neste repasse (0 quando não se aplica). */
function intermediationAmount(transfer: PortalTransfer): number {
  return (transfer.economic_breakdown?.intermediation_revenue_cents ?? 0) / 100;
}

function intermediationCommissionFunded(transfer: PortalTransfer): number {
  return (transfer.economic_breakdown?.commission_funded_cents ?? 0) / 100;
}

function BreakdownRow({ label, value, positive }: { label: string; value: number; positive?: boolean }) {
  return (
    <>
      <dt className="text-muted-foreground">{label}</dt>
      <dd className={clsx("text-right font-medium tabular-nums", positive ? "text-status-ok" : "text-foreground")}>
        {value < 0 ? "-" : ""}
        {formatCurrencyBRL(Math.abs(value))}
      </dd>
    </>
  );
}

