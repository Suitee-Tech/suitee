import { describe, expect, it } from "vitest";

import { getAdminLayoutRouteObject, getAdminLayoutRoutes } from "./adminRouteRegistry";

// P2/M19+M20 — as rotas do cliente precisam DECLARAR a permissão que abre a tela.
//
// A revisão de permissões de 2026-09-19 registrou `/configuracoes` acessível a
// todos, `/inteligencia` sem `module:intelligence`, o catch-all dentro do Layout
// e `/parceiros` subdeclarado (só `module:intelligence`, enquanto a página e a
// API operam `partnerships:read`/`partnerships:write`). O servidor é a guarda
// real; a declaração no cliente evita abrir casca/tela que só devolveria 403.
//
// Este teste trava o inventário: rota nova só entra declarando permissão ou
// estando na lista de rotas universais (com motivo no código).

// Rotas sem permissão por decisão de produto (cada uma com o motivo no
// registry): configurações e notificações são de qualquer membro, `access-denied`
// é a própria tela de recusa e a Central de Ajuda é aberta a todo usuário
// autenticado — `{ path: "ajuda", Component: HelpPage }` com o comentário no
// registry. A entrada de `ajuda` faltava aqui desde que a Central entrou, e o
// teste ficou vermelho sem que nenhuma rota tivesse regredido.
const UNIVERSAL_ROUTES = new Set(["configuracoes", "notificacoes", "access-denied", "ajuda"]);

describe("adminRouteRegistry", () => {
  it("declara permissões em toda rota, exceto as universais explícitas", () => {
    const routes = getAdminLayoutRoutes();
    const undeclared = routes
      .map((route: any) => route.path ?? "(index)")
      .filter((path: string) => path !== "(index)" && !UNIVERSAL_ROUTES.has(path))
      // rotas declaradas viram `element`; as sem declaração ficam com `Component`
      // direto (é exatamente o sinal que estamos vigiando).
      .filter((path: string) => {
        const route: any = routes.find((r: any) => (r.path ?? "(index)") === path);
        return Boolean(route?.Component) && !route?.element;
      });

    expect(undeclared).toEqual([]);
  });

  it("/parceiros exige o domínio de parcerias, não só o módulo do menu", () => {
    const partners: any = getAdminLayoutRoutes().find((route: any) => route.path === "parceiros");
    expect(partners).toBeTruthy();
    const element = partners.element as any;
    // A composição aplica uma guarda por permissão (reduceRight), então a árvore
    // tem um nível por permissão declarada.
    const declared = JSON.stringify(element);
    expect(declared).toContain("partnerships:read");
  });

  it("catch-all vive fora do Layout autenticado", () => {
    const layout: any = getAdminLayoutRouteObject();
    const paths = (layout.children ?? []).map((child: any) => child.path);
    expect(paths).not.toContain("*");
  });
});
