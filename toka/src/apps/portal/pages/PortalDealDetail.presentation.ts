import type { ChecklistItemDTO, DealStatus, Proposal } from "../../../shared/services/deal.service";
import type { PortalVisitCopy } from "../lib/portalPersona";
import type { PortalAppointment, PortalSignatureView } from "../services/portal.service";

export const PORTAL_DEAL_STAGES: ReadonlyArray<{ key: DealStatus; label: string }> = [
  { key: "NEGOTIATION", label: "Negociação" },
  { key: "PROPOSAL", label: "Proposta" },
  { key: "ANALYSIS", label: "Análise" },
  { key: "CONTRACT", label: "Contrato" },
  { key: "CONVERTED", label: "Concluído" },
];

export type PortalDealTone = "neutral" | "info" | "warning" | "success" | "critical";

export interface PortalDealNextAction {
  title: string;
  description: string;
  label?: string;
  stage?: DealStatus;
  action?: "stage" | "schedule";
  tone: PortalDealTone;
}

const APPOINTMENT_STATUS: Record<string, { label: string; tone: PortalDealTone }> = {
  AGENDADO: { label: "Agendada", tone: "neutral" },
  AGENDADA: { label: "Agendada", tone: "neutral" },
  CONFIRMADO: { label: "Confirmada", tone: "success" },
  CONFIRMADA: { label: "Confirmada", tone: "success" },
  CONCLUIDO: { label: "Concluída", tone: "info" },
  CONCLUIDA: { label: "Concluída", tone: "info" },
  CANCELADO: { label: "Cancelada", tone: "critical" },
  CANCELADA: { label: "Cancelada", tone: "critical" },
  REAGENDADO: { label: "Reagendada", tone: "warning" },
  REAGENDADA: { label: "Reagendada", tone: "warning" },
  PENDENTE: { label: "Aguardando confirmação", tone: "warning" },
};

function normalizeStatus(value?: string): string {
  return (value ?? "")
    .trim()
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "")
    .toUpperCase();
}

export function formatPortalDateTime(value?: string): string | null {
  if (!value) return null;
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return null;
  return new Intl.DateTimeFormat("pt-BR", {
    day: "2-digit",
    month: "short",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  })
    .format(parsed)
    .replace(",", " às");
}

export function formatAppointmentDate(date?: string, time?: string, endTime?: string): string {
  if (!date) return "Data a confirmar";
  const [year, month, day] = date.split("-").map(Number);
  if (!year || !month || !day) return [date, time].filter(Boolean).join(" · ");
  const dateLabel = new Intl.DateTimeFormat("pt-BR", {
    weekday: "short",
    day: "2-digit",
    month: "short",
  })
    .format(new Date(year, month - 1, day, 12))
    .replace(/\./g, "");
  const start = time?.slice(0, 5);
  const end = endTime?.slice(0, 5);
  if (!start) return dateLabel;
  return `${dateLabel}, ${start}${end ? `–${end}` : ""}`;
}

export function getAppointmentStatusMeta(status?: string) {
  const normalized = normalizeStatus(status);
  return APPOINTMENT_STATUS[normalized] ?? {
    label: status?.trim() || "Status não informado",
    tone: "neutral" as const,
  };
}

export function sortAppointments(appointments: PortalAppointment[]): PortalAppointment[] {
  return appointments.slice().sort((a, b) => {
    const aDate = `${a.date || "9999-12-31"}T${a.time || "23:59"}`;
    const bDate = `${b.date || "9999-12-31"}T${b.time || "23:59"}`;
    return aDate.localeCompare(bDate);
  });
}

export function sortProposalsNewestFirst(proposals: Proposal[]): Proposal[] {
  return proposals.slice().sort((a, b) => {
    const aTime = new Date(a.created_at).getTime();
    const bTime = new Date(b.created_at).getTime();
    if (Number.isNaN(aTime) && Number.isNaN(bTime)) return 0;
    if (Number.isNaN(aTime)) return 1;
    if (Number.isNaN(bTime)) return -1;
    return bTime - aTime;
  });
}

export function getChecklistProgress(items: ChecklistItemDTO[]) {
  const requiredItems = items.filter((item) => item.required);
  const basis = requiredItems.length > 0 ? requiredItems : items;
  const approved = basis.filter((item) => item.status === "APPROVED").length;
  const awaitingReview = basis.filter((item) => item.status === "UPLOADED").length;
  const needsAction = basis.filter((item) => item.status === "PENDING" || item.status === "REJECTED").length;
  return {
    total: basis.length,
    approved,
    awaitingReview,
    needsAction,
    percent: basis.length === 0 ? 0 : Math.round(((approved + awaitingReview) / basis.length) * 100),
  };
}

export function getPortalDealStatus(status: DealStatus): { label: string; tone: PortalDealTone } {
  if (status === "CONVERTED") return { label: "Concluído", tone: "success" };
  if (status === "CANCELLED") return { label: "Encerrado", tone: "critical" };
  return { label: "Em andamento", tone: "success" };
}

export function getPortalDealNextAction({
  status,
  checklist = [],
  proposals = [],
  appointments = [],
  signature,
  visits,
}: {
  status: DealStatus;
  checklist?: ChecklistItemDTO[];
  proposals?: Proposal[];
  appointments?: PortalAppointment[];
  signature?: PortalSignatureView | null;
  /** Vocabulário de visitas da persona; ausente ⇒ texto padrão de quem visita. */
  visits?: PortalVisitCopy;
}): PortalDealNextAction {
  if (status === "CANCELLED") {
    return {
      title: "Negociação encerrada",
      description: "Este processo foi encerrado. Se precisar de esclarecimentos, fale com a imobiliária.",
      tone: "critical",
    };
  }

  if (status === "CONVERTED") {
    return {
      title: "Jornada concluída",
      description: "O negócio foi concluído. Seus documentos e informações continuam disponíveis nesta página.",
      tone: "success",
    };
  }

  if (status === "CONTRACT") {
    const myStatus = normalizeStatus(signature?.my_signer?.status);
    const pendingOthers = (signature?.other_signers ?? []).filter((signer) => normalizeStatus(signer.status) !== "SIGNED");
    if (myStatus === "SIGNED") {
      return {
        title: pendingOthers.length > 0 ? "Aguardando as outras assinaturas" : "Contrato assinado",
        description:
          pendingOthers.length > 0
            ? `Sua parte está concluída. Ainda ${pendingOthers.length === 1 ? "falta 1 assinatura" : `faltam ${pendingOthers.length} assinaturas`}.`
            : "Todas as assinaturas foram concluídas.",
        stage: "CONTRACT",
        label: "Ver assinaturas",
        action: "stage",
        tone: "success",
      };
    }
    return {
      title: signature?.has_envelope ? "Seu contrato está pronto" : "Contrato em preparação",
      description: signature?.has_envelope
        ? "Revise e assine digitalmente para concluir sua participação."
        : "A imobiliária está preparando o contrato. Você será avisado quando puder assinar.",
      stage: "CONTRACT",
      label: signature?.has_envelope ? "Assinar contrato" : "Acompanhar contrato",
      action: "stage",
      tone: signature?.has_envelope ? "warning" : "info",
    };
  }

  if (status === "ANALYSIS") {
    const progress = getChecklistProgress(checklist);
    if (progress.needsAction > 0) {
      return {
        title: "Documentos aguardando você",
        description: `${progress.needsAction} ${progress.needsAction === 1 ? "item precisa" : "itens precisam"} ser enviado ou corrigido para a análise continuar.`,
        stage: "ANALYSIS",
        label: progress.needsAction === 1 ? "Resolver pendência" : "Resolver pendências",
        action: "stage",
        tone: "warning",
      };
    }
    return {
      title: progress.awaitingReview > 0 ? "Documentos em análise" : "Análise em andamento",
      description:
        progress.awaitingReview > 0
          ? "A imobiliária recebeu seus documentos e fará a conferência."
          : "Nenhuma ação é necessária agora. Acompanhe a atualização desta etapa.",
      stage: "ANALYSIS",
      label: "Acompanhar análise",
      action: "stage",
      tone: "info",
    };
  }

  if (status === "PROPOSAL") {
    const latest = sortProposalsNewestFirst(proposals)[0];
    const needsResponse = latest?.status === "SENT" || latest?.status === "COUNTER_OFFER";
    return {
      title: needsResponse ? "Proposta aguardando sua resposta" : "Proposta em andamento",
      description: needsResponse
        ? "Confira valores e condições antes de aceitar ou enviar uma contraproposta."
        : latest
          ? "Acompanhe o histórico e o retorno da imobiliária."
          : "A imobiliária ainda não enviou uma proposta para sua avaliação.",
      stage: "PROPOSAL",
      label: needsResponse ? "Analisar proposta" : "Acompanhar proposta",
      action: "stage",
      tone: needsResponse ? "warning" : "info",
    };
  }

  const activeAppointments = appointments.filter((appointment) => {
    const statusValue = normalizeStatus(appointment.status);
    return statusValue !== "CANCELADO" && statusValue !== "CANCELADA" && statusValue !== "CONCLUIDO" && statusValue !== "CONCLUIDA";
  });
  if (activeAppointments.length > 0) {
    const scheduled = visits?.scheduled;
    return {
      title: scheduled?.title ?? "Acompanhe sua visita",
      description: scheduled?.description ?? "Confira o horário e a confirmação da imobiliária. Você também pode solicitar outro horário.",
      stage: "NEGOTIATION",
      label: scheduled?.label ?? "Ver agendamento",
      action: "stage",
      tone: "info",
    };
  }
  // Quem anuncia não agenda visita ao próprio imóvel: o card aponta para o
  // acompanhamento, nunca para "Agendar visita".
  if (visits && !visits.canSchedule) {
    return {
      title: visits.notScheduled.title,
      description: visits.notScheduled.description,
      stage: "NEGOTIATION",
      label: visits.notScheduled.label,
      action: "stage",
      tone: "info",
    };
  }
  const notScheduled = visits?.notScheduled;
  return {
    title: notScheduled?.title ?? "Conheça o imóvel de perto",
    description: notScheduled?.description ?? "Solicite uma data e um horário. A imobiliária confirmará a disponibilidade.",
    stage: "NEGOTIATION",
    label: notScheduled?.label ?? "Agendar visita",
    action: "schedule",
    tone: "neutral",
  };
}
