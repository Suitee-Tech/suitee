import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { SidebarPlanIndicator, planIndicatorView, type PlanIndicatorView } from "./SidebarPlanIndicator";
import type { PlatformSubscriptionAccess } from "../../services/platformSubscription.service";

const plan = (tier: string, over: Partial<PlatformSubscriptionAccess["plan"]> = {}) => ({
  tier,
  name: tier === "pro" ? "Pro" : tier === "starter" ? "Starter" : "Enterprise",
  amount_cents: 98700,
  cycle: "MONTHLY" as const,
  description: "",
  limits: { leads: 1000, properties: 3000, users: 20 },
  contact_only: false,
  trial_days: 15,
  highlights: [],
  ...over,
});

const access = (over: Partial<PlatformSubscriptionAccess> = {}): PlatformSubscriptionAccess => ({
  access_control_enabled: true,
  subscription_enabled: true,
  entitled: true,
  status: "ACTIVE",
  plan: plan("pro"),
  features: ["crm", "reports"],
  ...over,
});

const now = new Date(2026, 9, 6, 12, 0, 0);
/** Data local ao meio-dia: o prazo do teste é contado em dias de calendário. */
const localISO = (year: number, month: number, day: number) => new Date(year, month - 1, day, 12, 0, 0).toISOString();

describe("planIndicatorView", () => {
  it("não anuncia nada sem consulta, com o controle desligado ou assinatura desabilitada", () => {
    expect(planIndicatorView(null, now)).toBeNull();
    expect(planIndicatorView(access({ access_control_enabled: false }), now)).toBeNull();
    expect(planIndicatorView(access({ subscription_enabled: false }), now)).toBeNull();
  });

  it("pede contratação quando o acesso está bloqueado", () => {
    const view = planIndicatorView(access({ entitled: false, status: "EXPIRED", plan: null }), now)!;
    expect(view.kind).toBe("restore");
    expect(view.label).toBe("Assinar para continuar");
    expect(view.tone).toBe("urgent");
  });

  it("pede regularização quando a cobrança falhou, mesmo com plano Pro", () => {
    const view = planIndicatorView(access({ status: "PAST_DUE" }), now)!;
    expect(view.kind).toBe("payment");
    expect(view.label).toBe("Regularizar pagamento");
    expect(view.tone).toBe("warning");
  });

  it("em teste do Pro mostra o prazo e o selo, em vez de vender o Pro", () => {
    const view = planIndicatorView(
      access({ status: "TRIALING", trial_ends_at: localISO(2026, 10, 20) }),
      now,
    )!;
    expect(view.kind).toBe("trial");
    expect(view.label).toBe("Pro em teste");
    expect(view.badge).toBe("14 dias");
    expect(view.tone).toBe("premium");
    expect(view.detail).toContain("Assine para manter");
    expect(view.detail).not.toContain("Assine o Pro");
  });

  it("usa o singular no último dia do teste e não inventa prazo sem data", () => {
    expect(planIndicatorView(access({ status: "TRIALING", trial_ends_at: localISO(2026, 10, 7) }), now)!.badge)
      .toBe("1 dia");
    expect(planIndicatorView(access({ status: "TRIALING", trial_ends_at: localISO(2026, 10, 1) }), now)!.badge)
      .toBe("0 dias");
    const semData = planIndicatorView(access({ status: "TRIALING", trial_ends_at: undefined }), now)!;
    expect(semData.badge).toBe("PRO");
    expect(semData.detail).toContain("no fim do teste");
  });

  it("em teste de plano inferior aponta o upgrade", () => {
    const view = planIndicatorView(
      access({ status: "TRIALING", plan: plan("starter"), trial_ends_at: localISO(2026, 11, 3) }),
      now,
    )!;
    expect(view.kind).toBe("upgrade");
    expect(view.label).toBe("Fazer upgrade");
    expect(view.badge).toBe("PRO");
    expect(view.detail).toContain("Starter");
    expect(view.detail).toContain("3/11/2026");
  });

  it("com Pro ativo destaca o que está liberado, sem chamada de venda", () => {
    const view = planIndicatorView(access(), now)!;
    expect(view.kind).toBe("active");
    expect(view.label).toBe("Plano Pro");
    expect(view.badge).toBe("PRO");
    expect(view.detail).toContain("Todos os recursos Pro liberados");
    expect(view.detail).toContain("20 usuários");
    expect(view.detail).toContain("1.000 leads");
    expect(view.detail).toContain("3.000 imóveis");
    expect(view.detail).not.toContain("Upgrade");
  });

  it("com Starter ativo vende o upgrade com o ganho concreto", () => {
    const view = planIndicatorView(access({ plan: plan("starter", { limits: { leads: 200, properties: 500, users: 5 } }) }), now)!;
    expect(view.kind).toBe("upgrade");
    expect(view.badge).toBe("PRO");
    expect(view.detail).toContain("Starter");
    expect(view.detail).toContain("portal do cliente");
    expect(view.detail).toContain("limites 5x maiores");
  });

  it("com Enterprise ativo não oferece upgrade (é o topo da escada)", () => {
    const view = planIndicatorView(access({ plan: plan("enterprise", { contact_only: true }) }), now)!;
    expect(view.kind).toBe("active");
    expect(view.badge).toBeUndefined();
    expect(view.label).toBe("Enterprise ativo");
    expect(view.detail).not.toContain("Upgrade");
  });

  it("com renovação cancelada mostra até quando o acesso vale", () => {
    const view = planIndicatorView(access({ status: "CANCELLED", paid_through: localISO(2026, 11, 20) }), now)!;
    expect(view.kind).toBe("payment");
    expect(view.label).toBe("Reativar assinatura");
    expect(view.detail).toContain("20/11/2026");
  });

  it("não anuncia plano quando o acesso está liberado sem plano identificado", () => {
    expect(planIndicatorView(access({ plan: null }), now)).toBeNull();
  });
});

const view = (over: Partial<PlanIndicatorView> = {}): PlanIndicatorView => ({
  kind: "trial",
  label: "Pro em teste",
  badge: "14 dias",
  detail: "Seu teste do Pro vai até 20/10/2026. Assine para manter portal do cliente.",
  tone: "premium",
  ...over,
});

describe("SidebarPlanIndicator", () => {
  it("leva para a assinatura com o estado no rótulo acessível", () => {
    render(
      <MemoryRouter>
        <SidebarPlanIndicator view={view()} expanded />
      </MemoryRouter>,
    );
    const link = screen.getByRole("link", { name: /Pro em teste/ });
    expect(link).toHaveAttribute("href", "/assinatura");
    expect(link).toHaveAttribute("data-plan-indicator", "trial");
    expect(screen.getByText("14 dias")).toBeInTheDocument();
    expect(screen.getByText(/vai até 20\/10\/2026/)).toBeInTheDocument();
  });

  it("com o menu recolhido mantém o item acessível pelo título", () => {
    render(
      <MemoryRouter>
        <SidebarPlanIndicator view={view({ tone: "calm", kind: "active", label: "Plano Pro" })} expanded={false} />
      </MemoryRouter>,
    );
    const link = screen.getByRole("link", { name: /Plano Pro/ });
    expect(link).toHaveAttribute("title", expect.stringContaining("Plano Pro"));
    expect(link.className).toContain("lg:justify-center");
  });
});
