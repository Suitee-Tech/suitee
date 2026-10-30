import { act, forwardRef, useImperativeHandle } from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { describe, expect, it, vi } from "vitest";

import { Settings } from "../Settings";

vi.mock("../../context/AuthContext", () => ({
  useAuth: () => ({
    user: { permission: "owner", id: "u1", current_company_id: "c1" },
    refreshUserData: vi.fn(),
  }),
}));

vi.mock("../../context/PermissionsContext", () => ({
  usePermissionsContext: () => ({ can: () => true }),
}));

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

vi.mock("./ProfileTab", () => ({
  ProfileTab: forwardRef(function ProfileTabMock({ onChange }: { onChange: () => void }, ref) {
    useImperativeHandle(ref, () => ({ save: async () => {} }));
    return <button onClick={onChange}>editar perfil</button>;
  }),
}));

vi.mock("./CompanyTab", () => ({
  CompanyTab: forwardRef(function CompanyTabMock({ onChange }: { onChange: () => void }, ref) {
    useImperativeHandle(ref, () => ({ save: async () => {} }));
    return <button onClick={onChange}>editar empresa</button>;
  }),
}));

function renderSettings() {
  const router = createMemoryRouter(
    [
      { path: "/", element: <Settings /> },
      { path: "/leads", element: <div>Página de leads</div> },
    ],
    { initialEntries: ["/"] },
  );
  render(<RouterProvider router={router} />);
  return router;
}

describe("Settings — alterações pendentes", () => {
  it("pede confirmação, permite continuar editando e só troca após descartar", async () => {
    renderSettings();

    fireEvent.click(screen.getByRole("button", { name: "editar perfil" }));
    expect(screen.getByText("Alterações não salvas")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Empresa & Equipe" }));
    const dialog = screen.getByRole("alertdialog", { name: "Descartar alterações não salvas?" });
    expect(dialog).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Continuar editando" }));
    await waitFor(() => expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument());
    expect(screen.getByRole("button", { name: "editar perfil" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "editar empresa" })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Empresa & Equipe" }));
    fireEvent.click(screen.getByRole("button", { name: "Descartar e trocar" }));

    expect(await screen.findByRole("button", { name: "editar empresa" })).toBeInTheDocument();
    expect(screen.queryByText("Alterações não salvas")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Salvar Alterações" })).toBeDisabled();
  });

  it("bloqueia navegação para outra rota e só sai após descartar", async () => {
    const router = renderSettings();

    fireEvent.click(screen.getByRole("button", { name: "editar perfil" }));
    await act(async () => {
      await router.navigate("/leads");
    });

    expect(await screen.findByRole("alertdialog", { name: "Sair sem salvar?" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Continuar editando" }));
    await waitFor(() => expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument());
    expect(router.state.location.pathname).toBe("/");

    await act(async () => {
      await router.navigate("/leads");
    });
    fireEvent.click(await screen.findByRole("button", { name: "Sair sem salvar" }));

    await waitFor(() => expect(router.state.location.pathname).toBe("/leads"));
    expect(await screen.findByText("Página de leads")).toBeInTheDocument();
  });
});
