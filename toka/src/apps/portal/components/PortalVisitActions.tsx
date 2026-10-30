import { useState } from "react";
import { AddToCalendarButton } from "../../../shared/components/AddToCalendarButton";
import { ConfirmDialog } from "../../../shared/components/ConfirmDialog";
import { portalService, type PortalOverviewLead } from "../services/portal.service";
import { isVisitClosed, visitStatusLabel, visitStatusClass } from "../pages/portalVisitPresentation";

const buttonClass = "inline-flex min-h-9 items-center gap-1.5 rounded-lg border border-border px-3 py-1.5 text-xs font-semibold hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary disabled:opacity-60";
function visitDate(iso?: string) {
  if (!iso) return "Data não informada";
  const [year, month, day] = iso.split("-");
  return day && month && year ? `${day}/${month}/${year}` : iso;
}

export function PortalVisitActions({ lead, dealId, loadCalendar, cancelLoading, onReschedule, onCancel }: {
  lead: PortalOverviewLead;
  /**
   * Negócio do lead. O `.ics` do portal é servido por negócio (a agenda do
   * negócio também devolve as visitas anteriores do lead), e sem ele não há
   * rota do cliente que entregue o arquivo — por isso o botão não aparece.
   */
  dealId?: string;
  /** Baixa pelo negócio quando existe; a visita pré-negócio usa o próprio lead. */
  loadCalendar?: () => Promise<Blob>;
  cancelLoading: boolean;
  onReschedule: (lead: PortalOverviewLead) => void;
  onCancel: (lead: PortalOverviewLead) => Promise<void>;
}) {
  const [confirmCancellation, setConfirmCancellation] = useState(false);
  const visit = lead.visit_request;
  if (!visit?.status) return null;
  const appointmentId = visit.appointment_id;
  const confirmed = ["Confirmado", "Reagendado", "Em Andamento"].includes(visit.status);
  const schedule = `${visitDate(visit.date)} · ${visit.time || "Horário não informado"}${visit.end_time ? `–${visit.end_time}` : ""}`;
  const property = visit.property_id && visit.property_id !== lead.recommended_property_id
    ? `Imóvel ${visit.property_id}` : lead.recommended_property?.title || "Imóvel da visita";
  return <div className="w-full flex flex-wrap items-center gap-2 mt-1">
    <span className={`inline-flex items-center gap-1.5 rounded-lg px-2 py-1 text-xs font-bold ${visitStatusClass(visit)}`}>
      {visitStatusLabel(visit)} · {schedule}
    </span>
    {(loadCalendar || dealId) && appointmentId && (
      <AddToCalendarButton
        appointmentId={appointmentId}
        status={visit.status}
        date={visit.date}
        time={visit.time}
        endTime={visit.end_time}
        load={loadCalendar ?? (() => portalService.downloadDealAppointmentCalendar(dealId!, appointmentId))}
      />
    )}
    {isVisitClosed(visit) ? <button type="button" onClick={() => onReschedule(lead)} className={`${buttonClass} text-foreground`}>Agendar novamente</button> : <>
      <button type="button" onClick={() => onReschedule(lead)} disabled={cancelLoading} className={`${buttonClass} text-foreground`}>Alterar horário</button>
      <button type="button" onClick={() => confirmed ? setConfirmCancellation(true) : void onCancel(lead).catch(() => {})} disabled={cancelLoading} className={`${buttonClass} text-status-critical`}>
        {cancelLoading ? "Cancelando…" : confirmed ? "Cancelar visita" : "Cancelar solicitação"}
      </button>
    </>}
    <ConfirmDialog open={confirmCancellation} title="Cancelar visita confirmada?" variant="warning"
      description={<>{property}<br />{schedule}<br />A visita será cancelada também na agenda da imobiliária. Para mudar o horário, use “Alterar horário”.</>}
      confirmLabel="Cancelar visita" cancelLabel="Manter visita"
      onCancel={() => setConfirmCancellation(false)}
      onConfirm={async () => { await onCancel(lead); setConfirmCancellation(false); }} />
  </div>;
}
