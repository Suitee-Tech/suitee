import { beforeEach, describe, expect, it } from "vitest";

import {
  PENDING_APPEARANCE_KEY,
  clearPendingAppearance,
  hasPendingAppearance,
  mergeServerAppearance,
  readPendingAppearance,
  writePendingAppearance,
} from "./pendingAppearance";

// Rascunho local da aba Aparência. O defeito que isto resolve: a aba aplica as
// mudanças na hora e grava no localStorage, mas o envio ao servidor só acontece
// no "Salvar Alterações" — e o ThemeProvider relê a aparência do servidor a cada
// carga, então uma recarga antes de salvar devolvia o valor do tenant e a
// configuração parecia não persistir.

interface Appearance {
  theme: string;
  primaryColor: string;
  loginTagline: string;
  sidebarStyle?: string;
}

const base: Appearance = { theme: "auto", primaryColor: "#013529", loginTagline: "Sua plataforma" };

describe("rascunho local da aparência", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("começa vazio", () => {
    expect(readPendingAppearance()).toBeNull();
    expect(hasPendingAppearance()).toBe(false);
  });

  it("grava, lê e limpa", () => {
    writePendingAppearance({ theme: "dark", primaryColor: "#7C3AED" });
    expect(readPendingAppearance()).toEqual({ theme: "dark", primaryColor: "#7C3AED" });
    expect(hasPendingAppearance()).toBe(true);

    clearPendingAppearance();
    expect(readPendingAppearance()).toBeNull();
  });

  it("ignora conteúdo corrompido em vez de quebrar o tema", () => {
    window.localStorage.setItem(PENDING_APPEARANCE_KEY, "{não é json");
    expect(readPendingAppearance()).toBeNull();

    window.localStorage.setItem(PENDING_APPEARANCE_KEY, "[1,2,3]");
    expect(readPendingAppearance()).toBeNull();

    window.localStorage.setItem(PENDING_APPEARANCE_KEY, "null");
    expect(readPendingAppearance()).toBeNull();
  });
});

describe("mergeServerAppearance", () => {
  it("sem rascunho, o servidor manda (comportamento do tenant)", () => {
    const merged = mergeServerAppearance<Appearance>(
      base,
      { theme: "light", loginTagline: "Do servidor" },
      null,
    );
    expect(merged.theme).toBe("light");
    expect(merged.loginTagline).toBe("Do servidor");
  });

  // O caso do relatório: o usuário trocou para escuro e recarregou sem salvar.
  it("com rascunho, a escolha local vence a leitura do servidor", () => {
    const merged = mergeServerAppearance<Appearance>(
      base,
      { theme: "auto", loginTagline: "Do servidor" },
      { theme: "dark" },
    );
    expect(merged.theme).toBe("dark");
    // Campos que o usuário não tocou continuam vindo do servidor.
    expect(merged.loginTagline).toBe("Do servidor");
    expect(merged.primaryColor).toBe("#013529");
  });

  it("o rascunho pode sobrescrever mais de um campo", () => {
    const merged = mergeServerAppearance<Appearance>(
      base,
      { primaryColor: "#013529", loginTagline: "Do servidor" },
      { primaryColor: "#7C3AED", loginTagline: "Minha frase" },
    );
    expect(merged.primaryColor).toBe("#7C3AED");
    expect(merged.loginTagline).toBe("Minha frase");
  });

  it("pickOne normaliza o valor já com o rascunho aplicado", () => {
    const pickTheme = (candidate: unknown, previous: unknown) =>
      ["light", "dark", "auto"].includes(String(candidate)) ? candidate : previous;

    const valido = mergeServerAppearance<Appearance>(base, { theme: "auto" }, { theme: "dark" }, pickTheme);
    expect(valido.theme).toBe("dark");

    // Valor inválido no rascunho não pode deixar o app sem tema.
    const invalido = mergeServerAppearance<Appearance>(base, { theme: "light" }, { theme: "roxo" }, pickTheme);
    expect(invalido.theme).toBe("auto");
  });
});
