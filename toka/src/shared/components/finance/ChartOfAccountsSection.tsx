import { useEffect, useState } from 'react';
import { BookOpen, Loader2, Search } from 'lucide-react';
import {
  financialReportService,
  type ChartOfAccounts,
  type FinancialAccountNature,
  type FinancialFlow,
} from '../../services/financialReport.service';
import { Button } from '../../ui/button';
import { Card } from '../../ui/card';
import { FinancialFlowSummary } from './FinancialFlowSummary';

const NATURE_LABELS: Record<FinancialAccountNature, string> = {
  asset: 'Ativo',
  liability: 'Passivo',
  equity: 'Patrimônio líquido',
  revenue: 'Receita',
  expense: 'Despesa',
};

const FLOW_LABELS: Record<FinancialFlow, string> = { sale: 'Venda', rent: 'Locação' };

const normalizeSearch = (value: string) => value.normalize('NFD').replace(/[\u0300-\u036f]/g, '').toLowerCase();

export function ChartOfAccountsSection() {
  const [catalogue, setCatalogue] = useState<ChartOfAccounts | null>(null);
  const [loading, setLoading] = useState(true);
  const [failed, setFailed] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const [flow, setFlow] = useState<FinancialFlow | 'all'>('all');
  const [search, setSearch] = useState('');

  useEffect(() => {
    let active = true;
    setLoading(true);
    setFailed(false);
    financialReportService.chartOfAccounts()
      .then((data) => { if (active) setCatalogue(data); })
      .catch(() => { if (active) setFailed(true); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [attempt]);

  const query = normalizeSearch(search.trim());
  const accounts = (catalogue?.accounts ?? []).filter((account) =>
    (flow === 'all' || account.flows.includes(flow)) &&
    (!query || normalizeSearch(`${account.code} ${account.name} ${account.description} ${NATURE_LABELS[account.nature]}`).includes(query)),
  );

  return (
    <div className="space-y-5">
      <FinancialFlowSummary flow={flow} />
      <Card className="overflow-hidden">
        <div className="space-y-4 p-5">
          <div>
            <h2 className="flex items-center gap-2 text-base font-semibold"><BookOpen className="size-4" />Plano de contas</h2>
            <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
              Contas usadas na classificação automática de vendas e locações. Valores de proprietários e
              vendedores são obrigações a pagar. A intermediação compõe a receita bruta, e as comissões
              dos corretores são despesas e obrigações até o pagamento. A administração é uma receita separada.
            </p>
          </div>
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div className="flex gap-1" role="group" aria-label="Modalidade do plano de contas">
              {([['all', 'Todas'], ['sale', 'Vendas'], ['rent', 'Locações']] as const).map(([value, label]) => (
                <Button key={value} type="button" size="sm" variant={flow === value ? 'secondary' : 'outline'} aria-pressed={flow === value} onClick={() => setFlow(value)}>
                  {label}
                </Button>
              ))}
            </div>
            <label className="relative block sm:max-w-xs sm:flex-1">
              <Search aria-hidden="true" className="absolute left-3 top-2.5 size-4 text-muted-foreground" />
              <input aria-label="Buscar conta por código ou nome" value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Buscar código ou conta" className="w-full rounded-lg border border-border bg-background py-2 pl-9 pr-3 text-sm" />
            </label>
          </div>
        </div>
        {loading ? (
          <div role="status" className="flex items-center justify-center gap-2 p-8 text-sm text-muted-foreground"><Loader2 className="size-4 animate-spin" />Carregando plano de contas…</div>
        ) : failed ? (
          <div role="alert" className="space-y-3 px-5 pb-5 text-sm">
            <p>Não foi possível carregar o plano de contas.</p>
            <Button type="button" variant="outline" size="sm" onClick={() => setAttempt((value) => value + 1)}>Tentar novamente</Button>
          </div>
        ) : (
          <>
            <div className="overflow-x-auto">
              <table className="w-full text-left text-sm">
                <caption className="sr-only">Contas financeiras aplicáveis a {flow === 'all' ? 'vendas e locações' : FLOW_LABELS[flow]}</caption>
                <thead className="border-y border-border bg-muted/40 text-xs text-muted-foreground">
                  <tr><th scope="col" className="px-5 py-3">Código</th><th scope="col" className="px-5 py-3">Conta e aplicação</th><th scope="col" className="px-5 py-3">Natureza</th><th scope="col" className="px-5 py-3">Modalidade</th></tr>
                </thead>
                <tbody className="divide-y divide-border">
                  {accounts.map((account) => (
                    <tr key={account.code}>
                      <td className="whitespace-nowrap px-5 py-4 align-top font-mono text-xs">{account.code}</td>
                      <td className="min-w-56 px-5 py-4 align-top"><span className="font-medium">{account.name}</span><p className="mt-1 text-xs leading-relaxed text-muted-foreground">{account.description}</p></td>
                      <td className="whitespace-nowrap px-5 py-4 align-top text-xs">{NATURE_LABELS[account.nature]}</td>
                      <td className="px-5 py-4 align-top text-xs">{account.flows.map((item) => FLOW_LABELS[item]).join(' e ')}</td>
                    </tr>
                  ))}
                  {accounts.length === 0 && <tr><td colSpan={4} className="p-8 text-center text-muted-foreground">Nenhuma conta encontrada para os filtros selecionados.</td></tr>}
                </tbody>
              </table>
            </div>
            <p className="border-t border-border px-5 py-3 text-xs text-muted-foreground">{accounts.length} de {catalogue?.accounts.length ?? 0} contas · Catálogo versão {catalogue?.version}</p>
          </>
        )}
      </Card>
    </div>
  );
}
