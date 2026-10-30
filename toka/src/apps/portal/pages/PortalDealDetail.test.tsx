import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { DealDTO } from "../../../shared/services/deal.service";
import type { PortalAppointment } from "../services/portal.service";
import { PortalDealDetail } from "./PortalDealDetail";

const mocks = vi.hoisted(() => ({
  getDealById: vi.fn(),
  getDeal: vi.fn(), fetchDeal: vi.fn(), updateDeal: vi.fn(), invalidateDeal: vi.fn(),
  updateDealAppointment: vi.fn(), cancelDealAppointment: vi.fn(), createDealAppointment: vi.fn(),
  getDealAppointments: vi.fn(), getDealProperty: vi.fn(), getDealChecklist: vi.fn(), getDealSignature: vi.fn(),
  getSite: vi.fn(),
  downloadDealAppointmentCalendar: vi.fn(),
  openDirectChannel: vi.fn(), openDealChannel: vi.fn(),
  listeners: new Set<(type: string, payload: Record<string, unknown>) => void>(),
  onMessage: vi.fn(),
}));
vi.mock("../services/portal.service", () => ({ portalService: mocks }));
vi.mock("../context/PortalAuthContext", () => ({ usePortalAuth: () => ({ isAuthenticated: true, user: { id: "client", email: "client@example.test" } }) }));
vi.mock("../context/PortalDealsContext", () => ({ usePortalDeals: () => mocks }));
vi.mock("../hooks/usePortalWebSocket", () => ({ usePortalWebSocket: () => ({ onMessage: mocks.onMessage }) }));

const deal: DealDTO = {
  id: "active-deal", type: "RENT", status: "PROPOSAL", tenant_id: "agency", property_id: "property",
  customer_id: "client-lead", owner_id: "owner", broker_id: "broker", value: 2000, commission: 0,
  created_at: "2026-09-17T12:00:00Z", updated_at: "2026-09-17T12:00:00Z", parties: [], signers: [], proposals: [], checklist: [],
};
const visit: PortalAppointment = {
  can_manage: true, updated_at: "2026-09-17T12:00:00Z",
  id: "lead-visit", title: "Visita solicitada antes da negociação", date: "2099-10-20", time: "14:00", end_time: "15:00",
  type: "Visita", status: "Confirmado", property: "Alameda dos Ipês, 100", created_at: "2026-09-16T12:00:00Z",
};
function page() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={client}><MemoryRouter initialEntries={["/portal/deals/active-deal"]}><Routes>
        <Route path="/portal/deals/:id" element={<PortalDealDetail />} />
        <Route path="/portal/mensagens" element={<div>Mensagens do portal</div>} />
      </Routes></MemoryRouter></QueryClientProvider>;
}
beforeEach(() => {
  vi.clearAllMocks(); mocks.listeners.clear();
  mocks.getDealById.mockResolvedValue(deal); mocks.getDeal.mockReturnValue(deal); mocks.fetchDeal.mockResolvedValue(deal);
  mocks.openDirectChannel.mockReset();
  mocks.openDealChannel.mockResolvedValue({ id: "deal-channel" });
  mocks.getDealAppointments.mockResolvedValue([visit]); mocks.getDealChecklist.mockResolvedValue([]);
  mocks.downloadDealAppointmentCalendar.mockResolvedValue(new Blob(["BEGIN:VCALENDAR"], { type: "text/calendar" }));
  Object.defineProperty(URL, "createObjectURL", { value: vi.fn(() => "blob:compromisso"), configurable: true });
  Object.defineProperty(URL, "revokeObjectURL", { value: vi.fn(), configurable: true });
  mocks.getDealProperty.mockResolvedValue({ id: "property", title: "Imóvel da negociação" });
  mocks.getSite.mockResolvedValue({
    name: "Imobiliária Exemplo",
    slug: "Exemplo",
    public_url: "https://Exemplo.imoveis.suitee.com.br",
    branding: { primary_color: "#1C905B" },
  });
  mocks.getDealSignature.mockResolvedValue(null);
  mocks.onMessage.mockImplementation((listener) => { mocks.listeners.add(listener); return () => mocks.listeners.delete(listener); });
  Element.prototype.scrollIntoView = vi.fn();
});
describe("agendamentos durante a jornada", () => {
  it("mostra o site da imobiliária do deal", async () => {
    render(page());
    expect(await screen.findByText("Site da imobiliária")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Visitar site/i })).toHaveAttribute(
      "href",
      "https://Exemplo.imoveis.suitee.com.br",
    );
  });

  it("abre os agendamentos pelo acesso rápido após avançar à proposta", async () => {
    render(page());
    fireEvent.click(await screen.findByRole("button", { name: "Ver agendamentos (1)" }));
    expect(await screen.findByText(visit.title)).toBeInTheDocument();
    expect(screen.getByText("Confirmado")).toBeInTheDocument();
    expect(screen.getByText(/14:00.*15:00/)).toBeInTheDocument();
    expect(screen.queryByText("Nenhuma visita agendada.")).not.toBeInTheDocument();
  });
  // O compromisso marcado e futuro vira arquivo no calendário de quem recebeu o
  // convite — e o download sai pela rota do PORTAL (a da agenda interna responde
  // 403 para o cliente).
  it("oferece o arquivo .ics de um compromisso confirmado e futuro", async () => {
    render(page());
    fireEvent.click(await screen.findByRole("button", { name: "Ver agendamentos (1)" }));
    fireEvent.click(await screen.findByRole("button", { name: "Adicionar ao calendário (.ics)" }));
    await waitFor(() => expect(mocks.downloadDealAppointmentCalendar).toHaveBeenCalledWith(deal.id, visit.id));
  });
  // Compromisso ainda pendente não é horário marcado: não vira evento no
  // calendário de ninguém.
  it("não oferece o arquivo .ics enquanto o compromisso está pendente", async () => {
    mocks.getDealAppointments.mockResolvedValue([{ ...visit, status: "Pendente" }]);
    render(page());
    fireEvent.click(await screen.findByRole("button", { name: "Ver agendamentos (1)" }));
    await screen.findByText(visit.title);
    expect(screen.queryByRole("button", { name: "Adicionar ao calendário (.ics)" })).not.toBeInTheDocument();
  });
  it("exibe falha da agenda com tentativa novamente, sem afirmar que não há visitas", async () => {
    mocks.getDealAppointments.mockRejectedValueOnce(new Error("agenda indisponível"));
    render(page());
    fireEvent.click(await screen.findByRole("button", { name: "Ver agendamentos" }));
    expect(await screen.findByText("Não foi possível carregar seus agendamentos.")).toBeInTheDocument();
    expect(screen.queryByText("Nenhuma visita agendada.")).not.toBeInTheDocument();
    const retryButtons = screen.getAllByRole("button", { name: "Tentar novamente" });
    fireEvent.click(retryButtons[retryButtons.length - 1]);
    await waitFor(() => expect(mocks.getDealAppointments).toHaveBeenCalledTimes(2));
    fireEvent.click(await screen.findByRole("button", { name: "Ver agendamentos (1)" }));
    expect(await screen.findByText(visit.title)).toBeInTheDocument();
  });
  it("mantém a jornada e as visitas visíveis durante uma atualização da agenda", async () => {
    render(page());
    fireEvent.click(await screen.findByRole("button", { name: "Ver agendamentos (1)" }));
    let resolve!: (appointments: PortalAppointment[]) => void;
    mocks.getDealAppointments.mockImplementationOnce(() => new Promise<PortalAppointment[]>(r => { resolve = r; }));
    await act(async () => { for (const listener of mocks.listeners) listener("appointment_update", { deal_id: deal.id }); });
    await waitFor(() => expect(mocks.getDealAppointments).toHaveBeenCalledTimes(2));
    expect(screen.getByText(visit.title)).toBeInTheDocument();
    expect(screen.getByText("Sua jornada")).toBeInTheDocument();
    expect(screen.getByText("Confirmado")).toBeInTheDocument();
    await act(async () => { resolve([{ ...visit, status: "Reagendado" }]); });
    expect(await screen.findByText("Reagendado")).toBeInTheDocument();
    expect(mocks.getDealProperty).toHaveBeenCalledTimes(1);
    expect(mocks.getDealChecklist).toHaveBeenCalledTimes(1);
    expect(mocks.getDealById).toHaveBeenCalledTimes(1);
  });
  it("preserva visitas já carregadas e avisa quando a atualização em segundo plano falha", async () => {
    render(page());
    fireEvent.click(await screen.findByRole("button", { name: "Ver agendamentos (1)" }));
    mocks.getDealAppointments.mockRejectedValueOnce(new Error("offline"));
    await act(async () => { for (const listener of mocks.listeners) listener("appointment_update", { deal_id: deal.id }); });
    expect(await screen.findByText("Algumas informações não foram atualizadas")).toBeInTheDocument();
    expect(screen.getByText(visit.title)).toBeInTheDocument();
    expect(screen.getByText("Confirmado")).toBeInTheDocument();
    expect(screen.queryByText("Nenhuma visita agendada.")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Tentar novamente" }));
    await waitFor(() => expect(screen.queryByText("Algumas informações não foram atualizadas")).not.toBeInTheDocument());
    expect(screen.getByText(visit.title)).toBeInTheDocument();
  });
  it("atualiza visitas do lead em tempo real mesmo sem vínculo ao negócio", async () => {
    render(page());
    await screen.findByRole("button", { name: "Ver agendamentos (1)" });
    mocks.getDealAppointments.mockResolvedValue([{ ...visit, time: "16:00", end_time: "17:00", status: "Reagendado" }]);
    await act(async () => { for (const listener of [...mocks.listeners]) listener("appointment_update", { tenant_id: "agency", customer_account_id: "client", appointment_id: visit.id }); });
    await waitFor(() => expect(mocks.getDealAppointments).toHaveBeenCalledTimes(2));
    fireEvent.click(await screen.findByRole("button", { name: "Ver agendamentos (1)" }));
    expect(await screen.findByText("Reagendado")).toBeInTheDocument();
    expect(screen.getByText(/16:00.*17:00/)).toBeInTheDocument();
  });
});


describe("ações de visita durante a negociação", () => {
  async function openVisits() {
    render(page());
    fireEvent.click(await screen.findByRole("button", { name: "Ver agendamentos (1)" }));
    await screen.findByText(visit.title);
  }
  it("reagenda a mesma visita com formulário preenchido e acompanha o novo horário", async () => {
    const next = { ...visit, time: "16:00", end_time: "17:00", status: "Reagendado", updated_at: "2026-09-17T12:01:00Z" };
    mocks.updateDealAppointment.mockResolvedValue(next);
    await openVisits();
    fireEvent.click(screen.getByRole("button", { name: "Alterar horário" }));
    expect(await screen.findByRole("heading", { name: "Alterar horário da visita" })).toBeInTheDocument();
    expect(screen.getByLabelText("Início")).toHaveValue("14:00");
    expect(screen.getByLabelText("Término")).toHaveValue("15:00");
    expect(screen.getByRole("grid", { name: "outubro 2099" })).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Início"), { target: { value: "16:00" } });
    mocks.getDealAppointments.mockResolvedValue([next]);
    fireEvent.click(screen.getByRole("button", { name: "Salvar novo horário" }));
    await waitFor(() => expect(mocks.updateDealAppointment).toHaveBeenCalledWith("active-deal", visit.id,
      expect.objectContaining({ date: visit.date, time: "16:00", end_time: "17:00", expected_updated_at: visit.updated_at })));
    expect(await screen.findByText("Reagendado")).toBeInTheDocument();
    expect(mocks.createDealAppointment).not.toHaveBeenCalled();
  });
  it("confirma o cancelamento e mantém a visita no acompanhamento", async () => {
    const cancelled = { ...visit, status: "Cancelado", can_manage: false };
    mocks.cancelDealAppointment.mockResolvedValue(cancelled);
    await openVisits();
    fireEvent.click(screen.getByRole("button", { name: "Cancelar visita" }));
    expect(await screen.findByRole("heading", { name: "Cancelar visita?" })).toBeInTheDocument();
    expect(mocks.cancelDealAppointment).not.toHaveBeenCalled();
    mocks.getDealAppointments.mockResolvedValue([cancelled]);
    const buttons = screen.getAllByRole("button", { name: "Cancelar visita" });
    fireEvent.click(buttons[buttons.length - 1]);
    await waitFor(() => expect(mocks.cancelDealAppointment).toHaveBeenCalledWith("active-deal",visit.id,visit.updated_at));
    expect(await screen.findByText("Cancelado")).toBeInTheDocument();
    expect(screen.getByText(visit.title)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Alterar horário" })).not.toBeInTheDocument();
  });
  it("mantém o status e permite tentar novamente quando o cancelamento falha", async () => {
    mocks.cancelDealAppointment.mockRejectedValue(new Error("offline"));
    await openVisits();
    fireEvent.click(screen.getByRole("button", { name: "Cancelar visita" }));
    const buttons = await screen.findAllByRole("button", { name: "Cancelar visita" });
    fireEvent.click(buttons[buttons.length - 1]);
    await waitFor(() => expect(mocks.cancelDealAppointment).toHaveBeenCalledTimes(1));
    expect(screen.getByText("Confirmado")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Cancelar visita?" })).toBeInTheDocument();
    expect(screen.queryByText("Cancelado")).not.toBeInTheDocument();
  });
  it("mostra erro de conflito no formulário e não encerra a alteração", async () => {
    mocks.updateDealAppointment.mockRejectedValue({ isAxiosError: true, response: { status: 409, data: { message: "A agenda foi alterada. Atualize a página antes de tentar novamente." } } });
    await openVisits();
    fireEvent.click(screen.getByRole("button", { name: "Alterar horário" }));
    fireEvent.click(await screen.findByRole("button", { name: "Salvar novo horário" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("A agenda foi alterada");
    expect(screen.getByRole("heading", { name: "Alterar horário da visita" })).toBeInTheDocument();
    expect(screen.getByText("Confirmado")).toBeInTheDocument();
  });
  it("mantém visitas sem permissão e concluídas somente para acompanhamento", async () => {
    mocks.getDealAppointments.mockResolvedValue([{ ...visit, can_manage: false }, { ...visit, id: "closed", status: "Concluido" }]);
    render(page());
    fireEvent.click(await screen.findByRole("button", { name: "Ver agendamentos (2)" }));
    expect(await screen.findByText("Concluido")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Alterar horário" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Cancelar visita" })).not.toBeInTheDocument();
    // Baixar o .ics é leitura: continua disponível para o compromisso marcado
    // mesmo quando o cliente não pode alterá-lo, e some para o já concluído.
    expect(screen.getAllByRole("button", { name: "Adicionar ao calendário (.ics)" })).toHaveLength(1);
  });
});

// O canal é por LEAD e o lead tem dono. O proprietário também vê o negócio no
// portal, mas o lead do negócio é o do OUTRO lado (locatário/comprador): abrir
// com `deal.customer_id` mandava o lead alheio para a checagem de posse e o
// servidor respondia 403 — "Erro ao falar com imobiliária ou falar com corretor".
describe("canal direto com o corretor", () => {
  it("usa o lead do PRÓPRIO assinante (proprietário), não o do cliente do negócio", async () => {
    const ownerDeal: DealDTO = {
      ...deal,
      customer_id: "lead-do-locatario",
      owner_id: "lead-do-proprietario",
      my_party_id: "party-owner",
      my_role: "owner" as DealDTO["my_role"],
      parties: [
        { id: "party-owner", lead_id: "lead-do-proprietario", account_id: "conta-proprietario", name: "Patrícia", email: "patricia@example.test", role: "owner" as any, is_primary: true, created_at: "2026-09-17T12:00:00Z", updated_at: "2026-09-17T12:00:00Z" },
        { id: "party-tenant", lead_id: "lead-do-locatario", account_id: "conta-locatario", name: "Vanessa", email: "vanessa@example.test", role: "tenant" as any, is_primary: false, created_at: "2026-09-17T12:00:00Z", updated_at: "2026-09-17T12:00:00Z" },
      ],
    };
    mocks.getDealById.mockResolvedValue(ownerDeal);
    mocks.getDeal.mockReturnValue(ownerDeal);
    mocks.fetchDeal.mockResolvedValue(ownerDeal);
    mocks.openDirectChannel.mockResolvedValue({ channel_id: "canal-1" });

    render(page());
    fireEvent.click(await screen.findByRole("button", { name: "Falar com a imobiliária" }));

    await waitFor(() =>
      expect(mocks.openDirectChannel).toHaveBeenCalledWith("lead-do-proprietario"),
    );
    expect(mocks.openDirectChannel).not.toHaveBeenCalledWith("lead-do-locatario");
  });

  it("sem lead próprio, abre o canal autorizado do negócio", async () => {
    const semLead: DealDTO = { ...deal, customer_id: "lead-do-locatario", my_party_id: "", parties: [] };
    mocks.getDealById.mockResolvedValue(semLead);
    mocks.getDeal.mockReturnValue(semLead);
    mocks.fetchDeal.mockResolvedValue(semLead);

    render(page());
    fireEvent.click(await screen.findByRole("button", { name: "Falar com a imobiliária" }));

    await waitFor(() => expect(screen.getByText("Mensagens do portal")).toBeInTheDocument());
    expect(mocks.openDirectChannel).not.toHaveBeenCalled();
    expect(mocks.openDealChannel).toHaveBeenCalledWith("active-deal");
  });
});
