import { useMemo, useState, type ReactNode } from "react";
import { useNavigate } from "react-router";
import {
  FileSignature,
  FileText,
  Search,
  CheckCircle2,
  Clock,
  Building2,
  ChevronRight,
  CircleSlash,
} from "lucide-react";
import { clsx } from "clsx";
import { motion } from "motion/react";
import { useQuery } from "@tanstack/react-query";

import { EmptyState } from "../ui/EmptyState";
import { PageState } from "../ui/page-state";
import { PageHeader } from "../ui/page-header";
import { useFocusMode } from "../context/FocusModeContext";
import { contractService } from "../services/contract.service";
import { Contract, ContractStatus } from "../contracts/types";
import { formatCurrencyBRL } from "../utils/format";

type StatusMeta = { label: string; badgeClass: string };

const STATUS_META: Record<string, StatusMeta> = {
  [ContractStatus.DRAFT]: {
    label: "Rascunho",
    badgeClass: "bg-muted text-muted-foreground border-border",
  },
  [ContractStatus.PENDING_SIGNATURE]: {
    label: "Em assinatura",
    badgeClass: "bg-status-warning-soft text-status-warning border-status-warning/30",
  },
  [ContractStatus.PARTIALLY_SIGNED]: {
    label: "Parcialmente assinado",
    badgeClass: "bg-status-info-soft text-status-info border-status-info/30",
  },
  [ContractStatus.SIGNED]: {
    label: "Assinado",
    badgeClass: "bg-status-info-soft text-status-info border-status-info/30",
  },
  [ContractStatus.ACTIVE]: {
    label: "Vigente",
    badgeClass: "bg-status-ok-soft text-status-ok border-status-ok/30",
  },
  [ContractStatus.EXPIRED]: {
    label: "Expirado",
    badgeClass: "bg-muted text-muted-foreground border-border",
  },
  [ContractStatus.TERMINATED]: {
    label: "Encerrado",
    badgeClass: "bg-status-critical-soft text-status-critical border-status-critical/30",
  },
};

function statusMetaFor(status: string): StatusMeta {
  return (
    STATUS_META[(status || "").toUpperCase()] ?? {
      label: status || "—",
      badgeClass: "bg-muted text-muted-foreground border-border",
    }
  );
}

function contractTypeLabel(type?: string): string | null {
  if (!type) return null;
  const t = type.toUpperCase();
  if (t === "SALE" || t === "VENDA") return "Venda";
  if (t === "RENTAL" || t === "LEASE" || t === "LOCACAO" || t === "LOCAÇÃO") return "Locação";
  return type;
}

function formatDateBR(iso?: string): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime()) || d.getFullYear() < 1970) return "—";
  return d.toLocaleDateString("pt-BR");
}

const FILTERS: { key: string; label: string }[] = [
  { key: "ALL", label: "Todos" },
  { key: ContractStatus.DRAFT, label: "Rascunho" },
  { key: ContractStatus.PENDING_SIGNATURE, label: "Em assinatura" },
  { key: ContractStatus.SIGNED, label: "Assinados" },
  { key: ContractStatus.ACTIVE, label: "Vigentes" },
  { key: ContractStatus.TERMINATED, label: "Encerrados" },
];

export function Contracts() {
  const navigate = useNavigate();
  const { focusMode } = useFocusMode();
  const [searchTerm, setSearchTerm] = useState("");
  const [statusFilter, setStatusFilter] = useState<string>("ALL");

  const contractsQuery = useQuery({
    queryKey: ["contracts", "list"],
    queryFn: () => contractService.getAll(),
    staleTime: 30_000,
  });

  const contracts = useMemo(() => contractsQuery.data ?? [], [contractsQuery.data]);

  const stats = useMemo(() => {
    const up = (s: string) => (s || "").toUpperCase();
    return {
      total: contracts.length,
      // O parcial conta como "em assinatura": o contrato está no meio do
      // processo, não concluído — antes ele sumia dos dois lados da contagem.
      pending: contracts.filter((c) =>
        [ContractStatus.PENDING_SIGNATURE, ContractStatus.PARTIALLY_SIGNED].includes(up(c.status) as ContractStatus),
      ).length,
      active: contracts.filter((c) =>
        [ContractStatus.SIGNED, ContractStatus.ACTIVE].includes(up(c.status) as ContractStatus),
      ).length,
      closed: contracts.filter((c) =>
        [ContractStatus.EXPIRED, ContractStatus.TERMINATED].includes(up(c.status) as ContractStatus),
      ).length,
    };
  }, [contracts]);

  const visible = useMemo(() => {
    const term = searchTerm.trim().toLowerCase();
    return contracts
      .filter((c) => statusFilter === "ALL" || (c.status || "").toUpperCase() === statusFilter)
      .filter((c) => {
        if (!term) return true;
        return [c.display_id, c.client, c.property_title, c.id]
          .filter(Boolean)
          .some((v) => String(v).toLowerCase().includes(term));
      })
      .slice()
      .sort(
        (a, b) =>
          new Date(b.updated_at || b.created_at || 0).getTime() -
          new Date(a.updated_at || a.created_at || 0).getTime(),
      );
  }, [contracts, searchTerm, statusFilter]);

  return (
    <div className="h-full flex flex-col overflow-hidden">
      <div className="max-w-7xl w-full mx-auto p-6 lg:p-8 space-y-6 flex-1 overflow-y-auto">
        <PageHeader
          title="Contratos"
          description="Todos os contratos da carteira — minutas, em assinatura, vigentes e encerrados."
        />

        <PageState
          loading={contractsQuery.isLoading && contracts.length === 0}
          error={contractsQuery.isError && contracts.length === 0 ? "A leitura da carteira de contratos falhou. A lista não foi tratada como vazia." : undefined}
          onRetry={() => contractsQuery.refetch()}
          retrying={contractsQuery.isFetching}
          skeleton={<ListSkeleton />}
        >
          <>
            {!focusMode && (
              <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
                <StatCard icon={<FileText className="w-4 h-4 text-primary-600" />} label="Total" value={stats.total} />
                <StatCard icon={<Clock className="w-4 h-4 text-status-warning" />} label="Em assinatura" value={stats.pending} />
                <StatCard icon={<CheckCircle2 className="w-4 h-4 text-status-ok" />} label="Vigentes" value={stats.active} />
                <StatCard icon={<CircleSlash className="w-4 h-4 text-muted-foreground" />} label="Encerrados" value={stats.closed} />
              </div>
            )}

            <div className="bg-card p-4 rounded-xl border border-border shadow-sm space-y-3">
              <div className="relative">
                <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
                <input
                  type="text"
                  value={searchTerm}
                  onChange={(e) => setSearchTerm(e.target.value)}
                  placeholder="Buscar por código, cliente ou imóvel..."
                  className="w-full pl-9 pr-4 py-2 bg-muted border border-border rounded-lg text-sm focus:ring-2 focus:ring-primary-500 outline-none"
                />
              </div>
              <div className="flex flex-wrap gap-2">
                {FILTERS.map((f) => (
                  <button
                    key={f.key}
                    type="button"
                    onClick={() => setStatusFilter(f.key)}
                    className={clsx(
                      "px-3 py-1.5 rounded-lg text-xs font-bold border transition-colors",
                      statusFilter === f.key
                        ? "bg-primary/10 text-primary border-primary/30"
                        : "bg-muted text-muted-foreground border-border hover:text-foreground",
                    )}
                  >
                    {f.label}
                  </button>
                ))}
              </div>
            </div>

            {visible.length === 0 ? (
              <EmptyState
                icon={FileSignature}
                message={
                  contracts.length === 0
                    ? "Nenhum contrato cadastrado ainda."
                    : "Nenhum contrato corresponde aos filtros."
                }
                className="border-dashed"
              />
            ) : (
              <div className="space-y-3">
                {visible.map((c) => (
                  <ContractRow key={c.id} contract={c} onClick={() => navigate(`/contratos/${c.id}`)} />
                ))}
              </div>
            )}
          </>
        </PageState>
      </div>
    </div>
  );
}

function ContractRow({ contract, onClick }: { contract: Contract; onClick: () => void }) {
  const meta = statusMetaFor(contract.status);
  const isSigned = [ContractStatus.SIGNED, ContractStatus.ACTIVE].includes(
    (contract.status || "").toUpperCase() as ContractStatus,
  );
  const typeLabel = contractTypeLabel(contract.type);

  return (
    <motion.div
      layout
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      onClick={onClick}
      className="bg-card rounded-xl border border-border shadow-sm hover:border-primary-300 transition-all p-4 flex items-center gap-4 group cursor-pointer"
    >
      <div
        className={clsx(
          "w-12 h-12 rounded-lg flex items-center justify-center border-2 shrink-0",
          isSigned ? "bg-status-ok-soft border-status-ok/20" : "bg-muted border-border",
        )}
      >
        <FileSignature
          className={clsx("w-6 h-6", isSigned ? "text-status-ok" : "text-muted-foreground")}
        />
      </div>
      <div className="flex-1 min-w-0">
        <div className="flex justify-between items-start gap-2">
          <div className="min-w-0">
            <h4 className="font-bold text-foreground truncate">
              {contract.display_id
                ? `Contrato ${contract.display_id}`
                : `Contrato ${contract.id.slice(0, 8)}`}
            </h4>
            <p className="text-xs text-muted-foreground mt-0.5 flex items-center gap-1.5 flex-wrap">
              <span className="truncate">{contract.client || "Cliente não informado"}</span>
              {contract.property_title && (
                <>
                  <span>•</span>
                  <span className="inline-flex items-center gap-1 truncate">
                    <Building2 className="w-3 h-3" />
                    {contract.property_title}
                  </span>
                </>
              )}
            </p>
          </div>
          <span
            className={clsx(
              "text-micro font-bold px-2 py-1 rounded-full border shrink-0",
              meta.badgeClass,
            )}
          >
            {meta.label}
          </span>
        </div>
        <div className="flex items-center justify-between mt-2 gap-3 flex-wrap">
          <div className="text-xs text-muted-foreground flex items-center gap-1.5 flex-wrap">
            {typeLabel && (
              <>
                <span>{typeLabel}</span>
                <span>•</span>
              </>
            )}
            <span>
              Vigência: {formatDateBR(contract.start_date)} – {formatDateBR(contract.end_date)}
            </span>
            {typeof contract.value === "number" && contract.value > 0 && (
              <>
                <span>•</span>
                <span className="font-semibold text-foreground">
                  {formatCurrencyBRL(contract.value)}
                </span>
              </>
            )}
          </div>
          <ChevronRight className="w-4 h-4 text-muted-foreground opacity-0 group-hover:opacity-100 transition-opacity shrink-0" />
        </div>
      </div>
    </motion.div>
  );
}

function StatCard({ icon, label, value }: { icon: ReactNode; label: string; value: number }) {
  return (
    <div className="bg-card p-4 rounded-xl border border-border shadow-sm">
      <div className="flex items-center gap-2 mb-2">
        {icon}
        <p className="text-xs font-bold text-muted-foreground uppercase">{label}</p>
      </div>
      <p className="text-2xl font-bold text-foreground">{value}</p>
    </div>
  );
}

function ListSkeleton() {
  return (
    <div className="space-y-3">
      {Array.from({ length: 4 }).map((_, i) => (
        <div key={i} className="bg-card rounded-xl border border-border shadow-sm p-4 animate-pulse">
          <div className="flex items-center gap-4">
            <div className="w-12 h-12 rounded-lg bg-muted" />
            <div className="flex-1 space-y-2">
              <div className="h-4 w-1/3 bg-muted rounded" />
              <div className="h-3 w-2/3 bg-muted rounded" />
            </div>
          </div>
        </div>
      ))}
    </div>
  );
}
