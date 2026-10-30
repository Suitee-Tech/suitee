import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useSearchParams } from "react-router";
import { AlertCircle, Handshake, Plus, RefreshCw } from "lucide-react";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { EmptyState } from "../ui/EmptyState";
import {
  partnershipService,
  counterpart,
  canViewPortfolio,
  type Partnership,
  type PartnershipStatus,
} from "../services/partnership.service";
import { useAuth } from "../context/AuthContext";
import { usePermissionsContext } from "../../lib/permissions/PermissionsContext";
import { useDataChangeRefresh } from "../hooks/useDataChangeRefresh";
import {
  NovoParceiroModal,
  GerenciarAcordoModal,
} from "../components/PartnerModals";
import { SharedPortfolioModal } from "../components/SharedPortfolioModal";
import { CounterpartyLancamentosLink } from "../components/lancamentos/CounterpartyLancamentosLink";

const statuses: Record<PartnershipStatus, string> = {
  pending: "Convite pendente",
  active: "Ativa",
  suspended: "Suspensa",
  inactive: "Encerrada ou recusada",
};
export function Partners() {
  const { user } = useAuth();
  const { can } = usePermissionsContext();
  const companyId = user?.current_company_id ?? "";
  const canManage = can("partnerships:write");
  const canReadShared = can("shared_content:read");
  const [params, setParams] = useSearchParams();
  const [search, setSearch] = useState("");
  const [filter, setFilter] = useState("all");
  const [newOpen, setNewOpen] = useState(false);
  const [selected, setSelected] = useState<Partnership | null>(null);
  const [portfolio, setPortfolio] = useState<Partnership | null>(null);
  const query = useQuery({
    queryKey: ["partnerships", companyId],
    queryFn: () => partnershipService.list(),
    enabled: Boolean(companyId),
  });
  const content = useQuery({
    queryKey: ["partner-content-counts", companyId],
    queryFn: () => partnershipService.getAllSharedProperties(),
    enabled: Boolean(companyId) && canReadShared,
  });
  const refresh = async () => {
    await Promise.all([
      query.refetch(),
      ...(canReadShared ? [content.refetch()] : []),
    ]);
  };
  useDataChangeRefresh(["partnerships", "properties"], refresh, {
    enabled: Boolean(companyId),
  });
  useEffect(() => {
    setSelected(null);
    setPortfolio(null);
    setNewOpen(false);
  }, [companyId]);
  useEffect(() => {
    if (params.get("empresa") && params.get("empresa") !== companyId) return;
    const id = params.get("acordo");
    const p = query.data?.find((p) => p.id === id);
    if (p) {
      setSelected(p);
      const next = new URLSearchParams(params);
      next.delete("acordo");
      next.delete("empresa");
      setParams(next, { replace: true });
    }
  }, [params, query.data, setParams, companyId]);
  useEffect(() => {
    if (!portfolio || !query.data) return;
    const current = query.data.find((p) => p.id === portfolio.id);
    if (!current || !canReadShared || !canViewPortfolio(current, companyId))
      setPortfolio(null);
    else if (current !== portfolio) setPortfolio(current);
  }, [query.data, portfolio, companyId, canReadShared]);
  const name = (p: Partnership) =>
    p.companies?.[counterpart(p, companyId)]?.name ||
    "Identificação do parceiro indisponível";
  const partners = useMemo(
    () =>
      [...(query.data ?? [])]
        .sort((a, b) =>
          String(b.created_at).localeCompare(String(a.created_at)),
        )
        .filter(
          (p) =>
            (filter === "all" || p.status === filter) &&
            (!search.trim() ||
              `${p.companies?.[counterpart(p, companyId)]?.name ?? ""} ${p.companies?.[counterpart(p, companyId)]?.tax_id ?? ""}`
                .toLowerCase()
                .includes(search.trim().toLowerCase())),
        ),
    [query.data, companyId, search, filter],
  );
  const counts = useMemo(() => {
    const out: Record<string, Set<string>> = {};
    for (const item of content.data ?? []) {
      (out[item.partnership_id] ??= new Set()).add(item.property.id);
    }
    return out;
  }, [content.data]);
  const total =
    content.isError || !canReadShared || content.isPending
      ? "—"
      : new Set(content.data?.map((p) => p.property.id)).size;
  return (
    <div className="h-full overflow-auto bg-muted p-4 sm:p-6">
      <div className="mb-6 flex flex-wrap items-center justify-between gap-4">
        <div>
          <h1 className="text-title font-bold">Parceiros & Franquias</h1>
          <p className="text-label text-muted-foreground">
            Revise os acordos e controle o que sua empresa compartilha.
          </p>
        </div>
        {canManage && (
          <Button onClick={() => setNewOpen(true)}>
            <Plus aria-hidden className="h-4 w-4" />
            Convidar parceiro
          </Button>
        )}
      </div>
      {params.get("acordo") && !query.isPending && !query.isError && (
        <p
          role="status"
          className="mb-4 rounded-lg border border-border bg-card p-4 text-label"
        >
          {params.get("empresa") && params.get("empresa") !== companyId
            ? "Esta notificação pertence a outra empresa. Selecione a empresa destinatária no menu para consultar o acordo."
            : "O acordo desta notificação não está disponível para a empresa atual."}
        </p>
      )}
      <div className="mb-4 flex flex-wrap gap-4 rounded-xl border border-border bg-card p-4 text-label">
        <span>
          Parcerias ativas:{" "}
          <strong>
            {query.data?.filter((p) => p.status === "active").length ?? "—"}
          </strong>
        </span>
        <span>
          Imóveis recebidos: <strong>{total}</strong>
        </span>
        {content.isError && canReadShared && (
          <Button size="sm" variant="outline" onClick={() => content.refetch()}>
            Atualizar contagem indisponível
          </Button>
        )}
      </div>
      <div className="mb-5 flex flex-wrap gap-3">
        <Input
          className="sm:max-w-sm"
          aria-label="Buscar parceiro"
          placeholder="Nome ou CNPJ do parceiro"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <label className="flex items-center gap-2 text-label">
          Situação
          <select
            aria-label="Situação"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            className="rounded-lg border border-border bg-card p-2"
          >
            <option value="all">Todas</option>
            {Object.entries(statuses).map(([value, label]) => (
              <option key={value} value={value}>
                {label}
              </option>
            ))}
          </select>
        </label>
      </div>
      {query.isPending ? (
        <p role="status">Carregando parceiros…</p>
      ) : query.isError ? (
        <EmptyState
          icon={AlertCircle}
          title="Não foi possível carregar os parceiros"
          message="Tente novamente para consultar os acordos."
          action={
            <Button onClick={() => query.refetch()}>
              <RefreshCw className="h-4 w-4" />
              Tentar novamente
            </Button>
          }
        />
      ) : partners.length === 0 ? (
        <EmptyState
          icon={Handshake}
          title="Nenhuma parceria encontrada"
          message="Confira os filtros ou envie um convite para outra empresa."
        />
      ) : (
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
          {partners.map((p) => {
            const company = p.companies?.[counterpart(p, companyId)];
            const incoming =
              p.status === "pending" && p.target_id === companyId;
            const view = canReadShared && canViewPortfolio(p, companyId);
            return (
              <article
                key={p.id}
                className="space-y-4 rounded-xl border border-border bg-card p-5"
              >
                <header>
                  <h2 className="text-label font-bold">{name(p)}</h2>
                  <p className="text-caption text-muted-foreground">
                    {company?.tax_id}
                    {company?.city ? ` · ${company.city}/${company.state}` : ""}
                  </p>
                  <p className="mt-2 text-label">
                    {statuses[p.status]} ·{" "}
                    {p.type === "franchise" ? "Franquia" : "Parceria"}
                  </p>
                </header>
                {p.status === "pending" && (
                  <p className="text-label">
                    {incoming
                      ? "Convite recebido. Revise os termos e as permissões antes de aceitar."
                      : "Convite enviado. Aguardando revisão da outra empresa."}
                  </p>
                )}
                {p.pending_financial && (
                  <p className="text-label text-status-warning">
                    Proposta financeira aguardando aceite.
                  </p>
                )}
                {p.status === "active" && (
                  <p className="text-label">
                    Imóveis recebidos:{" "}
                    {view && !content.isPending && !content.isError
                      ? (counts[p.id]?.size ?? 0)
                      : "—"}
                  </p>
                )}
                <div className="flex flex-wrap gap-2">
                  <Button onClick={() => setSelected(p)}>
                    {incoming
                      ? "Revisar convite"
                      : canManage
                        ? "Gerenciar acordo"
                        : "Consultar acordo"}
                  </Button>
                  <Button
                    variant="outline"
                    disabled={!view}
                    title={
                      !view
                        ? "Requer parceria ativa e permissão de leitura do portfólio."
                        : undefined
                    }
                    onClick={() => setPortfolio(p)}
                  >
                    Ver portfólio
                  </Button>
                </div>
                {p.status === "active" && (
                  <CounterpartyLancamentosLink
                    counterpartyKind="partner"
                    counterpartyId={counterpart(p, companyId)}
                  />
                )}
              </article>
            );
          })}
        </div>
      )}
      {newOpen && (
        <NovoParceiroModal
          onClose={() => setNewOpen(false)}
          onCreated={() => void refresh()}
        />
      )}
      {selected && (
        <GerenciarAcordoModal
          partnership={selected}
          companyId={companyId}
          partnerName={name(selected)}
          canEdit={canManage}
          onClose={() => setSelected(null)}
          onUpdated={() => void refresh()}
        />
      )}
      {portfolio && (
        <SharedPortfolioModal
          partnership={portfolio}
          companyId={companyId}
          partnerName={name(portfolio)}
          onClose={() => setPortfolio(null)}
        />
      )}
    </div>
  );
}
