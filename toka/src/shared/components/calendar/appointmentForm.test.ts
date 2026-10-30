import { describe, expect, it } from "vitest";
import type { Appointment } from "../../data";
import {
  END_BEFORE_START,
  REQUIRED_DATE,
  REQUIRED_TIME,
  appointmentFieldErrors,
  earliestAppointmentDate,
  EMPTY_APPOINTMENT_FORM,
  appointmentFormFrom,
  suggestedAppointmentTitle,
  validateAppointmentForm,
} from "./appointmentForm";

const base = { ...EMPTY_APPOINTMENT_FORM, title: "Visita — Maria Souza", client: "Maria Souza", date: "2030-01-10", time: "10:00", endTime: "11:00" };

const appointment = (over: Partial<Appointment> = {}): Appointment =>
  ({
    id: "apt-1",
    title: "Visita — Maria Souza",
    client: "Maria Souza",
    type: "Visita",
    date: "2030-01-10",
    time: "10:00",
    endTime: "11:00",
    location: "",
    status: "Pendente",
    priority: "Media",
    agentId: "agent-1",
    ...over,
  }) as Appointment;

describe("validateAppointmentForm", () => {
  it("aceita um compromisso completo", () => {
    expect(validateAppointmentForm(base)).toEqual([]);
  });

  it("exige título, cliente, data e hora com os mesmos nomes da agenda", () => {
    const errors = validateAppointmentForm({ ...EMPTY_APPOINTMENT_FORM, date: "", time: "" });
    expect(errors).toContain("Título");
    expect(errors).toContain("Cliente");
    expect(errors).toContain("Data");
    expect(errors).toContain("Hora");
  });

  it("não exige cliente em compromisso privado", () => {
    expect(validateAppointmentForm({ ...base, visibility: "private", client: "" })).toEqual([]);
  });

  it("recusa horário de fim anterior ao início", () => {
    expect(validateAppointmentForm({ ...base, time: "11:00", endTime: "10:00" })).toContain(
      "O fim precisa ser depois do início",
    );
  });

  it("recusa agendar no passado, mas permite corrigir um compromisso que já começou", () => {
    const now = new Date("2030-01-10T12:00:00");
    const past = { ...base, date: "2030-01-10", time: "09:00", endTime: "10:00" };
    expect(validateAppointmentForm(past, { now })).toContain("Não é possível agendar no passado");
    // Mesmo horário do compromisso original: o operador está salvando o registro.
    const original = appointment({ date: "2030-01-10", time: "09:00", endTime: "10:00" });
    expect(validateAppointmentForm(past, { now, editing: original })).toEqual([]);
  });
});

describe("appointmentFormFrom", () => {
  it("carrega o compromisso existente no formulário", () => {
    const form = appointmentFormFrom(appointment({ location: "Rua das Flores, 100", notes: "levar chave" }));
    expect(form.title).toBe("Visita — Maria Souza");
    expect(form.date).toBe("2030-01-10");
    expect(form.time).toBe("10:00");
    expect(form.location).toBe("Rua das Flores, 100");
    expect(form.notes).toBe("levar chave");
    expect(form.visibility).toBe("team");
  });

  it("trata campos ausentes sem quebrar", () => {
    const form = appointmentFormFrom({ id: "x", type: "Visita", status: "Pendente", priority: "Media", date: "2030-01-10", time: "10:00" } as Appointment);
    expect(form.client).toBe("");
    expect(form.reminder).toBe(30);
    expect(form.visibility).toBe("team");
  });
});

describe("suggestedAppointmentTitle", () => {
  it("junta o tipo com o nome do lead", () => {
    expect(suggestedAppointmentTitle("Visita", "Maria Souza")).toBe("Visita — Maria Souza");
    expect(suggestedAppointmentTitle("Ligacao", "  ")).toBe("Ligacao");
  });
});

describe("appointmentFieldErrors", () => {
  it("endereça o erro ao campo certo", () => {
    const errors = appointmentFieldErrors({ ...EMPTY_APPOINTMENT_FORM, title: "", client: "", date: "", time: "" });
    expect(errors.title).toBeTruthy();
    expect(errors.client).toBeTruthy();
    expect(errors.date).toBe(REQUIRED_DATE);
    expect(errors.time).toBe(REQUIRED_TIME);
    expect(errors.endTime).toBeUndefined();
  });

  it("marca data e início quando o horário escolhido já passou", () => {
    const now = new Date("2030-01-10T12:00:00");
    const errors = appointmentFieldErrors({ ...base, date: "2030-01-10", time: "09:00", endTime: "10:00" }, { now });
    expect(errors.date).toBe("Não é possível agendar no passado");
    expect(errors.time).toBe("Escolha um horário que ainda não passou");
  });

  it("permite salvar o compromisso que já começou quando é o original em edição", () => {
    const now = new Date("2030-01-10T12:00:00");
    const form = { ...base, date: "2030-01-10", time: "09:00", endTime: "10:00" };
    expect(appointmentFieldErrors(form, { now, editing: appointment({ date: "2030-01-10", time: "09:00", endTime: "10:00" }) })).toEqual({});
  });

  it("marca o fim quando ele não é posterior ao início", () => {
    expect(appointmentFieldErrors({ ...base, time: "11:00", endTime: "11:00" }).endTime).toBe(END_BEFORE_START);
    expect(appointmentFieldErrors({ ...base, time: "11:00", endTime: "10:00" }).endTime).toBe(END_BEFORE_START);
    expect(appointmentFieldErrors({ ...base, time: "11:00", endTime: "12:00" }).endTime).toBeUndefined();
  });

  it("não exige cliente em compromisso particular", () => {
    expect(appointmentFieldErrors({ ...base, visibility: "private", client: "" }).client).toBeUndefined();
  });
});

describe("earliestAppointmentDate", () => {
  it("é hoje, e nunca posterior à data do compromisso em edição", () => {
    const now = new Date("2030-01-10T12:00:00");
    expect(earliestAppointmentDate({ now })).toBe("2030-01-10");
    expect(earliestAppointmentDate({ now, editing: appointment({ date: "2029-12-01" }) })).toBe("2029-12-01");
    expect(earliestAppointmentDate({ now, editing: appointment({ date: "2030-02-01" }) })).toBe("2030-01-10");
  });
});
