import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { Calendar as CalendarIcon, Check, Trash2, X } from "lucide-react";
import { ExpandableSidePanel } from "../ExpandableSidePanel";
import { Button } from "../../ui/button";
import { ConfirmDialog } from "../ConfirmDialog";
import { useCalendarContext } from "../../context/CalendarContext";
import { useAgentsContext } from "../../context/AgentsContext";
import { EventForm } from "./EventForm";
import type { Appointment, AppointmentStatus, AppointmentType, Lead, Property } from "../../data";
import {
  EMPTY_APPOINTMENT_FORM,
  appointmentFormFrom,
  suggestedAppointmentTitle,
  validateAppointmentForm,
  type AppointmentFormValues,
} from "./appointmentForm";

/**
 * Agenda do lead, no contexto dele.
 *
 * A agenda completa (`pages/Calendar`) é o lugar de planejar o dia inteiro de
 * todo mundo; aqui o operador está olhando UM cliente e precisa agendar,
 * reagendar e mexer no status sem sair da página — antes a aba Agenda do lead
 * era só leitura, e quem quisesse marcar uma visita tinha que ir até a agenda,
 * abrir o formulário e procurar o lead de novo na lista.
 *
 * As regras de validação e os campos são os mesmos da agenda (ver
 * `appointmentForm.ts`): criar por aqui não pode aceitar o que a agenda recusa.
 */

const APPOINTMENT_TYPES: AppointmentType[] = ["Visita", "Reuniao", "Ligacao", "Assinatura", "Vistoria", "Lembrete", "Outro"];

const inputClass =
  "w-full bg-muted border border-border text-foreground text-sm rounded-xl px-3 py-2.5 outline-none focus:ring-2 focus:ring-primary-500/20 focus:border-primary-500 transition-all font-medium";

const labelClass = "block text-xs font-bold uppercase tracking-wide text-muted-foreground mb-1.5";

export interface LeadAppointmentLead {
  id: string;
  name: string;
  /** Imóvel de interesse do lead, quando existe: já entra no compromisso. */
  propertyId?: string | null;
}

export function LeadAppointmentPanel({
  open,
  lead,
  appointment,
  properties,
  leads,
  onClose,
  onSaved,
}: {
  open: boolean;
  lead: LeadAppointmentLead;
  /** Compromisso em edição; ausente = novo compromisso para este lead. */
  appointment?: Appointment | null;
  /** Listas do contexto usadas pelo formulário da agenda (imóveis e leads). */
  properties: Property[];
  leads: Lead[];
  onClose: () => void;
  onSaved?: (appointment: Appointment) => void;
}) {
  const { addAppointment, updateAppointment, deleteAppointment, currentUserId } = useCalendarContext();
  const { agents } = useAgentsContext();
  const [form, setForm] = useState<AppointmentFormValues>(EMPTY_APPOINTMENT_FORM);
  const [saving, setSaving] = useState(false);
  const [confirmRemove, setConfirmRemove] = useState(false);

  const editing = appointment ?? null;
  const defaultValue = useMemo(
    () => ({
      title: suggestedAppointmentTitle("Visita", lead.name),
      client: lead.name,
      leadId: lead.id,
      propertyId: lead.propertyId ?? "",
      agentId: currentUserId ?? "",
      type: "Visita" as AppointmentType,
    }),
    [currentUserId, lead.id, lead.name, lead.propertyId],
  );

  useEffect(() => {
    if (!open) return;
    setForm(editing ? appointmentFormFrom(editing) : { ...EMPTY_APPOINTMENT_FORM, ...defaultValue });
  }, [defaultValue, editing, open]);

  // Título sugerido acompanha o tipo enquanto o operador não editar o campo.
  const updateField = (field: string, value: any) => {
    setForm((prev) => {
      const next = { ...prev, [field]: value } as AppointmentFormValues;
      if (field === "type" && !editing) {
        next.title = suggestedAppointmentTitle(value as AppointmentType, lead.name);
      }
      if (field === "leadId" && !value) {
        // O painel é do lead: escolher outro cliente é permitido, esvaziar não.
        next.leadId = lead.id;
      }
      return next;
    });
  };

  const save = async () => {
    if (saving) return;
    const errors = validateAppointmentForm(form, { editing });
    if (errors.length) {
      toast.error(`Campos obrigatórios: ${errors.join(", ")}`);
      return;
    }
    setSaving(true);
    try {
      const payload = { ...form, leadId: lead.id, client: form.client || lead.name };
      if (editing) {
        const changed = Object.fromEntries(
          Object.entries(payload).filter(([key, value]) => value !== editing[key as keyof Appointment]),
        ) as Partial<Appointment>;
        const saved = await updateAppointment(editing.id, changed);
        if (!saved) return;
        toast.success("Compromisso atualizado");
        onSaved?.(saved);
      } else {
        const saved = await addAppointment(payload);
        if (!saved) return;
        toast.success("Compromisso agendado para este lead");
        onSaved?.(saved);
      }
      onClose();
    } finally {
      setSaving(false);
    }
  };

  const setStatus = async (status: AppointmentStatus) => {
    if (!editing || saving) return;
    setSaving(true);
    try {
      const saved = await updateAppointment(editing.id, { status });
      if (!saved) return;
      toast.success(status === "Confirmado" ? "Visita confirmada. O cliente foi notificado." : "Visita cancelada");
      onSaved?.(saved);
      onClose();
    } finally {
      setSaving(false);
    }
  };

  const remove = async () => {
    if (!editing) return;
    setSaving(true);
    try {
      const ok = await deleteAppointment(editing.id);
      if (!ok) return;
      toast.success("Compromisso removido");
      onClose();
    } finally {
      setSaving(false);
      setConfirmRemove(false);
    }
  };

  return (
    <>
      <ExpandableSidePanel
        ariaLabel={editing ? "Editar compromisso do lead" : "Agendar compromisso para o lead"}
        open={open}
        onClose={onClose}
        renderHeader={({ expandButton, closeButton }) => (
          <div className="px-6 py-4 border-b border-border bg-muted/50 flex items-center justify-between flex-shrink-0">
            <div className="min-w-0">
              <h3 className="text-base font-bold text-foreground tracking-tight flex items-center gap-2">
                <CalendarIcon className="size-4 text-primary" aria-hidden="true" />
                {editing ? "Editar compromisso" : "Agendar para o lead"}
              </h3>
              <p className="text-xs text-muted-foreground truncate">{lead.name}</p>
            </div>
            <div className="flex items-center gap-2">
              {expandButton}
              {closeButton}
            </div>
          </div>
        )}
      >
        {/* Corpo rolável com o MESMO formulário da agenda completa: campos,
            combos de cliente/imóvel/negócio, tipos, status, prioridade e
            validação. Quem agenda pelo lead não vê uma versão reduzida. */}
        <div className="flex-1 overflow-y-auto min-h-0 px-6 py-5">
          <EventForm form={form} updateField={updateField} agents={agents} properties={properties} leads={leads} editing={editing} />
        </div>

        <div className="px-6 py-4 border-t border-border flex flex-wrap items-center justify-between gap-2 flex-shrink-0">
          <div className="flex items-center gap-2">
            {editing && (
              <>
                <Button variant="outline" size="sm" disabled={saving} onClick={() => void setStatus("Confirmado")}>
                  <Check className="w-4 h-4" /> Confirmar visita
                </Button>
                <Button variant="outline" size="sm" disabled={saving} onClick={() => void setStatus("Cancelado")}>
                  <X className="w-4 h-4" /> Cancelar visita
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={saving}
                  className="text-destructive hover:text-destructive"
                  onClick={() => setConfirmRemove(true)}
                >
                  <Trash2 className="w-4 h-4" /> Remover
                </Button>
              </>
            )}
          </div>
          <div className="flex items-center gap-2">
            <Button variant="outline" onClick={onClose}>
              Fechar
            </Button>
            <Button onClick={() => void save()} disabled={saving}>
              {saving ? "Salvando…" : editing ? "Salvar alterações" : "Agendar"}
            </Button>
          </div>
        </div>
      </ExpandableSidePanel>

      <ConfirmDialog
        open={confirmRemove}
        title="Remover compromisso"
        description="Esta ação não pode ser desfeita."
        confirmLabel="Remover"
        variant="danger"
        onCancel={() => setConfirmRemove(false)}
        onConfirm={() => void remove()}
      />
    </>
  );
}
