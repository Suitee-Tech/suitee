import { describe, expect, it } from "vitest";

import { formatDocumentTitle, titleForPath } from "./documentTitle";

describe("título do documento por rota", () => {
  it("cobre rotas administrativas estáticas e dinâmicas", () => {
    expect(titleForPath("/")).toBe("Painel");
    expect(titleForPath("/leads")).toBe("Leads (CRM)");
    expect(titleForPath("/leads/abc-123")).toBe("Detalhes do lead");
    expect(titleForPath("/financeiro/cobrancas")).toBe("Recebimentos");
    expect(titleForPath("/financeiro/repasses/")).toBe("Repasses");
    expect(titleForPath("/locacao/abc")).toBe("Detalhes da locação");
    expect(titleForPath("/configuracoes")).toBe("Configurações");
  });

  it("cobre o portal e mantém o sufixo da marca", () => {
    expect(titleForPath("/portal")).toBe("Portal do Cliente");
    expect(titleForPath("/portal/imoveis/abc")).toBe("Detalhes do imóvel");
    expect(titleForPath("/portal/notificacoes")).toBe("Notificações");
    expect(formatDocumentTitle("/leads")).toBe("Leads (CRM) · suitee");
  });

  it("não deixa rota desconhecida sem título", () => {
    expect(titleForPath("/rota-que-nao-existe")).toBe("Página não encontrada");
  });
});
