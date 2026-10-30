import { useEffect, useMemo, useState, type ReactNode } from "react";
import {
  AlertTriangle,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  RefreshCw,
  Sparkles,
  X,
} from "lucide-react";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "../ui/dialog";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { cn } from "../ui/utils";
import type { PartyDTO } from "../services/deal.service";
import type { DocumentChecklistItem } from "../data";
import { dealService } from "../services/deal.service";
import type { DocumentDTO, ExtractedDocumentDTO, ExtractedFieldDTO } from "../services/document.service";
import { DocumentInlinePreview } from "./review/DocumentInlinePreview";
import {
  currentPartyValue,
  describeExtractionError,
  fieldDiverges,
  fieldFormat,
  formatFieldValue,
  isFieldInvalid,
  parseFieldInput,
  toDateInputValue,
} from "./review/review-utils";

type Decision = "pending" | "apply" | "keep" | "ignore";
type RowState = { decision: Decision; value: string };
type ReviewFilter = "all" | "divergences" | "low" | "high" | "missing";

function tierChip(tier: string): { label: string; cls: string } {
  switch (tier) {
    case "high":
      return { label: "Alta", cls: "bg-status-ok-soft text-status-ok" };
    case "medium":
      return { label: "Média", cls: "bg-status-warning-soft text-status-warning" };
    default:
      return { label: "Baixa", cls: "bg-status-critical-soft text-status-critical" };
  }
}

function pct(confidence: number): string {
  return `${Math.round(Math.max(0, Math.min(1, confidence)) * 100)}%`;
}

function decisionChip(decision: Decision): { label: string; cls: string } {
  switch (decision) {
    case "apply":
      return { label: "Aplicar", cls: "bg-primary/10 text-primary border-primary/20" };
    case "keep":
      return { label: "Manter atual", cls: "bg-muted text-muted-foreground border-border" };
    case "ignore":
      return { label: "Ignorado", cls: "bg-muted/60 text-muted-foreground/80 border-border" };
    default:
      return { label: "Pendente", cls: "bg-status-warning-soft text-status-warning border-status-warning/20" };
  }
}

/**
 * Workspace de revisão de OCR.
 *
 * Fase 1: documento ao lado, grupos por risco, decisão campo a campo,
 * "manter atual" explícito e apply preservando o endpoint atual.
 */
export function ReviewExtractedDataDialog({
  open,
  onOpenChange,
  dealId,
  party,
  document,
  extraction,
  onApplied,
  onReprocess,
  item,
  onApprove,
  onRequestCorrection,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  dealId: string;
  party: PartyDTO | null;
  document: DocumentDTO | null;
  extraction: ExtractedDocumentDTO | null | undefined;
  onApplied?: () => void;
  onReprocess?: () => void | Promise<void>;
  item?: DocumentChecklistItem | null;
  onApprove?: () => void | Promise<void>;
  onRequestCorrection?: (reason: string) => void | Promise<void>;
}) {
  const fields = useMemo(() => extraction?.fields ?? [], [extraction]);
  const [rows, setRows] = useState<Record<string, RowState>>({});
  const [filter, setFilter] = useState<ReviewFilter>("all");
  const [highExpanded, setHighExpanded] = useState(false);
  const [missingExpanded, setMissingExpanded] = useState(false);
  const [activeField, setActiveField] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState("");
  const [reprocessing, setReprocessing] = useState(false);
  const [reprocessError, setReprocessError] = useState("");
  const [applied, setApplied] = useState(false);
  const [approving, setApproving] = useState(false);
  const [showCorrection, setShowCorrection] = useState(false);
  const [correctionReason, setCorrectionReason] = useState("");
  const [correctionSending, setCorrectionSending] = useState(false);
  const [mobileView, setMobileView] = useState<"document" | "fields">("fields");

  useEffect(() => {
    if (!open) return;
    const initial: Record<string, RowState> = {};
    for (const field of fields) {
      const value = field.value ?? "";
      const current = currentPartyValue(party, field).trim();
      const invalid = isFieldInvalid(field, value);
      const diverges = fieldDiverges(party, field, value);
      const autoApply = field.tier === "high" && !invalid && !diverges && value.trim() !== "";
      // Extração vazia + valor atual existente: o padrão é MANTER o atual, não
      // deixar "Pendente". Sem valor extraído e sem atual, o campo é "Não
      // identificado" e só vira Aplicar se o operador digitar.
      const keepCurrent = value.trim() === "" && current !== "" && !invalid;
      initial[field.name] = {
        decision: autoApply ? "apply" : keepCurrent ? "keep" : "pending",
        value,
      };
    }
    setRows(initial);
    setFilter("all");
    setHighExpanded(false);
    setMissingExpanded(false);
    setActiveField(fields.find((field) => field.evidence)?.name ?? fields[0]?.name ?? null);
    setSaving(false);
    setSaveError("");
    setReprocessing(false);
    setReprocessError("");
    setApplied(false);
    setApproving(false);
    setShowCorrection(false);
    setCorrectionReason("");
    setCorrectionSending(false);
    setMobileView("fields");
  }, [open, document?.id, fields, party]);

  const updateRow = (name: string, patch: Partial<RowState>) => {
    setApplied(false);
    setRows((prev) => {
      const current = prev[name] ?? { decision: "pending", value: "" };
      return { ...prev, [name]: { ...current, ...patch } };
    });
  };

  const setDecision = (name: string, decision: Decision) => updateRow(name, { decision });
  const setValue = (name: string, value: string) => updateRow(name, { value, decision: "apply" });
  const applyExtractedValue = (field: ExtractedFieldDTO) =>
    updateRow(field.name, { value: field.value ?? "", decision: "apply" });

  const rowFor = (field: ExtractedFieldDTO): RowState =>
    rows[field.name] ?? { decision: "pending", value: field.value ?? "" };

  // A classificação dos grupos usa o valor ORIGINAL da extração, não o valor
  // que o operador está digitando. Se usássemos o valor vivo, cada tecla podia
  // mover o campo de "Não identificados" para "Divergências" (ou vice-versa),
  // remontando o input e perdendo o foco no meio da digitação.
  const originalFor = (field: ExtractedFieldDTO): string => field.value ?? "";

  const divergences = fields.filter((field) => {
    const original = originalFor(field);
    return original.trim() !== "" && fieldDiverges(party, field, original);
  });
  const missing = fields.filter((field) => originalFor(field).trim() === "");
  const low = fields.filter((field) => {
    const original = originalFor(field);
    if (original.trim() === "" || fieldDiverges(party, field, original)) return false;
    return field.tier !== "high" || isFieldInvalid(field, original);
  });
  const high = fields.filter((field) => {
    const original = originalFor(field);
    if (original.trim() === "" || fieldDiverges(party, field, original)) return false;
    return field.tier === "high" && !isFieldInvalid(field, original);
  });

  const selected = fields.filter((field) => {
    const row = rows[field.name];
    if (!row || row.decision !== "apply") return false;
    return row.value.trim() !== "" && !isFieldInvalid(field, row.value);
  });
  const selectedInvalid = fields.filter((field) => {
    const row = rows[field.name];
    return row?.decision === "apply" && row.value.trim() !== "" && isFieldInvalid(field, row.value);
  });
  const pending = fields.filter((field) => rowFor(field).decision === "pending").length;
  const extractedCount = fields.filter((field) => (field.value ?? "").trim() !== "").length;
  const noExtraction = fields.length > 0 && extractedCount === 0;
  const extractionError = extraction?.error ?? document?.extraction_error ?? "";
  const extractionFailed = extraction?.status === "failed" || Boolean(extractionError);
  const errorInfo = describeExtractionError(extractionError);
  const missingPending = missing.filter((field) => rowFor(field).decision === "pending").length;
  const overall = Math.round(Math.max(0, Math.min(1, extraction?.overall_confidence ?? 0)) * 100);
  const totalForBar = fields.length || 1;
  const segWidth = (count: number) => `${Math.round((count / totalForBar) * 100)}%`;
  const canApply = !!party && selected.length > 0 && !saving;

  const active =
    fields.find((field) => field.name === activeField) ??
    fields.find((field) => field.evidence) ??
    fields[0];

  const handleDecision = (field: ExtractedFieldDTO, decision: Decision) => {
    if (decision === "apply" && !rowFor(field).value.trim()) return;
    setDecision(field.name, decision);
  };

  const ignoreMissing = () => {
    setRows((prev) => {
      const next = { ...prev };
      for (const field of missing) {
        const current = next[field.name] ?? { decision: "pending" as Decision, value: field.value ?? "" };
        if (current.decision === "pending") {
          next[field.name] = { ...current, decision: "ignore" };
        }
      }
      return next;
    });
  };

  const reprocess = async () => {
    if (!onReprocess || reprocessing) return;
    setReprocessing(true);
    setReprocessError("");
    try {
      await onReprocess();
    } catch {
      setReprocessError("Não foi possível reenfileirar a extração. Tente novamente.");
    } finally {
      setReprocessing(false);
    }
  };

  const acceptHighConfidence = () => {
    setRows((prev) => {
      const next = { ...prev };
      for (const field of high) {
        const current = next[field.name] ?? { decision: "pending" as Decision, value: field.value ?? "" };
        next[field.name] = { ...current, decision: "apply" };
      }
      return next;
    });
  };

  const apply = async () => {
    if (!party || selected.length === 0 || saving) return;
    setSaving(true);
    setSaveError("");
    try {
      await dealService.applyExtractedFields(dealId, party.id, {
        document_id: document?.id,
        fields: selected.map((field) => ({
          name: field.name,
          target: field.target,
          value: rowFor(field).value.trim(),
          confidence: field.confidence,
        })),
      });
      onApplied?.();
      setApplied(true);
    } catch {
      setSaveError("Não foi possível aplicar os campos. Tente novamente.");
    } finally {
      setSaving(false);
    }
  };

  const approve = async () => {
    if (!onApprove || approving) return;
    setApproving(true);
    setSaveError("");
    try {
      await onApprove();
      onOpenChange(false);
    } catch {
      setSaveError("Não foi possível aprovar o documento. Tente novamente.");
    } finally {
      setApproving(false);
    }
  };

  const requestCorrection = async () => {
    if (!onRequestCorrection || !correctionReason.trim() || correctionSending) return;
    setCorrectionSending(true);
    setSaveError("");
    try {
      await onRequestCorrection(correctionReason.trim());
      onOpenChange(false);
    } catch {
      setSaveError("Não foi possível solicitar a correção. Tente novamente.");
    } finally {
      setCorrectionSending(false);
    }
  };

  const renderField = (field: ExtractedFieldDTO) => {
    const row = rowFor(field);
    const value = row.value;
    const current = currentPartyValue(party, field);
    const format = fieldFormat(field);
    const hasExtracted = (field.value ?? "").trim() !== "";
    const hasValue = value.trim() !== "";
    const hasCurrent = current.trim() !== "";
    const diverges = hasValue && fieldDiverges(party, field, value);
    const invalid = isFieldInvalid(field, value);
    const notIdentified = !hasExtracted && !hasValue && !hasCurrent;
    const manual = !hasExtracted && hasValue;
    const tier = tierChip(field.tier);
    const decision = decisionChip(row.decision);
    const displayValue = formatFieldValue(field, value);
    const displayCurrent = formatFieldValue(field, current);
    const dateInputValue = format === "date" ? toDateInputValue(value) : "";
    const invalidMessage =
      format === "cpf"
        ? "CPF/CNPJ inválido. Corrija o valor ou mantenha o atual."
        : format === "date"
          ? "Data inválida. Use o formato dd/mm/aaaa."
          : format === "phone"
            ? "Telefone incompleto. Informe DDD + número."
            : format === "cep"
              ? "CEP inválido. Informe os 8 dígitos."
              : format === "uf"
                ? "UF inválida. Use a sigla com 2 letras."
                : field.validation_message || "Valor inválido.";

    return (
      <article
        key={field.name}
        className={cn(
          "rounded-xl border bg-card p-3 shadow-sm transition-colors",
          diverges
            ? "border-status-warning/40"
            : invalid
              ? "border-status-critical/40"
              : notIdentified
                ? "border-dashed border-border bg-muted/20"
                : "border-border",
          row.decision === "ignore" && "opacity-70",
        )}
      >
        <div className="flex items-start gap-2">
          <input
            type="checkbox"
            className="mt-1 h-4 w-4 accent-primary disabled:cursor-not-allowed disabled:opacity-40"
            checked={row.decision === "apply"}
            disabled={!hasValue}
            title={!hasValue ? "Preencha um valor para aplicar" : undefined}
            onChange={(event) =>
              setDecision(field.name, event.target.checked ? "apply" : "pending")
            }
            onFocus={() => setActiveField(field.name)}
            aria-label={`Aplicar ${field.label || field.name}`}
          />
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-1.5">
              <span className="text-sm font-bold text-foreground">{field.label || field.name}</span>
              <span className="rounded-full border border-border bg-muted/50 px-1.5 py-0.5 text-micro font-bold uppercase tracking-wide text-muted-foreground">
                {hasExtracted ? "OCR" : manual ? "Manual" : "Sem valor"}
              </span>
              {hasExtracted ? (
                <span className={cn("rounded-full px-1.5 py-0.5 text-micro font-bold", tier.cls)}>
                  {tier.label} · {pct(field.confidence)}
                </span>
              ) : (
                <span className="rounded-full bg-muted px-1.5 py-0.5 text-micro font-bold text-muted-foreground">
                  {notIdentified ? "Não identificado" : "Sem extração"}
                </span>
              )}
              {diverges && (
                <span className="rounded-full bg-status-warning-soft px-1.5 py-0.5 text-micro font-bold text-status-warning">
                  Divergente
                </span>
              )}
              <span
                className={cn(
                  "rounded-full border px-1.5 py-0.5 text-micro font-bold",
                  notIdentified
                    ? "border-border bg-muted text-muted-foreground"
                    : decision.cls,
                )}
              >
                {notIdentified ? "Não identificado" : decision.label}
              </span>
            </div>

            <div className="mt-2 grid gap-2 sm:grid-cols-2">
              <div className="min-w-0">
                <span className="mb-1 block text-micro font-bold uppercase tracking-wide text-muted-foreground">
                  {hasExtracted ? "Valor extraído" : "Valor do campo"}
                </span>
                <Input
                  type={format === "date" ? "date" : format === "phone" ? "tel" : "text"}
                  inputMode={format === "phone" || format === "cep" || format === "cpf" ? "numeric" : undefined}
                  maxLength={format === "uf" ? 2 : undefined}
                  value={format === "date" ? dateInputValue : displayValue}
                  onChange={(event) =>
                    setValue(field.name, parseFieldInput(field, event.target.value))
                  }
                  onFocus={() => setActiveField(field.name)}
                  placeholder={
                    hasCurrent
                      ? "Sem valor extraído — digite para substituir"
                      : "Não identificado — preencha manualmente"
                  }
                  aria-label={`Valor de ${field.label || field.name}`}
                  className={cn(
                    "h-9 text-sm",
                    notIdentified && "border-dashed",
                    invalid && "border-status-critical",
                  )}
                />
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
                  title={displayCurrent}
                >
                  <span className="truncate">{displayCurrent || "Não informado"}</span>
                </div>
              </div>
            </div>

            {invalid && (
              <p className="mt-1 text-caption font-medium text-status-critical">{invalidMessage}</p>
            )}
            {!invalid && field.validation_status === "invalid" && (
              <p className="mt-1 text-caption font-medium text-status-warning">
                A validação original deste campo falhou. Revise antes de aplicar.
              </p>
            )}

            {field.evidence && (
              <p className="mt-1 line-clamp-2 text-caption italic text-muted-foreground">
                “{field.evidence}”
              </p>
            )}

            <div className="mt-2 flex flex-wrap items-center gap-1.5">
              <label className="flex items-center gap-1.5 text-micro font-semibold text-muted-foreground">
                <span className="sr-only">Decisão para {field.label || field.name}</span>
                <select
                  value={row.decision}
                  onChange={(event) => handleDecision(field, event.target.value as Decision)}
                  onFocus={() => setActiveField(field.name)}
                  className="h-8 rounded-md border border-border bg-card px-2 text-caption font-semibold text-foreground focus:outline-none focus:ring-2 focus:ring-primary/30"
                  aria-label={`Decisão de ${field.label || field.name}`}
                >
                  <option value="pending" disabled>
                    Pendente — escolha
                  </option>
                  <option value="apply" disabled={!hasValue || invalid}>
                    {hasExtracted ? "Aplicar OCR" : "Aplicar valor"}
                  </option>
                  <option value="keep" disabled={!hasCurrent}>
                    Manter atual
                  </option>
                  <option value="ignore">Ignorar</option>
                </select>
              </label>
              {hasExtracted && value !== field.value && (
                <button
                  type="button"
                  onClick={() => applyExtractedValue(field)}
                  className="rounded-md px-1.5 py-1 text-micro font-semibold text-primary hover:bg-primary/5 hover:underline"
                >
                  Restaurar OCR
                </button>
              )}
              {!hasValue && !hasCurrent && (
                <span className="text-micro text-muted-foreground">
                  Digite acima para habilitar “Aplicar valor”.
                </span>
              )}
            </div>
          </div>
        </div>
      </article>
    );
  };

  const renderGroup = (
    key: Exclude<ReviewFilter, "all" | "high">,
    title: string,
    items: ExtractedFieldDTO[],
    emptyText: string,
    icon?: ReactNode,
  ) => {
    if (items.length === 0 && filter !== key) return null;
    return (
      <section>
        <div className="mb-2 flex items-center gap-1.5">
          {icon}
          <h3 className="text-caption font-bold uppercase tracking-wide text-foreground">{title}</h3>
          <span className="text-micro text-muted-foreground">{items.length}</span>
          <span className="h-px flex-1 bg-border" />
        </div>
        {items.length === 0 ? (
          <p className="rounded-lg border border-dashed border-border px-3 py-2 text-xs text-muted-foreground">
            {emptyText}
          </p>
        ) : (
          <div className="space-y-2">{items.map((field) => renderField(field))}</div>
        )}
      </section>
    );
  };

  const filters: Array<{ key: ReviewFilter; label: string; count: number }> = [
    { key: "all", label: "Todos", count: fields.length },
    { key: "divergences", label: "Divergências", count: divergences.length },
    { key: "low", label: "Baixa confiança", count: low.length },
    { key: "high", label: "Alta confiança", count: high.length },
    { key: "missing", label: "Não identificados", count: missing.length },
  ];

  const showHigh = filter === "high" || highExpanded;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        showCloseButton={false}
        className="flex h-[min(94vh,920px)] w-[min(1280px,96vw)] max-w-none flex-col gap-0 overflow-hidden p-0 sm:max-w-none"
      >
        <header className="flex flex-none items-start justify-between gap-3 border-b border-border bg-card px-4 py-3 sm:px-5">
          <div className="flex min-w-0 items-start gap-2.5">
            <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
              <Sparkles className="h-4 w-4" />
            </div>
            <div className="min-w-0">
              <DialogTitle className="text-base font-bold text-foreground">Revisão de dados</DialogTitle>
              <DialogDescription className="mt-0.5 truncate text-xs text-muted-foreground">
                {document?.name || "Documento"} · {party?.name || "Parte não identificada"}
              </DialogDescription>
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-1.5">
            {extractionFailed ? (
              <span className="hidden rounded-full bg-status-critical-soft px-2 py-1 text-micro font-bold text-status-critical sm:inline">
                Falha de OCR
              </span>
            ) : noExtraction ? (
              <span className="hidden rounded-full bg-status-warning-soft px-2 py-1 text-micro font-bold text-status-warning sm:inline">
                Extração sem valores
              </span>
            ) : (
              <span className="hidden rounded-full bg-status-ok-soft px-2 py-1 text-micro font-bold text-status-ok sm:inline">
                OCR concluído
              </span>
            )}
            <span className="hidden rounded-full border border-border bg-card px-2 py-1 text-micro font-bold text-muted-foreground sm:inline">
              {overall}% confiança
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

        <div className="flex flex-none gap-1 border-b border-border bg-card px-3 py-2 lg:hidden">
          <button
            type="button"
            onClick={() => setMobileView("document")}
            className={cn(
              "rounded-md px-3 py-1 text-caption font-semibold transition-colors",
              mobileView === "document"
                ? "bg-primary/10 text-primary"
                : "text-muted-foreground hover:bg-muted",
            )}
          >
            Documento
          </button>
          <button
            type="button"
            onClick={() => setMobileView("fields")}
            className={cn(
              "rounded-md px-3 py-1 text-caption font-semibold transition-colors",
              mobileView === "fields"
                ? "bg-primary/10 text-primary"
                : "text-muted-foreground hover:bg-muted",
            )}
          >
            Campos
          </button>
        </div>

        <div className="grid min-h-0 flex-1 grid-cols-1 lg:grid-cols-[minmax(0,42%)_minmax(0,58%)]">
          <aside
            className={cn(
              "min-h-0 flex-col border-r border-border bg-muted/20 lg:flex",
              mobileView === "document" ? "flex" : "hidden",
            )}
          >
            <div className="min-h-0 flex-1">
              <DocumentInlinePreview document={document} />
            </div>
            <div className="flex-none border-t border-border bg-card p-3">
              <div className="flex items-center gap-1.5 text-micro font-bold uppercase tracking-wide text-muted-foreground">
                <Sparkles className="h-3 w-3" /> Evidência do campo
              </div>
              {active ? (
                <>
                  <p className="mt-1 line-clamp-2 text-xs italic text-foreground">
                    “{active.evidence || "Sem trecho de evidência para este campo."}”
                  </p>
                  <p className="mt-1 text-micro text-muted-foreground">
                    {active.label || active.name} · {pct(active.confidence)}
                  </p>
                </>
              ) : (
                <p className="mt-1 text-xs text-muted-foreground">Nenhum campo para revisar.</p>
              )}
            </div>
          </aside>

          <section
            className={cn(
              "flex min-h-0 flex-col",
              mobileView === "fields" ? "flex" : "hidden lg:flex",
            )}
          >
            <div className="flex-none border-b border-border bg-card px-4 py-3">
              <div className="flex items-start justify-between gap-2">
                <div className="min-w-0">
                  <h2 className="text-sm font-bold text-foreground">Campos para revisar</h2>
                  <p className="mt-0.5 text-caption text-muted-foreground">
                    {fields.length} campos · {divergences.length} divergências · {low.length} baixa · {missing.length} não identificados
                  </p>
                </div>
                <div className="shrink-0 text-right">
                  <div className="text-lg font-bold leading-none text-primary">{overall}%</div>
                  <div className="mt-0.5 text-micro uppercase tracking-wide text-muted-foreground">confiança</div>
                </div>
              </div>

              <div className="mt-2 flex h-1.5 overflow-hidden rounded-full bg-muted">
                <span className="bg-status-warning" style={{ width: segWidth(divergences.length) }} />
                <span className="bg-status-critical" style={{ width: segWidth(low.length) }} />
                <span className="bg-status-ok" style={{ width: segWidth(high.length) }} />
                <span className="bg-border" style={{ width: segWidth(missing.length) }} />
              </div>

              <div className="mt-2 flex gap-1 overflow-x-auto pb-0.5">
                {filters.map((item) => (
                  <button
                    key={item.key}
                    type="button"
                    onClick={() => {
                      setFilter(item.key);
                      if (item.key === "missing") setMissingExpanded(true);
                    }}
                    className={cn(
                      "shrink-0 rounded-md border px-2 py-1 text-micro font-semibold transition-colors",
                      filter === item.key
                        ? "border-primary/20 bg-primary/10 text-primary"
                        : "border-transparent text-muted-foreground hover:bg-muted",
                    )}
                    aria-pressed={filter === item.key}
                  >
                    {item.label} <span className="opacity-60">{item.count}</span>
                  </button>
                ))}
              </div>
            </div>

            <div className="min-h-0 flex-1 overflow-y-auto px-4 py-3">
              {!party && (
                <div className="mb-3 flex items-start gap-2 rounded-lg border border-status-critical/30 bg-status-critical-soft/40 p-2.5 text-caption text-status-critical">
                  <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                  Parte não identificada. Não é possível aplicar os campos; feche e vincule a parte antes de revisar.
                </div>
              )}

              {extractionFailed && (
                <div className="mb-3 rounded-xl border border-status-critical/30 bg-status-critical-soft/40 p-3">
                  <div className="flex flex-wrap items-start gap-2">
                    <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-status-critical" />
                    <div className="min-w-0 flex-1">
                      <p className="text-xs font-bold text-status-critical">
                        Não foi possível ler este documento.
                      </p>
                      <p className="mt-0.5 text-caption text-muted-foreground">{errorInfo.message}</p>
                      {errorInfo.detail && (
                        <details className="mt-1">
                          <summary className="cursor-pointer select-none text-micro text-muted-foreground hover:text-foreground">
                            Detalhes técnicos
                          </summary>
                          <pre className="mt-1 max-h-32 overflow-auto whitespace-pre-wrap break-words rounded-md bg-card/60 p-2 text-micro text-muted-foreground">
                            {errorInfo.detail}
                          </pre>
                        </details>
                      )}
                      {reprocessError && (
                        <p className="mt-1 text-caption font-medium text-status-critical">{reprocessError}</p>
                      )}
                    </div>
                    {onReprocess && (
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        className="h-8 gap-1.5 border-status-critical/40 bg-card text-caption text-status-critical hover:bg-status-critical-soft/60"
                        onClick={() => void reprocess()}
                        disabled={reprocessing}
                      >
                        <RefreshCw className={cn("h-3.5 w-3.5", reprocessing && "animate-spin")} />
                        {reprocessing ? "Reenfileirando..." : "Reprocessar IA"}
                      </Button>
                    )}
                  </div>
                </div>
              )}

              {noExtraction && !extractionFailed && (
                <div className="mb-3 rounded-xl border border-status-warning/30 bg-status-warning-soft/40 p-3">
                  <div className="flex flex-wrap items-start gap-2">
                    <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-status-warning" />
                    <div className="min-w-0 flex-1">
                      <p className="text-xs font-bold text-status-warning">
                        A IA não identificou valores neste documento.
                      </p>
                      <p className="mt-0.5 text-caption text-muted-foreground">
                        Preencha os campos manualmente ou reprocesse a extração. Os valores atuais da
                        parte continuam disponíveis em “Manter atual”.
                      </p>
                      {reprocessError && (
                        <p className="mt-1 text-caption font-medium text-status-critical">{reprocessError}</p>
                      )}
                    </div>
                    {onReprocess && (
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        className="h-8 gap-1.5 border-status-warning/40 bg-card text-caption text-status-warning hover:bg-status-warning-soft/60"
                        onClick={() => void reprocess()}
                        disabled={reprocessing}
                      >
                        <RefreshCw className={cn("h-3.5 w-3.5", reprocessing && "animate-spin")} />
                        {reprocessing ? "Reenfileirando..." : "Reprocessar IA"}
                      </Button>
                    )}
                  </div>
                </div>
              )}

              {fields.length === 0 ? (
                <p className="rounded-lg border border-dashed border-border px-3 py-6 text-center text-xs text-muted-foreground">
                  Nenhum campo foi identificado neste documento.
                </p>
              ) : (
                <div className="space-y-4">
                  {renderGroup(
                    "divergences",
                    "Divergências",
                    divergences,
                    "Nenhuma divergência entre OCR e o valor atual.",
                    <AlertTriangle className="h-3.5 w-3.5 text-status-warning" />,
                  )}
                  {renderGroup("low", "Baixa confiança", low, "Nenhum campo de baixa confiança.")}
                  {(high.length > 0 || filter === "high") && (
                    <section>
                      <div className="mb-2 flex items-center gap-1.5">
                        <CheckCircle2 className="h-3.5 w-3.5 text-status-ok" />
                        <h3 className="text-caption font-bold uppercase tracking-wide text-foreground">Alta confiança</h3>
                        <span className="text-micro text-muted-foreground">{high.length}</span>
                        <span className="h-px flex-1 bg-border" />
                        <button
                          type="button"
                          onClick={() => setHighExpanded((value) => !value)}
                          className="flex items-center gap-0.5 text-micro font-semibold text-muted-foreground hover:text-foreground"
                          aria-expanded={showHigh}
                        >
                          {showHigh ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
                          {showHigh ? "Recolher" : "Ver campos"}
                        </button>
                        <button
                          type="button"
                          onClick={acceptHighConfidence}
                          className="rounded-md border border-border px-2 py-1 text-micro font-semibold text-muted-foreground hover:bg-muted hover:text-foreground"
                        >
                          Aceitar todos
                        </button>
                      </div>
                      {high.length === 0 ? (
                        <p className="rounded-lg border border-dashed border-border px-3 py-2 text-xs text-muted-foreground">
                          Nenhum campo de alta confiança.
                        </p>
                      ) : showHigh ? (
                        <div className="space-y-2">{high.map((field) => renderField(field))}</div>
                      ) : (
                        <p className="rounded-lg border border-border bg-card px-3 py-2 text-caption text-muted-foreground">
                          {high.length} campos sem divergência e com validação válida. Todos já vêm selecionados; use “Ver campos” para conferir.
                        </p>
                      )}
                    </section>
                  )}

                  {(missing.length > 0 || filter === "missing") && (
                    <section>
                      <div className="mb-2 flex flex-wrap items-center gap-1.5">
                        <h3 className="text-caption font-bold uppercase tracking-wide text-foreground">
                          Não identificados
                        </h3>
                        <span className="text-micro text-muted-foreground">{missing.length}</span>
                        <span className="h-px flex-1 bg-border" />
                        {missingPending > 0 && (
                          <button
                            type="button"
                            onClick={ignoreMissing}
                            className="rounded-md border border-border px-2 py-1 text-micro font-semibold text-muted-foreground hover:bg-muted hover:text-foreground"
                          >
                            Ignorar pendentes ({missingPending})
                          </button>
                        )}
                        <button
                          type="button"
                          onClick={() => setMissingExpanded((value) => !value)}
                          className="flex items-center gap-0.5 text-micro font-semibold text-muted-foreground hover:text-foreground"
                          aria-expanded={missingExpanded || filter === "missing"}
                        >
                          {missingExpanded || filter === "missing" ? (
                            <ChevronDown className="h-3 w-3" />
                          ) : (
                            <ChevronRight className="h-3 w-3" />
                          )}
                          {missingExpanded || filter === "missing" ? "Recolher" : "Revisar"}
                        </button>
                      </div>
                      {missing.length === 0 ? (
                        <p className="rounded-lg border border-dashed border-border px-3 py-2 text-xs text-muted-foreground">
                          Nenhum campo ausente no documento.
                        </p>
                      ) : missingExpanded || filter === "missing" ? (
                        <div className="space-y-2">{missing.map((field) => renderField(field))}</div>
                      ) : (
                        <p className="rounded-lg border border-border bg-card px-3 py-2 text-caption text-muted-foreground">
                          {missing.length} campos sem valor extraído. Os que já têm valor atual ficam em
                          “Manter atual”; abra para preencher os demais manualmente.
                        </p>
                      )}
                    </section>
                  )}
                </div>
              )}
            </div>

            {showCorrection && onRequestCorrection && (
              <div className="flex-none border-t border-border bg-status-warning-soft/40 px-4 py-3">
                <p className="text-caption font-bold text-status-warning">Motivo da correção</p>
                <div className="mt-1 flex flex-col gap-2 sm:flex-row">
                  <Input
                    value={correctionReason}
                    onChange={(event) => setCorrectionReason(event.target.value)}
                    placeholder="Ex.: documento ilegível, CPF divergente, foto faltando..."
                    className="h-8"
                  />
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => setShowCorrection(false)}
                    disabled={correctionSending}
                  >
                    Cancelar
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    disabled={!correctionReason.trim() || correctionSending}
                    onClick={() => void requestCorrection()}
                  >
                    {correctionSending ? "Enviando..." : "Enviar correção"}
                  </Button>
                </div>
              </div>
            )}

            <footer className="flex flex-none flex-wrap items-center justify-between gap-2 border-t border-border bg-card px-4 py-3">
              {applied ? (
                <>
                  <div className="min-w-0">
                    <p className="text-xs font-bold text-status-ok">Campos aplicados à parte.</p>
                    <p className="text-micro text-muted-foreground">
                      Confira o documento e conclua a revisão.
                    </p>
                    {saveError && <p className="text-micro text-status-critical">{saveError}</p>}
                  </div>
                  <div className="flex shrink-0 flex-wrap justify-end gap-2">
                    {onRequestCorrection && (
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        onClick={() => setShowCorrection((value) => !value)}
                        disabled={approving || correctionSending}
                      >
                        Solicitar correção
                      </Button>
                    )}
                    {onApprove && item && (
                      <Button
                        type="button"
                        size="sm"
                        onClick={() => void approve()}
                        disabled={approving}
                      >
                        {approving ? "Aprovando..." : "Aprovar documento"}
                      </Button>
                    )}
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={() => onOpenChange(false)}
                      disabled={approving}
                    >
                      Fechar
                    </Button>
                  </div>
                </>
              ) : (
                <>
                  <div className="min-w-0">
                    <p className="text-xs text-muted-foreground">
                      <b className="text-foreground">{selected.length}</b> de {fields.length} campos para aplicar
                    </p>
                    {pending > 0 && (
                      <p className="text-micro text-status-warning">{pending} campo(s) ainda sem decisão</p>
                    )}
                    {missing.length > 0 && pending === 0 && selected.length === 0 && (
                      <p className="text-micro text-muted-foreground">
                        Nenhum valor para aplicar. Preencha manualmente ou ignore os não identificados.
                      </p>
                    )}
                    {selectedInvalid.length > 0 && (
                      <p className="text-micro text-status-critical">
                        {selectedInvalid.length} inválido(s) não serão aplicados
                      </p>
                    )}
                    {saveError && <p className="text-micro text-status-critical">{saveError}</p>}
                  </div>
                  <div className="flex shrink-0 flex-wrap justify-end gap-2">
                    {onRequestCorrection && (
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        onClick={() => setShowCorrection((value) => !value)}
                      >
                        Solicitar correção
                      </Button>
                    )}
                    <Button type="button" variant="outline" size="sm" onClick={() => onOpenChange(false)} disabled={saving}>
                      Cancelar
                    </Button>
                    <Button type="button" size="sm" disabled={!canApply} onClick={() => void apply()}>
                      {saving ? "Aplicando..." : `Aplicar ${selected.length} ${selected.length === 1 ? "campo" : "campos"}`}
                    </Button>
                  </div>
                </>
              )}
            </footer>
          </section>
        </div>
      </DialogContent>
    </Dialog>
  );
}
