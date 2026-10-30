import { Link } from "react-router";
import { Inbox, User, Building2, Briefcase, Crown, Home } from "lucide-react";
import { clsx } from "clsx";

type CounterpartyKind = "tenant" | "owner" | "broker" | "partner" | "external" | "desconhecido";

interface CommonProps {
  label?: string;
  icon?: typeof Inbox;
  className?: string;
}

type CounterpartyLancamentosLinkProps =
  | (CommonProps & {
      counterpartyKind: CounterpartyKind;
      counterpartyId: string;
      searchTerm?: never;
    })
  | (CommonProps & {
      counterpartyKind?: CounterpartyKind;
      counterpartyId?: never;
      searchTerm: string;
    });

const KIND_ICON: Record<CounterpartyKind, typeof Inbox> = {
  tenant: User,
  owner: Building2,
  broker: Briefcase,
  partner: Crown,
  external: Home,
  desconhecido: Inbox,
};

export function CounterpartyLancamentosLink(props: CounterpartyLancamentosLinkProps) {
  const { label, icon, className } = props;
  const isExact = "counterpartyId" in props && props.counterpartyId;
  if (!isExact && !props.searchTerm) return null;

  const params = new URLSearchParams();
  let resolvedKind: CounterpartyKind | undefined;
  if (isExact) {
    resolvedKind = props.counterpartyKind;
    params.set("counterparty_kind", props.counterpartyKind);
    params.set("counterparty_id", props.counterpartyId as string);
  } else {
    resolvedKind = props.counterpartyKind;
    params.set("search", props.searchTerm as string);
  }

  const Icon = icon ?? (resolvedKind ? KIND_ICON[resolvedKind] : Inbox);
  const href = `/financeiro/lancamentos?${params.toString()}`;

  return (
    <Link
      to={href}
      className={clsx(
        "inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-semibold border border-border bg-card text-foreground hover:border-primary hover:text-primary transition-colors",
        className,
      )}
      title="Abrir Inbox de Lançamentos filtrada por esta contraparte"
    >
      <Icon className="w-3.5 h-3.5" aria-hidden="true" />
      {label ?? "Ver lançamentos"}
    </Link>
  );
}

