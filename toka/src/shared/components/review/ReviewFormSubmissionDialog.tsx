import { useEffect, useMemo, useState } from "react";
import { AlertTriangle, FileText, Sparkles, UserRound, X } from "lucide-react";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "../../ui/dialog";
import { Button } from "../../ui/button";
import { Input } from "../../ui/input";
import { cn } from "../../ui/utils";
import type { DocumentChecklistItem } from "../../data";
import type { PartyDTO } from "../../services/deal.service";
import { dealService } from "../../services/deal.service";
import { formatCpfCnpj, parseCpfCnpj } from "../../utils/masks";
import {
  buildFormReviewFields,
  formCorrections,
  missingRequiredCorrections,
  sameFormValue,
  type FormReviewField,
} from "./form-review-utils";

function formatDateTime(value?: string): string {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  return date.toLocaleString("pt-BR", {
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function displayValue(field: FormReviewField, value: string): string {
  if (field.key === "cpf") return formatCpfCnpj(parseCpfCnpj(value));
  return value;
}

/**
 * Revisão do formulário preenchido pelo cliente.
 *
 * O painel esquerdo mostra o snapshot imutável da submissão; o direito mostra
 * o valor atual da party e permite corrigir antes de aprovar o item.
 */
export function ReviewFormSubmissionDialog({
  open,
  onOpenChange,
  dealId,
  party,
  item,
  onReviewed,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  dealId: string;
  party: PartyDTO | null;
  item: DocumentChecklistItem;
  onReviewed?: () => void;
}) {
  const fields = useMemo(() => buildFormReviewFields(item, party), [item, party]);
  const [corrections, setCorrections] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [correctionOpen, setCorrectionOpen] = useState(false);
  const [correctionReason, setCorrectionReason] = useState("");
  const [correctionError, setCorrectionError] = useState("");

  const submission = item.formSubmission;
  const hasSnapshot = !!submission;
  const readOnly = item.status !== "uploaded";
  const statusLabel =
    item.status === "approved"
      ? "Aprovado"
      : item.status === "rejected"
        ? "Rejeitado"
        : item.status === "uploaded"
          ? "Em análise"
          : "Aguardando cliente";
  const statusClass =
    item.status === "approved"
      ? "bg-status-ok-soft text-status-ok"
      : item.status === "rejected"
        ? "bg-status-critical-soft text-status-critical"
        : item.status === "uploaded"
          ? "bg-status-info-soft text-status-info"
          : "bg-status-warning-soft text-status-warning";
  const submittedAt = submission?.submittedAt ?? item.uploadedAt;
  const submittedBy = submission?.submittedBy ?? "Cliente";

  useEffect(() => {
    if (!open) return;
    const initial: Record<string, string> = {};
    for (const field of fields) initial[field.key] = field.current;
    setCorrections(initial);
    setSaving(false);
    setError("");
    setCorrectionOpen(false);
    setCorrectionReason("");
    setCorrectionError("");
  }, [open, item.id, fields]);

  const correctionsList = formCorrections(fields, corrections);
  const missing = missingRequiredCorrections(fields, corrections);
  const divergences = fields.filter((field) => !sameFormValue(field.key, field.submitted, field.current));
  const changedCount = correctionsList.length;
  const canApprove = !!party && !readOnly && missing.length === 0 && !saving;

  const setCorrection = (field: FormReviewField, value: string) => {
    setCorrections((prev) => ({ ...prev, [field.key]: value }));
  };

  const approve = async () => {
    if (!party || readOnly || missing.length > 0) return;
    setSaving(true);
    setError("");
    try {
      if (correctionsList.length > 0) {
        await dealService.applyExtractedFields(dealId, party.id, { fields: correctionsList });
      }
      await dealService.updateChecklistItemStatus(dealId, item.id, { status: "APPROVED" });
      onReviewed?.();
      onOpenChange(false);
    } catch {
      setError("Não foi possível aprovar o formulário. Tente novamente.");
    } finally {
      setSaving(false);
    }
  };

  const requestCorrection = async () => {
    if (readOnly) return;
    if (!correctionReason.trim()) {
      setCorrectionError("Informe o motivo da correção.");
      return;
    }
    setSaving(true);
    setCorrectionError("");
    try {
      await dealService.updateChecklistItemStatus(dealId, item.id, {
        status: "REJECTED",
        rejection_reason: correctionReason.trim(),
      });
      onReviewed?.();
      onOpenChange(false);
    } catch {
      setCorrectionError("Não foi possível solicitar a correção. Tente novamente.");
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        showCloseButton={false}
        className="flex h-[min(94vh,920px)] w-[min(1280px,96vw)] max-w-none flex-col gap-0 overflow-hidden p-0 sm:max-w-none"
      >
        <header className="flex flex-none items-start justify-between gap-3 border-b border-border bg-card px-4 py-3 sm:px-5">
          <div className="flex min-w-0 items-start gap-2.5">
            <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-status-info-soft text-status-info">
              <FileText className="h-4 w-4" />
            </div>
            <div className="min-w-0">
              <DialogTitle className="text-base font-bold text-foreground">Revisão de formulário</DialogTitle>
              <DialogDescription className="mt-0.5 truncate text-xs text-muted-foreground">
                {item.name} · {party?.name || "Parte não identificada"}
              </DialogDescription>
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-1.5">
            <span className="hidden rounded-full bg-status-info-soft px-2 py-1 text-micro font-bold text-status-info sm:inline">
              Formulário
            </span>
            <span className={cn("hidden rounded-full px-2 py-1 text-micro font-bold sm:inline", statusClass)}>
              {statusLabel}
            </span>
            <span className="hidden rounded-full border border-border bg-card px-2 py-1 text-micro font-bold text-muted-foreground sm:inline">
              {submittedAt ? `Enviado em ${formatDateTime(submittedAt)}` : "Sem snapshot"}
            </span>
            <button
              type="button"
              onClick={() => onOpenChange(false)}
              className="flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground"
              aria-label="Fechar revisão"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
        </header>

        <div className="grid min-h-0 flex-1 grid-cols-1 lg:grid-cols-[minmax(0,42%)_minmax(0,58%)]">
          <aside className="hidden min-h-0 flex-col border-r border-border bg-muted/20 lg:flex">
            <div className="flex-none border-b border-border bg-card px-4 py-3">
              <div className="flex items-center gap-1.5 text-micro font-bold uppercase tracking-wide text-muted-foreground">
                <UserRound className="h-3 w-3" /> Formulário do cliente
              </div>
              <p className="mt-1 text-sm font-bold text-foreground">{item.name}</p>
              {hasSnapshot ? (
                <p className="mt-0.5 text-caption text-muted-foreground">
                  {submittedBy} · {formatDateTime(submittedAt)}
                </p>
              ) : submittedAt ? (
                <p className="mt-0.5 text-caption text-status-warning">
                  Enviado em {formatDateTime(submittedAt)} · sem snapshot
                </p>
              ) : (
                <p className="mt-0.5 text-caption text-status-warning">Submissão sem snapshot</p>
              )}
            </div>
            <div className="min-h-0 flex-1 overflow-y-auto p-3">
              {!hasSnapshot && (
                <div className="mb-2 flex items-start gap-2 rounded-lg border border-status-warning/30 bg-status-warning-soft/40 p-2.5 text-caption text-status-warning">
                  <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                  {item.status === "pending"
                    ? "Aguardando o cliente preencher. Os campos abaixo são os solicitados."
                    : "Este item foi enviado antes do snapshot. Exibindo os valores atuais da parte; a revisão e a correção continuam disponíveis."}
                </div>
              )}
              <div className="space-y-2">
                {fields.map((field) => (
                  <div key={field.key} className="rounded-lg border border-border bg-card px-3 py-2">
                    <p className="text-micro font-bold uppercase tracking-wide text-muted-foreground">{field.label}</p>
                    <p className="mt-0.5 break-words text-sm text-foreground">
                      {displayValue(field, field.submitted) || "Não informado"}
                    </p>
                  </div>
                ))}
                {fields.length === 0 && (
                  <p className="text-caption text-muted-foreground">Nenhum campo solicitado neste formulário.</p>
                )}
              </div>
            </div>
            <div className="flex-none border-t border-border bg-card p-3">
              <div className="flex items-start gap-2">
                <Sparkles className="mt-0.5 h-3.5 w-3.5 shrink-0 text-primary" />
                <p className="text-caption text-muted-foreground">
                  Snapshot imutável da submissão. Corrigir a parte não altera o que o cliente enviou.
                </p>
              </div>
            </div>
          </aside>

          <section className="flex min-h-0 flex-col">
            <div className="flex-none border-b border-border bg-card px-4 py-3">
              <div className="flex items-start justify-between gap-2">
                <div className="min-w-0">
                  <h2 className="text-sm font-bold text-foreground">Campos do formulário</h2>
                  <p className="mt-0.5 text-caption text-muted-foreground">
                    {fields.length} campos · {divergences.length} divergências · {changedCount} correções
                  </p>
                </div>
                <span className="shrink-0 rounded-full bg-status-info-soft px-2 py-1 text-micro font-bold text-status-info">
                  {hasSnapshot ? "Snapshot" : "Sem snapshot"}
                </span>
              </div>
              {!readOnly && missing.length > 0 && (
                <p className="mt-2 rounded-md bg-status-critical-soft/50 px-2 py-1.5 text-caption text-status-critical">
                  Preencha os campos obrigatórios: {missing.map((field) => field.label).join(", ")}.
                </p>
              )}
              {readOnly && (
                <p className="mt-2 rounded-md border border-status-warning/30 bg-status-warning-soft/30 px-2 py-1.5 text-caption text-status-warning">
                  {item.status === "pending"
                    ? "Aguardando o cliente preencher e enviar. Os campos abaixo são os solicitados."
                    : item.status === "approved"
                      ? "Formulário aprovado. Exibindo os valores disponíveis para consulta."
                      : "Formulário rejeitado. Aguardando o cliente reenviar."}
                </p>
              )}
            </div>

            <div className="min-h-0 flex-1 overflow-y-auto px-4 py-3">
              {!party && (
                <div className="mb-3 flex items-start gap-2 rounded-lg border border-status-critical/30 bg-status-critical-soft/40 p-2.5 text-caption text-status-critical">
                  <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                  Parte não identificada. Não é possível aprovar ou corrigir o formulário.
                </div>
              )}
              {fields.length === 0 ? (
                <p className="rounded-lg border border-dashed border-border px-3 py-6 text-center text-caption text-muted-foreground">
                  Nenhum campo foi solicitado neste formulário.
                </p>
              ) : (
                <div className="space-y-2">
                  {fields.map((field) => {
                    const diverges = !sameFormValue(field.key, field.submitted, field.current);
                    const correction = corrections[field.key] ?? field.current;
                    const changed = !sameFormValue(field.key, correction, field.current);
                    return (
                      <article
                        key={field.key}
                        className={cn(
                          "rounded-xl border bg-card p-3 shadow-sm",
                          diverges ? "border-status-warning/40" : "border-border",
                        )}
                      >
                        <div className="flex flex-wrap items-center gap-1.5">
                          <span className="text-sm font-bold text-foreground">{field.label}</span>
                          <span className="rounded-full border border-status-info/20 bg-status-info-soft px-1.5 py-0.5 text-micro font-bold text-status-info">
                            Formulário
                          </span>
                          {field.required && (
                            <span className="rounded-full border border-border bg-muted/50 px-1.5 py-0.5 text-micro font-bold text-muted-foreground">
                              Obrigatório
                            </span>
                          )}
                          {diverges && (
                            <span className="rounded-full bg-status-warning-soft px-1.5 py-0.5 text-micro font-bold text-status-warning">
                              Divergente do valor atual
                            </span>
                          )}
                          {changed && (
                            <span className="rounded-full bg-status-ok-soft px-1.5 py-0.5 text-micro font-bold text-status-ok">
                              Correção
                            </span>
                          )}
                        </div>

                        <div className="mt-2 grid gap-2 sm:grid-cols-2">
                          <div className="min-w-0">
                            <span className="mb-1 block text-micro font-bold uppercase tracking-wide text-muted-foreground">
                              Enviado pelo cliente
                            </span>
                            <div className="flex h-9 items-center rounded-md border border-border bg-muted/30 px-2 text-sm text-foreground">
                              <span className="truncate">{displayValue(field, field.submitted) || "Não informado"}</span>
                            </div>
                          </div>
                          <div className="min-w-0">
                            <span className="mb-1 block text-micro font-bold uppercase tracking-wide text-muted-foreground">
                              Atual na parte
                            </span>
                            <div
                              className={cn(
                                "flex h-9 items-center rounded-md border px-2 text-sm",
                                diverges
                                  ? "border-status-warning/40 bg-status-warning-soft/40 text-status-warning"
                                  : "border-dashed border-border bg-muted/30 text-muted-foreground",
                              )}
                              title={displayValue(field, field.current)}
                            >
                              <span className="truncate">{displayValue(field, field.current) || "Não informado"}</span>
                            </div>
                          </div>
                        </div>

                        <div className="mt-2">
                          <span className="mb-1 block text-micro font-bold uppercase tracking-wide text-muted-foreground">
                            Corrigir valor na parte
                          </span>
                          <Input
                            value={displayValue(field, correction)}
                            onChange={(event) =>
                              setCorrection(
                                field,
                                field.key === "cpf"
                                  ? parseCpfCnpj(event.target.value)
                                  : event.target.value,
                              )
                            }
                            aria-label={`Corrigir ${field.label}`}
                            className="h-9 text-sm"
                            disabled={readOnly}
                          />
                        </div>

                        <div className="mt-2 flex flex-wrap gap-1.5">
                          <Button
                            type="button"
                            size="sm"
                            variant="outline"
                            className="h-7 border-primary/30 bg-primary/5 px-2 text-caption text-primary hover:bg-primary/10"
                            onClick={() => setCorrection(field, field.submitted)}
                            disabled={readOnly}
                          >
                            Usar valor enviado
                          </Button>
                          <Button
                            type="button"
                            size="sm"
                            variant="outline"
                            className="h-7 px-2 text-caption"
                            onClick={() => setCorrection(field, field.current)}
                            disabled={readOnly}
                          >
                            Manter atual
                          </Button>
                        </div>
                      </article>
                    );
                  })}
                </div>
              )}
            </div>

            {correctionOpen && !readOnly && (
              <div className="flex-none border-t border-border bg-status-warning-soft/30 px-4 py-3">
                <label htmlFor="form-correction-reason" className="text-caption font-bold text-foreground">
                  Motivo da correção
                </label>
                <textarea
                  id="form-correction-reason"
                  value={correctionReason}
                  onChange={(event) => setCorrectionReason(event.target.value)}
                  placeholder="Ex.: o CPF enviado não confere com o documento anexado."
                  className="mt-1 min-h-[64px] w-full rounded-md border border-border bg-card px-2 py-1.5 text-sm focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20"
                />
                {correctionError && <p className="mt-1 text-caption text-status-critical">{correctionError}</p>}
                <div className="mt-2 flex justify-end gap-2">
                  <Button type="button" variant="ghost" size="sm" onClick={() => setCorrectionOpen(false)} disabled={saving}>
                    Cancelar
                  </Button>
                  <Button type="button" variant="destructive" size="sm" onClick={() => void requestCorrection()} disabled={saving}>
                    {saving ? "Enviando..." : "Enviar solicitação"}
                  </Button>
                </div>
              </div>
            )}

            {readOnly ? (
              <footer className="flex flex-none flex-wrap items-center justify-between gap-2 border-t border-border bg-card px-4 py-3">
                <p className="min-w-0 text-caption text-muted-foreground">
                  Somente leitura · {statusLabel}.
                  {item.status === "pending" ? " Use “Solicitar” no item do checklist para reenviar." : ""}
                </p>
                <Button type="button" variant="outline" size="sm" onClick={() => onOpenChange(false)}>
                  Fechar
                </Button>
              </footer>
            ) : (
              <footer className="flex flex-none flex-wrap items-center justify-between gap-2 border-t border-border bg-card px-4 py-3">
                <div className="min-w-0">
                  <p className="text-caption text-muted-foreground">
                    <b className="text-foreground">{changedCount}</b> correção(ões) · {fields.length} campos
                  </p>
                  {error && <p className="text-caption text-status-critical">{error}</p>}
                </div>
                <div className="flex shrink-0 flex-wrap gap-2">
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => setCorrectionOpen((value) => !value)}
                    disabled={saving}
                  >
                    Solicitar correção
                  </Button>
                  <Button type="button" variant="outline" size="sm" onClick={() => onOpenChange(false)} disabled={saving}>
                    Cancelar
                  </Button>
                  <Button type="button" size="sm" disabled={!canApprove} onClick={() => void approve()}>
                    {saving ? "Aprovando..." : changedCount > 0 ? "Salvar correções e aprovar" : "Aprovar formulário"}
                  </Button>
                </div>
              </footer>
            )}
          </section>
        </div>
      </DialogContent>
    </Dialog>
  );
}
