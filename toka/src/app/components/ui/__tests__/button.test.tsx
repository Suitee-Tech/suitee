import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { Button } from "../button";

describe("Button", () => {
  it("renders its children as the label", () => {
    render(<Button>Salvar</Button>);
    expect(screen.getByRole("button", { name: "Salvar" })).toBeInTheDocument();
  });

  it("shows a spinner and sets aria-busy when loading", () => {
    render(<Button loading>Salvar</Button>);
    const button = screen.getByRole("button");
    expect(button).toHaveAttribute("aria-busy", "true");
    expect(button).toHaveAttribute("data-loading", "true");
    expect(button.querySelector("svg")).toBeInTheDocument();
  });

  it("disables the button while loading", () => {
    render(<Button loading>Salvar</Button>);
    expect(screen.getByRole("button")).toBeDisabled();
  });

  it("keeps the original label when loading without loadingText", () => {
    render(<Button loading>Salvar</Button>);
    expect(screen.getByRole("button")).toHaveTextContent("Salvar");
  });

  it("swaps the label for loadingText while loading", () => {
    render(
      <Button loading loadingText="Salvando…">
        Salvar
      </Button>,
    );
    const button = screen.getByRole("button");
    expect(button).toHaveTextContent("Salvando…");
    expect(button).not.toHaveTextContent("Salvar");
  });

  it("shows only the spinner for an icon button while loading", () => {
    render(
      <Button size="icon" loading aria-label="Atualizar">
        <span data-testid="icon">i</span>
      </Button>,
    );
    expect(screen.queryByTestId("icon")).not.toBeInTheDocument();
    expect(
      screen.getByRole("button").querySelector("svg"),
    ).toBeInTheDocument();
  });

  it("does not throw when loading is set with asChild", () => {
    expect(() =>
      render(
        <Button asChild loading>
          <a href="/destino">Ir</a>
        </Button>,
      ),
    ).not.toThrow();
    expect(screen.getByRole("link", { name: "Ir" })).toHaveAttribute(
      "aria-busy",
      "true",
    );
  });

  it("respects an explicit disabled prop", () => {
    render(<Button disabled>Salvar</Button>);
    expect(screen.getByRole("button")).toBeDisabled();
  });
});
