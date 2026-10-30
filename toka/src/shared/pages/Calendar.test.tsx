import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Appointment } from "../data";
import { calendarService } from "../services/calendar.service";
import { Calendar } from "./Calendar";
const mocks = vi.hoisted(() => ({ appointments: [] as Appointment[], isLoading: false, error: null as string | null, add: vi.fn(), update: vi.fn(), remove: vi.fn(), refresh: vi.fn(), success: vi.fn(), toastError: vi.fn() }));
vi.mock("../context/CalendarContext", () => ({ useCalendarContext: () => ({ appointments: mocks.appointments, isLoading: mocks.isLoading, error: mocks.error, addAppointment: mocks.add, updateAppointment: mocks.update, deleteAppointment: mocks.remove, refreshAppointments: mocks.refresh }) }));
vi.mock("../context/AgentsContext", () => ({ useAgentsContext: () => ({ agents: [] }) }));
vi.mock("../context/PropertiesContext", () => ({ usePropertiesContext: () => ({ properties: [] }) }));
vi.mock("../context/LeadsContext", () => ({ useLeadsContext: () => ({ leads: [] }) }));
vi.mock("../hooks/useEsteiraDeals", () => ({ useEsteiraDeals: () => ({ deals: [] }) }));
vi.mock("../context/ViewPreferencesContext", () => ({ useViewPreferences: () => [{ viewMode: "lista" }, vi.fn()] }));
vi.mock("../components/ExpandableSidePanel", () => ({ ExpandableSidePanel: ({ open, children }: any) => open ? <div>{children}</div> : null }));
vi.mock("../services/property.service", () => ({ propertyService: { getById: vi.fn().mockResolvedValue({ data: null }) } }));
vi.mock("sonner", () => ({ toast: { success: mocks.success, error: mocks.toastError } }));
const visit: Appointment = { id: "portal-visit", title: "Visita solicitada · Lucas", client: "Lucas", type: "Visita", date: "2099-10-20", time: "14:00", endTime: "15:30", location: "Imóvel", status: "Pendente", priority: "Media", requestedByClient: true, customerAccountId: "client", leadId: "lead", agentId: "broker", createdAt: "2026-09-16" };
function page() { return <MemoryRouter initialEntries={["/agenda?compromisso=portal-visit"]}><Calendar /></MemoryRouter>; }
describe("Agenda integration with portal visits", () => {
  beforeEach(() => { vi.clearAllMocks(); mocks.appointments = [visit]; mocks.isLoading = false; mocks.error = null; mocks.refresh.mockResolvedValue(undefined); });
  afterEach(cleanup);
  it("waits for loading before opening a notification and saves the existing appointment", async () => {
    mocks.appointments = []; mocks.isLoading = true;
    const view = render(page());
    expect(mocks.toastError).not.toHaveBeenCalled();
    mocks.appointments = [visit]; mocks.isLoading = false;
    view.rerender(page());
    expect(await screen.findByDisplayValue(visit.title)).toBeInTheDocument();
    mocks.update.mockResolvedValue({ ...visit, time: "16:00", endTime: "17:00" });
    fireEvent.change(screen.getByDisplayValue("14:00"), { target: { value: "16:00" } });
    fireEvent.click(screen.getByRole("button", { name: "Salvar" }));
    await waitFor(() => expect(mocks.update).toHaveBeenCalledWith("portal-visit", expect.objectContaining({ time: "16:00" })));
    await waitFor(() => expect(screen.queryByRole("button", { name: "Salvar" })).not.toBeInTheDocument());
    expect(mocks.add).not.toHaveBeenCalled();
  });
  it("mostra skeleton na área da agenda em vez do estado vazio enquanto carrega", () => {
    mocks.appointments = [];
    mocks.isLoading = true;
    render(<MemoryRouter><Calendar /></MemoryRouter>);

    expect(screen.getByTestId("calendar-skeleton")).toBeInTheDocument();
    expect(screen.queryByText("Nenhum compromisso")).not.toBeInTheDocument();
  });

  it("não dispara atualização periódica depois do carregamento", async () => {
    vi.useFakeTimers();
    try {
      render(page());
      await vi.advanceTimersByTimeAsync(31_000);
      expect(mocks.refresh).not.toHaveBeenCalled();
    } finally {
      vi.useRealTimers();
    }
  });

  it("keeps editing open and does not report success when saving fails", async () => {
    mocks.update.mockResolvedValue(null);
    render(page());
    const title = await screen.findByDisplayValue(visit.title);
    fireEvent.change(title, { target: { value: "Título que deve ser preservado" } });
    fireEvent.click(screen.getByRole("button", { name: "Salvar" }));
    await waitFor(() => expect(mocks.update).toHaveBeenCalled());
    await waitFor(() => expect(screen.getByRole("button", { name: "Salvar" })).toBeEnabled());
    expect(screen.getByDisplayValue("Título que deve ser preservado")).toBeInTheDocument();
    expect(mocks.success).not.toHaveBeenCalled();
  });
  it("lets the agency confirm a pending portal visit directly from its details", async () => {
    mocks.update.mockResolvedValue({ ...visit, status: "Confirmado" });
    render(<MemoryRouter><Calendar /></MemoryRouter>);
    expect(screen.getByText("Solicitações de visita do portal (1)")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Ver solicitação de Lucas" }));
    fireEvent.click(screen.getByRole("button", { name: "Confirmar visita" }));
    await waitFor(() => expect(mocks.update).toHaveBeenCalledWith("portal-visit", { status: "Confirmado" }));
    expect(mocks.remove).not.toHaveBeenCalled();
  });
});

// O botão vive no rodapé do painel de detalhes (EventDetail) e se esconde
// sozinho quando o compromisso não é elegível — a página não repete a regra.
describe("Adicionar ao calendário (.ics) no detalhe do compromisso", () => {
  let clickSpy: ReturnType<typeof vi.spyOn>;
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.isLoading = false; mocks.error = null; mocks.refresh.mockResolvedValue(undefined);
    // jsdom não implementa createObjectURL/revokeObjectURL nem a navegação do clique.
    Object.defineProperty(URL, "createObjectURL", { configurable: true, writable: true, value: vi.fn(() => "blob:ics") });
    Object.defineProperty(URL, "revokeObjectURL", { configurable: true, writable: true, value: vi.fn() });
    clickSpy = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});
  });
  afterEach(() => { cleanup(); clickSpy.mockRestore(); });

  it("oferece o .ics de um compromisso agendado e baixa o arquivo pelo serviço autenticado", async () => {
    mocks.appointments = [{ ...visit, status: "Agendado" }];
    const blob = new Blob(["BEGIN:VCALENDAR"], { type: "text/calendar" });
    const download = vi.spyOn(calendarService, "downloadCalendarFile").mockResolvedValue(blob);
    render(<MemoryRouter><Calendar /></MemoryRouter>);

    // Caminho de quem consulta depois: abrir o compromisso pela lista.
    fireEvent.click(screen.getByRole("button", { name: /Visita solicitada · Lucas/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Adicionar ao calendário (.ics)" }));

    await waitFor(() => expect(download).toHaveBeenCalledWith("portal-visit"));
    // Os bytes devolvidos pela rota autenticada são os que viram arquivo.
    expect(URL.createObjectURL).toHaveBeenCalledWith(blob);
    expect(clickSpy).toHaveBeenCalledTimes(1);
    download.mockRestore();
  });

  it("mostra o .ics no painel que abre logo depois de criar o compromisso", async () => {
    mocks.appointments = [];
    mocks.add.mockResolvedValue({ ...visit, id: "novo-compromisso", status: "Agendado", requestedByClient: false });
    render(<MemoryRouter><Calendar /></MemoryRouter>);

    // Compromisso particular dispensa o vínculo com lead, então o formulário
    // mínimo (título, data e horário futuros) já basta para salvar.
    fireEvent.click(screen.getByRole("button", { name: "Novo" }));
    fireEvent.change(screen.getByLabelText("Título *"), { target: { value: "Visita nova" } });
    fireEvent.change(screen.getByLabelText("Data *"), { target: { value: "2099-10-20" } });
    fireEvent.change(screen.getByLabelText("Início *"), { target: { value: "09:00" } });
    fireEvent.click(screen.getByRole("checkbox"));
    fireEvent.click(screen.getByRole("button", { name: "Salvar" }));

    expect(await screen.findByRole("button", { name: "Adicionar ao calendário (.ics)" })).toBeInTheDocument();
    expect(mocks.add).toHaveBeenCalledWith(expect.objectContaining({ title: "Visita nova", date: "2099-10-20" }));
  });

  it("não oferece o .ics enquanto o pedido está pendente (sem horário confirmado)", async () => {
    mocks.appointments = [visit];
    render(<MemoryRouter><Calendar /></MemoryRouter>);

    fireEvent.click(screen.getByRole("button", { name: /Visita solicitada · Lucas/ }));
    // O painel está aberto (a ação de editar o compromisso está lá) e ainda assim
    // não há arquivo para baixar: o pedido segue pendente.
    expect(await screen.findByRole("button", { name: "Editar compromisso" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Adicionar ao calendário (.ics)" })).not.toBeInTheDocument();
  });
});
