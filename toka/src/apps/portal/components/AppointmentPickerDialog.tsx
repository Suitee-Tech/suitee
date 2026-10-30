import { useEffect, useMemo, useRef, useState } from "react";
import { z } from "zod";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { addDays, isSameDay, startOfDay } from "date-fns";
import { ptBR } from "date-fns/locale";
import { AlertCircle, CalendarDays, CheckCircle2, Clock, Home, MessageSquareText } from "lucide-react";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "../../../shared/ui/dialog";
import { Button } from "../../../shared/ui/button";
import { Calendar } from "../../../shared/ui/calendar";
import { Input } from "../../../shared/ui/input";
import { Textarea } from "../../../shared/ui/textarea";
import {
  addMinutesToTime,
  appointmentDurationLabel,
  buildDefaultAppointmentValues,
  formatAppointmentDate,
  isAppointmentStartInPast,
  QUICK_APPOINTMENT_TIMES,
  timeToMinutes,
  type AppointmentDraft,
} from "./AppointmentPickerDialog.utils";

export type AppointmentPickerValues = AppointmentDraft;

const timeRegex = /^([01]\d|2[0-3]):([0-5]\d)(?::([0-5]\d))?$/;

const schema = z
  .object({
    date: z.date({ message: "Selecione uma data" }),
    startTime: z.string().min(1, "Informe o horário de início").regex(timeRegex, "Horário inválido"),
    endTime: z.string().min(1, "Informe o horário de término").regex(timeRegex, "Horário inválido"),
    note: z.string().max(500, "Use no máximo 500 caracteres").optional(),
  })
  .superRefine((values, context) => {
    const start = timeToMinutes(values.startTime);
    const end = timeToMinutes(values.endTime);

    if (start !== null && end !== null && end <= start) {
      context.addIssue({
        code: "custom",
        message: "O término precisa ser após o início",
        path: ["endTime"],
      });
    }

    if (values.date && start !== null && isAppointmentStartInPast(values.date, values.startTime)) {
      context.addIssue({
        code: "custom",
        message: "Escolha um horário futuro",
        path: ["startTime"],
      });
    }
  });

type SubmissionState = "editing" | "success";

export function AppointmentPickerDialog({
  open,
  onOpenChange,
  onSubmit,
  propertyLabel,
  defaultValues,
  mode = "create",
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (values: AppointmentPickerValues) => void | Promise<void>;
  propertyLabel?: string;
  defaultValues?: AppointmentPickerValues;
  mode?: "create" | "reschedule";
}) {
  const initialValues = useMemo(() => buildDefaultAppointmentValues(), []);
  const [referenceNow, setReferenceNow] = useState(() => new Date());
  // Mês exibido no calendário. `defaultMonth` só vale na MONTAGEM: se a visita
  // (ou a data escolhida) chega depois, o calendário ficava num mês sem
  // relação com a seleção — foi assim que o painel abriu em "julho 2028" com a
  // visita em setembro. Com o mês controlado a view SEMPRE acompanha a data
  // selecionada, e as setas continuam navegando.
  const [visibleMonth, setVisibleMonth] = useState<Date | null>(null);
  const [submissionState, setSubmissionState] = useState<SubmissionState>("editing");
  const [submissionError, setSubmissionError] = useState<string | null>(null);
  const [submittedValues, setSubmittedValues] = useState<AppointmentPickerValues | null>(null);
  const submissionLockRef = useRef(false);
  const endTimeWasEditedRef = useRef(false);

  const {
    handleSubmit,
    formState: { errors, isSubmitting },
    register,
    reset,
    setValue,
    watch,
  } = useForm<AppointmentPickerValues>({
    resolver: zodResolver(schema),
    defaultValues: initialValues,
    mode: "onTouched",
    reValidateMode: "onChange",
  });

  const date = watch("date");
  const startTime = watch("startTime");
  const endTime = watch("endTime");
  const note = watch("note") ?? "";

  useEffect(() => {
    if (!open) return;

    const now = new Date();
    setReferenceNow(now);
    reset(defaultValues ?? buildDefaultAppointmentValues(now));
    setVisibleMonth(null);
    setSubmissionState("editing");
    setSubmissionError(null);
    setSubmittedValues(null);
    submissionLockRef.current = false;
    endTimeWasEditedRef.current = false;
  }, [open, reset, defaultValues]);

  const quickDates = useMemo(() => {
    const today = startOfDay(referenceNow);
    return [
      { label: "Hoje", date: today },
      { label: "Amanhã", date: addDays(today, 1) },
    ];
  }, [referenceNow]);

  const startMinutes = timeToMinutes(startTime);
  const endMinutes = timeToMinutes(endTime);
  const hasInvalidRange = startMinutes !== null && endMinutes !== null && endMinutes <= startMinutes;
  const durationLabel = appointmentDurationLabel(startTime, endTime);

  const clearSubmissionFeedback = () => {
    if (submissionError) setSubmissionError(null);
  };

  const selectDate = (selectedDate: Date) => {
    const day = startOfDay(selectedDate);
    setValue("date", day, {
      shouldDirty: true,
      shouldTouch: true,
      shouldValidate: true,
    });
    // Atalhos "Hoje"/"Amanhã" e a seleção de um dia levam a view do calendário
    // para o mês da data escolhida (inclusive quando Amanhã cai no mês seguinte).
    setVisibleMonth(day);
    clearSubmissionFeedback();
  };

  const selectQuickTime = (selectedStart: string) => {
    const suggestedEnd = addMinutesToTime(selectedStart, 60);
    setValue("startTime", selectedStart, {
      shouldDirty: true,
      shouldTouch: true,
      shouldValidate: true,
    });
    if (suggestedEnd) {
      setValue("endTime", suggestedEnd, {
        shouldDirty: true,
        shouldTouch: true,
        shouldValidate: true,
      });
    }
    endTimeWasEditedRef.current = false;
    clearSubmissionFeedback();
  };

  const startTimeField = register("startTime", {
    setValueAs: (value) => (typeof value === "string" ? value : ""),
    onChange: (event) => {
      clearSubmissionFeedback();
      if (endTimeWasEditedRef.current) return;
      const suggestedEnd = addMinutesToTime(event.target.value, 60);
      if (suggestedEnd) {
        setValue("endTime", suggestedEnd, { shouldValidate: true });
      }
    },
  });

  const endTimeField = register("endTime", {
    setValueAs: (value) => (typeof value === "string" ? value : ""),
    onChange: () => {
      endTimeWasEditedRef.current = true;
      clearSubmissionFeedback();
    },
  });

  const noteField = register("note", {
    onChange: clearSubmissionFeedback,
  });

  const submit = async (values: AppointmentPickerValues) => {
    if (submissionLockRef.current) return;
    submissionLockRef.current = true;
    setSubmissionError(null);

    const normalizedValues: AppointmentPickerValues = {
      ...values,
      date: new Date(values.date),
      note: values.note?.trim() || undefined,
    };

    try {
      await onSubmit(normalizedValues);
      setSubmittedValues(normalizedValues);
      setSubmissionState("success");
    } catch (err) {
      setSubmissionError(err instanceof Error ? err.message : mode === "reschedule"
        ? "Não foi possível alterar o horário. Tente novamente."
        : "Não foi possível enviar sua solicitação. Confira sua conexão e tente novamente.");
    } finally {
      submissionLockRef.current = false;
    }
  };

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen && submissionLockRef.current) return;
    onOpenChange(nextOpen);
  };

  const finish = () => {
    submissionLockRef.current = false;
    onOpenChange(false);
  };

  const preventDismissWhileSubmitting = (event: Event) => {
    if (submissionLockRef.current) event.preventDefault();
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent
        className="flex max-h-[calc(100dvh-1rem)] flex-col gap-0 overflow-hidden p-0 sm:max-w-[660px]"
        onEscapeKeyDown={preventDismissWhileSubmitting}
        onPointerDownOutside={preventDismissWhileSubmitting}
      >
        {submissionState === "success" && submittedValues ? (
          <div className="flex flex-col">
            <DialogHeader className="px-6 pb-2 pt-8 text-center sm:text-center">
              <div className="mx-auto mb-2 flex size-12 items-center justify-center rounded-full bg-status-ok/10 text-status-ok">
                <CheckCircle2 className="size-6" aria-hidden="true" />
              </div>
              <DialogTitle>{mode === "reschedule" ? "Horário alterado" : "Solicitação enviada"}</DialogTitle>
              <DialogDescription>
                {mode === "reschedule" ? "O novo horário foi atualizado na agenda da imobiliária. Acompanhe o status da visita no portal." : "O corretor recebeu sua preferência e entrará em contato para confirmar a visita."}
              </DialogDescription>
            </DialogHeader>

            <div className="mx-5 my-5 rounded-2xl border border-border bg-muted/30 p-4 sm:mx-6">
              {propertyLabel && (
                <div className="mb-3 flex items-start gap-2 border-b border-border pb-3 text-sm font-medium text-foreground">
                  <Home className="mt-0.5 size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
                  <span>{propertyLabel}</span>
                </div>
              )}
              <div className="flex items-start gap-2 text-sm text-foreground">
                <CalendarDays className="mt-0.5 size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
                <span className="capitalize">{formatAppointmentDate(submittedValues.date)}</span>
              </div>
              <div className="mt-2 flex items-start gap-2 text-sm text-foreground">
                <Clock className="mt-0.5 size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
                <span>
                  Das {submittedValues.startTime} às {submittedValues.endTime}
                </span>
              </div>
              {submittedValues.note && (
                <div className="mt-2 flex items-start gap-2 text-sm text-muted-foreground">
                  <MessageSquareText className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
                  <span>{submittedValues.note}</span>
                </div>
              )}
            </div>

            <DialogFooter className="border-t border-border px-5 py-4 sm:px-6">
              <Button type="button" onClick={finish} className="w-full sm:w-auto">
                Concluir
              </Button>
            </DialogFooter>
          </div>
        ) : (
          <form onSubmit={handleSubmit(submit)} noValidate className="flex min-h-0 flex-1 flex-col overflow-hidden">
            <DialogHeader className="shrink-0 border-b border-border px-5 pb-4 pt-5 pr-12 sm:px-6 sm:pt-6">
              <div className="flex items-start gap-3">
                <div className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary">
                  <CalendarDays className="size-5" aria-hidden="true" />
                </div>
                <div className="min-w-0">
                  <DialogTitle>{mode === "reschedule" ? "Alterar horário da visita" : "Solicitar uma visita"}</DialogTitle>
                  <DialogDescription className="mt-1 leading-relaxed text-foreground/70">
                    {mode === "reschedule" ? "Escolha um novo horário para esta visita." : "Informe sua preferência. O horário será confirmado pelo corretor."}
                  </DialogDescription>
                </div>
              </div>

              {propertyLabel && (
                <div className="mt-2 flex items-center gap-2 rounded-xl bg-muted/50 px-3 py-2 text-left text-sm text-foreground">
                  <Home className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
                  <span className="truncate">{propertyLabel}</span>
                </div>
              )}
            </DialogHeader>

            <div className="min-h-0 overflow-y-auto overscroll-contain px-5 py-5 sm:px-6">
              <div className="grid gap-6 sm:grid-cols-[minmax(0,1.05fr)_minmax(0,0.95fr)]">
                <section aria-labelledby="appointment-date-heading" className="min-w-0">
                  <div className="mb-3 flex items-center gap-2">
                    <span className="flex size-6 items-center justify-center rounded-full bg-primary text-xs font-bold text-primary-foreground" aria-hidden="true">
                      1
                    </span>
                    <h3 id="appointment-date-heading" className="text-sm font-semibold text-foreground">
                      Escolha a data
                    </h3>
                  </div>

                  <div className="mb-3 flex gap-2" aria-label="Atalhos de data">
                    {quickDates.map((option) => {
                      const selected = date ? isSameDay(date, option.date) : false;
                      return (
                        <Button
                          key={option.label}
                          type="button"
                          size="sm"
                          variant={selected ? "default" : "outline"}
                          aria-pressed={selected}
                          onClick={() => selectDate(option.date)}
                          className="flex-1 border-muted-foreground"
                        >
                          {option.label}
                        </Button>
                      );
                    })}
                  </div>

                  <div className="flex justify-center rounded-2xl border border-border bg-card p-1 sm:p-2">
                    <Calendar
                      mode="single"
                      locale={ptBR}
                      month={visibleMonth ?? date ?? undefined}
                      onMonthChange={setVisibleMonth}
                      selected={date}
                      onSelect={(selectedDate) => {
                        if (selectedDate) selectDate(selectedDate);
                      }}
                      disabled={{ before: startOfDay(referenceNow) }}
                      aria-label="Calendário para selecionar a data da visita"
                      className="p-2"
                    />
                  </div>
                  {errors.date?.message && (
                    <p id="appointment-date-error" role="alert" className="mt-2 text-xs text-status-critical">
                      {errors.date.message}
                    </p>
                  )}
                  <p className="mt-2 text-center text-xs font-medium text-foreground/70 capitalize">
                    {date ? formatAppointmentDate(date) : "Nenhuma data selecionada"}
                  </p>
                </section>

                <div className="grid content-start gap-6">
                  <section aria-labelledby="appointment-time-heading">
                    <div className="mb-3 flex items-center gap-2">
                      <span className="flex size-6 items-center justify-center rounded-full bg-primary text-xs font-bold text-primary-foreground" aria-hidden="true">
                        2
                      </span>
                      <h3 id="appointment-time-heading" className="text-sm font-semibold text-foreground">
                        Escolha o horário
                      </h3>
                    </div>

                    <div className="mb-3 grid grid-cols-4 gap-1.5" aria-label="Sugestões de horário">
                      {QUICK_APPOINTMENT_TIMES.map((time) => {
                        const unavailable = date ? isAppointmentStartInPast(date, time, referenceNow) : false;
                        return (
                          <Button
                            key={time}
                            type="button"
                            size="sm"
                            variant={startTime === time ? "default" : "outline"}
                            aria-pressed={startTime === time}
                            disabled={unavailable}
                            onClick={() => selectQuickTime(time)}
                            // `border-muted-foreground`: a borda padrão
                            // (`border-input`, #e5e7eb sobre o card) mede 1,13:1
                            // — o chip não selecionado praticamente não tinha
                            // limite visível (WCAG 1.4.11 pede 3:1).
                            className="border-muted-foreground px-2 tabular-nums"
                          >
                            {time}
                          </Button>
                        );
                      })}
                    </div>

                    <div className="grid grid-cols-2 gap-3">
                      <div className="grid gap-1.5">
                        <label htmlFor="appointment-start-time" className="text-xs font-medium text-foreground">
                          Início
                        </label>
                        <Input
                          id="appointment-start-time"
                          type="time"
                          step={900}
                          aria-invalid={Boolean(errors.startTime)}
                          aria-describedby={errors.startTime ? "appointment-start-error" : undefined}
                          className="border-muted-foreground"
                          {...startTimeField}
                        />
                        {errors.startTime?.message && (
                          <p id="appointment-start-error" role="alert" className="text-xs text-status-critical">
                            {errors.startTime.message}
                          </p>
                        )}
                      </div>

                      <div className="grid gap-1.5">
                        <label htmlFor="appointment-end-time" className="text-xs font-medium text-foreground">
                          Término
                        </label>
                        <Input
                          id="appointment-end-time"
                          type="time"
                          step={900}
                          aria-invalid={Boolean(errors.endTime) || hasInvalidRange}
                          aria-describedby={errors.endTime || hasInvalidRange ? "appointment-end-error" : undefined}
                          className="border-muted-foreground"
                          {...endTimeField}
                        />
                        {(errors.endTime?.message || hasInvalidRange) && (
                          <p id="appointment-end-error" role="alert" className="text-xs text-status-critical">
                            {errors.endTime?.message ?? "O término precisa ser após o início"}
                          </p>
                        )}
                      </div>
                    </div>

                    <p className="mt-2 flex items-center gap-1.5 text-xs text-foreground/70" aria-live="polite">
                      <Clock className="size-3.5" aria-hidden="true" />
                      {durationLabel ? `Duração prevista: ${durationLabel}` : "Defina um intervalo válido no mesmo dia"}
                    </p>
                  </section>

                  <section aria-labelledby="appointment-notes-heading">
                    <div className="mb-3 flex items-center gap-2">
                      <span className="flex size-6 items-center justify-center rounded-full bg-primary text-xs font-bold text-primary-foreground" aria-hidden="true">
                        3
                      </span>
                      <h3 id="appointment-notes-heading" className="text-sm font-semibold text-foreground">
                        Conte algum detalhe <span className="font-normal text-foreground/70">(opcional)</span>
                      </h3>
                    </div>

                    <label htmlFor="appointment-note" className="sr-only">
                      Observações para a visita
                    </label>
                    <Textarea
                      id="appointment-note"
                      rows={3}
                      maxLength={500}
                      placeholder="Ex.: prefiro contato por WhatsApp ou preciso de acessibilidade."
                      aria-invalid={Boolean(errors.note)}
                      aria-describedby="appointment-note-help"
                      {...noteField}
                    />
                    <div id="appointment-note-help" className="mt-1.5 flex justify-between gap-3 text-xs text-foreground/70">
                      <span>Não inclua dados sensíveis.</span>
                      <span className="tabular-nums">{note.length}/500</span>
                    </div>
                    {errors.note?.message && (
                      <p role="alert" className="mt-1 text-xs text-status-critical">
                        {errors.note.message}
                      </p>
                    )}
                  </section>
                </div>
              </div>

              {submissionError && (
                <div className="mt-5 flex items-start gap-2 rounded-xl border border-status-critical/30 bg-status-critical/5 p-3 text-sm text-status-critical" role="alert">
                  <AlertCircle className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
                  <span>{submissionError}</span>
                </div>
              )}
            </div>

            <DialogFooter className="shrink-0 border-t border-border bg-background px-5 py-4 sm:px-6">
              <Button type="button" variant="outline" onClick={() => handleOpenChange(false)} disabled={isSubmitting} className="w-full sm:w-auto">
                Cancelar
              </Button>
              <Button type="submit" loading={isSubmitting} loadingText="Enviando solicitação..." className="w-full sm:w-auto sm:min-w-48">
                {mode === "reschedule" ? "Salvar novo horário" : "Solicitar visita"}
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  );
}
