import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { portalService, type PortalUser } from "../services/portal.service";
import { PortalProfile } from "./PortalProfile";

const mocks = vi.hoisted(() => ({
  user: null as PortalUser | null,
  get: vi.fn(),
  patch: vi.fn(),
  refreshUserData: vi.fn(),
}));

vi.mock("../services/portal-api", () => ({
  default: { get: mocks.get, patch: mocks.patch },
  unwrapPortalData: (response: any) => response.data.data,
}));
vi.mock("../context/PortalAuthContext", () => ({
  usePortalAuth: () => ({ user: mocks.user, refreshUserData: mocks.refreshUserData }),
}));
vi.mock("../components/MarketingConsentCard", () => ({ MarketingConsentCard: () => null }));
vi.mock("../components/LGPDDataExportCard", () => ({ LGPDDataExportCard: () => null }));
vi.mock("../components/DeleteAccountCard", () => ({ DeleteAccountCard: () => null }));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

async function loadUser(raw: Record<string, unknown>) {
  mocks.get.mockResolvedValueOnce({ data: { data: { id: "client", ...raw } } });
  mocks.user = await portalService.getCurrentUser();
}

function profile() {
  return <MemoryRouter><PortalProfile /></MemoryRouter>;
}

function edit() {
  fireEvent.click(screen.getByRole("button", { name: "Editar dados" }));
}

describe("PortalProfile name editing", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.user = null;
    vi.stubGlobal("ResizeObserver", class {
      observe() {}
      unobserve() {}
      disconnect() {}
    });
    mocks.patch.mockResolvedValue({});
    mocks.refreshUserData.mockResolvedValue(undefined);
  });
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("loads legacy names separately and saves without duplicating the surname", async () => {
    await loadUser({ firstname: "Lucas Mendes Barros", lastname: "", fullname: "Lucas Mendes Barros" });
    const view = render(profile());
    edit();
    expect(screen.getByLabelText(/^Nome\s*\*?$/)).toHaveValue("Lucas");
    expect(screen.getByLabelText(/^Sobrenome/)).toHaveValue("Mendes Barros");

    fireEvent.change(screen.getByLabelText(/^Nome\s*\*?$/), { target: { value: "Luís" } });
    mocks.refreshUserData.mockImplementationOnce(async () => {
      await loadUser({ firstname: "Luís", lastname: "Mendes Barros", fullname: "Luís Mendes Barros" });
    });
    fireEvent.click(screen.getByRole("button", { name: "Salvar" }));
    await waitFor(() => expect(mocks.patch).toHaveBeenCalledWith("/auth/me", expect.objectContaining({
      firstname: "Luís", lastname: "Mendes Barros",
    })));
    await screen.findByRole("button", { name: "Editar dados" });
    view.rerender(profile());
    edit();
    expect(screen.getByLabelText(/^Nome\s*\*?$/)).toHaveValue("Luís");
    expect(screen.getByLabelText(/^Sobrenome/)).toHaveValue("Mendes Barros");
  });

  it("uses the current account when it loads after the initial render", async () => {
    const view = render(profile());
    await loadUser({ firstname: "Lucas Mendes Barros", lastname: "" });
    view.rerender(profile());
    edit();
    expect(screen.getByLabelText(/^Nome\s*\*?$/)).toHaveValue("Lucas");
    expect(screen.getByLabelText(/^Sobrenome/)).toHaveValue("Mendes Barros");
  });

  it("discards cancelled changes when reopening the form", async () => {
    await loadUser({ firstname: "Lucas", lastname: "Mendes Barros" });
    render(profile());
    edit();
    fireEvent.change(screen.getByLabelText(/^Nome\s*\*?$/), { target: { value: "Outro" } });
    fireEvent.click(screen.getByRole("button", { name: "Cancelar" }));
    edit();
    expect(screen.getByLabelText(/^Nome\s*\*?$/)).toHaveValue("Lucas");
    expect(mocks.patch).not.toHaveBeenCalled();
  });

  it("preserves compound first names already stored separately", async () => {
    await loadUser({ firstname: "Maria Clara", lastname: "de Souza" });
    render(profile());
    edit();
    expect(screen.getByLabelText(/^Nome\s*\*?$/)).toHaveValue("Maria Clara");
    expect(screen.getByLabelText(/^Sobrenome/)).toHaveValue("de Souza");
  });
});
