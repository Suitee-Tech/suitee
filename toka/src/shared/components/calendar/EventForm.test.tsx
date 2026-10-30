import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { EventForm } from "./EventForm";
import { EMPTY_APPOINTMENT_FORM, type AppointmentFormValues } from "./appointmentForm";
import type { Appointment } from "../../data";

/**
 * Validação no próprio formulário de agendamento: agendar no passado e fim
 * anterior/igual ao início precisam aparecer no campo, não só no toast do
 * salvamento.
 */
function Harness({ initial = {}, editing = null }: { initial?: Partial<AppointmentFormValues>; editing?: Appointment | null }) {
  const [form, setForm] = useState<AppointmentFormValues>({ ...EMPTY_APPOINTMENT_FORM, ...initial });
  return (
    <EventForm
      form={form}
      updateField={(field, value) => setForm((prev) => ({ ...prev, [field]: value }))}
      agents={[]}
      properties={[]}
      leads={[]}
      editing={editing}
    />
  );
}

const yesterday = () => {
  const date = new Date();
  date.setDate(date.getDate() - 1);
  const pad = (value: number) => String(value).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
};

const tomorrow = () => {
  const date = new Date();
  date.setDate(date.getDate() + 1);
  const pad = (value: number) => String(value).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
};

describe("EventForm validação de data e horário", () => {
  it("acusa o agendamento no passado no campo de data", () => {
    render(<Harness initial={{ title: "Visita", client: "Maria Souza", date: yesterday(), time: "09:00", endTime: "10:00" }} />);

    const date = screen.getByLabelText("Data *");
    fireEvent.blur(date);

    expect(screen.getByText("Não é possível agendar no passado")).toBeInTheDocument();
    expect(screen.getByText("Escolha um horário que ainda não passou")).toBeInTheDocument();
    expect(date).toHaveAttribute("aria-invalid", "true");
  });

  it("limita o seletor de data a hoje (não dá para escolher o passado)", () => {
    render(<Harness />);
    const date = screen.getByLabelText("Data *") as HTMLInputElement;
    // `min` é a data de hoje no fuso LOCAL — a primeira versão desta asserção
    // comparava com `toISOString()`, que é UTC: entre 21h e 24h em Brasília o
    // UTC já está no dia seguinte e o teste reprovava sem nenhum defeito no
    // formulário.
    const now = new Date();
    const pad = (v: number) => String(v).padStart(2, "0");
    expect(date.min).toBe(`${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`);
  });

  it("acusa o fim anterior ou igual ao início, já durante a digitação", () => {
    render(<Harness initial={{ title: "Visita", client: "Maria Souza", date: tomorrow(), time: "11:00", endTime: "10:00" }} />);

    const end = screen.getByLabelText("Fim");
    // A borda vermelha aparece sem esperar o blur...
    expect(end.className).toContain("border-destructive");
    fireEvent.blur(end);
    // ...e a mensagem é a mesma do resumo do salvamento.
    expect(screen.getByText("O fim precisa ser depois do início")).toBeInTheDocument();
    expect(end).toHaveAttribute("aria-invalid", "true");
  });

  it("some com o erro quando o horário é corrigido", () => {
    render(<Harness initial={{ title: "Visita", client: "Maria Souza", date: tomorrow(), time: "11:00", endTime: "10:00" }} />);
    const end = screen.getByLabelText("Fim");
    fireEvent.blur(end);
    expect(screen.getByText("O fim precisa ser depois do início")).toBeInTheDocument();

    fireEvent.change(end, { target: { value: "12:00" } });
    expect(screen.queryByText("O fim precisa ser depois do início")).not.toBeInTheDocument();
  });

  it("não acende erro antes de o operador passar pelo campo", () => {
    render(<Harness />);
    expect(screen.queryByText("Informe a data")).not.toBeInTheDocument();
    expect(screen.queryByText("Informe o título do compromisso")).not.toBeInTheDocument();

    fireEvent.blur(screen.getByLabelText("Data *"));
    expect(screen.getByText("Informe a data")).toBeInTheDocument();
  });

  it("permite editar um compromisso que já aconteceu sem acusar passado", () => {
    const original = {
      id: "apt-old",
      title: "Visita antiga",
      client: "Maria Souza",
      type: "Visita",
      date: yesterday(),
      time: "09:00",
      endTime: "10:00",
      status: "Concluido",
      priority: "Media",
    } as Appointment;

    render(<Harness editing={original} initial={{ ...original }} />);
    fireEvent.blur(screen.getByLabelText("Data *"));

    expect(screen.queryByText("Não é possível agendar no passado")).not.toBeInTheDocument();
    expect((screen.getByLabelText("Data *") as HTMLInputElement).min).toBe(original.date);
  });
});
