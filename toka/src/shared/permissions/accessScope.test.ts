import { describe, expect, it } from "vitest";
import { hasTenantWideScope } from "./accessScope";

// Incidente de 2026-09-21 — "O perfil de Yuri mesmo sendo administrador não vê
// leads". A página filtrava por corretor usando `user.permission` (o PERFIL, que
// para um membro comum é `member`) e escondia tudo, mesmo com a API devolvendo os
// leads do tenant. Este helper espelha `pkg/auth.HasTenantWideScope` (os três
// eixos) e é a única regra usada pela tela.
describe("hasTenantWideScope (espelho de pkg/auth)", () => {
  it("reconhece o PAPEL mesmo quando o perfil do vínculo é member", () => {
    expect(hasTenantWideScope("member", "admin", "")).toBe(true);
    expect(hasTenantWideScope("member", "gestor", "")).toBe(true);
    expect(hasTenantWideScope("member", "operacoes", "")).toBe(true);
  });

  it("reconhece o PERFIL", () => {
    expect(hasTenantWideScope("owner", "corretor", "")).toBe(true);
    expect(hasTenantWideScope("manager", "", "")).toBe(true);
  });

  it("reconhece o PAPEL BASE de um perfil de acesso personalizado", () => {
    expect(hasTenantWideScope("member", "cr_3fd39ef5-6d9c-4f1a-af33-4a7cfa6f6301", "gestor")).toBe(true);
    expect(hasTenantWideScope("member", "cr_abc", "corretor")).toBe(false);
  });

  it("mantém corretor, auditor e cliente na própria carteira", () => {
    expect(hasTenantWideScope("member", "corretor", "")).toBe(false);
    expect(hasTenantWideScope("member", "auditor", "")).toBe(false);
    expect(hasTenantWideScope("member", "cliente", "")).toBe(false);
    expect(hasTenantWideScope("", "", "")).toBe(false);
  });

  it("tolera caixa e espaços vindos do token", () => {
    expect(hasTenantWideScope("  MEMBER ", " GESTOR ", "")).toBe(true);
  });
});
