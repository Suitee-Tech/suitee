import { readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";

import { LAYER, layerOf } from "./layers";
import { ALERT_DIALOG_LAYER_CLASS } from "./alert-dialog";
import { DIALOG_LAYER_CLASS } from "./dialog";
import { FLOATING_LAYER_CLASS } from "./popover";
import { ExpandableSidePanel } from "../components/ExpandableSidePanel";
import { ConfirmDialog } from "../components/ConfirmDialog";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

// Regressão relatada em 2026-10-05: "cancelar uma ordem de pagamento não é
// possível pois o alert de confirmação fica abaixo do painel lateral". O painel
// de ação (FinancialActionPanel) usa camada 60 e o painel de capacidades usa 200;
// o AlertDialog estava em `z-50` e ficava atrás dos dois. Estes testes travam a
// escala para que uma confirmação nunca volte a nascer atrás de um painel.
describe("escala de camadas", () => {
  it("mantém a ordem de leitura da tela", () => {
    expect(LAYER.panel).toBeLessThan(LAYER.panelAction);
    expect(LAYER.panelAction).toBeLessThan(LAYER.panelTop);
    expect(LAYER.panelTop).toBeLessThan(LAYER.dialog);
    expect(LAYER.dialog).toBeLessThan(LAYER.floating);
    expect(LAYER.floating).toBeLessThan(LAYER.floatingTop);
  });

  it("usa a escala nos primitivos", () => {
    expect(layerOf(ALERT_DIALOG_LAYER_CLASS)).toBe(LAYER.dialog);
    expect(layerOf(DIALOG_LAYER_CLASS)).toBe(LAYER.dialog);
    expect(layerOf(FLOATING_LAYER_CLASS)).toBe(LAYER.floating);
  });

  it("desenha a confirmação acima do painel que a abriu", () => {
    render(
      <ExpandableSidePanel
        open
        onClose={vi.fn()}
        zIndex={LAYER.panelTop}
        ariaLabel="Ordem de pagamento"
      >
        <ConfirmDialog
          open
          title="Cancelar ordem de pagamento"
          description="A ordem será encerrada sem registrar pagamento."
          confirmLabel="Cancelar ordem"
          variant="warning"
          onConfirm={vi.fn()}
          onCancel={vi.fn()}
        />
      </ExpandableSidePanel>,
    );

    const dialog = screen.getByRole("alertdialog");
    const dialogLayer = layerOf(dialog.className);
    expect(dialogLayer).not.toBeNull();
    // Acima do painel de maior camada da aplicação — a confirmação é a última
    // decisão antes do efeito, nunca pode ficar atrás da superfície que a abriu.
    expect(dialogLayer!).toBeGreaterThan(LAYER.panelTop);
  });

  // Nada fora dos primitivos pode subir acima da camada de diálogo: seria a
  // mesma escondida, agora por outro painel. Os primitivos de menu flutuante são
  // a exceção deliberada (precisam funcionar DENTRO dos diálogos).
  it("nenhum arquivo fora dos primitivos declara camada acima do diálogo", () => {
    const srcRoot = path.resolve(__dirname, "../..");
    const floatingPrimitives = new Set([
      "shared/ui/popover.tsx",
      "shared/ui/select.tsx",
      "shared/ui/dropdown-menu.tsx",
      "shared/ui/context-menu.tsx",
      "shared/ui/tooltip.tsx",
      "shared/ui/hover-card.tsx",
      "shared/ui/menubar.tsx",
      "shared/ui/searchable-select.tsx",
      "shared/ui/layers.ts",
      "shared/ui/layers.test.tsx",
    ]);

    const offenders: string[] = [];
    const walk = (dir: string) => {
      for (const entry of readdirSync(dir, { withFileTypes: true })) {
        const file = path.join(dir, entry.name);
        if (entry.isDirectory()) {
          walk(file);
          continue;
        }
        if (!/\.(ts|tsx)$/.test(entry.name) || /\.(test|spec)\./.test(entry.name)) continue;
        const relative = path.relative(srcRoot, file);
        if (floatingPrimitives.has(relative)) continue;
        const source = readFileSync(file, "utf8");
        const layers = [
          ...source.matchAll(/z-\[(\d+)\]/g),
          ...source.matchAll(/\bz-(\d+)\b/g),
          ...source.matchAll(/zIndex[=:]\s*\{?(\d+)/g),
        ].map((match) => Number(match[1]));
        for (const layer of layers) {
          if (layer > LAYER.dialog) offenders.push(`${relative} -> ${layer}`);
        }
      }
    };
    walk(srcRoot);

    expect(offenders).toEqual([]);
  });
});
