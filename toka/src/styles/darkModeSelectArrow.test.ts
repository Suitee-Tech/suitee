import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

// Modo noturno — <select> e os campos crus.
//
// Três defeitos encadeados, todos nascidos da mesma regra global:
//
//   1. A sequência asterisco + barra dentro de um COMENTÁRIO deste arquivo
//      fechava o comentário no meio; o texto que sobrava era lido como seletor e
//      engolia a regra seguinte até o primeiro `{`. A regra engolida era
//      justamente `.dark input…, .dark textarea, .dark select { cor/fundo/borda }`
//      — então o texto digitado nos campos crus herdava preto no fundo escuro e
//      a seta NATIVA do select ficava preta (invisível).
//   2. Para "resolver" a seta invisível, alguém pintou uma seta por
//      `background-image`, sem `background-repeat: no-repeat`: o SVG repetia por
//      todo o campo e virava uma fileira de setas.
//   3. Corrigido o repeat, sobrou a soma com a seta nativa — DUAS setas no mesmo
//      campo, bem visível nos filtros de Leads.
//
// Estado final: a seta pintada foi removida (a nativa volta a aparecer porque a
// regra engolida foi ressuscitada) e o comentário não pode mais se autodestruir.
const css = readFileSync(join(__dirname, "dark-mode.css"), "utf8");

interface Rule {
  selector: string;
  body: string;
}

function parseRules(source: string): Rule[] {
  const withoutComments = source.replace(/\/\*[\s\S]*?\*\//g, "");
  return Array.from(withoutComments.matchAll(/([^{}]+)\{([^{}]*)\}/g)).map((match) => ({
    selector: match[1].trim(),
    body: match[2],
  }));
}

const rules = parseRules(css);
const selectRules = rules.filter((rule) =>
  rule.selector.split(",").some((part) => /(^|\s|>)select(\b|:|\[)/.test(part.trim())),
);

describe("dark-mode.css — comentários", () => {
  // A invariante que faltava: um terminador de comentário só pode aparecer
  // fechando um comentário aberto. Um terminador "solto" significa que um
  // comentário fechou cedo e a próxima regra virou parte do texto dele.
  it("não tem terminador de comentário sobrando", () => {
    let open = 0;
    let line = 1;
    const stray: number[] = [];
    for (let i = 0; i < css.length - 1; i += 1) {
      if (css[i] === "\n") line += 1;
      if (css[i] === "/" && css[i + 1] === "*") {
        open += 1;
        i += 1;
        continue;
      }
      if (css[i] === "*" && css[i + 1] === "/") {
        open -= 1;
        if (open < 0) {
          stray.push(line);
          open = 0;
        }
        i += 1;
      }
    }
    expect(stray, "terminador solto: um comentário fechou cedo e engoliu a regra seguinte").toEqual([]);
    expect(open, "comentário aberto sem fechar no fim do arquivo").toBe(0);
  });

  // A regra que ficou morta por causa do comentário. Se ela sumir de novo, o
  // texto digitado volta a herdar preto no fundo escuro e a seta nativa do
  // select (desenhada com a cor do texto) some — foi o que levou alguém a
  // pintar uma seta por CSS e criar a seta duplicada.
  it("mantém viva a cor dos campos crus em <select>/<input>/<textarea>", () => {
    const rule = rules.find((candidate) => /\.dark select$/.test(candidate.selector.split(",").pop()!.trim()));
    expect(rule, "a regra `.dark select` sumiu (ou foi engolida por um comentário)").toBeDefined();
    expect(rule!.body).toMatch(/color\s*:\s*var\(/);
    // Fundo e borda NÃO entram aqui de propósito: quem os define é o componente
    // (`bg-card`/`border-border`). Esta asserção existe para o restyle de cinza
    // em todo campo cru do modo noturno não voltar pela porta dos fundos.
    expect(rule!.body).not.toMatch(/background-color\s*:/);
  });
});

describe("dark-mode.css — escala soft da marca", () => {
  // No tema claro os chips da marca são fundo bem claro + texto escuro. No
  // noturno isso deixa um retângulo claro numa tela escura e o texto da marca
  // escura cai para 3.48:1 (o par que o e2e mede). A inversão precisa cobrir
  // também os `hover:` — um hover claro sob texto claro é o mesmo defeito.
  const PAIRS: [string, string][] = [
    ["bg-primary-50", "var(--color-primary-900)"],
    ["bg-primary-100", "var(--color-primary-800)"],
    ["text-primary-700", "var(--color-primary-200)"],
    ["text-primary-800", "var(--color-primary-100)"],
  ];

  it.each(PAIRS)("inverte %s para %s", (utility, expected) => {
    const matching = rules.filter((rule) => {
      const selector = rule.selector.replace(/\\/g, ""); // .hover\:x:hover -> .hover:x:hover
      return selector.includes(`.dark .${utility}`) || selector.includes(`.dark .hover:${utility}:hover`);
    });
    expect(matching.length, `nenhuma regra do modo noturno para ${utility}`).toBeGreaterThan(0);
    expect(matching.some((rule) => rule.body.includes(expected))).toBe(true);
  });

  it("cobre o estado hover dos fundos claros", () => {
    // O seletor vem escapado do CSS (`.hover\:bg-primary-50:hover`), daí a
    // normalização antes de comparar.
    const hoverRule = rules.find((rule) =>
      rule.selector.replace(/\\/g, "").includes("hover:bg-primary-50:hover"),
    );
    expect(hoverRule, "hover:bg-primary-50 sem inversão no modo noturno").toBeDefined();
  });
});

describe("dark-mode.css — seta dos <select>", () => {
  it("tem regra para o select (senão as asserções abaixo passariam vazias)", () => {
    expect(selectRules.length).toBeGreaterThan(0);
  });

  // Uma seta pintada por CSS num <select> que mantém o controle nativo é uma
  // SEGUNDA seta; quem pinta precisa desligar a nativa na mesma regra.
  it("não pinta seta em select que mantém o controle nativo", () => {
    for (const rule of selectRules) {
      if (!/background-image\s*:/.test(rule.body)) continue;
      expect(
        rule.body,
        `regra "${rule.selector}" pinta uma seta sem appearance: none — vira a segunda seta do campo`,
      ).toMatch(/appearance\s*:\s*none/);
      expect(rule.body, `regra "${rule.selector}" pinta seta sem no-repeat`).toMatch(
        /background-repeat\s*:\s*no-repeat/,
      );
    }
  });

  // Regressão explícita da regra removida (o caso do print dos filtros de Leads).
  it("não recoloca a seta global do modo noturno", () => {
    const arrowRule = selectRules.find((rule) => /background-image\s*:\s*url\(/.test(rule.body));
    expect(
      arrowRule,
      "a seta por CSS voltou: o select nativo já desenha a dele e o campo fica com duas",
    ).toBeUndefined();
  });
});
