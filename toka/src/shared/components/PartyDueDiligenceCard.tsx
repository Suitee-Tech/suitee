import { useEffect, useRef, useState, type ComponentType, type ReactNode, type SVGProps } from "react";
import {
  BadgeCheck,
  Banknote,
  ChevronDown,
  FileCheck2,
  FileText,
  Gauge,
  Loader2,
  Paperclip,
  Pencil,
  Save,
  Scale,
  Shield,
  Trash2,
  XCircle,
} from "lucide-react";
import type {
  DueDiligenceCriterionReview,
  DueDiligenceCriterionStatus,
  PartyDTO,
  UpdateDueDiligenceDTO,
} from "../services/deal.service";
import type { AnalysisCriterion, DocumentDTO } from "../services/document.service";
import { ANALYSIS_CRITERIA, ANALYSIS_CRITERION_LABELS } from "../documents/types";
import { useRequestScopeGuard } from "../hooks/useRequestScopeGuard";
import { notifyApiError } from "../services/errorToast";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { Skeleton } from "../ui/skeleton";
import { Textarea } from "../ui/textarea";
import { cn } from "../ui/utils";

type IconComponent = ComponentType<SVGProps<SVGSVGElement>>;

const STATUS_META = {
  APPROVED: {
    label: "Aprovado",
    icon: BadgeCheck,
    className: "bg-status-ok-soft text-status-ok border-status-ok/20",
  },
  REJECTED: {
    label: "Reprovado",
    icon: XCircle,
    className: "bg-status-critical-soft text-status-critical border-status-critical/20",
  },
  PENDING: {
    label: "Pendente",
    icon: Shield,
    className: "bg-status-pending-soft text-status-pending border-status-pending/20",
  },
} as const;

const CRITERION_REVIEW_META: Record<
  DueDiligenceCriterionStatus,
  { label: string; className: string }
> = {
  pending: {
    label: "Pendente",
    className: "bg-status-pending-soft text-status-pending border-status-pending/20",
  },
  in_review: {
    label: "Em análise",
    className: "bg-status-info-soft text-status-info border-status-info/20",
  },
  approved: {
    label: "Aprovado",
    className: "bg-status-ok-soft text-status-ok border-status-ok/20",
  },
  rejected: {
    label: "Reprovado",
    className: "bg-status-critical-soft text-status-critical border-status-critical/20",
  },
  waived: {
    label: "Dispensado",
    className: "bg-muted text-muted-foreground border-border",
  },
};
export { ANALYSIS_CRITERIA, ANALYSIS_CRITERION_LABELS } from "../documents/types";

const CRITERION_META: Record<
  AnalysisCriterion,
  { label: string; description: string; emptyLabel: string; icon: IconComponent }
> = {
  score: {
    label: ANALYSIS_CRITERION_LABELS.score,
    description: "Consulta de crédito e risco financeiro",
    emptyLabel: "Score ainda não informado",
    icon: Gauge,
  },
  cnd: {
    label: ANALYSIS_CRITERION_LABELS.cnd,
    description: "Certidões e situação de regularidade",
    emptyLabel: "Situação da CND não informada",
    icon: FileCheck2,
  },
  legal_opinion: {
    label: ANALYSIS_CRITERION_LABELS.legal_opinion,
    description: "Conclusão e respaldo jurídico",
    emptyLabel: "Parecer ainda não informado",
    icon: Scale,
  },
  income: {
    label: ANALYSIS_CRITERION_LABELS.income,
    description: "Comprovantes de renda e capacidade financeira",
    emptyLabel: "Renda ainda não informada",
    icon: Banknote,
  },
};

export type AnalysisEvidenceByCriterion = Partial<Record<AnalysisCriterion, DocumentDTO[]>>;

const MIN_MONTHLY_INCOME = 1;
const MAX_MONTHLY_INCOME = 1_000_000;

function formatBRLInput(digits: string): string {
  if (!digits) return "";
  const value = Number(digits);
  if (!Number.isFinite(value)) return "";
  return value.toLocaleString("pt-BR", {
    style: "currency",
    currency: "BRL",
    maximumFractionDigits: 0,
  });
}

function parseBRLInput(digits: string): number | undefined {
  if (!digits) return undefined;
  const value = Number(digits);
  return Number.isSafeInteger(value) && value > 0 ? value : undefined;
}

function formatFileSize(bytes?: number): string {
  if (!bytes || bytes <= 0) return "";
  const units = ["B", "KB", "MB", "GB"];
  let value = bytes;
  let index = 0;
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024;
    index += 1;
  }
  return `${value.toFixed(index === 0 || value >= 10 ? 0 : 1)} ${units[index]}`;
}

function formatUploadDate(value?: string): string {
  if (!value) return "";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "" : date.toLocaleDateString("pt-BR");
}

type PartyDueDiligenceCardProps = {
  dealId: string;
  party: PartyDTO;
  onUpdatePartyDueDiligence: (dealId: string, partyId: string, data: UpdateDueDiligenceDTO) => Promise<unknown>;
  evidenceByCriterion?: AnalysisEvidenceByCriterion;
  evidenceLoading?: boolean;
  evidenceUnavailable?: boolean;
  uploadingCriterion?: AnalysisCriterion | null;
  deletingDocumentId?: string | null;
  onUploadEvidence?: (criterion: AnalysisCriterion, file: File) => Promise<void>;
  onOpenEvidence?: (document: DocumentDTO) => void;
  onRemoveEvidence?: (document: DocumentDTO) => void;
  onUpdateCriterion?: (
    criterion: AnalysisCriterion,
    status: DueDiligenceCriterionStatus,
  ) => Promise<void>;
  defaultOpen?: boolean;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
};

export function PartyDueDiligenceCard({
  dealId,
  party,
  onUpdatePartyDueDiligence,
  evidenceByCriterion = {},
  evidenceLoading = false,
  evidenceUnavailable = false,
  uploadingCriterion = null,
  deletingDocumentId = null,
  onUploadEvidence,
  onOpenEvidence,
  onRemoveEvidence,
  onUpdateCriterion,
  defaultOpen = true,
  open: controlledOpen,
  onOpenChange,
}: PartyDueDiligenceCardProps) {
  const [loading, setLoading] = useState(false);
  const [editing, setEditing] = useState(false);
  const [reviewingCriterion, setReviewingCriterion] = useState<AnalysisCriterion | null>(null);
  const [internalOpen, setInternalOpen] = useState(defaultOpen);
  const isOpen = controlledOpen ?? internalOpen;
  const setOpen = (value: boolean) => {
    onOpenChange?.(value);
    if (controlledOpen === undefined) setInternalOpen(value);
  };
  const saveRequestPending = useRef(false);
  const beginScopedRequest = useRequestScopeGuard(`${dealId}:${party.id}`);

  const dd = party.due_diligence;
  const criterionReviews = dd?.criteria ?? {};
  const normalizedStatus = (dd?.status ?? "PENDING").toUpperCase() as keyof typeof STATUS_META;
  const statusMeta = STATUS_META[normalizedStatus] ?? STATUS_META.PENDING;
  const StatusIcon = statusMeta.icon;

  const [scoreSerasa, setScoreSerasa] = useState<string>(dd?.score_serasa != null ? String(dd.score_serasa) : "");
  const [cndStatus, setCndStatus] = useState<string>(dd?.cnd_status ?? "");
  const [legalOpinion, setLegalOpinion] = useState<string>(dd?.legal_opinion ?? "");
  const [monthlyIncome, setMonthlyIncome] = useState<string>(
    dd?.monthly_income != null ? String(dd.monthly_income) : "",
  );
  const [status, setStatus] = useState<string>((dd?.status ?? "PENDING").toUpperCase());

  useEffect(() => {
    saveRequestPending.current = false;
    setLoading(false);
    setEditing(false);
  }, [dealId, party.id]);

  useEffect(() => {
    if (editing || loading) return;
    setScoreSerasa(dd?.score_serasa != null ? String(dd.score_serasa) : "");
    setCndStatus(dd?.cnd_status ?? "");
    setLegalOpinion(dd?.legal_opinion ?? "");
    setMonthlyIncome(dd?.monthly_income != null ? String(dd.monthly_income) : "");
    setStatus((dd?.status ?? "PENDING").toUpperCase());
  }, [dd?.cnd_status, dd?.legal_opinion, dd?.score_serasa, dd?.monthly_income, dd?.status, editing, loading]);

  const run = async (data: UpdateDueDiligenceDTO) => {
    if (saveRequestPending.current) return;
    const isCurrent = beginScopedRequest();
    saveRequestPending.current = true;
    setLoading(true);
    try {
      await onUpdatePartyDueDiligence(dealId, party.id, data);
      if (!isCurrent()) return;
      setEditing(false);
    } catch (error) {
      if (!isCurrent()) return;
      notifyApiError(error, { context: "Atualizar análise da parte" });
    } finally {
      if (isCurrent()) {
        saveRequestPending.current = false;
        setLoading(false);
      }
    }
  };

  const reviewCriterion = async (
    criterion: AnalysisCriterion,
    status: DueDiligenceCriterionStatus,
  ) => {
    if (!onUpdateCriterion) return;
    setReviewingCriterion(criterion);
    try {
      await onUpdateCriterion(criterion, status);
    } finally {
      setReviewingCriterion(null);
    }
  };

  const values: Record<AnalysisCriterion, string | number | undefined> = {
    score: dd?.score_serasa,
    cnd: dd?.cnd_status,
    legal_opinion: dd?.legal_opinion,
    income: dd?.monthly_income,
  };
  const incomeValue = parseBRLInput(monthlyIncome);
  const incomeError =
    monthlyIncome.trim() !== "" &&
    (incomeValue === undefined ||
      incomeValue < MIN_MONTHLY_INCOME ||
      incomeValue > MAX_MONTHLY_INCOME)
      ? "Informe um valor entre R$ 1 e R$ 1.000.000."
      : "";
  const filledCount = ANALYSIS_CRITERIA.filter((criterion) => {
    const value = values[criterion];
    return value !== undefined && value !== null && String(value).trim() !== "";
  }).length;
  const completedCount = ANALYSIS_CRITERIA.filter((criterion) => {
    const value = values[criterion];
    const hasValue = value !== undefined && value !== null && String(value).trim() !== "";
    return hasValue && (evidenceByCriterion[criterion]?.length ?? 0) > 0;
  }).length;

  const criterionEditors: Record<AnalysisCriterion, ReactNode> = {
    score: (
      <Input
        value={scoreSerasa}
        onChange={(event) => setScoreSerasa(event.target.value)}
        placeholder="Ex: 750"
        inputMode="numeric"
        aria-label="Score Serasa"
      />
    ),
    cnd: (
      <Input
        value={cndStatus}
        onChange={(event) => setCndStatus(event.target.value)}
        placeholder="Ex: NEGATIVA"
        aria-label="Situação da CND"
      />
    ),
    legal_opinion: (
      <Textarea
        value={legalOpinion}
        onChange={(event) => setLegalOpinion(event.target.value)}
        rows={3}
        placeholder="Descreva o parecer..."
        aria-label="Parecer jurídico"
      />
    ),
    income: (
      <div>
        <Input
          value={formatBRLInput(monthlyIncome)}
          onChange={(event) =>
            setMonthlyIncome(event.target.value.replace(/\D/g, "").slice(0, 9))
          }
          placeholder="R$ 0"
          inputMode="numeric"
          aria-label="Renda mensal (R$)"
          aria-invalid={Boolean(incomeError)}
          aria-describedby={incomeError ? `due-diligence-income-${party.id}-error` : undefined}
        />
        {incomeError && (
          <p
            id={`due-diligence-income-${party.id}-error`}
            className="mt-1 text-micro text-status-critical"
          >
            {incomeError}
          </p>
        )}
      </div>
    ),
  };

  return (
    <article className="overflow-hidden rounded-2xl border border-border bg-card shadow-sm">
      <header className="flex flex-col gap-3 p-4 sm:flex-row sm:items-start sm:justify-between sm:p-5">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h5 className="min-w-0 break-words text-base font-bold leading-tight text-foreground">
              {party.name || party.email || party.id}
            </h5>
            {party.is_primary && (
              <span className="rounded-full border border-border bg-muted px-2 py-0.5 text-micro font-bold text-muted-foreground">
                Parte principal
              </span>
            )}
          </div>
          {party.email && <p className="mt-1 break-all text-caption text-muted-foreground">{party.email}</p>}
          <span className={`mt-2 inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs font-bold ${statusMeta.className}`}>
            <StatusIcon className="h-3.5 w-3.5" aria-hidden="true" />
            {statusMeta.label}
          </span>

          <div className="mt-2 flex flex-wrap items-center gap-1.5">
            <span className="rounded-full bg-muted px-2 py-0.5 text-micro font-bold text-muted-foreground">
              {filledCount}/{ANALYSIS_CRITERIA.length} valores · {completedCount}/{ANALYSIS_CRITERIA.length} completos
            </span>
            {ANALYSIS_CRITERIA.map((criterion) => {
              const value = values[criterion];
              const hasValue =
                value !== undefined && value !== null && String(value).trim() !== "";
              const documents = evidenceByCriterion[criterion]?.length ?? 0;
              return (
                <span
                  key={criterion}
                  className="inline-flex items-center gap-1 rounded-full border border-border bg-card px-2 py-0.5 text-micro font-semibold text-muted-foreground"
                >
                  {ANALYSIS_CRITERION_LABELS[criterion]}
                  <span className={hasValue ? "text-status-ok" : "text-muted-foreground"} aria-hidden="true">
                    {hasValue ? "✓" : "—"}
                  </span>
                  {documents > 0 && (
                    <span className="tabular-nums text-primary">· {documents}</span>
                  )}
                </span>
              );
            })}
          </div>
        </div>

        <div className="flex w-full shrink-0 flex-wrap items-center justify-end gap-2 sm:w-auto">
          <Button
            size="sm"
            variant="outline"
            onClick={() => {
              if (isOpen) setEditing(false);
              setOpen(!isOpen);
            }}
            className="w-full gap-2 sm:w-auto"
            aria-expanded={isOpen}
          >
            <ChevronDown className={cn("h-4 w-4 transition-transform", isOpen && "rotate-180")} />
            {isOpen ? "Recolher análise" : "Abrir análise"}
          </Button>
          {isOpen && (
            <Button
              size="sm"
              variant="outline"
              onClick={() => setEditing((value) => !value)}
              disabled={loading}
              className="w-full gap-2 sm:w-auto"
              aria-expanded={editing}
            >
              <Pencil className="h-4 w-4" />
              {editing ? "Concluir edição" : "Editar análise"}
            </Button>
          )}
        </div>
      </header>

      {isOpen && (
        <>
          <div className="grid grid-cols-1 gap-3 border-t border-border bg-muted/20 p-4 sm:grid-cols-2 sm:p-5">
        {ANALYSIS_CRITERIA.map((criterion) => (
          <CriterionEvidenceCard
            key={criterion}
            criterion={criterion}
            value={values[criterion]}
            documents={evidenceByCriterion[criterion] ?? []}
            loading={evidenceLoading}
            unavailable={evidenceUnavailable}
            uploading={uploadingCriterion === criterion}
            deletingDocumentId={deletingDocumentId}
            review={criterionReviews[criterion]}
            onReview={onUpdateCriterion ? reviewCriterion : undefined}
            reviewing={reviewingCriterion === criterion}
            editor={editing ? criterionEditors[criterion] : undefined}
            onUpload={onUploadEvidence}
            onOpen={onOpenEvidence}
            onRemove={onRemoveEvidence}
          />
        ))}
      </div>

      {editing && (
        <section className="space-y-3 border-t border-border p-4 sm:p-5" aria-label="Status geral da análise da parte">
          <p className="text-caption text-muted-foreground">
            Os valores e comprovantes ficam em cada critério acima.
          </p>
          <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
            <label className="space-y-1 sm:max-w-xs">
              <span className="text-xs font-semibold text-muted-foreground">Status geral da parte</span>
              <select
                value={status}
                onChange={(event) => setStatus(event.target.value)}
                className="h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary"
              >
                <option value="PENDING">Pendente</option>
                <option value="APPROVED">Aprovado</option>
                <option value="REJECTED">Reprovado</option>
              </select>
            </label>
            <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
              <Button
                variant="outline"
                onClick={() => setEditing(false)}
                disabled={loading}
                className="w-full sm:w-auto"
              >
                Cancelar
              </Button>
              <Button
                onClick={() => {
                  if (incomeError) return;
                  const score = scoreSerasa.trim() ? Number(scoreSerasa.trim()) : undefined;
                  void run({
                    score_serasa: Number.isFinite(score) ? score : undefined,
                    cnd_status: cndStatus.trim() || undefined,
                    legal_opinion: legalOpinion.trim() || undefined,
                    monthly_income: incomeValue,
                    status,
                  });
                }}
                loading={loading}
                loadingText="Salvando…"
                disabled={Boolean(incomeError)}
                className="w-full gap-2 sm:w-auto"
              >
                <Save className="h-4 w-4" />
                Salvar análise
              </Button>
            </div>
          </div>
        </section>
      )}
        </>
      )}
    </article>
  );
}

function CriterionEvidenceCard({
  criterion,
  value,
  documents,
  loading,
  unavailable,
  uploading,
  deletingDocumentId,
  onUpload,
  onOpen,
  onRemove,
  review,
  onReview,
  reviewing,
  editor,
}: {
  criterion: AnalysisCriterion;
  value?: string | number;
  documents: DocumentDTO[];
  loading: boolean;
  unavailable: boolean;
  uploading: boolean;
  deletingDocumentId: string | null;
  onUpload?: (criterion: AnalysisCriterion, file: File) => Promise<void>;
  onOpen?: (document: DocumentDTO) => void;
  onRemove?: (document: DocumentDTO) => void;
  review?: DueDiligenceCriterionReview;
  onReview?: (
    criterion: AnalysisCriterion,
    status: DueDiligenceCriterionStatus,
  ) => Promise<void>;
  reviewing: boolean;
  editor?: ReactNode;
}) {
  const inputRef = useRef<HTMLInputElement>(null);
  const meta = CRITERION_META[criterion];
  const CriterionIcon = meta.icon;
  const hasValue = value !== undefined && value !== null && String(value).trim() !== "";
  const displayValue =
    criterion === "income" && typeof value === "number"
      ? value.toLocaleString("pt-BR", { style: "currency", currency: "BRL" })
      : String(value);
  const uploadNoun = "comprovante";
  const reviewMeta = review ? CRITERION_REVIEW_META[review.status] : null;

  return (
    <section className="flex min-w-0 flex-col rounded-xl border border-border bg-card p-3.5" aria-label={`Critério ${meta.label}`}>
      <div className="flex items-start justify-between gap-2">
        <div className="flex min-w-0 items-start gap-2.5">
          <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
            <CriterionIcon className="h-4 w-4" aria-hidden="true" />
          </span>
          <div className="min-w-0">
            <h6 className="text-sm font-bold text-foreground">{meta.label}</h6>
            <p className="mt-0.5 text-micro leading-snug text-muted-foreground">{meta.description}</p>
          </div>
        </div>
        <span
          className={`shrink-0 rounded-full border px-2 py-0.5 text-micro font-bold ${
            reviewMeta
              ? reviewMeta.className
              : hasValue
                ? "border-status-ok/20 bg-status-ok-soft text-status-ok"
                : "border-border bg-muted text-muted-foreground"
          }`}
        >
          {reviewMeta?.label ?? (hasValue ? "Preenchido" : "Pendente")}
        </span>
      </div>

      {!editor && (
        <div className="mt-3 min-h-10 rounded-lg bg-muted/60 px-3 py-2">
          <p className={`break-words text-sm ${hasValue ? "font-semibold text-foreground" : "text-muted-foreground"}`}>
            {hasValue ? displayValue : meta.emptyLabel}
          </p>
        </div>
      )}

      {editor && <div className="mt-2">{editor}</div>}

      {onReview && (
        <div className="mt-2 flex items-center gap-2 text-micro">
          <span className="shrink-0 font-semibold text-muted-foreground">Status</span>
          <select
            value={review?.status ?? "pending"}
            onChange={(event) =>
              void onReview(criterion, event.target.value as DueDiligenceCriterionStatus)
            }
            disabled={reviewing}
            aria-label={`Status do critério ${meta.label}`}
            className="h-8 w-full rounded-md border border-input bg-background px-2 text-xs focus:outline-none focus:ring-2 focus:ring-primary"
          >
            {(Object.keys(CRITERION_REVIEW_META) as DueDiligenceCriterionStatus[]).map((status) => (
              <option key={status} value={status}>
                {CRITERION_REVIEW_META[status].label}
              </option>
            ))}
          </select>
          {reviewing && <Loader2 className="h-3.5 w-3.5 shrink-0 animate-spin" aria-hidden="true" />}
        </div>
      )}
      {review?.status === "approved" && documents.length === 0 && (
        <p className="mt-1 text-micro font-semibold text-status-warning">
          Aprovado sem comprovante. Anexe para completar.
        </p>
      )}
      {review?.reviewed_at && (
        <p className="mt-1 text-micro text-muted-foreground">
          Revisado em {formatUploadDate(review.reviewed_at)}
        </p>
      )}

      <div className="mt-3 flex-1 border-t border-border pt-3">
        <div className="mb-2 flex items-center justify-between gap-2">
          <span className="text-xs font-bold text-foreground">Comprovantes</span>
          <span className="text-micro tabular-nums text-muted-foreground" aria-live="polite">
            {documents.length} {documents.length === 1 ? "arquivo" : "arquivos"}
          </span>
        </div>

        {loading ? (
          <div className="space-y-2" aria-label="Carregando evidências">
            <Skeleton className="h-8 w-full" />
            <Skeleton className="h-8 w-4/5" />
          </div>
        ) : unavailable ? (
          <p className="rounded-lg border border-status-critical/20 bg-status-critical-soft px-3 py-2 text-xs text-status-critical">
            Não foi possível carregar os comprovantes.
          </p>
        ) : documents.length === 0 ? (
          <p className="rounded-lg border border-dashed border-border px-3 py-2 text-xs text-muted-foreground">
            {hasValue
              ? "Nenhum comprovante. Anexe o arquivo que comprova este critério."
              : "Comece informando o valor ou anexe um comprovante."}
          </p>
        ) : (
          <ul className="space-y-1.5">
            {documents.map((document) => (
              <li key={document.id} className="flex min-w-0 items-center gap-2 rounded-lg border border-border bg-muted/30 px-2.5 py-2">
                <FileText className="h-3.5 w-3.5 shrink-0 text-muted-foreground" aria-hidden="true" />
                <div className="min-w-0 flex-1">
                  <button
                    type="button"
                    onClick={() => onOpen?.(document)}
                    disabled={!onOpen}
                    className="block w-full truncate text-left text-xs font-semibold text-foreground hover:text-primary disabled:cursor-default disabled:hover:text-foreground"
                    title={document.name}
                  >
                    {document.name}
                  </button>
                  <p className="truncate text-micro text-muted-foreground">
                    {[
                      formatFileSize(document.file_size),
                      formatUploadDate(document.upload_date),
                    ]
                      .filter(Boolean)
                      .join(" · ")}
                  </p>
                </div>
                {onRemove && (
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className="h-7 w-7 text-muted-foreground hover:bg-status-critical-soft hover:text-status-critical"
                    onClick={() => onRemove(document)}
                    loading={deletingDocumentId === document.id}
                    aria-label={`Remover ${document.name} de ${meta.label}`}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                )}
              </li>
            ))}
          </ul>
        )}
      </div>

      {onUpload && (
        <div className="mt-3">
          <input
            ref={inputRef}
            type="file"
            accept=".pdf,.doc,.docx,.jpg,.jpeg,.png"
            className="hidden"
            onChange={(event) => {
              const file = event.target.files?.[0];
              event.target.value = "";
              if (file) void onUpload(criterion, file);
            }}
          />
          <Button
            type="button"
            size="sm"
            variant="outline"
            className="w-full gap-2 text-primary hover:bg-primary/10"
            onClick={() => inputRef.current?.click()}
            disabled={uploading}
            aria-label={`${documents.length > 0 ? "Adicionar" : "Anexar"} ${uploadNoun} de ${meta.label}`}
          >
            {uploading ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Paperclip className="h-3.5 w-3.5" />}
            {uploading ? "Enviando…" : documents.length > 0 ? `Adicionar ${uploadNoun}` : `Anexar ${uploadNoun}`}
          </Button>
          <p className="mt-1.5 text-center text-micro text-muted-foreground">PDF, DOC, DOCX, JPG ou PNG · até 25 MB</p>
        </div>
      )}
    </section>
  );
}
