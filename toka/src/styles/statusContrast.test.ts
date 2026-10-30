import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

// Contraste de status e de marca em texto (WCAG 2.1 AA, 4.5:1).
//
// O token `--status-*` é cor de PREENCHIMENTO; usar o mesmo tom como texto
// sobre o card/soft reprovava (warning 1,98:1 no claro). O CSS é lido do
// arquivo de propósito: o defeito é justamente um token/override que sai do
// bundle sem nenhum teste de componente enxergar a cor final.

const tailwind = readFileSync(join(__dirname, "tailwind.css"), "utf8");
const contrast = readFileSync(join(__dirname, "contrast.css"), "utf8");

type Triple = [number, number, number];

function block(css: string, selector: ":root" | ".dark"): string {
  const start = css.indexOf(`${selector} {`);
  expect(start, `bloco ${selector} ausente`).toBeGreaterThan(-1);
  return css.slice(start, css.indexOf("\n}", start));
}

function token(cssBlock: string, name: string): Triple {
  const match = cssBlock.match(
    new RegExp(`--${name}:\\s*([\\d.]+)\\s+([\\d.]+)%\\s+([\\d.]+)%`),
  );
  expect(match, `token --${name} ausente`).not.toBeNull();
  return [Number(match![1]), Number(match![2]), Number(match![3])];
}

function rgb([h, s, l]: Triple): Triple {
  const sat = s / 100;
  const light = l / 100;
  const k = (n: number) => (n + h / 30) % 12;
  const a = sat * Math.min(light, 1 - light);
  const f = (n: number) => light - a * Math.max(-1, Math.min(k(n) - 3, Math.min(9 - k(n), 1)));
  return [f(0), f(8), f(4)].map((v) => Math.round(v * 255)) as Triple;
}

function luminance(color: Triple): number {
  const [r, g, b] = color.map((v) => {
    const c = v / 255;
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

function ratio(a: Triple, b: Triple): number {
  const [hi, lo] = [luminance(rgb(a)), luminance(rgb(b))].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
}

const STATUS = ["ok", "warning", "pending", "critical", "info", "neutral"] as const;

describe.each([
  { mode: "claro", tailwind: ":root" as const, contrast: ":root" as const },
  { mode: "escuro", tailwind: ".dark" as const, contrast: ".dark" as const },
])("contraste de status no tema $mode", ({ tailwind: twSel, contrast: ctSel }) => {
  const tw = block(tailwind, twSel);
  const ct = block(contrast, ctSel);
  const card = token(tw, "card");

  it.each(STATUS)("--status-%s tem par preenchimento/foreground em AA", (name) => {
    const fill = token(tw, `status-${name}`);
    const foreground = token(tw, `status-${name}-foreground`);
    expect(ratio(fill, foreground), `${name}: foreground sobre fill`).toBeGreaterThanOrEqual(4.5);
  });

  it.each(STATUS)("--status-%s-text passa sobre card e soft", (name) => {
    const text = token(ct, `status-${name}-text`);
    const soft = token(tw, `status-${name}-soft`);
    expect(ratio(text, card), `${name}: texto sobre card`).toBeGreaterThanOrEqual(4.5);
    expect(ratio(text, soft), `${name}: texto sobre soft`).toBeGreaterThanOrEqual(4.5);
  });
});

describe("aplicação de texto de marca e status", () => {
  it("aplica --primary-text no claro e no escuro", () => {
    expect(contrast).toContain(".text-primary {");
    expect(contrast).toContain("hsl(var(--primary-text, var(--primary)))");
  });

  it("redireciona cada text-status-* e preserva o par do preenchimento", () => {
    for (const name of STATUS) {
      expect(contrast).toContain(`.text-status-${name} { color: hsl(var(--status-${name}-text)); }`);
      expect(contrast).toContain(`.bg-status-${name}.text-status-${name} { color: hsl(var(--status-${name}-foreground)); }`);
    }
  });
});
