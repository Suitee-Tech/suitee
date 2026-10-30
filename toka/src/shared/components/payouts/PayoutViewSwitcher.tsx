import { LayoutList, List } from "lucide-react";
import { NavLink } from "react-router";
import { clsx } from "clsx";

const OPTIONS = [
  { to: "/financeiro/payouts", label: "Padrão", icon: LayoutList, end: true },
  { to: "/financeiro/payouts-lista", label: "Listagem simples", icon: List, end: false },
];

/**
 * Alternância explícita entre a visão de consolidação e a lista linha a linha.
 * O estado ativo usa o primary cheio (não um cinza claro): com fundo branco a
 * diferença entre selecionado e disponível era praticamente invisível.
 */
export function PayoutViewSwitcher() {
  return (
    <nav
      aria-label="Modo de visualização das ordens de pagamento"
      className="inline-flex flex-wrap items-center gap-0.5 rounded-xl border border-border bg-muted p-1"
    >
      {OPTIONS.map(({ to, label, icon: Icon, end }) => (
        <NavLink
          key={to}
          to={to}
          end={end}
          className={({ isActive }) =>
            clsx(
              "inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-bold transition-colors",
              isActive
                ? "bg-primary text-primary-foreground shadow-sm"
                : "text-foreground hover:bg-card",
            )
          }
        >
          <Icon className="size-3.5" aria-hidden="true" />
          {label}
        </NavLink>
      ))}
    </nav>
  );
}
