import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

// O interruptor DESLIGADO já ficou invisível em produção: o componente usava
// `data-[state=unchecked]:bg-switch-background`, mas o token
// `--switch-background` não existia em CSS nenhum — a classe não gerava
// background e o trilho sumia sobre superfícies claras (popover branco,
// rodapé muted, fundo creme).
//
// O invariante é lido do CSS porque o defeito é exatamente "o token saiu do
// arquivo": nenhum teste de componente enxerga contraste. A regra aplicada é
// a WCAG 1.4.11 (contraste de componente não textual >= 3:1).

const css = readFileSync(join(__dirname, "tailwind.css"), "utf8");

function themeBlock(selector: ":root" | ".dark"): string {
  const start = css.indexOf(`${selector} {`);
  expect(start, `bloco ${selector} não encontrado em tailwind.css`).toBeGreaterThan(-1);
  const end = css.indexOf("\n}", start);
  return css.slice(start, end);
}

function hslToken(block: string, name: string): [number, number, number] {
  const match = block.match(new RegExp(`--${name}:\\s*([\\d.]+)\\s+([\\d.]+)%\\s+([\\d.]+)%`));
  expect(match, `token --${name} não declarado`).not.toBeNull();
  return [Number(match![1]), Number(match![2]), Number(match![3])];
}

function toRgb([h, s, l]: [number, number, number]): [number, number, number] {
  const sat = s / 100;
  const light = l / 100;
  const k = (n: number) => (n + h / 30) % 12;
  const a = sat * Math.min(light, 1 - light);
  const f = (n: number) => light - a * Math.max(-1, Math.min(k(n) - 3, Math.min(9 - k(n), 1)));
  return [f(0), f(8), f(4)].map((v) => Math.round(v * 255)) as [number, number, number];
}

function luminance(rgb: [number, number, number]): number {
  const [r, g, b] = rgb.map((v) => {
    const c = v / 255;
    return c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

function contrast(a: [number, number, number], b: [number, number, number]): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
}

/** Mistura alfa sobre um fundo — aproxima `bg-muted/50` sobre o popover. */
function blend(
  fg: [number, number, number],
  bg: [number, number, number],
  alpha: number,
): [number, number, number] {
  const a = toRgb(fg);
  const b = toRgb(bg);
  return a.map((v, i) => Math.round(v * alpha + b[i] * (1 - alpha))) as [number, number, number];
}

const MIN_RATIO = 3;

describe.each([
  { theme: ":root" as const, label: "tema claro" },
  { theme: ".dark" as const, label: "tema escuro" },
])("contraste do switch desligado ($label)", ({ theme }) => {
  const block = themeBlock(theme);

  it("declara o token --switch-background", () => {
    expect(hslToken(block, "switch-background")).toBeDefined();
  });

  it("mantém >= 3:1 contra as superfícies onde o switch aparece", () => {
    const track = toRgb(hslToken(block, "switch-background"));
    // Todas as superfícies já em RGB — `blend` devolve RGB, não HSL.
    const surfaces: Record<string, [number, number, number]> = {
      popover: toRgb(hslToken(block, "popover")),
      card: toRgb(hslToken(block, "card")),
      background: toRgb(hslToken(block, "background")),
      // Rodapé dos controles: bg-muted/50 sobre o popover.
      mutedOverPopover: blend(hslToken(block, "muted"), hslToken(block, "popover"), 0.5),
    };

    for (const [surface, surfaceRgb] of Object.entries(surfaces)) {
      const ratio = contrast(track, surfaceRgb);
      expect(ratio, `switch-background vs ${surface}: ${ratio.toFixed(2)}:1`).toBeGreaterThanOrEqual(
        MIN_RATIO,
      );
    }
  });
});
