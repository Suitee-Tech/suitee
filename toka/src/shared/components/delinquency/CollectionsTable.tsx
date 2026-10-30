import { AlertCircle, ChevronLeft, ChevronRight, FileText, Gavel, Handshake, Phone, Search, Shield } from 'lucide-react';
import { clsx } from 'clsx';
import type { Collection } from '../../services/collection.service';
import { Button } from '../../ui/button';
import { formatCurrencyBRL } from '../../utils/format';
import { DELINQUENCY_TABS, GUARANTEE_STATUS_LABEL, GUARANTEE_TYPE_LABEL, JUDICIAL_STATUS_LABEL, STATUS_LABEL, statusBadgeClass, tabStatus, type DelinquencyTab } from './delinquencyUi';

export type CasePanelKind = 'procedimento' | 'acordo' | 'garantia' | 'judicial' | 'rescisao';

export function DelinquencyTabs({
  value,
  onChange,
}: {
  value: DelinquencyTab;
  onChange: (tab: DelinquencyTab) => void;
}) {
  return (
    <div role="tablist" aria-label="Seções de inadimplência" className="flex items-center gap-6 overflow-x-auto">
      {DELINQUENCY_TABS.map((tab) => {
        const selected = tab.id === value;
        return (
          <button
            key={tab.id}
            type="button"
            role="tab"
            id={`delinquency-tab-${tab.id}`}
            aria-selected={selected}
            aria-controls="delinquency-tabpanel"
            onClick={() => onChange(tab.id)}
            className={clsx(
              'flex items-center gap-2 px-4 py-2 text-sm rounded-lg whitespace-nowrap transition-colors',
              selected
                ? 'bg-card text-foreground font-bold shadow-sm border border-border'
                : 'text-muted-foreground hover:text-foreground font-medium',
            )}
          >
            {tab.id === 'inadimplentes' && <AlertCircle className="w-4 h-4" />}
            {tab.id === 'cobranca' && <Phone className="w-4 h-4" />}
            {tab.id === 'garantias' && <Shield className="w-4 h-4" />}
            {tab.id === 'judicial' && <Gavel className="w-4 h-4" />}
            {tab.label}
          </button>
        );
      })}
    </div>
  );
}

export function CollectionsTable({
  tab,
  onTabChange,
  items,
  loading,
  error,
  onReload,
  total,
  page,
  totalPages,
  isMobile,
  search,
  onSearchChange,
  onPageChange,
  canWrite,
  canTerminate,
  onAction,
  emptyMessage,
}: {
  tab: DelinquencyTab;
  onTabChange: (tab: DelinquencyTab) => void;
  items: Collection[];
  loading: boolean;
  error: string | null;
  onReload: () => void;
  total: number;
  page: number;
  totalPages: number;
  isMobile: boolean;
  search: string;
  onSearchChange: (value: string) => void;
  onPageChange: (page: number) => void;
  canWrite: boolean;
  canTerminate: boolean;
  onAction: (kind: CasePanelKind, collection: Collection) => void;
  emptyMessage: string;
}) {
  const term = search.trim().toLowerCase();
  const rows = term
    ? items.filter((item) =>
        [item.debtor_name, item.contract_id, item.id].some((field) => (field ?? '').toLowerCase().includes(term)),
      )
    : items;

  const statusFilter = tabStatus(tab);

  /**
   * Resumo do domínio da aba: Garantias mostra modalidade/valor/status do
   * acionamento e Judicial mostra o processo — as abas deixam de ser só um
   * filtro de status e passam a exibir o dado que as distingue.
   */
  const detailFor = (item: Collection): string | null => {
    if (tab === 'garantias') {
      if (!item.guarantee) return 'Sem acionamento registrado';
      return `${GUARANTEE_TYPE_LABEL[item.guarantee.type] ?? item.guarantee.type} · ${formatCurrencyBRL(item.guarantee.amount)} · ${
        GUARANTEE_STATUS_LABEL[item.guarantee.status] ?? item.guarantee.status
      }${item.guarantee.insurer_protocol ? ` · protocolo ${item.guarantee.insurer_protocol}` : ''}`;
    }
    if (tab === 'judicial') {
      if (!item.judicial) return 'Sem processo registrado';
      return `${item.judicial.case_number} · ${item.judicial.court} · ${
        JUDICIAL_STATUS_LABEL[item.judicial.status] ?? item.judicial.status
      }`;
    }
    return null;
  };
  const showDetail = tab === 'garantias' || tab === 'judicial';

  const renderActions = (item: Collection) => {
    const terminal = item.status === 'RESOLVED';
    const blockTitle = terminal ? 'Caso resolvido — não aceita novas transições' : undefined;
    return (
      <div className="flex items-center justify-end gap-1">
        {canWrite && (
          <>
            <Button
              variant="ghost"
              size="icon"
              onClick={() => onAction('procedimento', item)}
              className="size-9 text-muted-foreground hover:text-primary-600 hover:bg-primary-50"
              title="Procedimento de Cobrança"
            >
              <Phone className="w-4 h-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              onClick={() => onAction('acordo', item)}
              disabled={terminal}
              title={blockTitle ?? 'Acordo de Dívida'}
              className="size-9 text-muted-foreground hover:text-primary-600 hover:bg-primary-50"
            >
              <Handshake className="w-4 h-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              onClick={() => onAction('garantia', item)}
              disabled={terminal}
              title={blockTitle ?? 'Acionamento de Garantia'}
              className="size-9 text-muted-foreground hover:text-primary-600 hover:bg-primary-50"
            >
              <Shield className="w-4 h-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              onClick={() => onAction('judicial', item)}
              disabled={terminal}
              title={blockTitle ?? 'Processo Judicial'}
              className="size-9 text-muted-foreground hover:text-primary-600 hover:bg-primary-50"
            >
              <Gavel className="w-4 h-4" />
            </Button>
          </>
        )}
        <Button
          variant="ghost"
          size="icon"
          onClick={() => onAction('rescisao', item)}
          disabled={!canTerminate || terminal}
          title={
            canTerminate
              ? blockTitle ?? 'Rescisão de Contrato'
              : 'Seu perfil não tem permissão para encerrar contratos (contracts:write)'
          }
          className="size-9 text-muted-foreground hover:text-status-critical hover:bg-status-critical-soft"
        >
          <FileText className="w-4 h-4" />
        </Button>
      </div>
    );
  };

  return (
    <div className="bg-card rounded-2xl border border-border shadow-sm overflow-hidden flex flex-col">
      <div className="p-4 border-b border-border bg-muted/50 flex items-center gap-6 overflow-x-auto">
        <DelinquencyTabs value={tab} onChange={onTabChange} />
      </div>

      <div className="p-4 border-b border-border space-y-2">
        <div className="relative max-w-full">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
          <input
            type="search"
            aria-label="Buscar casos"
            value={search}
            onChange={(event) => onSearchChange(event.target.value)}
            placeholder="Buscar por inquilino, contrato ou identificador..."
            className="w-full pl-9 pr-4 py-2 bg-muted border border-border rounded-xl text-sm focus:outline-none focus:ring-2 focus:ring-primary-600/20 focus:border-primary-600 transition-all"
          />
        </div>
        <p className="text-xs text-muted-foreground">
          {statusFilter
            ? `Filtro do servidor: status ${STATUS_LABEL[statusFilter]}. `
            : 'Todos os status. '}
          {total} caso(s) no total; a busca filtra apenas os {items.length} caso(s) desta página — os indicadores acima
          vêm do servidor e não mudam com a busca.
        </p>
      </div>

      {error && (
        <div className="p-4 border-b border-border flex flex-wrap items-center justify-between gap-3 bg-status-critical-soft/40">
          <span className="text-sm text-status-critical">{error}</span>
          <Button variant="outline" size="sm" onClick={onReload}>
            Tentar novamente
          </Button>
        </div>
      )}

      <div className="overflow-x-auto" id="delinquency-tabpanel" role="tabpanel" aria-labelledby={`delinquency-tab-${tab}`}>
        {rows.length === 0 ? (
          <p className="p-8 text-center text-sm text-muted-foreground">
            {loading ? 'Carregando casos…' : term ? 'Nenhum caso desta página corresponde à busca.' : emptyMessage}
          </p>
        ) : isMobile ? (
          <div className="divide-y divide-border">
            {rows.map((item) => (
              <div key={item.id} className="p-4">
                <div className="flex items-center gap-2 min-w-0">
                  <span className="text-sm font-bold text-foreground">{item.contract_id}</span>
                  <span className={clsx('px-2.5 py-1 text-micro font-bold rounded-full whitespace-nowrap', statusBadgeClass(item.status))}>
                    {STATUS_LABEL[item.status] ?? item.status}
                  </span>
                </div>
                <div className="mt-2 text-sm font-medium text-foreground truncate">{item.debtor_name}</div>
                {showDetail && <p className="text-xs text-muted-foreground mt-1">{detailFor(item)}</p>}
                <div className="mt-2 flex items-center justify-between gap-3">
                  <div className="text-sm font-bold text-status-critical">{formatCurrencyBRL(item.total_debt)}</div>
                  <span className="px-2 py-0.5 bg-status-critical/90 text-white text-micro font-bold rounded-full whitespace-nowrap">
                    {item.days_overdue} dias
                  </span>
                </div>
                <div className="mt-3">{renderActions(item)}</div>
              </div>
            ))}
          </div>
        ) : (
          <table className="w-full text-left border-collapse min-w-[1000px]">
            <thead>
              <tr className="border-b border-border text-muted-foreground text-xs font-medium tracking-wide">
                <th className="px-6 py-4">Contrato</th>
                <th className="px-6 py-4">Inquilino</th>
                <th className="px-6 py-4">Valor em Atraso</th>
                <th className="px-6 py-4">Dias Atraso</th>
                <th className="px-6 py-4">Status</th>
                {showDetail && <th className="px-6 py-4">Detalhe</th>}
                <th className="px-6 py-4 text-right">Ações</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {rows.map((item) => (
                <tr key={item.id} className="hover:bg-muted/80 transition-colors">
                  <td className="px-6 py-4 text-sm font-bold text-foreground">{item.contract_id}</td>
                  <td className="px-6 py-4 text-sm font-medium text-foreground">{item.debtor_name}</td>
                  <td className="px-6 py-4 text-sm font-bold text-status-critical">{formatCurrencyBRL(item.total_debt)}</td>
                  <td className="px-6 py-4">
                    <span className="px-2 py-0.5 bg-status-critical/90 text-white text-micro font-bold rounded-full">
                      {item.days_overdue} dias
                    </span>
                  </td>
                  <td className="px-6 py-4">
                    <span className={clsx('px-3 py-1 text-caption font-bold rounded-full', statusBadgeClass(item.status))}>
                      {STATUS_LABEL[item.status] ?? item.status}
                    </span>
                  </td>
                  {showDetail && (
                    <td className="px-6 py-4 text-xs text-muted-foreground max-w-[280px]">{detailFor(item)}</td>
                  )}
                  <td className="px-6 py-4 text-right">{renderActions(item)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <div className="p-4 border-t border-border flex items-center justify-between gap-3">
        <span className="text-xs text-muted-foreground">
          Página {page} de {Math.max(totalPages, 1)}
        </span>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => onPageChange(page - 1)}
            disabled={loading || page <= 1}
            title="Página anterior"
          >
            <ChevronLeft className="w-4 h-4" /> Anterior
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => onPageChange(page + 1)}
            disabled={loading || page >= totalPages}
            title="Próxima página"
          >
            Próxima <ChevronRight className="w-4 h-4" />
          </Button>
        </div>
      </div>
    </div>
  );
}
