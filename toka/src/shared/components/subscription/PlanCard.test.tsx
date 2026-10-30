import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { PlanCard } from "./PlanCard";
import type { PlatformPlan, PlatformSubscription } from "../../services/platformSubscription.service";

const starterPlan: PlatformPlan = {
  tier: "starter", name: "Starter", amount_cents: 49700, cycle: "MONTHLY", description: "Para organizar a operação.",
  limits: { leads: 200, properties: 500, users: 5 }, contact_only: false, trial_days: 15,
  features: ["crm", "portfolio", "rentals", "finance", "calendar", "contracts", "documents"], highlights: [],
};
const proPlan: PlatformPlan = {
  tier: "pro", name: "Pro", amount_cents: 98700, cycle: "MONTHLY", description: "Para crescer com escala.",
  limits: { leads: 1000, properties: 3000, users: 20 }, contact_only: false, trial_days: 15,
  features: [...(starterPlan.features ?? []), "client_portal", "electronic_signature", "reports", "intelligence", "mcp"], highlights: [],
};
const proAnnual: PlatformPlan = { ...proPlan, cycle: "ANNUALLY", amount_cents: 947520 };

const state = (over: Partial<PlatformSubscription> = {}): PlatformSubscription => ({
  status: "TRIALING", plan: starterPlan, subscription_id: "subs_1", trial_ends_at: "2026-11-03T23:59:59.999Z",
  dev_mode: true, ...over,
});

const renderCard = (over: Partial<Parameters<typeof PlanCard>[0]> = {}) =>
  render(
    <PlanCard
      plan={proPlan}
      trialDays={0}
      state={state()}
      usage={{ leads: 10, properties: 5, users: 1 }}
      canManage
      enabled
      busy={false}
      recommended={false}
      onSelect={vi.fn()}
      {...over}
    />,
  );

describe("PlanCard com mudança agendada", () => {
  it("marca o alvo de um upgrade confirmado como liberado, sem prometer cobrança imediata", () => {
    renderCard({ state: state({ pending_change: { id: "subu_1", plan: proPlan, requested_at: "2026-10-06T00:00:00Z" } }) });

    expect(screen.getByRole("button", { name: "Upgrade aplicado" })).toBeInTheDocument();
    expect(screen.getByText("Benefícios liberados para toda a equipe; o novo valor começa na próxima renovação.")).toBeInTheDocument();
  });

  it("trata redução como redução: benefícios mantidos até o fim do período", () => {
    renderCard({
      plan: starterPlan,
      state: state({
        plan: proPlan,
        pending_change: { id: "subu_2", plan: starterPlan, requested_at: "2026-10-06T00:00:00Z" },
      }),
    });

    expect(screen.getByRole("button", { name: "Mudança agendada" })).toBeInTheDocument();
    expect(screen.getByText("Redução aplicada após o fim do período atual, na próxima renovação.")).toBeInTheDocument();
  });

  it("troca de ciclo do mesmo plano é mudança de cobrança, não redução", () => {
    renderCard({ plan: proAnnual, state: state({ plan: proPlan }) });

    expect(screen.getByRole("button", { name: "Mudar para Pro" })).toBeEnabled();
    expect(screen.getByText("A mudança de cobrança começa na próxima renovação.")).toBeInTheDocument();
  });

  it("aponta upgrade quando o alvo acrescenta limites e recursos", () => {
    renderCard({ plan: proPlan, state: state({ status: "ACTIVE" }) });

    expect(screen.getByRole("button", { name: "Fazer upgrade para Pro" })).toBeEnabled();
    expect(screen.getByText("A mudança de cobrança começa na próxima renovação.")).toBeInTheDocument();
  });
});

// Em produção (2026-10-08), quem estava em trial e clicava em outro plano com um
// checkout em aberto ficava preso: o botão vinha desabilitado e o texto mandava
// "encerre o checkout". Agora o plano pode ser trocado — o checkout anterior é
// substituído — e o plano do checkout em aberto vira "Continuar pagamento".
describe("checkout em aberto com outro plano", () => {
  const comCheckout = (tier: string, cycle: "MONTHLY" | "ANNUALLY" = "MONTHLY") => state({
    status: "PENDING",
    plan: starterPlan,
    subscription_id: "subs_1",
    checkout: { id: "bill_1", url: "https://app.abacatepay.com/pay/bill_1", plan: { ...starterPlan, tier, cycle } },
  } as Partial<PlatformSubscription>);

  it("libera o outro plano e explica que o checkout será substituído", () => {
    renderCard({ state: comCheckout("starter") });
    const botao = screen.getByRole("button", { name: /Assinar/i });
    expect(botao).toBeEnabled();
    expect(screen.getByText(/checkout em aberto do Starter/i)).toBeInTheDocument();
  });

  it("no plano do checkout em aberto, o botão leva ao pagamento", () => {
    renderCard({ state: comCheckout("pro") });
    expect(screen.getByRole("button", { name: /Continuar pagamento/i })).toBeEnabled();
    expect(screen.getByText(/Checkout em aberto/i)).toBeInTheDocument();
  });
});
