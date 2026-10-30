import { Bell, Calendar as CalendarIcon, Car, ClipboardCheck, FileSignature, FileText, Home, Key, Phone, Users, Video, Wrench } from "lucide-react";
import type { AppointmentPriority, AppointmentStatus, AppointmentType } from "../../data";

/**
 * Configuração visual e opções da agenda (tipo, status, prioridade).
 *
 * Saiu de `pages/Calendar` junto com o formulário para que a agenda completa e
 * o detalhe do lead mostrem os MESMOS rótulos, cores e opções — inclusive a
 * lista de tipos aceitos no seletor.
 */
export const TYPE_CONFIG: Record<
  AppointmentType,
  { color: string; bg: string; border: string; icon: any; label: string }
> = {
  Entrega: { color: "text-foreground", bg: "bg-muted", border: "border-border", icon: Home, label: "Entrega" },
  Visita: {
    color: "text-stage-contract",
    bg: "bg-stage-contract/15",
    border: "border-stage-contract/30",
    icon: Home,
    label: "Visita",
  },
  Reuniao: {
    color: "text-primary-700",
    bg: "bg-primary-100",
    border: "border-primary-200",
    icon: Video,
    label: "Reunião",
  },
  Reunião: {
    color: "text-primary-700",
    bg: "bg-primary-100",
    border: "border-primary-200",
    icon: Video,
    label: "Reunião",
  },
  Ligacao: {
    color: "text-status-warning",
    bg: "bg-status-warning-soft",
    border: "border-status-warning/30",
    icon: Phone,
    label: "Ligação",
  },
  Assinatura: {
    color: "text-status-ok",
    bg: "bg-status-ok-soft",
    border: "border-status-ok/30",
    icon: FileText,
    label: "Assinatura",
  },
  Vistoria: {
    color: "text-status-info",
    bg: "bg-status-info-soft",
    border: "border-status-info/30",
    icon: ClipboardCheck,
    label: "Vistoria",
  },
  Lembrete: {
    color: "text-status-critical",
    bg: "bg-status-critical-soft",
    border: "border-status-critical/30",
    icon: Bell,
    label: "Lembrete",
  },
  Outro: {
    color: "text-foreground",
    bg: "bg-muted",
    border: "border-border",
    icon: CalendarIcon,
    label: "Outro",
  },
};

export const DEFAULT_TYPE_CONFIG = {
  color: "text-foreground",
  bg: "bg-muted",
  border: "border-border",
  icon: CalendarIcon,
  label: "Outro",
};

export const STATUS_CONFIG: Record<AppointmentStatus, { color: string; bg: string; label: string }> = {
  "Em Andamento": { color: "text-status-info", bg: "bg-status-info-soft", label: "Em andamento" },
  Pendente: { color: "text-muted-foreground", bg: "bg-muted", label: "Pendente" },
  Confirmado: { color: "text-status-ok", bg: "bg-status-ok-soft", label: "Confirmado" },
  Concluido: { color: "text-status-info", bg: "bg-status-info-soft", label: "Concluído" },
  Cancelado: { color: "text-status-critical", bg: "bg-status-critical-soft", label: "Cancelado" },
  Reagendado: { color: "text-status-warning", bg: "bg-status-warning-soft", label: "Reagendado" },
  Agendado: { color: "text-muted-foreground", bg: "bg-muted", label: "Agendado" },
};

export const PRIORITY_CONFIG: Record<AppointmentPriority, { color: string; bg: string; dot: string }> =
  {
    Alta: {
      color: "text-status-critical",
      bg: "bg-status-critical-soft",
      dot: "bg-status-critical",
    },
    Media: {
      color: "text-status-warning",
      bg: "bg-status-warning-soft",
      dot: "bg-status-warning",
    },
    Baixa: {
      color: "text-muted-foreground",
      bg: "bg-muted",
      dot: "bg-muted-foreground/30",
    },
  };

export const ALL_TYPES: AppointmentType[] = [
  "Entrega",
  "Visita",
  "Reuniao",
  "Reunião",
  "Ligacao",
  "Assinatura",
  "Vistoria",
  "Lembrete",
  "Outro",
];
