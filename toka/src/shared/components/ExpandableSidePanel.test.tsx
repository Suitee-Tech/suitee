import { useState } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { ExpandableSidePanel } from "./ExpandableSidePanel";

function Panels() {
  const [open, setOpen] = useState(false);
  const [nested, setNested] = useState(false);
  return <>
    <button onClick={() => setOpen(true)}>Abrir compromisso</button>
    <button>Calendário ao fundo</button>
    <ExpandableSidePanel open={open} onClose={() => setOpen(false)} ariaLabel="Compromisso"
      renderHeader={({ expandButton, closeButton }) => <>{expandButton}{closeButton}</>}>
      <button onClick={() => setNested(true)}>Editar compromisso</button>
      <ExpandableSidePanel open={nested} onClose={() => setNested(false)} ariaLabel="Edição"
        renderHeader={({ closeButton }) => closeButton}>
        <input aria-label="Título" />
      </ExpandableSidePanel>
    </ExpandableSidePanel>
  </>;
}

describe("ExpandableSidePanel keyboard layers", () => {
  it("contains focus and restores the opener when closed", async () => {
    const user = userEvent.setup();
    render(<Panels />);
    const opener = screen.getByRole("button", { name: "Abrir compromisso" });
    await user.click(opener);
    const panel = screen.getByRole("dialog", { name: "Compromisso" });
    await waitFor(() => expect(panel.contains(document.activeElement)).toBe(true));
    for (let i = 0; i < 6; i++) {
      await user.tab();
      expect(panel.contains(document.activeElement)).toBe(true);
    }
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(opener).toHaveFocus();
  });

  it("Escape dismisses only the top layer and restores focus inside its parent", async () => {
    const user = userEvent.setup();
    render(<Panels />);
    await user.click(screen.getByRole("button", { name: "Abrir compromisso" }));
    const edit = screen.getByRole("button", { name: "Editar compromisso" });
    await user.click(edit);
    expect(screen.getByRole("dialog", { name: "Edição" })).toBeInTheDocument();
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Edição" })).not.toBeInTheDocument());
    expect(screen.getByRole("dialog", { name: "Compromisso" })).toBeInTheDocument();
    expect(edit).toHaveFocus();
    await user.click(screen.getByRole("button", { name: "Expandir painel" }));
    await user.keyboard("{Escape}");
    expect(screen.getByRole("dialog", { name: "Compromisso" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Expandir painel" })).toHaveAttribute("aria-pressed", "false");
  });
});
