import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";

import { FieldRow, SettingsInput, SettingsSelect, Toggle } from "./_shared";

describe("campos compartilhados de Configurações", () => {
  it("associa label, descrição, obrigatoriedade e erro ao input", () => {
    render(
      <FieldRow label="Telefone" description="Com DDD" required>
        <SettingsInput value="" onChange={vi.fn()} error="Campo obrigatório" />
      </FieldRow>,
    );

    const input = screen.getByLabelText(/telefone/i);
    expect(input).toHaveAttribute("aria-required", "true");
    expect(input).toHaveAttribute("aria-invalid", "true");
    expect(input.getAttribute("aria-describedby")).toContain(screen.getByText("Com DDD").id);
    expect(input.getAttribute("aria-describedby")).toContain(screen.getByRole("alert").id);
    expect(screen.getByRole("alert")).toHaveTextContent("Campo obrigatório");
  });

  it("dá nome e descrição ao select e ao switch", () => {
    render(
      <>
        <FieldRow label="Frequência" description="Intervalo de entrega">
          <SettingsSelect value="24h" onChange={vi.fn()} options={[{ value: "24h", label: "Diária" }]} />
        </FieldRow>
        <FieldRow label="Modo escuro" description="Preferência do usuário">
          <Toggle checked={false} onChange={vi.fn()} />
        </FieldRow>
      </>,
    );

    const select = screen.getByLabelText("Frequência");
    expect(select).toHaveAttribute("aria-describedby", screen.getByText("Intervalo de entrega").id);
    const toggle = screen.getByRole("switch", { name: "Modo escuro" });
    expect(toggle).toHaveAttribute("aria-describedby", screen.getByText("Preferência do usuário").id);
  });

  it("associa o erro exibido pelo select", () => {
    render(
      <FieldRow label="Frequência">
        <SettingsSelect value="24h" onChange={vi.fn()} options={[]} error="Escolha uma frequência" />
      </FieldRow>,
    );
    expect(screen.getByLabelText("Frequência")).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByRole("alert")).toHaveTextContent("Escolha uma frequência");
  });
});
