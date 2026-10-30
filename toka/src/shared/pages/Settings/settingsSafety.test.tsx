import { act, createRef } from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { GeneralTab } from "./GeneralTab";
import { NotificationsTab } from "./NotificationsTab";
import type { TabRef } from "./_shared";

const api = vi.hoisted(() => ({
  getPreferences: vi.fn(), getCompanySettings: vi.fn(), getNotifications: vi.fn(),
  updatePreferences: vi.fn(), updateCompanySettings: vi.fn(), updateNotifications: vi.fn(),
}));
vi.mock("../../services/settings.service", () => ({ settingsService: api }));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), info: vi.fn() } }));
const channels = [
  { id: "a", label: "Lead", category: "Leads", email: false, whatsapp: false, inApp: false },
  { id: "b", label: "Visita", category: "Leads", email: false, whatsapp: true, inApp: false },
];
beforeEach(() => {
  vi.clearAllMocks();
  api.getPreferences.mockResolvedValue({ timezone: "America/Manaus", session_timeout: 60 });
  api.getCompanySettings.mockResolvedValue({ currency: "BRL", autoSave: false, compactTables: true });
  api.getNotifications.mockResolvedValue({ channels, deliveryFreq: "daily" });
});

describe("configurações carregadas antes de editar ou salvar", () => {
  it("bloqueia defaults após leitura parcial, recupera e salva os valores canônicos", async () => {
    api.getCompanySettings.mockRejectedValueOnce(new Error("offline"));
    const ref = createRef<TabRef>();
    render(<GeneralTab ref={ref} onChange={vi.fn()} />);
    await screen.findByRole("alert");
    expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
    await expect(ref.current!.save()).rejects.toThrow("Carregue");
    expect(api.updatePreferences).not.toHaveBeenCalled();
    expect(api.updateCompanySettings).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Tentar novamente" }));
    await screen.findByText("Exportar dados");
    await act(() => ref.current!.save());
    expect(api.updatePreferences).toHaveBeenCalledWith(expect.objectContaining({ timezone: "America/Manaus", session_timeout: 60 }));
    // O fuso é da IMOBILIÁRIA: vai junto do restante das configurações da
    // empresa (é ele que manda nos lançamentos e relatórios).
    expect(api.updateCompanySettings).toHaveBeenCalledWith({
      currency: "BRL",
      autoSave: false,
      compactTables: true,
      timezone: "America/Manaus",
    });
    expect(screen.queryByRole("button", { name: /Exportar|Importar/ })).not.toBeInTheDocument();
  });

  it("bloqueia gravação durante a leitura e após erro de notificações", async () => {
    api.getNotifications.mockRejectedValueOnce(new Error("offline"));
    const ref = createRef<TabRef>();
    render(<NotificationsTab ref={ref} onChange={vi.fn()} />);
    await expect(ref.current!.save()).rejects.toThrow("Carregue");
    await screen.findByRole("alert");
    await expect(ref.current!.save()).rejects.toThrow("Carregue");
    expect(api.updateNotifications).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Tentar novamente" }));
    await screen.findByRole("checkbox", { name: "Todas as notificações por E-mail" });
    await act(() => ref.current!.save());
    expect(api.updateNotifications).toHaveBeenCalledWith(expect.objectContaining({ channels, deliveryFreq: "daily" }));
    expect(screen.queryByRole("button", { name: "Testar" })).not.toBeInTheDocument();
  });

  it("deriva globais desligado, misto e ligado dos canais persistidos", async () => {
    render(<NotificationsTab onChange={vi.fn()} />);
    const email = await screen.findByRole("checkbox", { name: "Todas as notificações por E-mail" });
    expect(email).toHaveAttribute("aria-checked", "false");
    expect(screen.getByRole("checkbox", { name: "Todas as notificações por WhatsApp" })).toHaveAttribute("aria-checked", "mixed");
    fireEvent.click(email);
    await waitFor(() => expect(email).toHaveAttribute("aria-checked", "true"));
    fireEvent.click(email);
    expect(email).toHaveAttribute("aria-checked", "false");
  });
});


it("recolhe cada categoria independentemente e nomeia os horários silenciosos", async () => {
  api.getNotifications.mockResolvedValue({ channels: [...channels, { ...channels[0], id: 'c', category: 'Vistorias', label: 'Vistoria' }] });
  render(<NotificationsTab onChange={vi.fn()} />);
  const category = await screen.findByRole("button", { name: "Leads (2 gatilhos)" });
  expect(category).toHaveAttribute("aria-expanded", "true");
  fireEvent.click(category);
  expect(category).toHaveAttribute("aria-expanded", "false");
  expect(screen.queryByRole("button", { name: "Lead: E-mail" })).not.toBeInTheDocument();
  await waitFor(() => expect(screen.getByRole("button", { name: "Vistoria: E-mail" })).toBeVisible());
  fireEvent.click(category);
  expect(screen.getByRole("button", { name: "Lead: E-mail" })).toBeVisible();
  fireEvent.click(screen.getByRole("switch", { name: "Horário silencioso" }));
  expect(screen.getByLabelText("Início do horário silencioso")).toHaveValue("22:00");
  expect(screen.getByLabelText("Fim do horário silencioso")).toHaveValue("07:00");
});
