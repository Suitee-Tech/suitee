import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { PortalVisitActions } from "./PortalVisitActions";
import type { PortalOverviewLead } from "../services/portal.service";
import { visitStatusClass } from "../pages/portalVisitPresentation";
const mocks = vi.hoisted(() => ({ downloadDealAppointmentCalendar: vi.fn() }));
vi.mock("../services/portal.service", () => ({ portalService: mocks }));
const lead: PortalOverviewLead = { id: "lead", name: "Cliente", has_preferences: true, matched_properties: [], recommended_property_id: "property", recommended_property: { id: "property", title: "Apartamento Jardim" }, visit_request: { property_id: "property", appointment_id: "appointment", date: "2099-10-20", time: "14:00", end_time: "15:00", status: "Confirmado" } };
function setup(status = "Confirmado", onCancel = vi.fn().mockResolvedValue(undefined)) {
  const current = { ...lead, visit_request: { ...lead.visit_request, status } };
  render(<PortalVisitActions lead={current} dealId="deal" cancelLoading={false} onReschedule={vi.fn()} onCancel={onCancel} />);
  return onCancel;
}
beforeEach(() => {
  vi.clearAllMocks();
  mocks.downloadDealAppointmentCalendar.mockResolvedValue(new Blob(["BEGIN:VCALENDAR"], { type: "text/calendar" }));
  Object.defineProperty(URL, "createObjectURL", { value: vi.fn(() => "blob:compromisso"), configurable: true });
  Object.defineProperty(URL, "revokeObjectURL", { value: vi.fn(), configurable: true });
});
describe("portal visit actions", () => {
  it("requires contextual confirmation before cancelling a confirmed visit", async () => {
    const user = userEvent.setup();
    const cancel = setup();
    await user.click(screen.getByRole("button", { name: "Cancelar visita" }));
    expect(cancel).not.toHaveBeenCalled();
    const dialog = screen.getByRole("alertdialog");
    expect(dialog).toHaveTextContent("Apartamento Jardim");
    expect(dialog).toHaveTextContent("20/10/2099 · 14:00–15:00");
    await user.click(screen.getByRole("button", { name: "Manter visita" }));
    expect(cancel).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "Cancelar visita" }));
    await user.click(screen.getAllByRole("button", { name: "Cancelar visita" }).pop()!);
    await waitFor(() => expect(cancel).toHaveBeenCalledTimes(1));
  });
  it("names pending cancellation as a request", async () => {
    const user = userEvent.setup();
    const cancel = setup("Aguardando");
    await user.click(screen.getByRole("button", { name: "Cancelar solicitação" }));
    expect(cancel).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
  });
  it("keeps confirmation open when cancellation fails", async () => {
    const user = userEvent.setup();
    setup("Confirmado", vi.fn().mockRejectedValue(new Error("offline")));
    await user.click(screen.getByRole("button", { name: "Cancelar visita" }));
    await user.click(screen.getAllByRole("button", { name: "Cancelar visita" }).pop()!);
    await waitFor(() => expect(screen.getByRole("button", { name: "Manter visita" })).not.toBeDisabled());
    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
  });
  it("distinguishes pending, confirmed and cancelled status styles", () => {
    const styles = ["Aguardando", "Confirmado", "Cancelado"].map((status) => visitStatusClass({ status }));
    expect(new Set(styles).size).toBe(3);
  });
  // A visita confirmada e futura vira arquivo no calendário de quem recebeu o
  // convite — e o download tem de sair pela rota do PORTAL (a da agenda interna
  // responde 403 para o cliente).
  it("offers the calendar file for a confirmed future visit, downloaded through the portal", async () => {
    const user = userEvent.setup();
    setup();
    const button = screen.getByRole("button", { name: "Adicionar ao calendário (.ics)" });
    await user.click(button);
    await waitFor(() => expect(mocks.downloadDealAppointmentCalendar).toHaveBeenCalledWith("deal", "appointment"));
  });
  // Pedido ainda sem horário marcado não é compromisso de ninguém.
  it("hides the calendar file while the visit is still pending", () => {
    setup("Pendente");
    expect(screen.queryByRole("button", { name: "Adicionar ao calendário (.ics)" })).not.toBeInTheDocument();
  });
  // Sem negócio não há rota do cliente que entregue o arquivo: melhor não
  // oferecer o botão do que abrir um download que sempre falha.
  it("hides the calendar file when nothing can serve it", () => {
    render(<PortalVisitActions lead={lead} cancelLoading={false} onReschedule={vi.fn()} onCancel={vi.fn()} />);
    expect(screen.queryByRole("button", { name: "Adicionar ao calendário (.ics)" })).not.toBeInTheDocument();
  });
});
