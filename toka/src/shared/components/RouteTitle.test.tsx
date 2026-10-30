import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, useNavigate } from "react-router";
import { afterEach, describe, expect, it } from "vitest";

import { RouteTitle } from "./RouteTitle";

function Harness() {
  const navigate = useNavigate();
  return (
    <>
      <RouteTitle />
      <button type="button" onClick={() => navigate("/configuracoes")}>ir para configurações</button>
    </>
  );
}

afterEach(() => {
  document.title = "";
});

describe("RouteTitle", () => {
  it("atualiza o título quando a rota muda", async () => {
    render(
      <MemoryRouter initialEntries={["/leads"]}>
        <Harness />
      </MemoryRouter>,
    );

    await waitFor(() => expect(document.title).toBe("Leads (CRM) · suitee"));
    fireEvent.click(screen.getByRole("button", { name: "ir para configurações" }));
    await waitFor(() => expect(document.title).toBe("Configurações · suitee"));
  });
});
