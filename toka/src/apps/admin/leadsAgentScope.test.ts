import { describe, expect, it } from "vitest";
import { leadMatchesAgentScope } from "./Leads";

// Incidente de 2026-09-21 — "O perfil de Yuri mesmo sendo administrador não vê
// leads".
//
// A lista filtrava por corretor no cliente usando `user.permission` (o PERFIL do
// vínculo). Num membro comum o perfil é `member`, mesmo quando o PAPEL atribuído
// é `admin`: o filtro derrubava todos os leads de outros corretores e a tela
// mostrava "Nenhum lead encontrado", ainda que a API tivesse devolvido tudo.
// O recorte agora usa o critério compartilhado (perfil + papel + papel base).
describe("recorte por corretor na lista de leads", () => {
  const doOutroCorretor = { agentId: "outro-corretor" };
  const meu = { agentId: "eu" };
  const compartilhado = { agentId: "outro-corretor", sharedFrom: "imobiliaria-parceira" };

  it("admin/gestor (visão de tenant) vê o lead de qualquer corretor", () => {
    expect(leadMatchesAgentScope(doOutroCorretor, "eu", true)).toBe(true);
    expect(leadMatchesAgentScope(meu, "eu", true)).toBe(true);
  });

  it("corretor vê o próprio lead e o compartilhado por parceria", () => {
    expect(leadMatchesAgentScope(meu, "eu", false)).toBe(true);
    expect(leadMatchesAgentScope(compartilhado, "eu", false)).toBe(true);
  });

  it("corretor não vê lead de outro corretor", () => {
    expect(leadMatchesAgentScope(doOutroCorretor, "eu", false)).toBe(false);
  });

  it("lead sem responsável não aparece para corretor (mas aparece para gestão)", () => {
    expect(leadMatchesAgentScope({}, "eu", false)).toBe(false);
    expect(leadMatchesAgentScope({}, "eu", true)).toBe(true);
  });
});
