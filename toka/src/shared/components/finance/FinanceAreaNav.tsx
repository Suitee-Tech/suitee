import { Link, useLocation } from "react-router";
import { clsx } from "clsx";

const FINANCE_AREAS: ReadonlyArray<{ to: string; label: string; end?: boolean; aliases?: string[] }> = [
  { to: "/financeiro", label: "Painel", end: true },
  { to: "/financeiro/lancamentos", label: "Operações" },
  { to: "/financeiro/cobrancas", label: "Recebimentos", aliases: ["/financeiro/regua"] },
  { to: "/financeiro/comissoes", label: "Comissões" },
  { to: "/financeiro/adiantamentos", label: "Adiantamentos" },
  { to: "/financeiro/impostos", label: "Impostos retidos" },
  { to: "/financeiro/repasses", label: "Repasses" },
  { to: "/financeiro/contas", label: "Contas" },
  { to: "/financeiro/conciliacao", label: "Conferência" },
  { to: "/financeiro/payouts", label: "Ordens de pagamento", aliases: ["/financeiro/payouts-lista"] },
];

/** Navegação canônica compartilhada por todas as áreas do financeiro. */
export function FinanceAreaNav() {
  const { pathname } = useLocation();

  return (
    <nav aria-label="Áreas do financeiro" className="max-w-full overflow-x-auto pb-1">
      <div className="flex w-max min-w-full gap-1 rounded-xl border border-border bg-muted p-1">
        {FINANCE_AREAS.map((item) => {
          const isActive = item.end
            ? pathname === item.to
            : pathname === item.to || pathname.startsWith(`${item.to}/`) || item.aliases?.includes(pathname);
          return (
          <Link
            key={item.to}
            to={item.to}
            aria-current={isActive ? "page" : undefined}
            className={clsx(
              "rounded-lg px-3 py-2 text-sm font-semibold whitespace-nowrap transition-colors",
              isActive
                ? "bg-card text-primary-700 shadow-sm"
                : "text-muted-foreground hover:bg-card/70 hover:text-foreground",
            )}
          >
            {item.label}
          </Link>
          );
        })}
      </div>
    </nav>
  );
}
