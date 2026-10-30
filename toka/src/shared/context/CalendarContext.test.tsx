import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { CalendarProvider, useCalendarContext } from "./CalendarContext";
import { calendarService, type ApiAppointment } from "../services/calendar.service";
vi.mock("../services/calendar.service", () => ({ calendarService: { getAll: vi.fn(), update: vi.fn() } }));
vi.mock("../hooks/useDataChangeRefresh", () => ({ useDataChangeRefresh: () => undefined }));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
const empty: any[] = [];
let calendar: ReturnType<typeof useCalendarContext>;
function Harness() { calendar = useCalendarContext(); return <div><button onClick={() => calendar.setScope("team")}>Equipe</button>{calendar.appointments.map((a) => <p key={a.id}>{a.status}</p>)}{calendar.error && <p role="alert">{calendar.error}</p>}</div>; }
function page(canViewTeam = false, companyId = "agency", permissionsLoading = false) { return <CalendarProvider isAuthenticated companyId={companyId} permissionsLoading={permissionsLoading} agents={empty} properties={empty} leads={empty} setLeads={vi.fn()} canViewTeam={canViewTeam}><Harness /></CalendarProvider>; }
const appointment: ApiAppointment = { id: "visit", title: "Visita", type: "Visita", client: "Lucas", date: "2099-10-20", time: "14:00", endTime: "15:30", status: "Pendente", requestedByClient: true, customerAccountId: "client" };
describe("CalendarContext mutation results", () => {
  beforeEach(() => { vi.clearAllMocks(); vi.mocked(calendarService.getAll).mockResolvedValue({ data: [appointment], meta: {} }); vi.spyOn(console, "error").mockImplementation(() => {}); });
  afterEach(() => { cleanup(); vi.restoreAllMocks(); });
  it.each([
    { type: "Entrega", status: "Em Andamento" },
    { type: "Tipo legado", status: "Status legado" },
    { type: "Visita", status: "Agendado" },
  ])("preserves $type/$status while editing the title", async ({ type, status }) => {
    const stored = { ...appointment, type, status } as ApiAppointment;
    vi.mocked(calendarService.getAll).mockResolvedValue({ data: [stored], meta: {} });
    vi.mocked(calendarService.update).mockResolvedValue({ data: { ...stored, title: "Título atualizado" } });
    render(page());
    await screen.findByText(status);
    expect(calendar.appointments[0]).toMatchObject({ type, status });
    await act(async () => { await calendar.updateAppointment("visit", { title: "Título atualizado" }); });
    expect(calendarService.update).toHaveBeenCalledWith("visit", { title: "Título atualizado" });
    expect(calendar.appointments[0]).toMatchObject({ type, status, title: "Título atualizado" });
  });
  it("waits for the API and returns the saved appointment", async () => {
    render(page());
    await screen.findByText("Pendente");
    let resolve!: (value: any) => void;
    vi.mocked(calendarService.update).mockReturnValue(new Promise((done) => { resolve = done; }));
    let result: unknown;
    let pending!: Promise<unknown>;
    act(() => { pending = calendar.updateAppointment("visit", { status: "Confirmado" }); void pending.then((value) => { result = value; }); });
    expect(result).toBeUndefined();
    await act(async () => { resolve({ data: { ...appointment, status: "Confirmado" } }); await pending; });
    expect(result).toMatchObject({ id: "visit", status: "Confirmado", requestedByClient: true });
  });
  it("returns failure and rolls back optimistic status even when rereading also fails", async () => {
    render(page());
    await screen.findByText("Pendente");
    vi.mocked(calendarService.update).mockRejectedValue({ response: { status: 409 } });
    vi.mocked(calendarService.getAll).mockRejectedValue(new Error("offline"));
    let result: unknown;
    await act(async () => { result = await calendar.updateAppointment("visit", { status: "Confirmado" }); });
    expect(result).toBeNull();
    await waitFor(() => expect(screen.getByText("Pendente")).toBeInTheDocument());
    expect(screen.getByRole("alert")).toBeInTheDocument();
  });
  it("keeps cleared links cleared after a successful save", async () => {
    const stored = { ...appointment, propertyId: "property", property: "Imóvel", dealId: "old", updatedAt: "2026-09-17T10:00:00Z" };
    vi.mocked(calendarService.getAll).mockResolvedValue({ data: [stored], meta: {} });
    vi.mocked(calendarService.update).mockResolvedValue({ data: { ...stored, propertyId: undefined, property: "", dealId: "selected", updatedAt: "2026-09-17T10:01:00Z" } });
    render(page()); await screen.findByText("Pendente");
    await act(async () => { await calendar.updateAppointment("visit", { propertyId: "", contractId: "selected" }); });
    expect(calendarService.update).toHaveBeenCalledWith("visit", { propertyId: "", dealId: "selected", expectedUpdatedAt: stored.updatedAt });
    expect(calendar.appointments[0]).toMatchObject({ propertyId: undefined, location: "", contractId: "selected" });
  });
  it("aborts a collection when its scope unmounts", async () => {
    const view = render(page()); await screen.findByText("Pendente");
    const signal = vi.mocked(calendarService.getAll).mock.calls[0][1]?.signal;
    expect(signal?.aborted).toBe(false); view.unmount(); expect(signal?.aborted).toBe(true);
  });

  it("não recarrega a agenda quando as permissões são revalidadas", async () => {
    const view = render(page(false, "agency"));
    await screen.findByText("Pendente");
    const calls = vi.mocked(calendarService.getAll).mock.calls.length;

    view.rerender(page(false, "agency", true));
    await act(async () => {});

    expect(screen.getByText("Pendente")).toBeInTheDocument();
    expect(vi.mocked(calendarService.getAll).mock.calls.length).toBe(calls);
  });

  it("reaproveita os compromissos ao remontar e revalida em segundo plano", async () => {
    const first = render(page(false, "cache-agency"));
    await screen.findByText("Pendente");
    first.unmount();

    let resolve!: (value: any) => void;
    vi.mocked(calendarService.getAll).mockReturnValue(new Promise((done) => { resolve = done; }));
    render(page(false, "cache-agency"));

    expect(screen.getByText("Pendente")).toBeInTheDocument();
    expect(calendar.isLoading).toBe(false);
    await act(async () => { resolve({ data: [appointment], meta: {} }); });
  });

  it("loads the team scope only after a manager explicitly selects it", async () => {
    render(page(true));
    await screen.findByText("Pendente");
    expect(calendarService.getAll).toHaveBeenCalledWith({ scope: "mine" }, expect.anything());
    fireEvent.click(screen.getByRole("button", { name: "Equipe" }));
    await waitFor(() => expect(calendarService.getAll).toHaveBeenCalledWith({ scope: "team" }, expect.anything()));
    expect(calendar.scope).toBe("team");
  });

});
