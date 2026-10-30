import { describe, expect, it } from "vitest";
import { getHelpTopic, helpIndexBody, helpTopics, searchHelp } from "./helpContent";

describe("helpContent", () => {
  it("carrega todos os tópicos do manifest com corpo não vazio", () => {
    expect(helpTopics.length).toBeGreaterThanOrEqual(20);
    for (const t of helpTopics) {
      expect(t.id).toBeTruthy();
      expect(t.title).toBeTruthy();
      expect(t.summary).toBeTruthy();
      expect(t.body.trim().length).toBeGreaterThan(200);
    }
  });

  it("busca ranqueia o tópico certo para termos do domínio", () => {
    const hits = searchHelp("gerar DIMOB", 5);
    expect(hits.length).toBeGreaterThan(0);
    expect(hits[0].topic.id).toBe("fiscal");

    const hitsLeads = searchHelp("cadastrar lead", 5);
    expect(hitsLeads.length).toBeGreaterThan(0);
    expect(hitsLeads[0].topic.id).toBe("leads");
  });

  it("resolve tópico por id e mantém links entre tópicos", () => {
    const topic = getHelpTopic("leads");
    expect(topic).toBeDefined();
    expect(topic?.body).toContain("/ajuda?t=roleta");
  });

  it("reescreve TODOS os links .md para a rota interna da ajuda", () => {
    // Regressão: link `./03-leads.md` cru no SPA navegava para um arquivo .md
    // inexistente na rota e quebrava. Nenhum href .md pode sobrar.
    const bodies = [helpIndexBody, ...helpTopics.map((t) => t.body)];
    const mdLinks: string[] = [];
    const helpLinks: string[] = [];
    for (const body of bodies) {
      for (const m of body.matchAll(/\]\(([^)\s]+)\)/g)) {
        if (/\.md(?:#|$)/.test(m[1])) mdLinks.push(m[1]);
        if (m[1].startsWith("/ajuda?t=")) helpLinks.push(m[1]);
      }
    }
    expect(mdLinks).toEqual([]);
    expect(helpLinks.length).toBeGreaterThan(30);
    // Todo destino aponta para um tópico existente do manifest.
    const ids = new Set(helpTopics.map((t) => t.id));
    for (const link of helpLinks) {
      const id = link.replace("/ajuda?t=", "").split("#")[0];
      expect(ids.has(id)).toBe(true);
    }
  });

  it("busca sem correspondência retorna vazio", () => {
    expect(searchHelp("zzzqqq www", 5)).toHaveLength(0);
  });
});
