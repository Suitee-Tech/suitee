import { useEffect, useMemo, useRef, useState } from "react";
import { AlertCircle, FileText, Loader2, Paperclip, RefreshCw, ShieldCheck, Trash2, Users } from "lucide-react";
import { toast } from "sonner";
import { ANALYSIS_CRITERIA, ANALYSIS_CRITERION_LABELS, PartyDueDiligenceCard, type AnalysisEvidenceByCriterion } from "../../../../shared/components/PartyDueDiligenceCard";
import { DocumentViewerDialog } from "../../../../shared/components/DocumentViewerDialog";
import { partyRoleLabel } from "../../../../shared/deals/partyLabels";
import { useRequestScopeGuard } from "../../../../shared/hooks/useRequestScopeGuard";
import { getCategoryMeta, getStatusMeta, normalizeBackendStatus } from "../../../../shared/lib/documents";
import type { DealDTO, DueDiligence, DueDiligenceCriterionStatus, UpdateDueDiligenceDTO } from "../../../../shared/services/deal.service";
import { documentService, type AnalysisCriterion, type DocumentDTO } from "../../../../shared/services/document.service";
import { Button } from "../../../../shared/ui/button";
import { StatusPill, type StatusTone } from "./PanelPrimitives";

const MAX_ANALYSIS_FILE_SIZE = 25 * 1024 * 1024;

const CRITERION_UPLOAD_META: Record<
  AnalysisCriterion,
  { label: string; category: "comprovante" | "fiscal" | "juridico" }
> = {
  score: { label: "Score", category: "comprovante" },
  cnd: { label: "CND", category: "fiscal" },
  legal_opinion: { label: "Parecer", category: "juridico" },
  income: { label: "Renda", category: "comprovante" },
};

function isScopedAnalysisDocument(document: DocumentDTO): boolean {
  return Boolean(
    document.analysis_party_id &&
      document.analysis_criterion &&
      ANALYSIS_CRITERIA.includes(document.analysis_criterion),
  );
}

function countFilledCriteria(dueDiligence?: DueDiligence): number {
  if (!dueDiligence) return 0;
  return [dueDiligence.score_serasa, dueDiligence.cnd_status, dueDiligence.legal_opinion, dueDiligence.monthly_income].filter(
    (value) => value !== undefined && value !== null && String(value).trim() !== "",
  ).length;
}

export function MultiPartyAnalysis({
  dealDetails,
  aggregateDueDiligence,
  onUpdatePartyDueDiligence,
  onUpdateCriterion,
}: {
  dealDetails: DealDTO;
  aggregateDueDiligence?: DueDiligence;
  onUpdatePartyDueDiligence: (
    dealId: string,
    partyId: string,
    data: UpdateDueDiligenceDTO,
  ) => Promise<unknown>;
  onUpdateCriterion: (
    partyId: string,
    criterion: AnalysisCriterion,
    status: DueDiligenceCriterionStatus,
  ) => Promise<void>;
}) {
  const relevantParties = useMemo(
    () => (dealDetails.parties ?? []).filter((party) => party.role !== "witness"),
    [dealDetails.parties],
  );
  const [documents, setDocuments] = useState<DocumentDTO[]>([]);
  const [documentsLoading, setDocumentsLoading] = useState(true);
  const [documentsError, setDocumentsError] = useState(false);
  const [showPending, setShowPending] = useState(false);
  const [openPartyId, setOpenPartyId] = useState<string | null>(null);
  const [uploadingKeys, setUploadingKeys] = useState<Set<string>>(() => new Set());
  const [deletingDocumentId, setDeletingDocumentId] = useState<string | null>(null);
  const [viewer, setViewer] = useState<{ url: string; title?: string } | null>(null);
  const uploadPending = useRef(new Set<string>());
  const beginScopedRequest = useRequestScopeGuard(dealDetails.id);

  const loadDocuments = async () => {
    const isCurrent = beginScopedRequest();
    setDocumentsLoading(true);
    setDocumentsError(false);
    try {
      const response = await documentService.getByEntity(dealDetails.id, "analysis");
      if (!isCurrent()) return;
      setDocuments(response?.data ?? []);
    } catch (error) {
      if (!isCurrent()) return;
      console.error("Não foi possível carregar os documentos da análise", error);
      setDocuments([]);
      setDocumentsError(true);
    } finally {
      if (isCurrent()) setDocumentsLoading(false);
    }
  };

  useEffect(() => {
    uploadPending.current.clear();
    setUploadingKeys(new Set());
    setDeletingDocumentId(null);
    setViewer(null);
    setShowPending(false);
    setOpenPartyId(null);
    void loadDocuments();
    // loadDocuments is scoped by dealDetails.id and guarded against stale responses.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [dealDetails.id]);

  const uploadDocument = async (
    file: File,
    scope?: { partyId: string; partyName: string; criterion: AnalysisCriterion },
  ) => {
    if (file.size > MAX_ANALYSIS_FILE_SIZE) {
      toast.error("O arquivo excede o limite de 25 MB.");
      return;
    }

    const key = scope ? `${scope.partyId}:${scope.criterion}` : "general";
    if (uploadPending.current.has(key)) return;

    const isCurrent = beginScopedRequest();
    uploadPending.current.add(key);
    setUploadingKeys((current) => new Set(current).add(key));
    try {
      const criterionMeta = scope ? CRITERION_UPLOAD_META[scope.criterion] : null;
      const created = await documentService.upload(file, {
        name: file.name,
        category: criterionMeta?.category ?? "comprovante",
        entity_id: dealDetails.id,
        entity_type: "analysis",
        analysis_party_id: scope?.partyId,
        analysis_criterion: scope?.criterion,
        source: "toca-deal",
        notes: scope
          ? `Evidência de ${criterionMeta?.label} da parte ${scope.partyName}`
          : "Documento geral da análise de Due Diligence",
      });
      if (!isCurrent()) return;
      if (created?.id) {
        setDocuments((current) => [created, ...current.filter((document) => document.id !== created.id)]);
      } else {
        await loadDocuments();
      }
      setDocumentsError(false);
      toast.success(scope ? `Evidência de ${criterionMeta?.label} anexada.` : "Documento geral anexado à análise.");
    } catch (error) {
      if (!isCurrent()) return;
      console.error("Não foi possível anexar o documento da análise", error);
      toast.error(scope ? "Não foi possível anexar a evidência." : "Não foi possível anexar o documento geral.");
    } finally {
      uploadPending.current.delete(key);
      if (isCurrent()) {
        setUploadingKeys((current) => {
          const next = new Set(current);
          next.delete(key);
          return next;
        });
      }
    }
  };

  const openDocument = async (document: DocumentDTO) => {
    const isCurrent = beginScopedRequest();
    try {
      const url = await documentService.getPresignedDownloadUrl(document.id);
      if (!isCurrent()) return;
      if (!url) {
        toast.error("Arquivo ainda não disponível.");
        return;
      }
      setViewer({ url, title: document.name });
    } catch {
      if (isCurrent()) toast.error("Não foi possível abrir o comprovante.");
    }
  };

  const removeDocument = async (document: DocumentDTO) => {
    if (deletingDocumentId) return;
    const isCurrent = beginScopedRequest();
    setDeletingDocumentId(document.id);
    try {
      await documentService.delete(document.id);
      if (!isCurrent()) return;
      setDocuments((current) => current.filter((item) => item.id !== document.id));
      toast.success("Comprovante removido.");
    } catch {
      if (isCurrent()) toast.error("Não foi possível remover o comprovante.");
    } finally {
      if (isCurrent()) setDeletingDocumentId(null);
    }
  };

  const roleOrder = dealDetails.type === "SALE"
    ? ["buyer", "owner", "guarantor", "spouse", "attorney", "legal_representative", "guardian", "curator"]
    : ["tenant", "owner", "guarantor", "spouse", "attorney", "legal_representative", "guardian", "curator"];
  const partiesByRole = relevantParties.reduce<Record<string, typeof relevantParties>>((groups, party) => {
    groups[party.role] = [...(groups[party.role] ?? []), party];
    return groups;
  }, {});
  const orderedRoles = [...roleOrder, ...Object.keys(partiesByRole).filter((role) => !roleOrder.includes(role))];
  const activePartyIds = new Set(relevantParties.map((party) => party.id));
  const generalDocuments = documents.filter(
    (document) => !isScopedAnalysisDocument(document) || !activePartyIds.has(document.analysis_party_id ?? ""),
  );
  const scopedDocumentsCount = documents.length - generalDocuments.length;
  const pendingCriteria = relevantParties.flatMap((party) =>
    ANALYSIS_CRITERIA.flatMap((criterion) => {
      const dd = party.due_diligence;
      const value =
        criterion === "score"
          ? dd?.score_serasa
          : criterion === "cnd"
            ? dd?.cnd_status
            : criterion === "legal_opinion"
              ? dd?.legal_opinion
              : dd?.monthly_income;
      const hasValue = value !== undefined && value !== null && String(value).trim() !== "";
      const evidenceCount = documents.filter(
        (document) =>
          document.analysis_party_id === party.id &&
          document.analysis_criterion === criterion,
      ).length;
      if (hasValue && evidenceCount > 0) return [];
      return [{ party, criterion, hasValue, evidenceCount }];
    }),
  );
  const missingValueCount = pendingCriteria.filter((item) => !item.hasValue).length;
  const missingEvidenceCount = pendingCriteria.filter(
    (item) => item.hasValue && item.evidenceCount === 0,
  ).length;
  const filledCriteria = relevantParties.reduce(
    (total, party) => total + countFilledCriteria(party.due_diligence),
    0,
  );
  const totalCriteria = relevantParties.length * ANALYSIS_CRITERIA.length;
  const completedCriteria = totalCriteria - pendingCriteria.length;
  const aggregateStatus = (aggregateDueDiligence?.status ?? "PENDING").toUpperCase();
  const effectiveStatus =
    aggregateStatus === "REJECTED"
      ? "REJECTED"
      : pendingCriteria.length > 0
        ? aggregateStatus === "APPROVED"
          ? "APPROVED_WITH_PENDING"
          : "PENDING"
        : "APPROVED";
  const aggregateTone: StatusTone =
    effectiveStatus === "APPROVED"
      ? "ok"
      : effectiveStatus === "REJECTED"
        ? "critical"
        : effectiveStatus === "APPROVED_WITH_PENDING"
          ? "warning"
          : "pending";
  const aggregateLabel: Record<string, string> = {
    APPROVED: "Aprovado",
    APPROVED_WITH_PENDING: "Aprovado com pendências",
    REJECTED: "Reprovado",
    PENDING: "Em andamento",
  };

  return (
    <div className="space-y-5">
      <section className="overflow-hidden rounded-2xl border border-border bg-card shadow-sm">
        <header className="flex flex-col gap-2 border-b border-border px-4 py-3 sm:flex-row sm:items-center sm:justify-between sm:px-5">
          <div className="flex min-w-0 items-center gap-2.5">
            <ShieldCheck className="h-4 w-4 shrink-0 text-primary" aria-hidden="true" />
            <h3 className="text-label font-bold uppercase tracking-wide text-foreground">Resumo da análise</h3>
          </div>
          <StatusPill tone={aggregateTone}>{aggregateLabel[effectiveStatus] ?? effectiveStatus}</StatusPill>
        </header>
        <div className="grid grid-cols-1 gap-px bg-border sm:grid-cols-2 xl:grid-cols-4">
          <AnalysisSummaryMetric label="Partes analisadas" value={String(relevantParties.length)} helper="por papel no negócio" />
          <AnalysisSummaryMetric label="Critérios preenchidos" value={`${filledCriteria}/${totalCriteria}`} helper="Score, CND, Parecer e Renda" />
          <AnalysisSummaryMetric label="Critérios completos" value={`${completedCriteria}/${totalCriteria}`} helper="valor + comprovante" />
          <AnalysisSummaryMetric label="Comprovantes anexados" value={String(scopedDocumentsCount)} helper="por parte e critério" />
        </div>
        <p className="border-t border-border px-4 py-3 text-caption text-muted-foreground sm:px-5">
          O status geral é derivado das análises individuais. Enquanto houver pendências, ele fica em andamento; anexe os comprovantes para concluir.
        </p>

        <div className="flex flex-col gap-2 border-t border-border px-4 py-3 sm:flex-row sm:items-center sm:justify-between sm:px-5">
          <p className="text-caption text-muted-foreground">
            {pendingCriteria.length === 0
              ? "Todos os critérios têm valor e comprovante."
              : `${pendingCriteria.length} pendência(s): ${missingValueCount} sem valor · ${missingEvidenceCount} sem comprovante.`}
          </p>
          {pendingCriteria.length > 0 && (
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="w-full gap-2 sm:w-auto"
              onClick={() => setShowPending((value) => !value)}
            >
              {showPending ? "Ocultar pendências" : "Ver pendências"}
            </Button>
          )}
        </div>

        {showPending && pendingCriteria.length > 0 && (
          <ul className="divide-y divide-border border-t border-border bg-card">
            {pendingCriteria.map(({ party, criterion, hasValue }) => (
              <li
                key={`${party.id}:${criterion}`}
                className="flex flex-col gap-2 px-4 py-2 sm:flex-row sm:items-center sm:justify-between sm:px-5"
              >
                <div className="flex min-w-0 flex-wrap items-center gap-2">
                  <span className="truncate text-sm font-semibold text-foreground">{party.name}</span>
                  <span className="rounded-full border border-border bg-muted px-2 py-0.5 text-micro font-bold text-muted-foreground">
                    {ANALYSIS_CRITERION_LABELS[criterion]}
                  </span>
                  <span
                    className={`text-micro font-semibold ${
                      hasValue ? "text-status-warning" : "text-status-critical"
                    }`}
                  >
                    {hasValue ? "Sem comprovante" : "Sem valor"}
                  </span>
                </div>
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  className="h-8 self-start sm:self-auto"
                  onClick={() => {
                    setOpenPartyId(party.id);
                    document
                      .getElementById(`analysis-party-${party.id}`)
                      ?.scrollIntoView({ behavior: "smooth", block: "start" });
                  }}
                >
                  Abrir
                </Button>
              </li>
            ))}
          </ul>
        )}
      </section>

      {filledCriteria === 0 && scopedDocumentsCount === 0 && relevantParties.length > 0 && (
        <div className="flex flex-col gap-2 rounded-xl border border-primary/20 bg-primary/5 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
          <p className="text-sm text-muted-foreground">
            Comece pelo Score da primeira parte. Preencha o valor e anexe o comprovante no mesmo card.
          </p>
          <Button
            type="button"
            size="sm"
            variant="outline"
            className="w-full gap-2 sm:w-auto"
            onClick={() => {
              setOpenPartyId(relevantParties[0].id);
              document
                .getElementById(`analysis-party-${relevantParties[0].id}`)
                ?.scrollIntoView({ behavior: "smooth", block: "start" });
            }}
          >
            Abrir primeira parte
          </Button>
        </div>
      )}

      <div className="space-y-4">
        {orderedRoles.map((role) => {
          const roleParties = partiesByRole[role] ?? [];
          if (roleParties.length === 0) return null;
          const roleFilled = roleParties.reduce(
            (total, party) => total + countFilledCriteria(party.due_diligence),
            0,
          );
          const roleTotal = roleParties.length * ANALYSIS_CRITERIA.length;
          const roleCompleted = roleParties.reduce((total, party) => {
            const dd = party.due_diligence;
            return (
              total +
              ANALYSIS_CRITERIA.filter((criterion) => {
                const value =
                  criterion === "score"
                    ? dd?.score_serasa
                    : criterion === "cnd"
                      ? dd?.cnd_status
                      : criterion === "legal_opinion"
                        ? dd?.legal_opinion
                        : dd?.monthly_income;
                const hasValue =
                  value !== undefined && value !== null && String(value).trim() !== "";
                const evidenceCount = documents.filter(
                  (document) =>
                    document.analysis_party_id === party.id &&
                    document.analysis_criterion === criterion,
                ).length;
                return hasValue && evidenceCount > 0;
              }).length
            );
          }, 0);

          return (
            <section key={role} className="overflow-hidden rounded-2xl border border-border bg-muted/20">
              <header className="flex flex-col gap-2 border-b border-border bg-card px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
                <div className="flex min-w-0 items-center gap-2.5">
                  <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                    <Users className="h-4 w-4" aria-hidden="true" />
                  </span>
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <h4 className="text-label font-bold uppercase tracking-wide text-foreground">{partyRoleLabel(role)}</h4>
                      <span className="rounded-full bg-muted px-2 py-0.5 text-micro font-bold tabular-nums text-muted-foreground">
                        {roleParties.length}
                      </span>
                    </div>
                    <p className="mt-0.5 text-caption text-muted-foreground">
                      Análises e documentos das partes com este papel
                    </p>
                  </div>
                </div>
                <span className="text-xs font-semibold tabular-nums text-muted-foreground">
                  {roleFilled} de {roleTotal} valores · {roleCompleted} completos
                </span>
              </header>
              <div className="space-y-3 p-3 sm:p-4">
                {roleParties.map((party) => {
                  const evidenceByCriterion = ANALYSIS_CRITERIA.reduce<AnalysisEvidenceByCriterion>(
                    (byCriterion, criterion) => {
                      byCriterion[criterion] = documents.filter(
                        (document) =>
                          document.analysis_party_id === party.id && document.analysis_criterion === criterion,
                      );
                      return byCriterion;
                    },
                    {},
                  );

                  return (
                    <div key={party.id} id={`analysis-party-${party.id}`}>
                      <PartyDueDiligenceCard
                        dealId={dealDetails.id}
                        party={party}
                        open={openPartyId === party.id}
                        onOpenChange={(open) => setOpenPartyId(open ? party.id : null)}
                        onUpdatePartyDueDiligence={onUpdatePartyDueDiligence}
                        onUpdateCriterion={(criterion, status) =>
                          onUpdateCriterion(party.id, criterion, status)
                        }
                        evidenceByCriterion={evidenceByCriterion}
                        evidenceLoading={documentsLoading}
                        evidenceUnavailable={documentsError}
                        uploadingCriterion={
                          ANALYSIS_CRITERIA.find((criterion) => uploadingKeys.has(`${party.id}:${criterion}`)) ?? null
                        }
                        deletingDocumentId={deletingDocumentId}
                        onUploadEvidence={(criterion, file) =>
                          uploadDocument(file, {
                            partyId: party.id,
                            partyName: party.name || party.email || party.id,
                            criterion,
                          })
                        }
                        onOpenEvidence={(document) => void openDocument(document)}
                        onRemoveEvidence={(document) => void removeDocument(document)}
                      />
                    </div>
                  );
                })}
              </div>
            </section>
          );
        })}
      </div>

      <AnalysisAttachments
        documents={generalDocuments}
        loading={documentsLoading}
        error={documentsError}
        uploading={uploadingKeys.has("general")}
        deletingDocumentId={deletingDocumentId}
        onUpload={(file) => uploadDocument(file)}
        onOpen={openDocument}
        onRemove={removeDocument}
        onRetry={loadDocuments}
      />

      {documentsError && (
        <div className="flex flex-col gap-3 rounded-xl border border-status-critical/20 bg-status-critical-soft px-4 py-3 sm:flex-row sm:items-center sm:justify-between" role="alert">
          <div className="flex min-w-0 items-start gap-2.5">
            <AlertCircle className="mt-0.5 h-4 w-4 shrink-0 text-status-critical" aria-hidden="true" />
            <div>
              <p className="text-sm font-bold text-foreground">Evidências temporariamente indisponíveis</p>
              <p className="text-caption text-muted-foreground">Os resultados continuam visíveis, mas os anexos não puderam ser carregados.</p>
            </div>
          </div>
          <Button type="button" size="sm" variant="outline" onClick={() => void loadDocuments()} className="w-full gap-2 sm:w-auto">
            <RefreshCw className="h-3.5 w-3.5" />
            Tentar novamente
          </Button>
        </div>
      )}

      {viewer && (
        <DocumentViewerDialog
          open
          onOpenChange={(open) => !open && setViewer(null)}
          url={viewer.url}
          title={viewer.title}
        />
      )}
    </div>
  );
}

function AnalysisSummaryMetric({ label, value, helper }: { label: string; value: string; helper: string }) {
  return (
    <div className="bg-card px-4 py-3 sm:px-5">
      <p className="text-micro font-bold uppercase tracking-wide text-muted-foreground">{label}</p>
      <p className="mt-0.5 text-lg font-bold tabular-nums text-foreground">{value}</p>
      <p className="text-micro text-muted-foreground">{helper}</p>
    </div>
  );
}

/** Documentos gerais, sem vínculo com uma parte ou quesito específico. */
function AnalysisAttachments({
  documents,
  loading,
  error,
  uploading,
  deletingDocumentId,
  onUpload,
  onOpen,
  onRemove,
  onRetry,
}: {
  documents: DocumentDTO[];
  loading: boolean;
  error: boolean;
  uploading: boolean;
  deletingDocumentId: string | null;
  onUpload: (file: File) => Promise<void>;
  onOpen: (document: DocumentDTO) => Promise<void>;
  onRemove: (document: DocumentDTO) => Promise<void>;
  onRetry: () => Promise<void>;
}) {
  const fileInputRef = useRef<HTMLInputElement | null>(null);

  return (
    <section className="overflow-hidden rounded-2xl border border-border bg-card shadow-sm">
      <header className="flex flex-col gap-3 border-b border-border px-4 py-3 sm:flex-row sm:items-center sm:justify-between sm:px-5">
        <div className="flex min-w-0 items-center gap-2.5">
          <Paperclip className="h-4 w-4 shrink-0 text-primary" aria-hidden="true" />
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h3 className="text-label font-bold uppercase tracking-wide text-foreground">Documentos de apoio</h3>
              <span className="text-caption tabular-nums text-muted-foreground">{documents.length}</span>
            </div>
            <p className="mt-0.5 text-caption text-muted-foreground">
              Use esta área apenas para documentos que apoiam a análise inteira.
            </p>
          </div>
        </div>
        <input
          ref={fileInputRef}
          type="file"
          accept=".pdf,.doc,.docx,.jpg,.jpeg,.png"
          className="hidden"
          onChange={(event) => {
            const file = event.target.files?.[0];
            event.target.value = "";
            if (file) void onUpload(file);
          }}
        />
        <Button
          size="sm"
          variant="outline"
          className="w-full gap-2 text-primary hover:bg-primary/10 sm:w-auto"
          disabled={uploading}
          onClick={() => fileInputRef.current?.click()}
        >
          {uploading ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Paperclip className="h-3.5 w-3.5" />}
          {uploading ? "Enviando…" : "Anexar documento de apoio"}
        </Button>
      </header>
      <div className="px-4 py-3 sm:px-5">
        <p className="mb-3 text-caption text-muted-foreground">
          Para um critério específico, anexe o comprovante no próprio critério.
        </p>
        {loading && <p className="text-xs text-muted-foreground">Carregando documentos…</p>}
        {!loading && error && (
          <div className="flex flex-col gap-2 rounded-xl border border-status-critical/20 bg-status-critical-soft px-3 py-2.5 sm:flex-row sm:items-center sm:justify-between" role="alert">
            <span className="text-xs font-semibold text-status-critical">Não foi possível carregar os documentos gerais.</span>
            <Button type="button" size="sm" variant="ghost" className="h-8 gap-1.5" onClick={() => void onRetry()}>
              <RefreshCw className="h-3.5 w-3.5" />
              Tentar novamente
            </Button>
          </div>
        )}
        {!loading && !error && documents.length === 0 && (
          <p className="rounded-xl border border-dashed border-border px-3 py-3 text-xs text-muted-foreground">
            Nenhum documento de apoio anexado. Os comprovantes específicos ficam nos cards das partes.
          </p>
        )}
        {!loading && !error && documents.length > 0 && (
          <ul className="space-y-2">
            {documents.map((document) => {
              const category = getCategoryMeta(document.category);
              const status = getStatusMeta(normalizeBackendStatus(document.status));
              return (
                <li key={document.id} className="flex min-w-0 flex-col gap-2 rounded-xl border border-border bg-muted/30 px-3 py-2.5 sm:flex-row sm:items-center sm:justify-between">
                  <div className="flex min-w-0 items-center gap-2">
                    <FileText className="h-4 w-4 shrink-0 text-muted-foreground" />
                    <div className="min-w-0">
                      <p className="truncate text-sm font-semibold text-foreground" title={document.name}>{document.name}</p>
                      <div className="mt-0.5 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                        <span className={`inline-flex items-center rounded-full border px-1.5 py-0.5 text-micro font-bold ${category.badgeClass}`}>
                          {category.label}
                        </span>
                        <span className={`inline-flex items-center rounded-full border px-1.5 py-0.5 text-micro font-bold ${status.badgeClass}`}>
                          {status.label}
                        </span>
                      </div>
                    </div>
                  </div>
                  <div className="flex shrink-0 items-center justify-end gap-1">
                    <Button type="button" variant="link" size="sm" onClick={() => void onOpen(document)} className="h-auto p-0 text-xs font-bold">
                      Abrir
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      onClick={() => void onRemove(document)}
                      loading={deletingDocumentId === document.id}
                      className="h-8 w-8 text-muted-foreground hover:bg-status-critical-soft hover:text-status-critical"
                      aria-label={`Remover ${document.name}`}
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </section>
  );
}
