import { describe, expect, it } from "vitest";
import type { Company, User } from "../services/auth.service";
import { getActiveCompany, getCompanyDisplayName } from "./tenant";

function company(id: string, name: string, legalName?: string): Company {
  return {
    id,
    name,
    legal_name: legalName,
    tax_id: "",
    email: "",
    type: "company",
    is_active: true,
  };
}

function user(currentCompanyId?: string): User {
  return {
    id: "user-1",
    email: "corretor@imobiliaria.test",
    firstname: "Ana",
    lastname: "Souza",
    role: "admin",
    current_company_id: currentCompanyId,
  };
}

describe("getActiveCompany", () => {
  const Exemplo = company("company-Exemplo", "Exemplo Imóveis", "Exemplo Imóveis Ltda");
  const recife = company("company-recife", "Recife Imóveis");

  it("usa a empresa do current_company_id", () => {
    expect(getActiveCompany(user("company-recife"), [Exemplo, recife])).toBe(recife);
  });

  it("cai para a primeira empresa quando o vínculo atual não existe mais", () => {
    expect(getActiveCompany(user("company-removida"), [Exemplo, recife])).toBe(Exemplo);
  });

  it("prefere a lista do AuthContext, que é recarregada após salvar Configurações", () => {
    const stale = { ...user("company-recife"), companies: [company("company-recife", "Nome antigo")] };
    expect(getActiveCompany(stale, [Exemplo, recife])).toBe(recife);
  });

  it("usa user.companies como fallback enquanto a lista não chegou", () => {
    const withCompanies = { ...user("company-Exemplo"), companies: [Exemplo] };
    expect(getActiveCompany(withCompanies, [])).toBe(Exemplo);
  });

  it("devolve undefined sem empresas", () => {
    expect(getActiveCompany(user("company-Exemplo"), [])).toBeUndefined();
    expect(getActiveCompany(null, undefined)).toBeUndefined();
  });
});

describe("getCompanyDisplayName", () => {
  it("prefere o nome fantasia", () => {
    expect(getCompanyDisplayName(company("c", "Exemplo Imóveis", "Exemplo Imóveis Ltda"))).toBe("Exemplo Imóveis");
  });

  it("cai para a razão social quando o nome fantasia está vazio", () => {
    expect(getCompanyDisplayName(company("c", "  ", "Exemplo Imóveis Ltda"))).toBe("Exemplo Imóveis Ltda");
  });

  it("devolve undefined quando não há nome utilizável", () => {
    expect(getCompanyDisplayName(company("c", "", ""))).toBeUndefined();
    expect(getCompanyDisplayName(undefined)).toBeUndefined();
  });
});
