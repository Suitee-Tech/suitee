import { describe, expect, it } from "vitest";
import { planAddsBenefits, planRemovesBenefits, type PlatformPlan } from "./platformSubscription.service";

const plan = (over: Partial<PlatformPlan>): PlatformPlan => ({
  tier: "pro",
  name: "Pro",
  amount_cents: 98700,
  cycle: "MONTHLY",
  description: "",
  limits: { leads: 1000, properties: 3000, users: 20 },
  contact_only: false,
  trial_days: 15,
  features: ["crm", "client_portal"],
  ...over,
});

const starter = plan({ tier: "starter", name: "Starter", amount_cents: 49700, limits: { leads: 200, properties: 500, users: 5 }, features: ["crm"] });
const pro = plan({});
const proAnual = plan({ cycle: "ANNUALLY", amount_cents: 947520 });
const proComMaisLeads = plan({ limits: { leads: 1500, properties: 3000, users: 20 } });
const proSemFeatures = plan({ features: undefined });
const proComRecursoAMais = plan({ features: ["crm", "client_portal", "reports"] });
const enterprise = plan({ tier: "enterprise", name: "Enterprise", contact_only: true, limits: { leads: 0, properties: 0, users: 0 } });

describe("planAddsBenefits", () => {
  it("reconhece upgrade entre tiers", () => {
    expect(planAddsBenefits(starter, pro)).toBe(true);
    expect(planAddsBenefits(pro, starter)).toBe(false);
  });

  it("não confunde troca de ciclo, mesmo plano ou preço maior com ganho", () => {
    expect(planAddsBenefits(pro, proAnual)).toBe(false);
    expect(planAddsBenefits(pro, pro)).toBe(false);
    expect(planAddsBenefits(pro, enterprise)).toBe(false);
  });

  it("exige manter todos os limites e recursos atuais", () => {
    expect(planAddsBenefits(pro, plan({ limits: { leads: 1500, properties: 3000, users: 20 }, features: ["crm"] }))).toBe(false);
    expect(planAddsBenefits(pro, proComMaisLeads)).toBe(true);
    expect(planAddsBenefits(pro, proComRecursoAMais)).toBe(true);
  });

  it("sem a lista de recursos (plano antigo) decide só por limites", () => {
    expect(planAddsBenefits(proSemFeatures, proComRecursoAMais)).toBe(false);
    expect(planAddsBenefits(proSemFeatures, plan({ limits: { leads: 1500, properties: 3000, users: 20 } }))).toBe(true);
    expect(planAddsBenefits(proComRecursoAMais, proSemFeatures)).toBe(false);
  });

  it("sem os dois planos não há ganho", () => {
    expect(planAddsBenefits(undefined, pro)).toBe(false);
    expect(planAddsBenefits(pro, undefined)).toBe(false);
    expect(planAddsBenefits(null, null)).toBe(false);
  });
});

describe("planRemovesBenefits", () => {
  it("é o ganho na direção contrária", () => {
    expect(planRemovesBenefits(pro, starter)).toBe(true);
    expect(planRemovesBenefits(starter, pro)).toBe(false);
    expect(planRemovesBenefits(pro, pro)).toBe(false);
  });
});
