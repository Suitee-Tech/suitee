import type { Appointment, AppointmentPriority, AppointmentStatus, AppointmentType } from "../../data";

/**
 * Regras puras do compromisso (agenda): valores iniciais, conversão de um
 * compromisso existente para o formulário e validação.
 *
 * Vive fora das páginas porque o MESMO compromisso é criado/editado em dois
 * lugares — a agenda completa (`pages/Calendar`) e a página de detalhes do lead,
 * que agenda no contexto daquele cliente. Regra duplicada é regra que diverge:
 * aqui a validação é uma só, e o formulário do lead não pode aceitar o que a
 * agenda recusa.
 */
export interface AppointmentFormValues {
  title: string;
  client: string;
  type: AppointmentType;
  date: string;
  time: string;
  endTime: string;
  location: string;
  status: AppointmentStatus;
  priority: AppointmentPriority;
  description: string;
  notes: string;
  agentId: string;
  propertyId: string;
  contractId: string;
  leadId: string;
  isAllDay: boolean;
  reminder: number;
  visibility: "team" | "private";
}

export const EMPTY_APPOINTMENT_FORM: AppointmentFormValues = {
  title: "",
  client: "",
  type: "Visita",
  date: "",
  time: "09:00",
  endTime: "10:00",
  location: "",
  status: "Pendente",
  priority: "Media",
  description: "",
  notes: "",
  agentId: "",
  propertyId: "",
  contractId: "",
  leadId: "",
  isAllDay: false,
  reminder: 30,
  visibility: "team",
};

/** Converte um compromisso existente no estado do formulário (modo edição). */
export function appointmentFormFrom(apt: Appointment): AppointmentFormValues {
  return {
    title: apt.title ?? "",
    client: apt.client ?? "",
    type: apt.type,
    date: apt.date,
    time: apt.time,
    endTime: apt.endTime ?? "",
    location: apt.location ?? "",
    status: apt.status,
    priority: apt.priority,
    description: apt.description ?? "",
    notes: apt.notes ?? "",
    agentId: apt.agentId ?? "",
    propertyId: apt.propertyId ?? "",
    contractId: apt.contractId ?? "",
    leadId: apt.leadId ?? "",
    isAllDay: apt.isAllDay ?? false,
    reminder: apt.reminder ?? 30,
    visibility: apt.visibility === "private" ? "private" : "team",
  };
}

/**
 * Devolve os problemas que impedem de salvar. Mensagens iguais às que a agenda
 * já mostrava — o operador aprende uma vez.
 *
 * `editing` é o compromisso original: reagendar um compromisso que já começou é
 * permitido (o operador está corrigindo o registro), marcar um novo no passado
 * não é.
 */
export function validateAppointmentForm(
  form: AppointmentFormValues,
  options: { editing?: Appointment | null; now?: Date } = {},
): string[] {
  const fieldErrors = appointmentFieldErrors(form, options);
  // O resumo (toast do salvamento) usa o rótulo do campo quando falta
  // preenchimento e o MOTIVO quando a regra é de conteúdo — "no passado" e
  // "fim antes do início" precisam aparecer para o operador entender.
  const summary: string[] = [];
  if (fieldErrors.title) summary.push("Título");
  if (fieldErrors.client) summary.push("Cliente");
  if (fieldErrors.date) summary.push(fieldErrors.date === REQUIRED_DATE ? "Data" : fieldErrors.date);
  if (fieldErrors.time) summary.push(fieldErrors.time === REQUIRED_TIME ? "Hora" : fieldErrors.time);
  if (fieldErrors.endTime) summary.push(fieldErrors.endTime);
  return summary;
}

/** Erros por campo, para mostrar embaixo do input correspondente. */
export const REQUIRED_DATE = "Informe a data";
export const REQUIRED_TIME = "Informe o horário de início";
export const END_BEFORE_START = "O fim precisa ser depois do início";
export const PAST_DATE = "Não é possível agendar no passado";

export interface AppointmentFieldErrors {
  title?: string;
  client?: string;
  date?: string;
  time?: string;
  endTime?: string;
}

/**
 * Validação campo a campo do compromisso — a MESMA regra do resumo usado no
 * salvamento (`validateAppointmentForm`), agora endereçada ao input que precisa
 * de correção. O formulário mostra a mensagem no campo (e marca o input); a
 * agenda e o painel do lead compartilham o formulário, então a validação vale
 * nos dois lugares.
 *
 * `editing` é o compromisso original: reagendar o que já começou é permitido
 * (o operador está corrigindo o registro), criar no passado não é.
 */
export function appointmentFieldErrors(
  form: AppointmentFormValues,
  options: { editing?: Appointment | null; now?: Date } = {},
): AppointmentFieldErrors {
  const { editing = null, now = new Date() } = options;
  const errors: AppointmentFieldErrors = {};

  if (!form.title.trim()) errors.title = "Informe o título do compromisso";
  if (form.visibility !== "private" && !form.client.trim()) errors.client = "Informe o cliente do compromisso";

  const keepsOriginalSlot = editing !== null && form.date === editing.date && form.time === editing.time;
  if (!form.date) {
    errors.date = REQUIRED_DATE;
  } else if (form.time && !keepsOriginalSlot) {
    const scheduled = new Date(`${form.date}T${form.time}:00`);
    if (!Number.isNaN(scheduled.getTime()) && scheduled <= now) {
      errors.date = PAST_DATE;
      errors.time = "Escolha um horário que ainda não passou";
    }
  }

  if (!form.time) {
    errors.time = REQUIRED_TIME;
  } else if (form.endTime && form.time >= form.endTime) {
    errors.endTime = END_BEFORE_START;
  }

  return errors;
}

/**
 * Data mínima aceita no seletor: hoje, ou a data do próprio compromisso quando o
 * operador está corrigindo um que já aconteceu (senão o registro antigo ficaria
 * impossível de editar).
 */
export function earliestAppointmentDate(
  options: { editing?: Appointment | null; now?: Date } = {},
): string {
  const { editing = null, now = new Date() } = options;
  const today = toDateInputValue(now);
  if (editing?.date && editing.date < today) return editing.date;
  return today;
}

/** Data no formato do input `type="date"` (yyyy-mm-dd) no fuso local. */
export function toDateInputValue(date: Date): string {
  const pad = (value: number) => String(value).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

/** Título sugerido ao agendar a partir do lead — o operador ajusta se quiser. */
export function suggestedAppointmentTitle(type: AppointmentType, leadName: string): string {
  const name = leadName.trim();
  if (!name) return type;
  return `${type} — ${name}`;
}
