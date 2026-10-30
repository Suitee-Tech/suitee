import { CalendarPlus } from "lucide-react";
import { Button } from "../ui/button";
import { calendarService } from "../services/calendar.service";
import { downloadBlobAsFile } from "../utils/downloadFile";

/** Campos mínimos para decidir se um compromisso vira arquivo de calendário. */
export interface CalendarEligibility {
  status?: string;
  date?: string;
  time?: string;
  endTime?: string;
}

/**
 * A mesma regra do backend: só horário marcado, futuro e com término definido
 * vira evento. Pedido ainda pendente, cancelado ou concluído não aparece como
 * compromisso no calendário de ninguém.
 */
export function canAddToCalendar({ status, date, time, endTime }: CalendarEligibility): boolean {
  if (!status || !["Agendado", "Confirmado", "Reagendado"].includes(status)) return false;
  if (!date || !time || !endTime) return false;
  const start = new Date(`${date}T${time}`);
  if (Number.isNaN(start.getTime()) || start.getTime() < Date.now()) return false;
  const end = new Date(`${date}T${endTime}`);
  return !Number.isNaN(end.getTime()) && end.getTime() > start.getTime();
}

/** Nome de arquivo seguro, espelhando o do backend. */
export function calendarFileName({ date }: CalendarEligibility): string {
  return `compromisso-${date ?? "agenda"}.ics`;
}

export interface AddToCalendarButtonProps extends CalendarEligibility {
  /** Id do compromisso; sem ele (ex.: pedido sem agenda) o botão não aparece. */
  appointmentId?: string;
  /** Baixa pelo portal (rota do cliente) em vez da agenda interna. */
  load?: () => Promise<Blob>;
  variant?: "default" | "outline" | "ghost";
  size?: "default" | "sm" | "icon";
  className?: string;
  label?: string;
}

/**
 * Baixa uma cópia do compromisso para o aplicativo de calendário da pessoa.
 * O arquivo é uma cópia pontual: alterações posteriores não atualizam o que já
 * foi importado.
 */
export function AddToCalendarButton({
  appointmentId,
  load,
  variant = "outline",
  size = "sm",
  className,
  label = "Adicionar ao calendário (.ics)",
  ...eligibility
}: AddToCalendarButtonProps) {
  if (!appointmentId || !canAddToCalendar(eligibility)) return null;
  const download = load ?? (() => calendarService.downloadCalendarFile(appointmentId));
  return (
    <Button
      variant={variant}
      size={size}
      className={className}
      title="Baixa o arquivo .ics, que abre no Google Agenda, Apple Calendar ou Outlook"
      aria-label={label}
      onClick={() => void downloadBlobAsFile(
        calendarFileName(eligibility),
        download,
        "Não foi possível gerar o arquivo do calendário. Tente novamente.",
      )}
    >
      <CalendarPlus className="h-4 w-4" />
      <span className={size === "icon" ? "sr-only" : undefined}>{label}</span>
    </Button>
  );
}
