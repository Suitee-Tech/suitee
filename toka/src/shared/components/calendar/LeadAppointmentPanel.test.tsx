import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { LeadAppointmentPanel } from "./LeadAppointmentPanel";
import type { Appointment } from "../../data";

const mocks = vi.hoisted(() => ({
  addAppointment: vi.fn(),
  updateAppointment: vi.fn(),
  deleteAppointment: vi.fn(),
  currentUserId: "agent-1",
  agents: [{ id: "agent-1", name: "Ana Corretora" }],
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
}));

vi.mock("../../context/CalendarContext", () => ({
  useCalendarContext: () => ({
    addAppointment: mocks.addAppointment,
    updateAppointment: mocks.updateAppointment,
    deleteAppointment: mocks.deleteAppointment,
    currentUserId: mocks.currentUserId,
  }),
}));
vi.mock("../../context/AgentsContext", () => ({ useAgentsContext: () => ({ agents: mocks.agents }) }));
vi.mock("sonner", () => ({ toast: { success: mocks.toastSuccess, error: mocks.toastError } }));

const saved = (over: Partial<Appointment> = {}): Appointment =>
  ({ id: "apt-9", title: "Visita — Maria Souza", client: "Maria Souza", type: "Visita", date: "2030-01-10", time: "10:00", endTime: "11:00", status: "Pendente", priority: "Media", leadId: "lead-1", ...over }) as Appointment;

const editing: Appointment = saved({ id: "apt-1", time: "09:00", endTime: "10:00", agentId: "agent-1" });

const lead = { id: "lead-1", name: "Maria Souza", propertyId: "prop-7" };

const fillRequired = () => {
  fireEvent.change(screen.getByLabelText("Data *"), { target: { value: "2030-01-10" } });
  // O formulário é o MESMO da agenda: "Início *" e "Fim" (e o fim já vem
  // sugerido como início + 1h).
  fireEvent.change(screen.getByLabelText("Início *"), { target: { value: "10:00" } });
};

describe("LeadAppointmentPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.addAppointment.mockResolvedValue(saved());
    mocks.updateAppointment.mockResolvedValue(saved());
    mocks.deleteAppointment.mockResolvedValue(true);
  });

  it("agenda para o lead com os dados dele já preenchidos", async () => {
    const onClose = vi.fn();
    render(<LeadAppointmentPanel open lead={lead} properties={[]} leads={[]} onClose={onClose} />);

    // Título, cliente, lead, imóvel de interesse e corretor vêm prontos.
    expect(screen.getByLabelText("Título *")).toHaveValue("Visita — Maria Souza");
    // O cliente do compromisso é o próprio lead (aparece no painel e no campo).
    expect(screen.getAllByText("Maria Souza").length).toBeGreaterThan(0);
    // O corretor responsável aparece já selecionado no combo da agenda.
    expect(screen.getByText("Ana Corretora")).toBeInTheDocument();
    // Campos da agenda completa continuam disponíveis (não é um formulário reduzido).
    expect(screen.getByText("Compromisso particular")).toBeInTheDocument();

    fillRequired();
    fireEvent.click(screen.getByRole("button", { name: "Agendar" }));

    await waitFor(() => expect(mocks.addAppointment).toHaveBeenCalledTimes(1));
    expect(mocks.addAppointment.mock.calls[0][0]).toMatchObject({
      leadId: "lead-1",
      client: "Maria Souza",
      propertyId: "prop-7",
      agentId: "agent-1",
      date: "2030-01-10",
      time: "10:00",
    });
    expect(mocks.toastSuccess).toHaveBeenCalledWith("Compromisso agendado para este lead");
    await waitFor(() => expect(onClose).toHaveBeenCalled());
  });

  it("não chama a API quando falta campo obrigatório", () => {
    render(<LeadAppointmentPanel open lead={lead} properties={[]} leads={[]} onClose={() => {}} />);
    fireEvent.change(screen.getByLabelText("Título *"), { target: { value: "" } });
    fireEvent.click(screen.getByRole("button", { name: "Agendar" }));

    expect(mocks.addAppointment).not.toHaveBeenCalled();
    expect(mocks.toastError).toHaveBeenCalledWith("Campos obrigatórios: Título, Data");
  });

  it("edita um compromisso existente enviando só o que mudou", async () => {
    render(<LeadAppointmentPanel open lead={lead} appointment={editing} properties={[]} leads={[]} onClose={() => {}} />);

    expect(screen.getByLabelText("Título *")).toHaveValue("Visita — Maria Souza");
    // O status é editável no próprio formulário (não é mais um texto informativo).
    expect(screen.getByLabelText("Status")).toHaveValue("Pendente");

    fireEvent.change(screen.getByLabelText("Início *"), { target: { value: "14:00" } });
    fireEvent.click(screen.getByRole("button", { name: "Salvar alterações" }));

    await waitFor(() => expect(mocks.updateAppointment).toHaveBeenCalledTimes(1));
    const [id, patch] = mocks.updateAppointment.mock.calls[0];
    expect(id).toBe("apt-1");
    // Início 14:00 → o próprio formulário sugere o fim 15:00 (início + 1h).
    expect(patch).toMatchObject({ time: "14:00", endTime: "15:00" });
    expect(patch).not.toHaveProperty("title");
    expect(mocks.toastSuccess).toHaveBeenCalledWith("Compromisso atualizado");
  });

  it("confirma e cancela a visita direto no painel", async () => {
    render(<LeadAppointmentPanel open lead={lead} appointment={editing} properties={[]} leads={[]} onClose={() => {}} />);

    fireEvent.click(screen.getByRole("button", { name: /Confirmar visita/ }));
    await waitFor(() => expect(mocks.updateAppointment).toHaveBeenCalledWith("apt-1", { status: "Confirmado" }));
    expect(mocks.toastSuccess).toHaveBeenCalledWith("Visita confirmada. O cliente foi notificado.");

    mocks.updateAppointment.mockClear();
    fireEvent.click(screen.getByRole("button", { name: /Cancelar visita/ }));
    await waitFor(() => expect(mocks.updateAppointment).toHaveBeenCalledWith("apt-1", { status: "Cancelado" }));
  });

  it("remove o compromisso só depois da confirmação", async () => {
    render(<LeadAppointmentPanel open lead={lead} appointment={editing} properties={[]} leads={[]} onClose={() => {}} />);

    fireEvent.click(screen.getByRole("button", { name: /Remover/ }));
    expect(mocks.deleteAppointment).not.toHaveBeenCalled();

    const dialog = await screen.findByRole("alertdialog");
    fireEvent.click(within(dialog).getByRole("button", { name: "Remover" }));
    await waitFor(() => expect(mocks.deleteAppointment).toHaveBeenCalledWith("apt-1"));
    expect(mocks.toastSuccess).toHaveBeenCalledWith("Compromisso removido");
  });

  it("sugere o título pelo tipo escolhido em um compromisso novo", () => {
    render(<LeadAppointmentPanel open lead={lead} properties={[]} leads={[]} onClose={() => {}} />);
    fireEvent.change(screen.getByLabelText("Tipo"), { target: { value: "Ligacao" } });
    expect(screen.getByLabelText("Título *")).toHaveValue("Ligacao — Maria Souza");
  });
});
