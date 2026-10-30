import { format, startOfDay } from "date-fns";
import { ptBR } from "date-fns/locale";

export type AppointmentDraft = {
  date: Date;
  startTime: string;
  endTime: string;
  note?: string;
};

export const DEFAULT_APPOINTMENT_DURATION_MINUTES = 60;
export const QUICK_APPOINTMENT_TIMES = ["09:00", "11:00", "14:00", "16:00"] as const;

export function timeToMinutes(value: string): number | null {
  const match = /^([01]\d|2[0-3]):([0-5]\d)(?::[0-5]\d)?$/.exec(value);
  if (!match) return null;
  return Number(match[1]) * 60 + Number(match[2]);
}

export function addMinutesToTime(value: string, minutes: number): string | null {
  const base = timeToMinutes(value);
  if (base === null) return null;

  const result = base + minutes;
  if (result < 0 || result >= 24 * 60) return null;

  const hours = Math.floor(result / 60);
  const remainder = result % 60;
  return `${String(hours).padStart(2, "0")}:${String(remainder).padStart(2, "0")}`;
}

export function combineDateAndTime(date: Date, time: string): Date | null {
  const minutes = timeToMinutes(time);
  if (minutes === null || Number.isNaN(date.getTime())) return null;

  const result = new Date(date);
  result.setHours(Math.floor(minutes / 60), minutes % 60, 0, 0);
  return result;
}

export function isAppointmentStartInPast(date: Date, time: string, now = new Date()): boolean {
  const scheduledAt = combineDateAndTime(date, time);
  return scheduledAt !== null && scheduledAt.getTime() <= now.getTime();
}

export function appointmentDurationLabel(startTime: string, endTime: string): string | null {
  const start = timeToMinutes(startTime);
  const end = timeToMinutes(endTime);
  if (start === null || end === null || end <= start) return null;

  const duration = end - start;
  const hours = Math.floor(duration / 60);
  const minutes = duration % 60;

  if (hours === 0) return `${minutes} min`;
  if (minutes === 0) return `${hours}h`;
  return `${hours}h${String(minutes).padStart(2, "0")}`;
}

export function buildDefaultAppointmentValues(now = new Date()): AppointmentDraft {
  const candidate = new Date(now.getTime() + DEFAULT_APPOINTMENT_DURATION_MINUTES * 60_000);
  candidate.setSeconds(0, 0);

  const remainder = candidate.getMinutes() % 30;
  if (remainder > 0) candidate.setMinutes(candidate.getMinutes() + (30 - remainder));

  let startTime = format(candidate, "HH:mm");
  let endTime = addMinutesToTime(startTime, DEFAULT_APPOINTMENT_DURATION_MINUTES);

  // Perto do fim do dia, evita abrir o diálogo com um intervalo impossível
  // (por exemplo, 23:00–00:00 em uma visita que precisa terminar no mesmo dia).
  if (!endTime) {
    candidate.setDate(candidate.getDate() + 1);
    candidate.setHours(10, 0, 0, 0);
    startTime = "10:00";
    endTime = "11:00";
  }

  return {
    date: startOfDay(candidate),
    startTime,
    endTime,
    note: "",
  };
}

export function formatAppointmentDate(date: Date): string {
  return format(date, "EEEE, d 'de' MMMM", { locale: ptBR });
}

