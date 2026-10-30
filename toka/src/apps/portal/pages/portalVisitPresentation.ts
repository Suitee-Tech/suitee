import type { PortalVisitRequest } from "../services/portal.service";
import { parseISO } from "date-fns";
import type { AppointmentDraft } from "../components/AppointmentPickerDialog.utils";

export function isVisitClosed(visit?: PortalVisitRequest) {
  return ["Cancelado", "Concluido", "Concluído"].includes(visit?.status ?? "");
}

export function visitPickerDefaults(visit?: PortalVisitRequest): AppointmentDraft | undefined {
  if (!visit?.date || !visit.time || !visit.end_time) return undefined;
  const date = parseISO(`${visit.date}T00:00:00`);
  if (Number.isNaN(date.getTime())) return undefined;
  return { date, startTime: visit.time.slice(0, 5), endTime: visit.end_time.slice(0, 5), note: visit.note || "" };
}

export function visitStatusLabel(visit: PortalVisitRequest) {
  switch (visit.status) {
    case "Confirmado": return "Visita confirmada";
    case "Reagendado": return "Visita reagendada";
    case "Em Andamento": return "Visita em andamento";
    case "Concluido":
    case "Concluído": return "Visita concluída";
    case "Cancelado": return "Visita cancelada";
    default: return "Visita aguardando confirmação";
  }
}

export function visitStatusClass(visit: PortalVisitRequest): string {
  switch (visit.status) {
    case "Confirmado": return "bg-status-ok-soft text-status-ok";
    case "Reagendado":
    case "Em Andamento": return "bg-primary/10 text-primary";
    case "Concluido":
    case "Concluído": return "bg-muted text-muted-foreground";
    case "Cancelado": return "bg-status-critical-soft text-status-critical";
    default: return "bg-status-warning-soft text-status-warning";
  }
}
