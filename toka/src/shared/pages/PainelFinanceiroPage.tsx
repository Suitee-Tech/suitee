import { ArrowLeftRight, BadgeDollarSign, BookOpenText, ChevronRight, HandCoins, Landmark, Receipt, Repeat, Wallet } from "lucide-react";
import { useNavigate } from "react-router";

import FinanceDashboard from "./FinanceDashboard";
import { FinanceAreaNav } from "../components/finance/FinanceAreaNav";

const SHORTCUTS = [
  {
    to: "/financeiro/lancamentos",
    label: "Operações",
    desc: "Uma visão por fato econômico, obrigações e caixa.",
    icon: BookOpenText,
    accent: "bg-muted text-foreground",
  },
  {
    to: "/financeiro/cobrancas",
    label: "Recebimentos",
    desc: "Cobranças a receber e pagamentos recebidos.",
    icon: Receipt,
    accent: "bg-status-ok-soft text-status-ok",
  },
  {
    to: "/financeiro/comissoes",
    label: "Comissões",
    desc: "Aprovação, pagamento e deduções dos corretores.",
    icon: HandCoins,
    accent: "bg-primary-50 text-primary-600",
  },
  {
    to: "/financeiro/repasses",
    label: "Repasses",
    desc: "Repasse ao proprietário (locação e venda).",
    icon: ArrowLeftRight,
    accent: "bg-status-info-soft text-status-info",
  },
  {
    to: "/financeiro/contas",
    label: "Contas & encargos",
    desc: "Obrigações administradas, pagamentos e referências.",
    icon: Landmark,
    accent: "bg-status-warning-soft text-status-warning",
  },
  {
    to: "/financeiro/conciliacao",
    label: "Conferência",
    desc: "Baixas e vencimentos dos lançamentos internos.",
    icon: Repeat,
    accent: "bg-muted text-muted-foreground",
  },
  {
    to: "/financeiro/payouts",
    label: "Ordens de pagamento",
    desc: "Aprovação e liquidação de obrigações off-platform.",
    icon: BadgeDollarSign,
    accent: "bg-status-info-soft text-status-info",
  },
];

/**
 * Página "Painel financeiro" — visão executiva. O FinanceDashboard já é
 * autônomo (busca via financialReportService). Os cards-atalho guiam do
 * analisar (Painel) para as áreas operacionais, sem barra de abas.
 */
export default function PainelFinanceiroPage() {
  const navigate = useNavigate();

  return (
    <div className="p-6 max-w-7xl mx-auto space-y-6">
      <FinanceAreaNav />
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold flex items-center gap-2">
            <Wallet className="w-6 h-6 text-primary-600" />
            Painel financeiro
          </h1>
          <p className="text-sm text-muted-foreground mt-1">
            Visão executiva: KPIs, DRE, fluxo de caixa e saldo do proprietário.
          </p>
        </div>
      </div>

      <FinanceDashboard embedded />

      <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4">
        {SHORTCUTS.map((s) => (
          <button
            key={s.to}
            type="button"
            onClick={() => navigate(s.to)}
            className="group rounded-2xl border border-border bg-card p-4 text-left shadow-sm transition-all hover:border-primary-200 hover:shadow-md hover:-translate-y-0.5"
          >
            <div className={`mb-3 inline-flex h-11 w-11 items-center justify-center rounded-xl border border-border ${s.accent}`}>
              <s.icon className="h-5 w-5" />
            </div>
            <p className="flex items-center gap-1 text-sm font-bold text-foreground">
              {s.label}
              <ChevronRight className="h-4 w-4 text-muted-foreground transition-transform group-hover:translate-x-0.5" />
            </p>
            <p className="mt-1 text-xs text-muted-foreground">{s.desc}</p>
          </button>
        ))}
      </div>
    </div>
  );
}
