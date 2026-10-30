import { NavLink } from "react-router";
import { clsx } from "clsx";
import { AlertCircle, ChevronRight, Crown, Sparkles, TrendingUp, type LucideIcon } from "lucide-react";
import type { PlatformSubscriptionAccess } from "../../services/platformSubscription.service";
import { dateBR, daysLabel, remainingDays } from "../../utils/subscription";

/**
 * Indicador do plano no menu lateral.
 *
 * O item fixo "Assine o Pro" só fazia sentido para quem não tinha plano: em
 * teste do Pro ou com o Pro ativo ele repetia a mesma chamada de venda para
 * quem já estava dentro. Aqui o indicador conta o ESTADO (teste com prazo,
 * plano ativo, pagamento pendente) e aponta sempre o próximo passo útil —
 * upgrade quando há plano acima, recursos liberados quando não há.
 *
 * A decisão é uma função pura (`planIndicatorView`) para caber em teste sem
 * montar a tela; o componente só cuida da aparência.
 */

export type PlanIndicatorKind = "restore" | "payment" | "trial" | "upgrade" | "active";
export type PlanIndicatorTone = "urgent" | "warning" | "premium" | "calm";

export interface PlanIndicatorView {
  kind: PlanIndicatorKind;
  /** Rótulo curto do menu (com o prazo do teste quando existe). */
  label: string;
  /** Selo do plano, quando o estado é de plano (PRO/ENTERPRISE). */
  badge?: string;
  /** Frase de apoio: o que acontece agora e o que o clique resolve. */
  detail: string;
  tone: PlanIndicatorTone;
}

const numberBR = new Intl.NumberFormat("pt-BR");

/** "até 20 usuários, 1.000 leads e 3.000 imóveis" — destaque do que o plano libera. */
function limitsSummary(access: PlatformSubscriptionAccess): string {
  const limits = access.plan?.limits;
  if (!limits || (!limits.users && !limits.leads && !limits.properties)) return "";
  return `até ${numberBR.format(limits.users)} usuários, ${numberBR.format(limits.leads)} leads e ${numberBR.format(limits.properties)} imóveis`;
}

/** Recursos que o Pro acrescenta sobre o Starter, na voz do produto. */
const PRO_GAIN = "portal do cliente, assinatura digital, relatórios com DIMOB e informes, inteligência e MCP";

export function planIndicatorView(
  access: PlatformSubscriptionAccess | null | undefined,
  now: Date = new Date(),
): PlanIndicatorView | null {
  // Sem consulta respondida, com o controle desligado no ambiente ou sem
  // assinatura habilitada não existe plano para anunciar.
  if (!access || !access.subscription_enabled || !access.access_control_enabled) return null;

  const tier = access.plan?.tier ?? "";
  const planName = access.plan?.name || (tier ? tier[0].toUpperCase() + tier.slice(1) : "plano");
  const endsAt = access.trial_ends_at ? dateBR(access.trial_ends_at) : "";
  const trialDays = access.trial_ends_at ? remainingDays(access.trial_ends_at, now) : null;
  const trialEnds = endsAt ? `até ${endsAt}` : "no fim do teste";

  if (!access.entitled) {
    return {
      kind: "restore",
      label: "Assinar para continuar",
      detail: "Seu acesso aos recursos premium está bloqueado. Escolha um plano para retomar o trabalho.",
      tone: "urgent",
    };
  }

  if (access.status === "PAST_DUE") {
    return {
      kind: "payment",
      label: "Regularizar pagamento",
      detail: "A cobrança não foi concluída. Atualize o cartão para não perder os recursos.",
      tone: "warning",
    };
  }

  if (access.status === "CANCELLED") {
    return {
      kind: "payment",
      label: "Reativar assinatura",
      detail: access.paid_through
        ? `A renovação foi cancelada; você mantém o ${planName} até ${dateBR(access.paid_through)}.`
        : "A renovação foi cancelada. Reative para manter os recursos.",
      tone: "warning",
    };
  }

  if (access.status === "TRIALING" && tier === "pro") {
    // O selo carrega o prazo: o rótulo já diz que o plano é o Pro, então o
    // espaço curto do menu vai para a informação que muda (os dias restantes).
    return {
      kind: "trial",
      label: "Pro em teste",
      badge: trialDays !== null ? daysLabel(trialDays) : "PRO",
      detail: `Seu teste do Pro vai ${trialEnds}. Assine para manter ${PRO_GAIN}.`,
      tone: "premium",
    };
  }

  if (access.status === "TRIALING") {
    return {
      kind: "upgrade",
      // "Fazer upgrade" + selo PRO cabe no menu recolhível sem truncar; o
      // detalhe nomeia o plano e o que ele acrescenta.
      label: "Fazer upgrade",
      badge: "PRO",
      detail: `Seu teste do ${planName} vai ${trialEnds}. No Pro entram ${PRO_GAIN}, com limites 5x maiores.`,
      tone: "premium",
    };
  }

  if (tier === "enterprise") {
    return {
      kind: "active",
      label: "Enterprise ativo",
      detail: "Plano sob medida em vigor. Fale com o time para ajustar escopo, limites ou integrações.",
      tone: "calm",
    };
  }

  if (tier === "pro") {
    const limits = limitsSummary(access);
    return {
      kind: "active",
      label: "Plano Pro",
      badge: "PRO",
      detail: `Todos os recursos Pro liberados${limits ? ` — ${limits}` : ""}. Revise limites, pagamentos e comprovantes.`,
      tone: "calm",
    };
  }

  if (tier) {
    const limits = limitsSummary(access);
    return {
      kind: "upgrade",
      label: "Fazer upgrade",
      badge: "PRO",
      detail: `Você está no ${planName}${limits ? ` (${limits})` : ""}. O Pro libera ${PRO_GAIN}, com limites 5x maiores.`,
      tone: "premium",
    };
  }

  return null;
}

const toneClass: Record<PlanIndicatorTone, string> = {
  urgent: "border-destructive/15 bg-destructive/[0.025] hover:border-destructive/25 hover:bg-destructive/[0.045]",
  warning: "border-status-warning/20 bg-status-warning-soft/45 hover:border-status-warning/30 hover:bg-status-warning-soft/70",
  premium: "border-primary/15 bg-primary/[0.035] hover:border-primary/25 hover:bg-primary/[0.06]",
  calm: "border-border/80 bg-muted/35 hover:border-primary/20 hover:bg-muted/60",
};

const iconClass: Record<PlanIndicatorTone, string> = {
  urgent: "bg-destructive/10 text-destructive ring-1 ring-destructive/25",
  warning: "bg-status-warning-soft text-status-warning ring-1 ring-status-warning/30",
  premium: "bg-primary/10 text-primary ring-1 ring-primary/15",
  calm: "bg-background text-primary ring-1 ring-border/70",
};

const toneIcon: Record<PlanIndicatorTone, LucideIcon> = {
  urgent: Sparkles,
  warning: AlertCircle,
  premium: Crown,
  calm: Crown,
};

/** O upgrade tem seta de avanço; o plano ativo mantém a coroa. */
function iconFor(view: PlanIndicatorView): LucideIcon {
  if (view.kind === "upgrade") return TrendingUp;
  return toneIcon[view.tone];
}

export function SidebarPlanIndicator({
  view,
  expanded,
  onNavigate,
}: {
  view: PlanIndicatorView;
  expanded: boolean;
  onNavigate?: () => void;
}) {
  const Icon = iconFor(view);
  const accessibleName = `${view.label} — ${view.detail}`;

  return (
    <NavLink
      to="/assinatura"
      onClick={onNavigate}
      title={accessibleName}
      aria-label={accessibleName}
      data-plan-indicator={view.kind}
      className={({ isActive }) =>
        clsx(
          "group flex items-center gap-2.5 rounded-xl border px-3 py-2.5 transition-all duration-200",
          "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/40 focus-visible:ring-offset-1 focus-visible:ring-offset-card",
          toneClass[view.tone],
          isActive && "border-primary/35 bg-primary/[0.06] shadow-sm",
          !expanded && "lg:justify-center lg:px-0 lg:py-3",
        )
      }
    >
      <span className={clsx("flex size-8 shrink-0 items-center justify-center rounded-full", iconClass[view.tone])}>
        <Icon className="size-4" aria-hidden="true" />
      </span>
      <span className={clsx("min-w-0 flex-1 flex-col gap-0.5", expanded ? "flex" : "flex lg:hidden")}>
        {/* O selo desce para a linha de baixo quando o rótulo não cabe: o nome
            do estado é a informação que não pode truncar. */}
        <span className="flex flex-wrap items-center gap-x-2 gap-y-0.5">
          <span className="truncate text-label font-semibold tracking-[-0.01em] text-foreground">{view.label}</span>
          {view.badge && (
            <span
              className={clsx(
                "shrink-0 rounded-md border px-1.5 py-0.5 text-micro font-semibold uppercase tracking-wide",
                view.tone === "premium"
                  ? "border-primary/10 bg-primary/10 text-primary"
                  : "border-border/70 bg-background/80 text-muted-foreground",
              )}
            >
              {view.badge}
            </span>
          )}
        </span>
        <span className="line-clamp-2 text-caption font-normal text-muted-foreground">{view.detail}</span>
      </span>
      {expanded && (
        <ChevronRight
          className="-mr-1 size-3.5 shrink-0 text-muted-foreground/45 transition-all duration-200 group-hover:translate-x-0.5 group-hover:text-primary"
          aria-hidden="true"
        />
      )}
    </NavLink>
  );
}
