import { useEffect, useMemo, useState, type ReactNode } from "react";
import {
  CheckCircle,
  ChevronDown,
  Clock,
  FileText,
  MoreHorizontal,
  Paperclip,
  Plus,
  RefreshCw,
  Search,
  Send,
  Settings2,
  Sparkles,
  Trash2,
  UploadCloud,
  XCircle,
} from "lucide-react";
import { clsx } from "clsx";
import { toast } from "sonner";
import { ExpandableSidePanel } from "../../../../shared/components/ExpandableSidePanel";
import { Button } from "../../../../shared/ui/button";
import { Badge } from "../../../../shared/ui/badge";
import { ConfirmDialog } from "../../../../shared/components/ConfirmDialog";
import { DocumentViewerDialog } from "../../../../shared/components/DocumentViewerDialog";
import { SignedDocumentsSection } from "../../../../shared/components/signature/SignedDocumentsSection";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "../../../../shared/ui/dropdown-menu";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "../../../../shared/ui/sheet";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "../../../../shared/ui/dialog";
import { useDealChecklistsContext } from "../../../../shared/context/DealChecklistsContext";
import { DocumentChecklistItem, ChecklistTemplateId, ChecklistItemCategory, DEFAULT_CHECKLIST_TEMPLATES, CHECKLIST_CATEGORY_LABELS } from "../../../../shared/data";
import { flags } from "../../../../shared/config/flags";
import type { Contract } from "../../../../shared/contracts/types";
import type { ChecklistRequestedFieldDTO, ChecklistStatsDTO, ChecklistTemplateDTO, DealDTO, PartyDTO } from "../../../../shared/services/deal.service";
import { dealService } from "../../../../shared/services/deal.service";
import { PARTY_ROLE_LABELS } from "../../../../shared/deals/partyLabels";
import { documentService, type DocumentDTO } from "../../../../shared/services/document.service";
import { ReviewExtractedDataDialog } from "../../../../shared/components/ReviewExtractedDataDialog";
import { ReviewFormSubmissionDialog } from "../../../../shared/components/review/ReviewFormSubmissionDialog";
import { describeExtractionError, summarizeExtraction } from "../../../../shared/components/review/review-utils";
import { useDealsContext } from "../../../../shared/context/DealsContext";
import { getCategoryMeta, getStatusMeta, normalizeBackendStatus } from "../../../../shared/lib/documents";
import { toGatewayUrl, getAccessToken, withGatewayToken } from "../../../../shared/services/api";
import { useQuery } from "@tanstack/react-query";

type DocStatusFilter = "all" | "pending" | "solicited" | "analysis" | "rejected" | "approved";
type BatchRequestState = { partyId?: string; label: string; items: DocumentChecklistItem[] };

type PendingReviewRisk = "divergence" | "low" | "review" | "failed" | "empty" | "form";
type PendingReviewFilter = "all" | "divergence" | "low" | "failed" | "form";
type PendingReviewItem = {
  key: string;
  kind: "ocr" | "form";
  partyId: string;
  partyName: string;
  partyRole?: string;
  party?: PartyDTO;
  title: string;
  risk: PendingReviewRisk;
  summary: string;
  technicalError?: string;
  age: string;
  document?: DocumentDTO;
  item?: DocumentChecklistItem;
};
type PendingReviewGroup = {
  partyId: string;
  partyName: string;
  partyRole?: string;
  items: PendingReviewItem[];
};

const pendingRiskMeta: Record<
  PendingReviewRisk,
  { label: string; variant: "warning" | "critical" | "info" | "neutral" }
> = {
  divergence: { label: "Divergência", variant: "warning" },
  low: { label: "Baixa confiança", variant: "warning" },
  review: { label: "OCR", variant: "neutral" },
  failed: { label: "Falha de OCR", variant: "critical" },
  empty: { label: "Sem dados", variant: "warning" },
  form: { label: "Formulário", variant: "info" },
};

const pendingRiskOrder: Record<PendingReviewRisk, number> = {
  divergence: 0,
  failed: 1,
  empty: 2,
  low: 3,
  form: 4,
  review: 5,
};

function formatReviewAge(value?: string): string {
  if (!value) return "";
  const timestamp = new Date(value).getTime();
  if (Number.isNaN(timestamp)) return "";
  const minutes = Math.max(0, Math.floor((Date.now() - timestamp) / 60_000));
  if (minutes < 1) return "agora";
  if (minutes < 60) return `há ${minutes} min`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `há ${hours} h`;
  const days = Math.floor(hours / 24);
  return `há ${days} d`;
}

function checklistStatusDot(item: DocumentChecklistItem) {
  if (item.status === "approved") return "bg-status-ok";
  if (item.status === "uploaded") return "bg-status-info";
  if (item.status === "rejected") return "bg-status-critical";
  return item.solicited ? "bg-status-pending" : "bg-status-warning";
}

function checklistStatusLabel(item: DocumentChecklistItem) {
  if (item.status === "approved") return "Aprovado";
  if (item.status === "uploaded") return "Em análise";
  if (item.status === "rejected") return "Rejeitado";
  return item.solicited ? "Solicitado" : "A solicitar";
}

export function DocumentationTab({ dealDetails, latestContract }: { dealDetails: DealDTO | null; latestContract?: Contract | null }) {
  const { dealChecklists, initDealChecklist, addChecklistItem, removeChecklistItem, updateChecklistItemStatus, solicitChecklistItem, solicitAllPending } = useDealChecklistsContext();
  const [isAddingItem, setIsAddingItem] = useState(false);
  const [newItemName, setNewItemName] = useState("");
  const [newItemCategory, setNewItemCategory] = useState<ChecklistItemCategory>("pessoal");
  const [newItemRequired, setNewItemRequired] = useState(true);
  const [newItemMode, setNewItemMode] = useState<"upload" | "form">("upload");
  const [selectedPartyId, setSelectedPartyId] = useState<string>("");
  const [rejectingId, setRejectingId] = useState<string | null>(null);
  const [rejectReason, setRejectReason] = useState("");
  const [viewer, setViewer] = useState<{ url: string; title?: string } | null>(null);
  const [requesting, setRequesting] = useState<{ itemId: string; partyId?: string } | null>(null);
  const [requestedBase, setRequestedBase] = useState({
    full_name: { enabled: false, required: true },
    cpf: { enabled: false, required: true },
    birth_date: { enabled: false, required: true },
    phone: { enabled: false, required: true },
  });
  const [requestedMode, setRequestedMode] = useState<"upload" | "form">("upload");
  const [requestedCustom, setRequestedCustom] = useState<ChecklistRequestedFieldDTO[]>([]);
  const [showAdvancedKey, setShowAdvancedKey] = useState(false);
  const [openPicker, setOpenPicker] = useState(false);
  const [pickerQuery, setPickerQuery] = useState("");
  const [pickerSelected, setPickerSelected] = useState<Record<string, boolean>>({});
  const [selectedTemplateByParty, setSelectedTemplateByParty] = useState<Record<string, ChecklistTemplateId | "">>({});
  const [selectedTemplateGeneral, setSelectedTemplateGeneral] = useState<ChecklistTemplateId | "">("");
  const [confirmAction, setConfirmAction] = useState<
    | null
    | { kind: "replace"; partyId: string | null; templateId: ChecklistTemplateId }
    | { kind: "clear"; partyId: string | null }
    | { kind: "remove-item"; itemId: string; itemName: string }
  >(null);

  const [statusFilter, setStatusFilter] = useState<DocStatusFilter>("all");
  const [search, setSearch] = useState("");
  const [configOpen, setConfigOpen] = useState(false);
  const [batchRequest, setBatchRequest] = useState<BatchRequestState | null>(null);

  const dealId = dealDetails?.id || "";
  const parties = dealDetails?.parties ?? [];

  // OCR + IA: documentos do negócio já extraídos, para o operador revisar/validar.
  const { fetchDealById } = useDealsContext();
  const [review, setReview] = useState<{ party: PartyDTO; document: DocumentDTO; item?: DocumentChecklistItem } | null>(null);
  const [formReview, setFormReview] = useState<{ party: PartyDTO; item: DocumentChecklistItem } | null>(null);
  const [reviewFilter, setReviewFilter] = useState<PendingReviewFilter>("all");
  const [extractionBusy, setExtractionBusy] = useState<Record<string, boolean>>({});
  const extractionDocsQuery = useQuery({
    queryKey: ["deal-extraction-docs", dealId],
    queryFn: () => documentService.getByEntity(dealId, "deal"),
    enabled: !!dealId,
    // Enquanto a IA processa, a fila e o botão do item se atualizam sozinhos.
    refetchInterval: (query) => {
      const docs = query.state.data?.data ?? [];
      return docs.some(
        (doc) => doc.extraction_status === "pending" || doc.extraction_status === "processing",
      )
        ? 5000
        : false;
    },
  });
  const resolveExtractionParty = (doc: DocumentDTO): PartyDTO | undefined =>
    parties.find((p) => p.id === doc.extraction?.applied_party_id) ?? (parties.length === 1 ? parties[0] : undefined);
  const allDocuments = extractionDocsQuery.data?.data ?? [];
  const findLinkedDocumentIn = (
    documents: DocumentDTO[],
    item: DocumentChecklistItem,
  ): DocumentDTO | undefined => {
    // O file_url do documento espelhado carrega o ID do item do checklist:
    // /api/v1/deals/:dealId/checklist/:itemId/download. É o vínculo mais estável
    // porque o nome do item ("Documento de Identidade") difere do arquivo ("RG.pdf").
    const byItemId = documents.find((doc) => doc.file_url?.includes(`/checklist/${item.id}/download`));
    if (byItemId) return byItemId;
    if (item.fileUrl) {
      const byUrl = documents.find((doc) => doc.file_url === item.fileUrl);
      if (byUrl) return byUrl;
    }
    if (item.fileName) {
      const byName = documents.find(
        (doc) => doc.name === item.fileName || doc.name.endsWith(item.fileName as string),
      );
      if (byName) return byName;
    }
    return documents.find((doc) => doc.name === item.name);
  };
  const findLinkedDocument = (item: DocumentChecklistItem): DocumentDTO | undefined =>
    findLinkedDocumentIn(allDocuments, item);

  // Phase 4/W4 — fetch the canonical template set from the backend.
  // Falls back to the bundled DEFAULT_CHECKLIST_TEMPLATES on error so
  // the tab stays usable when /checklist-templates is unreachable
  // (e.g. before the backend rolls out the endpoint).
  const templatesQuery = useQuery({
    queryKey: ["checklist-templates"],
    queryFn: () => dealService.listChecklistTemplates(),
    staleTime: 5 * 60_000,
  });
  const templates: ChecklistTemplateDTO[] = (templatesQuery.data && templatesQuery.data.length > 0
    ? templatesQuery.data
    : (DEFAULT_CHECKLIST_TEMPLATES as unknown as ChecklistTemplateDTO[])).map((t) => ({
      id: t.id,
      label: t.label,
      items: t.items.map((i) => ({
        name: i.name,
        category: i.category,
        required: i.required,
        description: i.description,
        // Fallback local usa keys camelCase; normaliza para o shape da API
        // (snake_case) para que kits e seleção individual sejam idênticos.
        request_mode: (i as any).request_mode ?? (i as any).requestMode ?? undefined,
        requested_fields: (i as any).requested_fields ?? (i as any).requestedFields ?? undefined,
      })),
    }));
  const roleOptionLabel: Record<string, string> = PARTY_ROLE_LABELS;
  const partyNameById = parties.reduce<Record<string, string>>((acc, p) => {
    acc[p.id] = p.name || p.email || p.lead_id || p.id;
    return acc;
  }, {});
  const partyRoleById = parties.reduce<Record<string, string>>((acc, p) => {
    acc[p.id] = p.role;
    return acc;
  }, {});
  const checklist = dealChecklists[dealId] || [];
  const formReviewItems = checklist.filter(
    (item) => item.requestMode === "form" && item.status === "uploaded" && !!item.partyId,
  );

  // “Revisões pendentes” é uma fila acionável por parte: some quando o item
  // do checklist já foi aprovado ou quando a extração já foi aplicada. Inclui
  // OCR concluído, falha e vazio, além dos formulários aguardando revisão.
  const approvedDocumentIds = new Set<string>(
    checklist
      .filter((item) => item.status === "approved")
      .map((item) => findLinkedDocument(item)?.id)
      .filter((id): id is string => Boolean(id)),
  );
  const rejectedDocumentIds = new Set<string>(
    checklist
      .filter((item) => item.status === "rejected")
      .map((item) => findLinkedDocument(item)?.id)
      .filter((id): id is string => Boolean(id)),
  );

  const reviewableDocs = (extractionDocsQuery.data?.data ?? []).filter(
    (doc) =>
      !approvedDocumentIds.has(doc.id) &&
      !rejectedDocumentIds.has(doc.id) &&
      !doc.extraction?.applied_party_id &&
      (doc.extraction_status === "completed" ||
        doc.extraction_status === "failed" ||
        doc.extraction_status === "empty"),
  );

  const reviewQueueItems: PendingReviewItem[] = [
    ...reviewableDocs
      .map((doc) => ({
        doc,
        linkedItem: checklist.find((entry) => findLinkedDocument(entry)?.id === doc.id),
      }))
      .filter(
        (entry): entry is { doc: DocumentDTO; linkedItem: DocumentChecklistItem } =>
          !!entry.linkedItem,
      )
      .map(({ doc, linkedItem }) => {
        const target =
          resolveExtractionParty(doc) ??
          (linkedItem.partyId
            ? parties.find((party) => party.id === linkedItem.partyId)
            : undefined);
        const summary = summarizeExtraction(doc.extraction, target ?? null);
        const confidence = Math.round((doc.extraction?.overall_confidence ?? 0) * 100);
        const errorInfo = describeExtractionError(
          doc.extraction_error ?? doc.extraction?.error,
        );
        const risk: PendingReviewRisk =
          doc.extraction_status === "failed"
            ? "failed"
            : doc.extraction_status === "empty" || (doc.extraction?.fields?.length ?? 0) === 0
              ? "empty"
              : summary.divergences > 0
                ? "divergence"
                : summary.low > 0
                  ? "low"
                  : "review";
        const summaryText =
          risk === "failed"
            ? errorInfo.message
            : risk === "empty"
              ? "Sem valores identificados — revise ou preencha manualmente"
              : `OCR · ${summary.total} campos · ${summary.divergences} divergências · ${summary.low} baixa · ${confidence}%`;
        return {
          key: `ocr:${doc.id}`,
          kind: "ocr" as const,
          partyId: target?.id ?? "",
          partyName: target ? partyNameById[target.id] ?? target.name : "Parte não identificada",
          partyRole: target?.role,
          party: target,
          title: doc.name,
          risk,
          summary: summaryText,
          technicalError: risk === "failed" ? errorInfo.detail : undefined,
          age: formatReviewAge(doc.extraction?.extracted_at ?? doc.updated_at),
          document: doc,
          item: linkedItem,
        };
      }),
    ...formReviewItems.map((item) => {
      const target = parties.find((p) => p.id === item.partyId);
      const submittedAt = item.formSubmission?.submittedAt;
      const fieldCount =
        item.formSubmission?.requestedFields?.length ?? item.requestedFields?.length ?? 0;
      return {
        key: `form:${item.id}`,
        kind: "form" as const,
        partyId: item.partyId ?? "",
        partyName: target ? partyNameById[target.id] ?? target.name : "Parte não identificada",
        partyRole: target?.role,
        party: target,
        title: item.name,
        risk: "form" as const,
        summary: `Formulário · ${fieldCount} campos${
          submittedAt
            ? ` · enviado em ${new Date(submittedAt).toLocaleDateString("pt-BR")}`
            : " · sem snapshot"
        }`,
        age: formatReviewAge(submittedAt),
        item,
      };
    }),
  ];

  const reviewQueueGroups = Array.from(
    reviewQueueItems
      .reduce((map, item) => {
        const key = item.partyId || "__unassigned__";
        const existing = map.get(key);
        if (existing) {
          existing.items.push(item);
        } else {
          map.set(key, {
            partyId: item.partyId,
            partyName: item.partyName,
            partyRole: item.partyRole,
            items: [item],
          });
        }
        return map;
      }, new Map<string, PendingReviewGroup>())
      .values(),
  )
    .map((group) => ({
      ...group,
      items: [...group.items].sort(
        (a, b) => pendingRiskOrder[a.risk] - pendingRiskOrder[b.risk],
      ),
    }))
    .sort(
      (a, b) =>
        (a.partyId ? 0 : 1) - (b.partyId ? 0 : 1) ||
        a.partyName.localeCompare(b.partyName),
    );

  const reviewCounts = {
    all: reviewQueueItems.length,
    divergence: reviewQueueItems.filter((item) => item.risk === "divergence").length,
    low: reviewQueueItems.filter((item) => item.risk === "low").length,
    failed: reviewQueueItems.filter((item) => item.risk === "failed" || item.risk === "empty").length,
    form: reviewQueueItems.filter((item) => item.risk === "form").length,
  };

  const visibleReviewGroups = reviewQueueGroups
    .map((group) => ({
      ...group,
      items: group.items.filter(
        (item) =>
          reviewFilter === "all" ||
          item.risk === reviewFilter ||
          (reviewFilter === "failed" && item.risk === "empty"),
      ),
    }))
    .filter((group) => group.items.length > 0);
  const isMultiParty = flags.multiPartyUI && parties.length > 0;

  // Parte-alvo da solicitação atual (diálogo "Solicitar dados") e badge de
  // status de cada campo (já preenchido / faltando) com base no cadastro dela.
  const solicitParty = useMemo(
    () => parties.find((p) => p.id === requesting?.partyId),
    [parties, requesting?.partyId],
  );
  const fieldStatusBadge = (filled: boolean) =>
    solicitParty ? (
      <span
        className={clsx(
          "text-caption font-bold px-1.5 py-0.5 rounded-full",
          filled ? "text-status-ok bg-status-ok-soft" : "text-status-warning bg-status-warning-soft",
        )}
      >
        {filled ? "já preenchido" : "faltando"}
      </span>
    ) : null;

  const openSolicitDialog = (item: DocumentChecklistItem) => {
    const base = {
      full_name: { enabled: false, required: true },
      cpf: { enabled: false, required: true },
      birth_date: { enabled: false, required: true },
      phone: { enabled: false, required: true },
    };
    const custom: ChecklistRequestedFieldDTO[] = [];
    for (const f of item.requestedFields ?? []) {
      const k = (f?.key ?? "").trim();
      if (k === "full_name") base.full_name = { enabled: true, required: !!f.required };
      else if (k === "cpf") base.cpf = { enabled: true, required: !!f.required };
      else if (k === "birth_date") base.birth_date = { enabled: true, required: !!f.required };
      else if (k === "phone") base.phone = { enabled: true, required: !!f.required };
      else if (k) custom.push({ key: k, label: (f.label ?? k).trim() || k, type: (f.type ?? "text").trim() || "text", required: !!f.required });
    }
    setRequestedBase(base);
    setRequestedCustom(custom);
    setRequestedMode(item.requestMode ?? "upload");
    setRequesting({ itemId: item.id, partyId: item.partyId });
  };

  const openFormReview = (item: DocumentChecklistItem) => {
    if (!item.partyId) {
      toast.error("Vincule uma parte a este formulário antes de revisar os campos.");
      return;
    }
    const target = parties.find((party) => party.id === item.partyId);
    if (!target) {
      toast.error("Parte do formulário não encontrada.");
      return;
    }
    setFormReview({ party: target, item });
  };

  const requestExtraction = async (doc: DocumentDTO) => {
    setExtractionBusy((prev) => ({ ...prev, [doc.id]: true }));
    try {
      await documentService.reExtract(doc.id);
      toast.success("Extração de IA solicitada. Os campos aparecerão nesta tela.");
      await extractionDocsQuery.refetch();
    } catch {
      toast.error("Não foi possível solicitar a extração de IA.");
    } finally {
      setExtractionBusy((prev) => ({ ...prev, [doc.id]: false }));
    }
  };

  const openDocumentReview = async (item: DocumentChecklistItem) => {
    let doc = findLinkedDocument(item);
    if (!doc) {
      try {
        toast.success("Reconciliando o documento do checklist…");
        await dealService.backfillChecklistDocuments();
        const refreshed = await extractionDocsQuery.refetch();
        doc = findLinkedDocumentIn(refreshed.data?.data ?? [], item);
      } catch {
        toast.error("Não foi possível reconciliar o documento. Reenvie o arquivo ou tente novamente.");
        return;
      }
      if (!doc) {
        toast.error("Documento do checklist não encontrado após a reconciliação.");
        return;
      }
    }
    const target = parties.find((party) => party.id === item.partyId) ?? resolveExtractionParty(doc);
    if (!target) {
      toast.error("Parte do documento não identificada.");
      return;
    }
    const hasFields =
      doc.extraction_status === "completed" && (doc.extraction?.fields?.length ?? 0) > 0;
    if (hasFields) {
      setReview({ party: target, document: doc, item });
      return;
    }
    if (doc.extraction_status === "pending" || doc.extraction_status === "processing") {
      toast.success("A IA já está processando este documento.");
      return;
    }
    await requestExtraction(doc);
  };

  const confirmSolicit = async () => {
    if (!dealId || !requesting) return;
    if (requestedMode === "form" && !requesting.partyId) {
      toast.error("Solicitações de apenas preenchimento exigem uma parte-alvo do negócio.");
      return;
    }
    const slug = (v: string) => v.trim().toLowerCase().replace(/[^a-z0-9]+/g, "_").replace(/^_+|_+$/g, "");
    const fields: ChecklistRequestedFieldDTO[] = [];
    if (requesting.partyId) {
      if (requestedBase.full_name.enabled) fields.push({ key: "full_name", label: "Nome completo", type: "text", required: requestedBase.full_name.required });
      if (requestedBase.cpf.enabled) fields.push({ key: "cpf", label: "CPF", type: "text", required: requestedBase.cpf.required });
      if (requestedBase.birth_date.enabled) fields.push({ key: "birth_date", label: "Data de nascimento", type: "date", required: requestedBase.birth_date.required });
      if (requestedBase.phone.enabled) fields.push({ key: "phone", label: "Telefone/WhatsApp", type: "text", required: requestedBase.phone.required });
      for (const f of requestedCustom) {
        const label = (f.label ?? "").trim();
        const key = (f.key ?? "").trim() || slug(label);
        if (!label || !key) continue;
        fields.push({ key, label, type: (f.type ?? "text").trim() || "text", required: !!f.required });
      }
    }
    await solicitChecklistItem(dealId, requesting.itemId, requesting.partyId || undefined, fields.length ? fields : undefined, requestedMode);
    setRequesting(null);
  };

  const handleAddItem = () => {
    if (!newItemName.trim() || !dealId) return;
    addChecklistItem(dealId, { name: newItemName.trim(), category: newItemCategory, required: newItemRequired, partyId: selectedPartyId || undefined, requestMode: newItemMode });
    setNewItemName("");
    setNewItemCategory("pessoal");
    setNewItemRequired(true);
    setNewItemMode("upload");
    setIsAddingItem(false);
  };

  const handleReject = (itemId: string) => {
    if (!rejectReason.trim()) {
      toast.error("Informe o motivo da rejeição.");
      return;
    }
    updateChecklistItemStatus(dealId, itemId, "rejected", rejectReason.trim());
    setRejectingId(null);
    setRejectReason("");
    toast.success("Documento rejeitado.");
  };

  const applyTemplate = async (templateId: ChecklistTemplateId, partyId: string | null, mode: "append" | "replace") => {
    if (!dealId) return;
    const template = templates.find((t) => t.id === templateId);
    if (!template) {
      toast.error("Modelo de checklist não encontrado.");
      return;
    }

    const normalize = (s: string) => s.trim().toLowerCase();
    const partyKey = partyId ?? "";
    const current = checklist.filter((i) => (partyKey ? i.partyId === partyKey : !i.partyId));

    if (mode === "replace") {
      for (const item of current) {
        await removeChecklistItem(dealId, item.id);
      }
      setReview(null);
      setFormReview(null);
      await extractionDocsQuery.refetch();
    }

    const existingNames = new Set(
      checklist
        .filter((i) => (partyKey ? i.partyId === partyKey : !i.partyId))
        .map((i) => normalize(i.name)),
    );

    let added = 0;
    for (const item of template.items) {
      const key = normalize(item.name);
      if (existingNames.has(key) && mode === "append") continue;
      await addChecklistItem(dealId, {
        name: item.name,
        category: item.category,
        required: item.required,
        description: item.description,
        partyId: partyKey || undefined,
        requestMode: item.request_mode ?? undefined,
        requestedFields: item.requested_fields,
      } as any);
      existingNames.add(key);
      added++;
    }

    toast.success(
      mode === "replace"
        ? `Checklist "${template.label}" aplicado (${added} itens).`
        : `Checklist "${template.label}" complementado (${added} itens).`,
    );
  };

  const applyPickerSelection = async () => {
    if (!dealId) return;
    const partyKey = selectedPartyId || "";
    const normalize = (s: string) => s.trim().toLowerCase();
    const existing = new Set(
      checklist
        .filter((i) => (partyKey ? i.partyId === partyKey : !i.partyId))
        .map((i) => normalize(i.name)),
    );

    const selectedKeys = Object.entries(pickerSelected)
      .filter(([, v]) => v)
      .map(([k]) => k);
    let added = 0;
    for (const key of selectedKeys) {
      const [tplId, name] = key.split("::");
      const tpl = templates.find((t) => t.id === (tplId as any));
      const item = tpl?.items.find((i) => i.name === name);
      if (!item) continue;
      if (existing.has(normalize(item.name))) continue;
      await addChecklistItem(dealId, {
        name: item.name,
        category: item.category,
        required: item.required,
        description: item.description,
        partyId: partyKey || undefined,
        requestMode: item.request_mode ?? undefined,
        requestedFields: item.requested_fields,
      } as any);
      existing.add(normalize(item.name));
      added++;
    }
    toast.success(`${added} documento(s) adicionado(s) ao checklist.`);
    setPickerSelected({});
    setPickerQuery("");
    setOpenPicker(false);
  };

  const clearPartyChecklist = async (partyId: string | null) => {
    if (!dealId) return;
    const partyKey = partyId ?? "";
    const current = checklist.filter((i) => (partyKey ? i.partyId === partyKey : !i.partyId));
    for (const item of current) {
      await removeChecklistItem(dealId, item.id);
    }
    setReview(null);
    setFormReview(null);
    await extractionDocsQuery.refetch();
    toast.success("Checklist removido.");
  };

  const canRequestItem = (item: DocumentChecklistItem) =>
    item.status === "pending" && !item.solicited && (item.requestMode !== "form" || !!item.partyId);

  const statusMatches = (item: DocumentChecklistItem) => {
    if (statusFilter === "all") return true;
    if (statusFilter === "analysis") return item.status === "uploaded";
    if (statusFilter === "pending") return item.status === "pending" && !item.solicited;
    if (statusFilter === "solicited") return item.status === "pending" && !!item.solicited;
    return item.status === statusFilter;
  };

  const searchMatches = (item: DocumentChecklistItem) => {
    const q = search.trim().toLowerCase();
    if (!q) return true;
    return item.name.toLowerCase().includes(q) || (item.description ?? "").toLowerCase().includes(q);
  };

  const openConfig = (partyId?: string | null) => {
    setSelectedPartyId(partyId ?? "");
    setIsAddingItem(false);
    setConfigOpen(true);
  };

  const openBatchRequest = (partyId?: string) => {
    const scoped = checklist.filter((item) => {
      if (!canRequestItem(item)) return false;
      if (partyId === undefined) return true;
      if (partyId === "") return !item.partyId;
      return item.partyId === partyId;
    });
    if (scoped.length === 0) {
      toast.info("Não há documentos pendentes para solicitar neste escopo.");
      return;
    }
    setBatchRequest({
      partyId,
      label:
        partyId === undefined
          ? "todas as partes"
          : partyId === ""
            ? "Geral"
            : `${roleOptionLabel[partyRoleById[partyId]] ?? partyRoleById[partyId]} — ${partyNameById[partyId] ?? partyId}`,
      items: scoped,
    });
  };

  const confirmBatchRequest = async () => {
    if (!batchRequest || !dealId) return;
    await solicitAllPending(dealId, batchRequest.partyId);
    setBatchRequest(null);
  };

  const requestableTotal = checklist.filter(canRequestItem).length;

  const configScopes: Array<{
    key: string;
    partyId: string | null;
    initial: string;
    tabLabel: string;
    title: string;
    subtitle: string;
    items: DocumentChecklistItem[];
    templateId: ChecklistTemplateId | "";
    setTemplateId: (value: ChecklistTemplateId | "") => void;
  }> = [
    {
      key: "general",
      partyId: null,
      initial: "G",
      tabLabel: "Geral",
      title: "Geral",
      subtitle: "Itens que não pertencem a uma parte específica",
      items: checklist.filter((item) => !item.partyId),
      templateId: selectedTemplateGeneral,
      setTemplateId: setSelectedTemplateGeneral,
    },
    ...(isMultiParty
      ? parties.map((party) => ({
          key: party.id,
          partyId: party.id,
          initial: (partyNameById[party.id] ?? "?").charAt(0).toUpperCase(),
          tabLabel: partyNameById[party.id] ?? party.id,
          title: `${roleOptionLabel[party.role] ?? party.role} — ${partyNameById[party.id] ?? party.id}`,
          subtitle: party.email ?? "",
          items: checklist.filter((item) => item.partyId === party.id),
          templateId: selectedTemplateByParty[party.id] ?? "",
          setTemplateId: (value: ChecklistTemplateId | "") =>
            setSelectedTemplateByParty((prev) => ({ ...prev, [party.id]: value })),
        }))
      : []),
  ];

  const activeScope =
    configScopes.find((scope) => (scope.partyId ?? "") === selectedPartyId) ?? configScopes[0];
  if (!activeScope) return null;
  const activeTemplate = activeScope.templateId
    ? templates.find((template) => template.id === activeScope.templateId)
    : undefined;

  const groups = [
    {
      key: "general",
      partyId: "" as const,
      avatar: "G",
      title: "Geral",
      subtitle: isMultiParty ? "Itens que não pertencem a uma parte específica" : "Checklist do negócio",
      items: isMultiParty ? checklist.filter((i) => !i.partyId) : checklist,
    },
    ...(isMultiParty
      ? parties.map((p) => ({
          key: p.id,
          partyId: p.id,
          avatar: (partyNameById[p.id] ?? "?").charAt(0).toUpperCase(),
          title: `${roleOptionLabel[p.role] ?? p.role} — ${partyNameById[p.id] ?? p.id}`,
          subtitle: p.email ?? "",
          items: checklist.filter((i) => i.partyId === p.id),
        }))
      : []),
  ];

  const visibleCount = groups.reduce(
    (total, group) => total + group.items.filter((item) => statusMatches(item) && searchMatches(item)).length,
    0,
  );

  const renderChecklistItem = (item: DocumentChecklistItem) => {
    const status = item.status;
    const isApproved = status === "approved";
    const isUploaded = status === "uploaded";
    const isRejected = status === "rejected";
    const isPending = status === "pending";
    const requested = isPending && !!item.solicited;
    const linkedDocument = item.requestMode === "form" ? undefined : findLinkedDocument(item);
    const extractionProcessing =
      !!linkedDocument &&
      (!!extractionBusy[linkedDocument.id] ||
        linkedDocument.extraction_status === "pending" ||
        linkedDocument.extraction_status === "processing");
    const extractionReady =
      !!linkedDocument &&
      linkedDocument.extraction_status === "completed" &&
      (linkedDocument.extraction?.fields?.length ?? 0) > 0;

    const openFile = () => {
      const fileUrl = item.fileUrl || (dealId && item.id ? `/api/v1/deals/${dealId}/checklist/${item.id}/download` : "");
      if (!fileUrl) {
        toast.error("Arquivo ainda não disponível.");
        return;
      }
      setViewer({ url: withGatewayToken(toGatewayUrl(fileUrl), getAccessToken()), title: item.name });
    };

    const statusIcon = isApproved ? (
      <CheckCircle className="h-4 w-4" />
    ) : isUploaded ? (
      <Clock className="h-4 w-4" />
    ) : isRejected ? (
      <XCircle className="h-4 w-4" />
    ) : (
      <UploadCloud className="h-4 w-4" />
    );

    const statusClass = isApproved
      ? "bg-status-ok-soft text-status-ok"
      : isUploaded
        ? "bg-status-info-soft text-status-info"
        : isRejected
          ? "bg-status-critical-soft text-status-critical"
          : "bg-status-warning-soft text-status-warning";

    const partyLabel = item.partyId
      ? `${roleOptionLabel[partyRoleById[item.partyId]] ?? partyRoleById[item.partyId]} — ${partyNameById[item.partyId] ?? item.partyId}`
      : null;

    const statusBadge = isApproved ? (
      <Badge variant="success">Aprovado</Badge>
    ) : isUploaded ? (
      <Badge variant="info">Em análise</Badge>
    ) : isRejected ? (
      <Badge variant="critical">Rejeitado</Badge>
    ) : requested ? (
      <Badge variant="info">Solicitado</Badge>
    ) : (
      <Badge variant="warning">A solicitar</Badge>
    );

    return (
      <div
        key={item.id}
        className="flex flex-col gap-2 border-b border-border/60 py-3 last:border-0 sm:flex-row sm:items-start sm:justify-between sm:gap-3"
      >
        <div className="flex min-w-0 items-start gap-2.5">
          <span className={clsx("mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-full", statusClass)}>
            {statusIcon}
          </span>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-1.5">
              <p className="break-words text-sm font-semibold text-foreground">{item.name}</p>
              {item.ocrValidated && (
                <Badge variant="info" className="gap-0.5">
                  <Sparkles className="h-2.5 w-2.5" /> IA
                </Badge>
              )}
              {statusBadge}
            </div>
            <div className="mt-1 flex flex-wrap items-center gap-1.5 text-caption text-muted-foreground">
              <Badge variant="neutral">{CHECKLIST_CATEGORY_LABELS[item.category]}</Badge>
              {item.required && <Badge variant="warning">Obrigatório</Badge>}
              {partyLabel && <Badge variant="outline" className="max-w-full truncate">{partyLabel}</Badge>}
              {item.requestMode === "form" && <Badge variant="info">Apenas preenchimento</Badge>}
              {requested && item.solicitedAt && (
                <span>Solicitado em {new Date(item.solicitedAt).toLocaleDateString("pt-BR")}</span>
              )}
              {item.rejectionReason && <span className="font-medium text-status-critical">{item.rejectionReason}</span>}
              {isUploaded && <span className="font-medium text-status-info">Enviado pelo cliente</span>}
              {item.fileUrl && (
                <button type="button" className="font-bold text-status-ok hover:underline" onClick={openFile}>
                  Ver arquivo
                </button>
              )}
            </div>
          </div>
        </div>
        <div className="flex shrink-0 flex-wrap items-center gap-1.5 sm:justify-end">
          {isUploaded && (
            <Button
              size="sm"
              variant="outline"
              className="h-8 gap-1"
              disabled={item.requestMode !== "form" && extractionProcessing}
              title={
                !linkedDocument && item.requestMode !== "form"
                  ? "Reconcilia o documento no painel e dispara a IA"
                  : undefined
              }
              onClick={() => {
                if (item.requestMode === "form") openFormReview(item);
                else void openDocumentReview(item);
              }}
            >
              {item.requestMode === "form" ? (
                <FileText className="h-3.5 w-3.5" />
              ) : (
                <Sparkles className="h-3.5 w-3.5" />
              )}
              {item.requestMode === "form"
                ? "Revisar campos"
                : extractionProcessing
                  ? "IA processando..."
                  : extractionReady
                    ? "Revisar dados"
                    : linkedDocument
                      ? "Extrair com IA"
                      : "Reparar e extrair"}
            </Button>
          )}
          {isUploaded && item.requestMode !== "form" && (
            <Button size="sm" variant="outline" className="h-8 gap-1" onClick={openFile}>
              <FileText className="h-3.5 w-3.5" /> Ver arquivo
            </Button>
          )}
          {isUploaded && item.requestMode !== "form" && (
            <Button
              size="sm"
              className="h-8 gap-1"
              onClick={() => {
                void updateChecklistItemStatus(dealId, item.id, "approved");
              }}
            >
              <CheckCircle className="h-3.5 w-3.5" /> Aprovar
            </Button>
          )}
          {isPending && (
            <Button
              size="sm"
              variant="outline"
              className="h-8 gap-1"
              onClick={() => openSolicitDialog(item)}
            >
              <Send className="h-3.5 w-3.5" /> {requested ? "Reenviar" : "Solicitar"}
            </Button>
          )}
          {isRejected && (
            <Button size="sm" className="h-8 gap-1" onClick={() => openSolicitDialog(item)}>
              <Send className="h-3.5 w-3.5" /> Solicitar novamente
            </Button>
          )}
          {isApproved && item.fileUrl && (
            <Button size="sm" variant="outline" className="h-8 gap-1" onClick={openFile}>
              <FileText className="h-3.5 w-3.5" /> Ver arquivo
            </Button>
          )}
          {item.requestMode === "form" && !isUploaded && (
            <Button size="sm" variant="outline" className="h-8 gap-1" onClick={() => openFormReview(item)}>
              <FileText className="h-3.5 w-3.5" /> Ver campos
            </Button>
          )}
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="icon" className="h-8 w-8" aria-label={`Ações de ${item.name}`}>
                <MoreHorizontal className="h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-48">
              {item.requestMode === "form" && (
                <DropdownMenuItem onSelect={() => openFormReview(item)}>
                  <FileText className="mr-2 h-4 w-4" /> {isUploaded ? "Revisar campos" : "Ver campos"}
                </DropdownMenuItem>
              )}
              {!isApproved && item.requestMode !== "form" && (
                <DropdownMenuItem
                  onSelect={() => {
                    void updateChecklistItemStatus(dealId, item.id, "approved");
                  }}
                >
                  <CheckCircle className="mr-2 h-4 w-4" /> Aprovar
                </DropdownMenuItem>
              )}
              {(isUploaded || isPending) && (
                <DropdownMenuItem
                  onSelect={() => {
                    setRejectingId(item.id);
                    setRejectReason("");
                  }}
                >
                  <XCircle className="mr-2 h-4 w-4" /> Rejeitar
                </DropdownMenuItem>
              )}
              {(isPending || isRejected) && (
                <DropdownMenuItem onSelect={() => openSolicitDialog(item)}>
                  <Send className="mr-2 h-4 w-4" />{" "}
                  {isRejected ? "Solicitar novamente" : requested ? "Reenviar solicitação" : "Solicitar"}
                </DropdownMenuItem>
              )}
              {item.fileUrl && (
                <DropdownMenuItem onSelect={openFile}>
                  <FileText className="mr-2 h-4 w-4" /> Ver arquivo
                </DropdownMenuItem>
              )}
              <DropdownMenuSeparator />
              <DropdownMenuItem
                className="text-status-critical"
                onSelect={() =>
                  setConfirmAction({ kind: "remove-item", itemId: item.id, itemName: item.name })
                }
              >
                <Trash2 className="mr-2 h-4 w-4" /> Remover
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>
    );
  };

  return (
    <div className="space-y-5">
      {checklist.length === 0 ? (
        <div className="rounded-2xl border border-border bg-card p-6 text-center shadow-sm">
          <UploadCloud className="mx-auto mb-3 h-10 w-10 text-muted-foreground/60" />
          <h3 className="font-bold text-foreground">Checklist de Documentos</h3>
          <p className="mt-1 text-sm text-muted-foreground">
            Selecione um modelo para iniciar o checklist deste negócio ou configure por parte.
          </p>
          <div className="mx-auto mt-4 max-w-sm space-y-2">
            {templates.map((tpl) => (
              <button
                key={tpl.id}
                type="button"
                onClick={() => {
                  if (dealId) void initDealChecklist(dealId, tpl.id as ChecklistTemplateId);
                }}
                className="w-full rounded-xl border border-border bg-muted px-4 py-3 text-left transition-colors hover:border-primary/30 hover:bg-primary/10"
              >
                <span className="text-sm font-bold text-foreground">{tpl.label}</span>
                <span className="ml-2 text-xs text-muted-foreground">({tpl.items.length} docs)</span>
              </button>
            ))}
          </div>
          <div className="mt-4 flex flex-wrap justify-center gap-2">
            <Button variant="outline" size="sm" onClick={() => openConfig()}>
              <Settings2 className="h-3.5 w-3.5" /> Configurar pacotes
            </Button>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                openConfig();
                setIsAddingItem(true);
              }}
            >
              <Plus className="h-3.5 w-3.5" /> Criar checklist personalizado
            </Button>
          </div>
        </div>
      ) : (
        <>
          <DocumentationSummary
            stats={dealDetails?.checklist_stats}
            checklist={checklist}
            partiesCount={Math.max(parties.length, 1)}
            requestableTotal={requestableTotal}
            activeFilter={statusFilter}
            onFilter={setStatusFilter}
            onConfigure={() => openConfig()}
            onBatch={() => openBatchRequest()}
          />

            <PendingReviewsQueue
              groups={visibleReviewGroups}
              counts={reviewCounts}
              filter={reviewFilter}
              onFilter={setReviewFilter}
              onReviewOcr={(item) => {
                if (item.document && item.party) {
                  setReview({ party: item.party, document: item.document, item: item.item });
                }
              }}
              onReviewForm={(item) => {
                if (item.party && item.item) {
                  setFormReview({ party: item.party, item: item.item });
                }
              }}
              onReprocessOcr={async (item) => {
                if (!item.document) return;
                try {
                  await documentService.reExtract(item.document.id);
                  toast.success("OCR reenfileirado. A revisão será atualizada quando terminar.");
                  await extractionDocsQuery.refetch();
                } catch {
                  toast.error("Não foi possível reenfileirar o OCR.");
                }
              }}
            />

            {review && (
              <ReviewExtractedDataDialog
                open={!!review}
                onOpenChange={(o) => {
                  if (!o) setReview(null);
                }}
                dealId={dealId}
                party={review.party}
                document={review.document}
                extraction={review.document.extraction}
                item={review.item}
                onApprove={review.item ? async () => {
                  await updateChecklistItemStatus(dealId, review.item!.id, "approved");
                  toast.success("Documento aprovado.");
                  await extractionDocsQuery.refetch();
                  try {
                    await fetchDealById(dealId);
                  } catch {
                    /* refresh best-effort */
                  }
                } : undefined}
                onRequestCorrection={review.item ? async (reason) => {
                  await updateChecklistItemStatus(dealId, review.item!.id, "rejected", reason);
                  toast.success("Correção solicitada ao cliente.");
                  await extractionDocsQuery.refetch();
                  try {
                    await fetchDealById(dealId);
                  } catch {
                    /* refresh best-effort */
                  }
                } : undefined}
                onReprocess={async () => {
                  if (!review.document) return;
                  await documentService.reExtract(review.document.id);
                  toast.success("Extração de IA reenfileirada.");
                  setReview(null);
                  await extractionDocsQuery.refetch();
                }}
                onApplied={async () => {
                  toast.success("Dados aplicados à parte.");
                  await extractionDocsQuery.refetch();
                  try {
                    await fetchDealById(dealId);
                  } catch {
                    /* refresh best-effort — a aba de Partes recarrega no próximo load */
                  }
                }}
              />
            )}


                      {formReview && (
              <ReviewFormSubmissionDialog
                open={!!formReview}
                onOpenChange={(o) => {
                  if (!o) setFormReview(null);
                }}
                dealId={dealId}
                party={formReview.party}
                item={formReview.item}
                onReviewed={async () => {
                  toast.success("Formulário revisado.");
                  setFormReview(null);
                  await fetchDealById(dealId);
                  await extractionDocsQuery.refetch();
                }}
              />
            )}

          <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
            <div>
              <h3 className="text-title font-bold text-foreground">Pendências por parte</h3>
              <p className="text-caption text-muted-foreground">
                A parte é o eixo: veja o que falta, para quem, e aja no contexto.
              </p>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <div className="relative">
                <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
                <input
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                  placeholder="Buscar documento…"
                  className="h-9 w-full rounded-lg border border-border bg-card pl-8 pr-3 text-xs text-foreground outline-none focus:border-primary sm:w-56"
                />
              </div>
              <Button variant="outline" size="sm" className="h-9 gap-1.5" onClick={() => openConfig()}>
                <Settings2 className="h-3.5 w-3.5" /> Configurar
              </Button>
            </div>
          </div>

          <div className="flex flex-col gap-3">
            {groups.map((group) => {
              const visible = group.items.filter((item) => statusMatches(item) && searchMatches(item));
              if ((statusFilter !== "all" || search.trim()) && visible.length === 0) return null;
              const approved = group.items.filter((item) => item.status === "approved").length;
              const uploaded = group.items.filter((item) => item.status === "uploaded").length;
              const required = group.items.filter((item) => item.required).length;
              const requiredApproved = group.items.filter(
                (item) => item.required && item.status === "approved",
              ).length;
              const requestable = group.items.filter(canRequestItem).length;
              return (
                <PartyGroup
                  key={group.key}
                  avatar={group.avatar}
                  title={group.title}
                  subtitle={group.subtitle}
                  total={group.items.length}
                  approved={approved}
                  uploaded={uploaded}
                  required={required}
                  requiredApproved={requiredApproved}
                  defaultOpen={group.key === "general" || group.items.some((item) => item.status !== "approved")}
                  forceOpen={statusFilter !== "all" || !!search.trim()}
                  actions={
                    <>
                      {requestable > 0 && (
                        <Button
                          size="sm"
                          variant="outline"
                          className="h-8 gap-1 border-primary/20 bg-primary/10 text-xs font-bold text-primary hover:bg-primary/15 hover:text-primary"
                          onClick={() => openBatchRequest(group.partyId)}
                        >
                          <Send className="h-3.5 w-3.5" /> Solicitar {requestable}
                        </Button>
                      )}
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-8 w-8"
                            aria-label={`Ações de ${group.title}`}
                          >
                            <MoreHorizontal className="h-4 w-4" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end" className="w-52">
                          <DropdownMenuItem onSelect={() => openConfig(group.partyId)}>
                            <Settings2 className="mr-2 h-4 w-4" /> Configurar pacotes
                          </DropdownMenuItem>
                          <DropdownMenuItem
                            onSelect={() => {
                              setSelectedPartyId(group.partyId);
                              setIsAddingItem(true);
                              setConfigOpen(true);
                            }}
                          >
                            <Plus className="mr-2 h-4 w-4" /> Adicionar documento
                          </DropdownMenuItem>
                          <DropdownMenuItem
                            onSelect={() => {
                              setSelectedPartyId(group.partyId);
                              setOpenPicker(true);
                            }}
                          >
                            <Paperclip className="mr-2 h-4 w-4" /> Selecionar da biblioteca
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem
                            className="text-status-critical"
                            onSelect={() => setConfirmAction({ kind: "clear", partyId: group.partyId || null })}
                          >
                            <Trash2 className="mr-2 h-4 w-4" /> Limpar checklist
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </>
                  }
                >
                  {visible.length === 0 ? (
                    <p className="py-3 text-xs text-muted-foreground">Nenhum documento neste escopo.</p>
                  ) : (
                    visible.map(renderChecklistItem)
                  )}
                </PartyGroup>
              );
            })}
            {visibleCount === 0 && (
              <div className="rounded-2xl border border-border bg-card p-6 text-center text-sm text-muted-foreground">
                Nenhum documento encontrado com esses filtros.
              </div>
            )}
          </div>

                      {/* Reject reason modal inline */}
            {rejectingId && (
              <div className="mt-3 p-4 bg-status-critical-soft rounded-xl border border-status-critical/30 animate-in fade-in">
                <h4 className="text-sm font-bold text-status-critical mb-2">Motivo da Rejeição</h4>
                <input
                  value={rejectReason}
                  onChange={(e) => setRejectReason(e.target.value)}
                  placeholder="Ex: Documento ilegível, fora da validade..."
                  className="w-full px-3 py-2 bg-card border border-status-critical/30 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-status-critical mb-2"
                  onKeyDown={(e) => e.key === "Enter" && handleReject(rejectingId)}
                />
                <div className="flex gap-2">
                  <Button variant="ghost" size="sm" onClick={() => setRejectingId(null)} className="flex-1 text-xs font-bold">
                    Cancelar
                  </Button>
                  <Button variant="destructive" size="sm" onClick={() => handleReject(rejectingId)} className="flex-1 text-xs font-bold">
                    Rejeitar
                  </Button>
                </div>
              </div>
            )}


        </>
      )}

      <SignedDocumentsSection contract={latestContract ?? null} title="Contrato assinado" />
      <DealDocumentsSection dealId={dealId} />

            <ExpandableSidePanel
              open={openPicker}
              onClose={() => setOpenPicker(false)}
              defaultSize="lg"
              expandedSize="2xl"
              zIndex={70}
              renderHeader={({ expandButton, closeButton }) => (
                <div className="flex-none px-6 py-5 border-b border-border bg-card">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <h2 className="text-lg font-bold text-foreground">Selecionar documentos</h2>
                      <p className="text-sm text-muted-foreground mt-0.5">
                        Adicione documentos ao checklist de {selectedPartyId ? "uma parte" : "Geral"} sem aplicar um pacote completo.
                      </p>
                    </div>
                    <div className="flex items-center gap-2 shrink-0">
                      {expandButton}
                      {closeButton}
                    </div>
                  </div>
                </div>
              )}
            >
              <div className="flex-1 overflow-y-auto px-6 py-5 space-y-4">
                <div className="space-y-1">
                  <div className="text-xs font-semibold text-muted-foreground">Buscar</div>
                  <input
                    value={pickerQuery}
                    onChange={(e) => setPickerQuery(e.target.value)}
                    placeholder="Ex: CPF, RG, certidão, procuração..."
                    className="w-full px-3 py-2 bg-card border border-border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary"
                  />
                </div>

                <div className="space-y-3">
                  {templates.map((tpl) => {
                    const items = tpl.items.filter((i) => {
                      if (!pickerQuery.trim()) return true;
                      const q = pickerQuery.trim().toLowerCase();
                      return i.name.toLowerCase().includes(q) || (i.description ?? "").toLowerCase().includes(q);
                    });
                    if (items.length === 0) return null;
                    return (
                      <div key={tpl.id} className="border border-border rounded-2xl bg-card overflow-hidden">
                        <div className="px-4 py-3 border-b border-border flex items-center justify-between">
                          <div className="text-sm font-bold text-foreground">{tpl.label}</div>
                          <div className="text-xs text-muted-foreground">{items.length}</div>
                        </div>
                        <div className="divide-y divide-border">
                          {items.map((it) => {
                            const key = `${tpl.id}::${it.name}`;
                            const checked = !!pickerSelected[key];
                            return (
                              <label key={key} className="px-4 py-3 flex items-start gap-3 cursor-pointer hover:bg-muted">
                                <input
                                  type="checkbox"
                                  checked={checked}
                                  onChange={(e) =>
                                    setPickerSelected((prev) => ({
                                      ...prev,
                                      [key]: e.target.checked,
                                    }))
                                  }
                                  className="mt-1 rounded border-border text-primary focus:ring-primary"
                                />
                                <div className="min-w-0">
                                  <div className="text-sm font-semibold text-foreground">{it.name}</div>
                                  <div className="mt-1 flex items-center gap-2 flex-wrap">
                                    <span className="text-micro bg-muted text-muted-foreground px-1.5 py-0.5 rounded font-medium">
                                      {CHECKLIST_CATEGORY_LABELS[it.category]}
                                    </span>
                                    {it.request_mode === "form" && (
                                      <span className="text-micro bg-status-info-soft text-status-info px-1.5 py-0.5 rounded font-bold border border-status-info/20">
                                        Apenas preenchimento
                                      </span>
                                    )}
                                    {it.required && (
                                      <span className="text-micro bg-status-warning-soft text-status-warning px-1.5 py-0.5 rounded font-bold border border-status-warning/20">
                                        Obrigatório
                                      </span>
                                    )}
                                  </div>
                                  {it.description && <div className="text-xs text-muted-foreground mt-1">{it.description}</div>}
                                </div>
                              </label>
                            );
                          })}
                        </div>
                      </div>
                    );
                  })}
                </div>
              </div>

              <div className="flex-none px-6 py-4 border-t border-border bg-card">
                <div className="flex justify-end gap-2">
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() => {
                      setPickerSelected({});
                      setPickerQuery("");
                      setOpenPicker(false);
                    }}
                  >
                    Cancelar
                  </Button>
                  <Button
                    type="button"
                    onClick={applyPickerSelection}
                  >
                    Adicionar selecionados
                  </Button>
                </div>
              </div>
            </ExpandableSidePanel>

      <Sheet
        open={configOpen}
        onOpenChange={(open) => {
          setConfigOpen(open);
          if (!open) setIsAddingItem(false);
        }}
      >
        <SheetContent side="right" className="w-full sm:max-w-[560px]">
          <SheetHeader>
            <SheetTitle>Configurar pacotes</SheetTitle>
            <SheetDescription>
              Aplique um pacote ou adicione documentos a uma parte por vez.
            </SheetDescription>
          </SheetHeader>
          <div className="flex-1 space-y-4 overflow-y-auto px-4 pb-6">
            <div>
              <p className="text-micro font-bold uppercase tracking-wide text-muted-foreground">Parte</p>
              <div className="mt-2 flex gap-1.5 overflow-x-auto pb-1" role="tablist" aria-label="Selecionar parte">
                {configScopes.map((scope) => {
                  const active = (scope.partyId ?? "") === selectedPartyId;
                  return (
                    <button
                      key={scope.key}
                      type="button"
                      role="tab"
                      aria-selected={active}
                      onClick={() => {
                        setSelectedPartyId(scope.partyId ?? "");
                        setIsAddingItem(false);
                      }}
                      className={clsx(
                        "flex shrink-0 items-center gap-1.5 rounded-full border px-2.5 py-1.5 text-caption font-semibold transition-colors",
                        active
                          ? "border-primary/40 bg-primary/10 text-primary"
                          : "border-border bg-card text-muted-foreground hover:bg-muted",
                      )}
                    >
                      <span className="flex h-5 w-5 items-center justify-center rounded-full bg-card text-micro font-bold">
                        {scope.initial}
                      </span>
                      <span className="max-w-32 truncate">{scope.tabLabel}</span>
                      <span
                        className={clsx(
                          "rounded-full border px-1.5 text-micro",
                          active
                            ? "border-primary/20 bg-card text-primary"
                            : "border-border bg-muted text-muted-foreground",
                        )}
                      >
                        {scope.items.length}
                      </span>
                    </button>
                  );
                })}
              </div>
            </div>

            <div className="flex items-start gap-2.5">
              <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full border border-primary/20 bg-primary/10 text-xs font-bold text-primary">
                {activeScope.initial}
              </span>
              <div className="min-w-0">
                <div className="break-words text-sm font-bold text-foreground">{activeScope.title}</div>
                <div className="break-words text-caption text-muted-foreground">{activeScope.subtitle}</div>
              </div>
              <Badge variant="neutral" className="ml-auto shrink-0">
                {activeScope.items.length} {activeScope.items.length === 1 ? "item" : "itens"}
              </Badge>
            </div>

            <section className="rounded-2xl border border-border bg-card p-4 shadow-sm">
              <div className="flex items-center justify-between gap-2">
                <h3 className="text-sm font-bold text-foreground">Checklist atual</h3>
                <span className="text-caption text-muted-foreground">
                  {activeScope.items.length} {activeScope.items.length === 1 ? "documento" : "documentos"}
                </span>
              </div>
              {activeScope.items.length === 0 ? (
                <p className="mt-2 text-caption text-muted-foreground">
                  Nenhum item neste escopo. Aplique um pacote para começar.
                </p>
              ) : (
                <ul className="mt-2 max-h-52 space-y-1.5 overflow-y-auto pr-1">
                  {activeScope.items.map((item) => (
                    <li key={item.id} className="flex items-start gap-2 text-caption">
                      <span
                        className={clsx("mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full", checklistStatusDot(item))}
                        aria-hidden="true"
                      />
                      <span className="min-w-0 flex-1">
                        <span className="block break-words text-sm font-medium text-foreground">{item.name}</span>
                        <span className="text-micro text-muted-foreground">
                          {CHECKLIST_CATEGORY_LABELS[item.category]}
                          {item.required ? " · Obrigatório" : ""}
                          {item.requestMode === "form" ? " · Apenas preenchimento" : ""}
                        </span>
                      </span>
                      <span className="shrink-0 text-micro text-muted-foreground">
                        {checklistStatusLabel(item)}
                      </span>
                    </li>
                  ))}
                </ul>
              )}
            </section>

            <section className="rounded-2xl border border-border bg-card p-4 shadow-sm">
              <div className="flex items-center justify-between gap-2">
                <h3 className="text-sm font-bold text-foreground">Aplicar um pacote</h3>
                <span className="text-micro text-muted-foreground">modelo</span>
              </div>
              <label
                htmlFor="config-template"
                className="mt-2 block text-micro font-bold uppercase tracking-wide text-muted-foreground"
              >
                {activeScope.templateId ? "Modelo selecionado (rascunho)" : "Modelo de documentos"}
              </label>
              <div className="mt-1 flex flex-col gap-2 sm:flex-row">
                <select
                  id="config-template"
                  value={activeScope.templateId}
                  onChange={(e) => activeScope.setTemplateId(e.target.value as ChecklistTemplateId | "")}
                  className="h-9 min-w-0 flex-1 rounded-lg border border-border bg-background px-3 text-sm focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20"
                >
                  <option value="">Selecione um modelo…</option>
                  {templates.map((tpl) => (
                    <option key={tpl.id} value={tpl.id}>
                      {tpl.label} ({tpl.items.length} docs)
                    </option>
                  ))}
                </select>
                <Button
                  size="sm"
                  className="h-9 shrink-0"
                  disabled={!activeScope.templateId}
                  onClick={() => {
                    if (activeScope.templateId) {
                      void applyTemplate(
                        activeScope.templateId as ChecklistTemplateId,
                        activeScope.partyId,
                        "append",
                      );
                    }
                  }}
                >
                  Aplicar pacote
                </Button>
              </div>

              {activeTemplate ? (
                <div className="mt-3 rounded-xl border border-border/60 bg-muted/30 px-3 py-2">
                  <div className="flex items-center justify-between gap-2">
                    <span className="text-micro font-bold uppercase tracking-wide text-muted-foreground">
                      O pacote inclui
                    </span>
                    <span className="text-micro text-muted-foreground">
                      {activeTemplate.items.length} documentos
                    </span>
                  </div>
                  <ul className="mt-1.5 max-h-40 space-y-1 overflow-y-auto pr-1">
                    {activeTemplate.items.map((item, index) => (
                      <li
                        key={`${item.name}-${index}`}
                        className="flex items-start gap-1.5 text-caption text-foreground"
                      >
                        <CheckCircle className="mt-0.5 h-3 w-3 shrink-0 text-status-ok" />
                        <span className="min-w-0 break-words">{item.name}</span>
                      </li>
                    ))}
                  </ul>
                </div>
              ) : (
                <p className="mt-3 rounded-xl border border-dashed border-border px-3 py-2 text-caption text-muted-foreground">
                  Selecione um modelo para ver os documentos do pacote.
                </p>
              )}
              <p className="mt-2 text-caption text-muted-foreground">
                Aplicar complementa o checklist; não remove os itens atuais.
              </p>

              <div className="mt-3 border-t border-border pt-2">
                <div className="text-micro font-bold uppercase tracking-wide text-muted-foreground">
                  Ações avançadas
                </div>
                <div className="mt-1 flex flex-wrap items-center gap-3">
                  <button
                    type="button"
                    disabled={!activeScope.templateId}
                    className="text-caption font-semibold text-status-warning hover:underline disabled:cursor-not-allowed disabled:opacity-40"
                    onClick={() => {
                      if (activeScope.templateId) {
                        setConfirmAction({
                          kind: "replace",
                          partyId: activeScope.partyId,
                          templateId: activeScope.templateId as ChecklistTemplateId,
                        });
                      }
                    }}
                  >
                    Substituir pacote
                  </button>
                  <button
                    type="button"
                    disabled={activeScope.items.length === 0}
                    className="text-caption font-semibold text-status-critical hover:underline disabled:cursor-not-allowed disabled:opacity-40"
                    onClick={() => setConfirmAction({ kind: "clear", partyId: activeScope.partyId })}
                  >
                    Limpar checklist
                  </button>
                </div>
              </div>
            </section>

            {isAddingItem && (
              <p className="text-caption text-muted-foreground">
                Adicionando em <b className="text-foreground">{activeScope.title}</b>
              </p>
            )}
            {/* Add item form */}
            {isAddingItem && (
              <div className="mb-4 p-4 bg-muted rounded-xl border border-border animate-in fade-in slide-in-from-top-2">
                <h4 className="font-bold text-sm text-foreground mb-3">Novo Documento</h4>
                <div className="space-y-3">
                  <input
                    value={newItemName}
                    onChange={(e) => setNewItemName(e.target.value)}
                    placeholder="Nome do documento"
                    className="w-full px-3 py-2 bg-card border border-border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary"
                  />
                  <div className="flex gap-3">
                    <select
                      value={newItemCategory}
                      onChange={(e) => setNewItemCategory(e.target.value as ChecklistItemCategory)}
                      className="flex-1 px-3 py-2 bg-card border border-border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary"
                    >
                      {(Object.entries(CHECKLIST_CATEGORY_LABELS) as [ChecklistItemCategory, string][]).map(([k, v]) => (
                        <option key={k} value={k}>
                          {v}
                        </option>
                      ))}
                    </select>
                    <label className="flex items-center gap-2 text-sm text-foreground cursor-pointer">
                      <input
                        type="checkbox"
                        checked={newItemRequired}
                        onChange={(e) => setNewItemRequired(e.target.checked)}
                        className="rounded border-border text-primary focus:ring-primary"
                      />
                      Obrigatório
                    </label>
                    <label className="flex items-center gap-2 text-sm text-foreground cursor-pointer" title="O cliente preenche os dados no Portal sem anexar arquivo (ex.: CPF, telefone).">
                      <input
                        type="checkbox"
                        checked={newItemMode === "form"}
                        onChange={(e) => setNewItemMode(e.target.checked ? "form" : "upload")}
                        className="rounded border-border text-primary focus:ring-primary"
                      />
                      Apenas preenchimento (sem anexo)
                    </label>
                  </div>
                  <div className="flex gap-2 pt-1">
                    <Button variant="ghost" size="sm" onClick={() => setIsAddingItem(false)} className="flex-1 text-xs font-bold">
                      Cancelar
                    </Button>
                    <Button size="sm" onClick={handleAddItem} className="flex-1 text-xs font-bold">
                      Adicionar
                    </Button>
                  </div>
                </div>
              </div>
            )}
          </div>
          <SheetFooter className="border-t border-border pt-3">
            <div className="w-full">
              <p className="mb-2 text-caption text-muted-foreground">
                Adicionar em <b className="text-foreground">{activeScope.title}</b>
              </p>
              <div className="flex justify-between gap-2">
                <Button variant="outline" size="sm" className="gap-1.5" onClick={() => setOpenPicker(true)}>
                  <Paperclip className="h-3.5 w-3.5" /> Selecionar docs
                </Button>
                <Button size="sm" className="gap-1.5" onClick={() => setIsAddingItem(true)}>
                  <Plus className="h-3.5 w-3.5" /> Adicionar documento
                </Button>
              </div>
            </div>
          </SheetFooter>
        </SheetContent>
      </Sheet>

        <ConfirmDialog
          open={!!confirmAction}
          title={
            confirmAction?.kind === "replace"
              ? "Substituir checklist"
              : confirmAction?.kind === "remove-item"
                ? "Remover item do checklist"
                : "Limpar checklist"
          }
          description={
            confirmAction?.kind === "replace"
              ? "Isso vai remover os documentos atuais desse escopo e aplicar o novo modelo."
              : confirmAction?.kind === "remove-item"
                ? `Remover "${confirmAction.itemName}" deste checklist? Esta ação não pode ser desfeita.`
                : "Isso vai remover os documentos atuais desse escopo."
          }
          confirmLabel={
            confirmAction?.kind === "replace"
              ? "Substituir"
              : confirmAction?.kind === "remove-item"
                ? "Remover"
                : "Limpar"
          }
          cancelLabel="Cancelar"
          variant={confirmAction?.kind === "replace" ? "warning" : "danger"}
          onCancel={() => setConfirmAction(null)}
          onConfirm={async () => {
            const action = confirmAction;
            if (!action) return;
            try {
              if (action.kind === "remove-item") {
                await removeChecklistItem(dealId, action.itemId);
                if (review?.item?.id === action.itemId) setReview(null);
                if (formReview?.item?.id === action.itemId) setFormReview(null);
                await extractionDocsQuery.refetch();
              } else if (action.kind === "clear") {
                await clearPartyChecklist(action.partyId);
              } else {
                await applyTemplate(action.templateId, action.partyId, "replace");
              }
              setConfirmAction(null);
            } catch (e) {
              toast.error("Não foi possível aplicar a alteração no checklist.");
              // Mantém o diálogo aberto para nova tentativa.
              throw e;
            }
          }}
        />

        {viewer && (
          <DocumentViewerDialog open={!!viewer} onOpenChange={(o) => !o && setViewer(null)} url={viewer.url} title={viewer.title} />
        )}

                  <Dialog open={!!requesting} onOpenChange={(o) => !o && setRequesting(null)}>
              <DialogContent className="max-w-lg">
                <DialogHeader>
                  <DialogTitle>Solicitar dados antes do upload</DialogTitle>
                  {solicitParty && (
                    <p className="text-xs text-muted-foreground">
                      Solicitando para{" "}
                      <span className="font-semibold text-foreground">{solicitParty.name || solicitParty.email || "parte"}</span>{" "}
                      ({roleOptionLabel[solicitParty.role] ?? solicitParty.role})
                    </p>
                  )}
                </DialogHeader>

                <div className="space-y-4">
                  <div className="space-y-1.5">
                    <label className="text-xs font-bold text-foreground">Parte do negócio</label>
                    {parties.length === 0 ? (
                      <div className="rounded-xl border border-status-warning/30 bg-status-warning-soft p-3">
                        <p className="text-xs text-status-warning font-medium">
                          Este negócio ainda não tem partes cadastradas. Cadastre uma parte no painel de Partes para solicitar dados.
                        </p>
                      </div>
                    ) : (
                      <>
                        <select
                          value={requesting?.partyId ?? ""}
                          onChange={(e) => setRequesting((r) => (r ? { ...r, partyId: e.target.value || undefined } : r))}
                          className="h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                        >
                          <option value="">Selecione a parte…</option>
                          {parties.map((p) => (
                            <option key={p.id} value={p.id}>
                              {(roleOptionLabel[p.role] ?? p.role)} — {p.name || p.email || p.id}
                            </option>
                          ))}
                        </select>
                        {!requesting?.partyId && (
                          <p className="text-xs text-status-warning font-medium">
                            Selecione a parte que vai receber a solicitação no Portal.
                          </p>
                        )}
                      </>
                    )}
                  </div>

                  <div className="space-y-1.5">
                    <label className="text-xs font-bold text-foreground">Tipo de solicitação</label>
                    <div className="grid grid-cols-2 gap-2">
                      <button
                        type="button"
                        onClick={() => setRequestedMode("upload")}
                        className={clsx(
                          "rounded-lg border px-3 py-2 text-xs font-bold transition-colors",
                          requestedMode === "upload"
                            ? "border-primary bg-primary/10 text-primary"
                            : "border-border bg-card text-muted-foreground hover:bg-muted",
                        )}
                      >
                        Anexar documento
                      </button>
                      <button
                        type="button"
                        onClick={() => setRequestedMode("form")}
                        className={clsx(
                          "rounded-lg border px-3 py-2 text-xs font-bold transition-colors",
                          requestedMode === "form"
                            ? "border-primary bg-primary/10 text-primary"
                            : "border-border bg-card text-muted-foreground hover:bg-muted",
                        )}
                      >
                        Apenas preenchimento (formulário)
                      </button>
                    </div>
                    {requestedMode === "form" && (
                      <p className="text-xs text-status-info font-medium">
                        O cliente não anexará arquivo — a solicitação é cumprida quando os campos abaixo forem
                        preenchidos no Portal (ex.: CPF e telefone).
                      </p>
                    )}
                  </div>

                  <div className="space-y-2">
                    <div className="flex items-center justify-between gap-3">
                      <p className="text-xs font-bold text-foreground/80">Dados do cliente</p>
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        disabled={!requesting?.partyId}
                        onClick={() =>
                          setRequestedBase((p) => ({
                            ...p,
                            full_name: { enabled: true, required: true },
                            cpf: { enabled: true, required: true },
                          }))
                        }
                        className="text-xs font-bold"
                      >
                        Nome + CPF
                      </Button>
                    </div>

                    <div className="flex items-center justify-between gap-3">
                      <div className="flex items-center gap-2 min-w-0">
                        <label className="flex items-center gap-2 text-sm font-medium text-foreground cursor-pointer">
                          <input
                            type="checkbox"
                            checked={requestedBase.full_name.enabled}
                            disabled={!requesting?.partyId}
                            onChange={(e) => setRequestedBase((p) => ({ ...p, full_name: { ...p.full_name, enabled: e.target.checked } }))}
                            className="rounded border-border text-primary focus:ring-primary"
                          />
                          Nome completo
                        </label>
                        {fieldStatusBadge(!!(solicitParty?.name ?? "").trim())}
                      </div>
                      <label className={clsx("flex items-center gap-2 text-xs font-bold", requestedBase.full_name.enabled ? "text-foreground" : "text-muted-foreground")}>
                        <input
                          type="checkbox"
                          disabled={!requesting?.partyId || !requestedBase.full_name.enabled}
                          checked={requestedBase.full_name.required}
                          onChange={(e) => setRequestedBase((p) => ({ ...p, full_name: { ...p.full_name, required: e.target.checked } }))}
                          className="rounded border-border text-primary focus:ring-primary"
                        />
                        Obrigatório
                      </label>
                    </div>

                    <div className="flex items-center justify-between gap-3">
                      <div className="flex items-center gap-2 min-w-0">
                        <label className="flex items-center gap-2 text-sm font-medium text-foreground cursor-pointer">
                          <input
                            type="checkbox"
                            checked={requestedBase.cpf.enabled}
                            disabled={!requesting?.partyId}
                            onChange={(e) => setRequestedBase((p) => ({ ...p, cpf: { ...p.cpf, enabled: e.target.checked } }))}
                            className="rounded border-border text-primary focus:ring-primary"
                          />
                          CPF
                        </label>
                        {fieldStatusBadge(!!(solicitParty?.document ?? "").trim())}
                      </div>
                      <label className={clsx("flex items-center gap-2 text-xs font-bold", requestedBase.cpf.enabled ? "text-foreground" : "text-muted-foreground")}>
                        <input
                          type="checkbox"
                          disabled={!requesting?.partyId || !requestedBase.cpf.enabled}
                          checked={requestedBase.cpf.required}
                          onChange={(e) => setRequestedBase((p) => ({ ...p, cpf: { ...p.cpf, required: e.target.checked } }))}
                          className="rounded border-border text-primary focus:ring-primary"
                        />
                        Obrigatório
                      </label>
                    </div>

                    <div className="flex items-center justify-between gap-3">
                      <div className="flex items-center gap-2 min-w-0">
                        <label className="flex items-center gap-2 text-sm font-medium text-foreground cursor-pointer">
                          <input
                            type="checkbox"
                            checked={requestedBase.birth_date.enabled}
                            disabled={!requesting?.partyId}
                            onChange={(e) => setRequestedBase((p) => ({ ...p, birth_date: { ...p.birth_date, enabled: e.target.checked } }))}
                            className="rounded border-border text-primary focus:ring-primary"
                          />
                          Data de nascimento
                        </label>
                        {fieldStatusBadge(!!(solicitParty?.birth_date ?? "").trim())}
                      </div>
                      <label className={clsx("flex items-center gap-2 text-xs font-bold", requestedBase.birth_date.enabled ? "text-foreground" : "text-muted-foreground")}>
                        <input
                          type="checkbox"
                          disabled={!requesting?.partyId || !requestedBase.birth_date.enabled}
                          checked={requestedBase.birth_date.required}
                          onChange={(e) => setRequestedBase((p) => ({ ...p, birth_date: { ...p.birth_date, required: e.target.checked } }))}
                          className="rounded border-border text-primary focus:ring-primary"
                        />
                        Obrigatório
                      </label>
                    </div>

                    <div className="flex items-center justify-between gap-3">
                      <div className="flex items-center gap-2 min-w-0">
                        <label className="flex items-center gap-2 text-sm font-medium text-foreground cursor-pointer">
                          <input
                            type="checkbox"
                            checked={requestedBase.phone.enabled}
                            disabled={!requesting?.partyId}
                            onChange={(e) => setRequestedBase((p) => ({ ...p, phone: { ...p.phone, enabled: e.target.checked } }))}
                            className="rounded border-border text-primary focus:ring-primary"
                          />
                          Telefone/WhatsApp
                        </label>
                        {fieldStatusBadge(!!(solicitParty?.phone ?? "").trim())}
                      </div>
                      <label className={clsx("flex items-center gap-2 text-xs font-bold", requestedBase.phone.enabled ? "text-foreground" : "text-muted-foreground")}>
                        <input
                          type="checkbox"
                          disabled={!requesting?.partyId || !requestedBase.phone.enabled}
                          checked={requestedBase.phone.required}
                          onChange={(e) => setRequestedBase((p) => ({ ...p, phone: { ...p.phone, required: e.target.checked } }))}
                          className="rounded border-border text-primary focus:ring-primary"
                        />
                        Obrigatório
                      </label>
                    </div>
                  </div>

                  <div className="pt-4 border-t border-border">
                    <div className="flex items-center justify-between gap-3 mb-2">
                      <p className="text-xs font-bold text-foreground/80">Campos personalizados</p>
                      <div className="flex items-center gap-3">
                        <label className="flex items-center gap-1.5 text-[11px] font-medium text-muted-foreground cursor-pointer">
                          <input
                            type="checkbox"
                            checked={showAdvancedKey}
                            onChange={(e) => setShowAdvancedKey(e.target.checked)}
                            className="rounded border-border text-primary focus:ring-primary"
                          />
                          Chave técnica
                        </label>
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          disabled={!requesting?.partyId}
                          onClick={() => setRequestedCustom((prev) => [...prev, { key: "", label: "", type: "text", required: true }])}
                          className="text-xs font-bold"
                        >
                          Adicionar campo
                        </Button>
                      </div>
                    </div>
                    <div className="space-y-2">
                      {requestedCustom.map((f, idx) => (
                        <div key={`${f.key}-${idx}`} className="grid grid-cols-1 sm:grid-cols-12 gap-2 items-center">
                          <input
                            value={f.label}
                            onChange={(e) => setRequestedCustom((p) => p.map((x, i) => (i === idx ? { ...x, label: e.target.value } : x)))}
                            disabled={!requesting?.partyId}
                            placeholder="Label (ex: Profissão)"
                            className={clsx(
                              showAdvancedKey ? "sm:col-span-5" : "sm:col-span-8",
                              "w-full px-3 py-2 bg-card border border-border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary",
                            )}
                          />
                          {showAdvancedKey && (
                            <input
                              value={f.key}
                              onChange={(e) => setRequestedCustom((p) => p.map((x, i) => (i === idx ? { ...x, key: e.target.value } : x)))}
                              disabled={!requesting?.partyId}
                              placeholder="Key (opcional)"
                              className="sm:col-span-3 w-full px-3 py-2 bg-card border border-border rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-primary"
                            />
                          )}
                          <select
                            value={f.type}
                            onChange={(e) => setRequestedCustom((p) => p.map((x, i) => (i === idx ? { ...x, type: e.target.value } : x)))}
                            disabled={!requesting?.partyId}
                            className="sm:col-span-2 h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                          >
                            <option value="text">Texto</option>
                            <option value="date">Data</option>
                            <option value="number">Número</option>
                          </select>
                          <label className="sm:col-span-1 flex items-center gap-2 text-xs font-bold text-foreground justify-start sm:justify-center">
                            <input
                              type="checkbox"
                              checked={!!f.required}
                              onChange={(e) => setRequestedCustom((p) => p.map((x, i) => (i === idx ? { ...x, required: e.target.checked } : x)))}
                              disabled={!requesting?.partyId}
                              className="rounded border-border text-primary focus:ring-primary"
                            />
                            Req.
                          </label>
                          <Button
                            type="button"
                            variant="ghost"
                            size="icon"
                            onClick={() => setRequestedCustom((p) => p.filter((_, i) => i !== idx))}
                            disabled={!requesting?.partyId}
                            className="sm:col-span-1 size-10 text-muted-foreground hover:text-status-critical hover:bg-status-critical-soft"
                            title="Remover campo"
                          >
                            <Trash2 className="w-4 h-4" />
                          </Button>
                        </div>
                      ))}
                    </div>
                  </div>

                  <div className="rounded-xl border border-border bg-muted/40 p-3">
                    <p className="text-[11px] font-bold text-foreground/70 mb-1">O cliente verá no Portal:</p>
                    {(() => {
                      const preview = [
                        requestedBase.full_name.enabled ? { label: "Nome completo", required: requestedBase.full_name.required } : null,
                        requestedBase.cpf.enabled ? { label: "CPF", required: requestedBase.cpf.required } : null,
                        requestedBase.birth_date.enabled ? { label: "Data de nascimento", required: requestedBase.birth_date.required } : null,
                        requestedBase.phone.enabled ? { label: "Telefone/WhatsApp", required: requestedBase.phone.required } : null,
                        ...requestedCustom
                          .filter((f) => (f.label ?? "").trim())
                          .map((f) => ({ label: (f.label ?? "").trim(), required: !!f.required })),
                      ].filter((f): f is { label: string; required: boolean } => f !== null);
                      if (preview.length === 0) {
                        return (
                          <p className="text-xs text-muted-foreground">
                            {requestedMode === "form"
                              ? "Nenhum campo selecionado — selecione os dados que o cliente deve preencher."
                              : "Nenhum campo selecionado — o cliente apenas anexará o documento."}
                          </p>
                        );
                      }
                      return (
                        <p className="text-xs text-foreground">
                          {requestedMode === "form" && <span className="font-bold text-status-info">Sem anexo: </span>}
                          {preview.map((f, i) => (
                            <span key={f.label}>
                              {i > 0 ? ", " : ""}
                              {f.label}
                              {f.required && <span className="text-status-critical">*</span>}
                            </span>
                          ))}
                        </p>
                      );
                    })()}
                  </div>
                </div>

                <DialogFooter className="gap-2 sm:gap-2">
                  <Button type="button" variant="outline" onClick={() => setRequesting(null)}>
                    Cancelar
                  </Button>
                  <Button type="button" onClick={confirmSolicit}>
                    Solicitar
                  </Button>
                </DialogFooter>
              </DialogContent>
            </Dialog>

      <Dialog open={!!batchRequest} onOpenChange={(open) => !open && setBatchRequest(null)}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>Solicitar {batchRequest?.items.length ?? 0} documentos</DialogTitle>
          </DialogHeader>
          <div className="space-y-3">
            <p className="text-sm text-muted-foreground">
              As solicitações vão para {batchRequest?.label ?? "as partes"}. O cliente recebe no Portal do Cliente e
              por e-mail; itens já solicitados não aparecem aqui.
            </p>
            <ul className="max-h-64 space-y-2 overflow-y-auto rounded-xl border border-border bg-muted/30 p-3">
              {batchRequest?.items.map((item) => (
                <li key={item.id} className="flex items-start justify-between gap-3 text-sm">
                  <span className="font-medium text-foreground">{item.name}</span>
                  <span className="shrink-0 text-caption text-muted-foreground">
                    {item.partyId ? partyNameById[item.partyId] ?? "Parte" : "Geral"}
                  </span>
                </li>
              ))}
            </ul>
            <p className="text-caption text-muted-foreground">
              Você pode ajustar campos e tipo de cada solicitação depois, no item.
            </p>
          </div>
          <DialogFooter className="gap-2">
            <Button variant="outline" onClick={() => setBatchRequest(null)}>
              Cancelar
            </Button>
            <Button
              onClick={() => {
                void confirmBatchRequest();
              }}
              className="gap-1.5"
            >
              <Send className="h-3.5 w-3.5" /> Solicitar {batchRequest?.items.length ?? 0}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}


function DocumentationSummary({
  stats,
  checklist,
  partiesCount,
  requestableTotal,
  activeFilter,
  onFilter,
  onConfigure,
  onBatch,
}: {
  stats?: ChecklistStatsDTO;
  checklist: DocumentChecklistItem[];
  partiesCount: number;
  requestableTotal: number;
  activeFilter: DocStatusFilter;
  onFilter: (filter: DocStatusFilter) => void;
  onConfigure: () => void;
  onBatch: () => void;
}) {
  const effective = useMemo<ChecklistStatsDTO | null>(() => {
    if (stats) return stats;
    if (!checklist.length) return null;
    const requiredItems = checklist.filter((i) => i.required);
    const requiredApproved = requiredItems.filter((i) => i.status === "approved").length;
    return {
      total: checklist.length,
      required: requiredItems.length,
      approved: checklist.filter((i) => i.status === "approved").length,
      required_approved: requiredApproved,
      pending: checklist.filter((i) => i.status === "pending").length,
      uploaded: checklist.filter((i) => i.status === "uploaded").length,
      rejected: checklist.filter((i) => i.status === "rejected").length,
      required_complete: requiredItems.length === 0 || requiredApproved === requiredItems.length,
    };
  }, [stats, checklist]);

  if (!effective || effective.total === 0) return null;

  const requiredItems = checklist.filter((i) => i.required);
  const required = requiredItems.length || effective.required;
  const reqApproved = requiredItems.filter((i) => i.status === "approved").length;
  const reqUploaded = requiredItems.filter((i) => i.status === "uploaded").length;
  const reqPending = requiredItems.filter((i) => i.status === "pending").length;
  const reqRejected = requiredItems.filter((i) => i.status === "rejected").length;
  const pendingCount = checklist.filter((i) => i.status === "pending" && !i.solicited).length;
  const solicitedCount = checklist.filter((i) => i.status === "pending" && !!i.solicited).length;
  const complete = required === 0 || reqApproved >= required;
  const remaining = Math.max(0, required - reqApproved);
  const pct = (value: number) => (required > 0 ? `${(value / required) * 100}%` : "0%");
  const chips: Array<{ id: DocStatusFilter; label: string; count: number; dot: string }> = [
    { id: "approved", label: "Aprovados", count: effective.approved, dot: "bg-status-ok" },
    { id: "analysis", label: "Em análise", count: effective.uploaded, dot: "bg-status-info" },
    { id: "pending", label: "A solicitar", count: pendingCount, dot: "bg-status-warning" },
    { id: "solicited", label: "Solicitados", count: solicitedCount, dot: "bg-status-info" },
    { id: "rejected", label: "Rejeitados", count: effective.rejected, dot: "bg-status-critical" },
  ];

  return (
    <section
      className={clsx(
        "rounded-2xl border p-5 shadow-sm",
        complete ? "border-status-ok/30 bg-status-ok-soft" : "border-border bg-card",
      )}
    >
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <p className="text-micro font-bold uppercase tracking-wide text-muted-foreground">Documentação do negócio</p>
          <h3 className="mt-1 text-title-lg font-bold text-foreground">
            {complete ? "Documentação obrigatória completa" : `${reqApproved} de ${required} obrigatórios aprovados`}
          </h3>
          <p className="mt-1 text-caption text-muted-foreground">
            {effective.total} documentos · {partiesCount} {partiesCount === 1 ? "parte" : "partes"} ·{" "}
            {Math.max(0, effective.total - effective.approved)} ainda não aprovados
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" size="sm" onClick={onConfigure} className="gap-1.5">
            <Settings2 className="h-3.5 w-3.5" /> Configurar pacotes
          </Button>
          {requestableTotal > 0 ? (
            <Button size="sm" onClick={onBatch} className="gap-1.5">
              <Send className="h-3.5 w-3.5" /> Solicitar {requestableTotal}
            </Button>
          ) : solicitedCount > 0 ? (
            <Button variant="outline" size="sm" onClick={() => onFilter("solicited")} className="gap-1.5">
              <Clock className="h-3.5 w-3.5" /> Ver {solicitedCount} solicitados
            </Button>
          ) : (
            <Button size="sm" disabled className="gap-1.5">
              <CheckCircle className="h-3.5 w-3.5" /> Tudo em dia
            </Button>
          )}
        </div>
      </div>
      <div className="mt-4 flex h-2 overflow-hidden rounded-full bg-muted">
        <span className="block bg-status-ok" style={{ width: pct(reqApproved) }} />
        <span className="block bg-status-info" style={{ width: pct(reqUploaded) }} />
        <span className="block bg-status-warning" style={{ width: pct(reqPending) }} />
        <span className="block bg-status-critical" style={{ width: pct(reqRejected) }} />
      </div>
      <p className="mt-1.5 text-micro text-muted-foreground">
        Barra dos {required} documentos obrigatórios · {reqApproved} aprovados, {reqUploaded} em análise, {reqPending}{" "}
        pendentes, {reqRejected} rejeitados{solicitedCount > 0 ? ` · ${solicitedCount} já solicitados` : ""}
      </p>
      <div className="mt-3 flex flex-wrap items-center gap-1.5">
        {chips.map((chip) => (
          <button
            key={chip.id}
            type="button"
            onClick={() => onFilter(chip.id)}
            className={clsx(
              "inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-micro font-bold transition-colors",
              activeFilter === chip.id
                ? "border-foreground/20 bg-background text-foreground shadow-sm"
                : "border-border bg-muted/40 text-muted-foreground hover:bg-muted",
            )}
          >
            <span className={clsx("h-1.5 w-1.5 rounded-full", chip.dot)} />
            {chip.count} {chip.label}
          </button>
        ))}
        {activeFilter !== "all" && (
          <button
            type="button"
            onClick={() => onFilter("all")}
            className="text-micro font-bold text-primary hover:underline"
          >
            Limpar filtro
          </button>
        )}
      </div>
      <div
        className={clsx(
          "mt-3 flex items-start gap-2 rounded-xl border px-3 py-2 text-caption leading-relaxed",
          complete
            ? "border-status-ok/20 bg-status-ok-soft text-status-ok"
            : "border-status-warning/20 bg-status-warning-soft text-status-warning",
        )}
      >
        {complete ? (
          <CheckCircle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
        ) : (
          <Clock className="mt-0.5 h-3.5 w-3.5 shrink-0" />
        )}
        <span>
          {complete
            ? "Tudo pronto para a assinatura."
            : `Faltam ${remaining} obrigatórios.${reqUploaded ? ` ${reqUploaded} em análise.` : ""}${
                reqRejected ? ` ${reqRejected} rejeitado(s).` : ""
              }${solicitedCount ? ` ${solicitedCount} já solicitados.` : ""}`}
        </span>
      </div>
    </section>
  );
}

function PendingReviewsQueue({
  groups,
  counts,
  filter,
  onFilter,
  onReviewOcr,
  onReviewForm,
  onReprocessOcr,
}: {
  groups: PendingReviewGroup[];
  counts: { all: number; divergence: number; low: number; failed: number; form: number };
  filter: PendingReviewFilter;
  onFilter: (filter: PendingReviewFilter) => void;
  onReviewOcr: (item: PendingReviewItem) => void;
  onReviewForm: (item: PendingReviewItem) => void;
  onReprocessOcr?: (item: PendingReviewItem) => void | Promise<void>;
}) {
  if (counts.all === 0) return <></>;

  const filters: Array<{ key: PendingReviewFilter; label: string; count: number }> = [
    { key: "all", label: "Pendentes", count: counts.all },
    { key: "divergence", label: "Divergências", count: counts.divergence },
    { key: "low", label: "Baixa confiança", count: counts.low },
    { key: "failed", label: "Falha / sem dados", count: counts.failed },
    { key: "form", label: "Formulário", count: counts.form },
  ];

  return (
    <div className="rounded-2xl border border-primary/20 bg-primary/5 p-4">
      <div className="flex items-start justify-between gap-2">
        <div className="flex min-w-0 items-start gap-2">
          <Sparkles className="mt-0.5 h-4 w-4 shrink-0 text-primary" />
          <div className="min-w-0">
            <h4 className="text-sm font-bold text-foreground">Revisões pendentes</h4>
            <p className="text-caption text-muted-foreground">
              OCR e formulários aguardando revisão, agrupados por parte.
            </p>
          </div>
        </div>
        <Badge variant="info" className="shrink-0">
          {counts.all}
        </Badge>
      </div>

      <div className="mt-2 flex flex-wrap gap-1">
        {filters.map((item) => (
          <button
            key={item.key}
            type="button"
            onClick={() => onFilter(item.key)}
            className={clsx(
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

      {groups.length === 0 ? (
        <p className="mt-3 rounded-lg border border-dashed border-border bg-card px-3 py-2 text-xs text-muted-foreground">
          Nenhuma revisão com este filtro.
        </p>
      ) : (
        <div className="mt-3 space-y-2.5">
          {groups.map((group) => (
            <section
              key={group.partyId || "unassigned"}
              className="overflow-hidden rounded-xl border border-border bg-card"
            >
              <div className="flex items-center gap-2 border-b border-border bg-muted/20 px-3 py-2">
                <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full border border-primary/20 bg-primary/10 text-micro font-bold text-primary">
                  {group.partyName.charAt(0).toUpperCase()}
                </span>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-bold text-foreground">{group.partyName}</p>
                  <p className="text-micro text-muted-foreground">
                    {group.partyRole
                      ? PARTY_ROLE_LABELS[group.partyRole as PartyDTO["role"]] ?? group.partyRole
                      : "Sem parte"}
                  </p>
                </div>
                <span className="text-micro font-semibold text-muted-foreground">
                  {group.items.length}
                </span>
              </div>
              <div className="divide-y divide-border/60">
                {group.items.map((item) => {
                  const meta = pendingRiskMeta[item.risk];
                  return (
                    <div
                      key={item.key}
                      className="flex flex-col gap-2 px-3 py-2.5 sm:flex-row sm:items-start sm:justify-between sm:gap-3"
                    >
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-1.5">
                          <p className="break-words text-sm font-semibold text-foreground">
                            {item.title}
                          </p>
                          <Badge variant={meta.variant}>{meta.label}</Badge>
                          {item.age && (
                            <span className="text-micro text-muted-foreground">{item.age}</span>
                          )}
                        </div>
                        <p className="mt-0.5 break-words text-caption text-muted-foreground">
                          {item.summary}
                        </p>
                        {item.technicalError && (
                          <details className="mt-1">
                            <summary className="cursor-pointer select-none text-micro text-muted-foreground hover:text-foreground">
                              Detalhes técnicos
                            </summary>
                            <pre className="mt-1 max-h-24 overflow-auto whitespace-pre-wrap break-words rounded-md bg-muted/60 p-2 text-micro text-muted-foreground">
                              {item.technicalError}
                            </pre>
                          </details>
                        )}
                      </div>
                      <div className="flex shrink-0 flex-wrap items-start justify-end gap-1.5">
                        {item.risk === "failed" && onReprocessOcr && (
                          <Button
                            size="sm"
                            variant="outline"
                            className="gap-1.5"
                            onClick={() => void onReprocessOcr(item)}
                          >
                            <RefreshCw className="h-3.5 w-3.5" /> Reprocessar
                          </Button>
                        )}
                        <Button
                          size="sm"
                          variant="outline"
                          className="gap-1.5"
                          disabled={!item.party}
                          title={!item.party ? "Vincule o documento a uma parte para revisar" : undefined}
                          onClick={() =>
                            item.kind === "ocr" ? onReviewOcr(item) : onReviewForm(item)
                          }
                        >
                          {item.kind === "ocr" ? (
                            <Sparkles className="h-3.5 w-3.5" />
                          ) : (
                            <FileText className="h-3.5 w-3.5" />
                          )}
                          Revisar
                        </Button>
                      </div>
                    </div>
                  );
                })}
              </div>
            </section>
          ))}
        </div>
      )}
    </div>
  );
}

function PartyGroup({
  avatar,
  title,
  subtitle,
  total,
  approved,
  uploaded,
  required,
  requiredApproved,
  actions,
  defaultOpen,
  forceOpen,
  children,
}: {
  avatar: string;
  title: string;
  subtitle: string;
  total: number;
  approved: number;
  uploaded: number;
  required: number;
  requiredApproved: number;
  actions: ReactNode;
  defaultOpen: boolean;
  forceOpen: boolean;
  children: ReactNode;
}) {
  const [open, setOpen] = useState(defaultOpen);
  useEffect(() => {
    if (forceOpen) setOpen(true);
  }, [forceOpen]);
  const complete = required > 0 && requiredApproved === required;
  const pct = (value: number) => (total > 0 ? `${(value / total) * 100}%` : "0%");

  return (
    <section className="flex flex-col overflow-hidden rounded-2xl border border-border bg-card shadow-sm">
      <div className="flex flex-wrap items-start justify-between gap-3 p-4">
        <button
          type="button"
          onClick={() => setOpen((value) => !value)}
          aria-expanded={open}
          className="flex min-w-0 flex-1 items-start gap-3 text-left"
        >
          <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full border border-primary/20 bg-primary/10 text-xs font-bold text-primary">
            {avatar}
          </span>
          <span className="min-w-0">
            <span className="flex flex-wrap items-center gap-2">
              <span className="text-sm font-bold text-foreground">{title}</span>
              {total > 0 && (
                <Badge variant={complete ? "success" : "warning"}>
                  {complete ? "Completo" : "Em andamento"}
                </Badge>
              )}
            </span>
            {subtitle && <span className="mt-0.5 block truncate text-caption text-muted-foreground">{subtitle}</span>}
            <span className="mt-2 flex flex-wrap items-center gap-2">
              <span className="flex h-1.5 w-28 overflow-hidden rounded-full bg-muted">
                <span className="block bg-status-ok" style={{ width: pct(approved) }} />
                <span className="block bg-status-info" style={{ width: pct(uploaded) }} />
              </span>
              <span className="text-caption font-medium text-muted-foreground">
                {approved}/{total} aprovados{uploaded > 0 ? ` · ${uploaded} em análise` : ""}
              </span>
            </span>
          </span>
          <ChevronDown
            className={clsx(
              "mt-1 h-4 w-4 shrink-0 text-muted-foreground transition-transform",
              open && "rotate-180",
            )}
          />
        </button>
        <div className="flex shrink-0 flex-wrap items-center justify-end gap-1.5">
          {actions}
        </div>
      </div>
      {open && <div className="flex-1 border-t border-border px-4 pb-2">{children}</div>}
    </section>
  );
}


function DealDocumentsSection({ dealId }: { dealId: string }) {
  const [docs, setDocs] = useState<DocumentDTO[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [open, setOpen] = useState(false);
  const [viewer, setViewer] = useState<{ url: string; title: string; mime?: string } | null>(null);

  useEffect(() => {
    if (!dealId || !open || docs !== null) return;
    let cancelled = false;
    setLoading(true);
    documentService
      .getByEntity(dealId, "deal")
      .then((res) => {
        if (!cancelled) setDocs(res?.data ?? []);
      })
      .catch(() => {
        if (!cancelled) setDocs([]);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [dealId, open, docs]);

  const count = docs?.length ?? 0;

  const openDoc = async (d: DocumentDTO) => {
    try {
      const url = await documentService.getPresignedDownloadUrl(d.id);
      if (!url) {
        toast.error("Arquivo ainda não disponível.");
        return;
      }
      setViewer({ url, title: d.name, mime: d.file_type });
    } catch {
      toast.error("Não foi possível abrir o documento.");
    }
  };

  return (
    <div className="bg-card rounded-2xl border border-border shadow-sm">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="w-full flex items-center justify-between gap-3 p-4 text-left hover:bg-muted/50 rounded-2xl transition-colors"
      >
        <div className="flex items-center gap-3">
          <div className="p-2 rounded-xl bg-muted">
            <FileText className="w-4 h-4 text-muted-foreground" />
          </div>
          <div>
            <h3 className="text-sm font-bold text-foreground">Documentos do negócio</h3>
            <p className="text-xs text-muted-foreground">
              Anexos, aditivos e contratos vinculados a este negócio
              {docs !== null && (
                <>
                  {" · "}
                  <span className="font-semibold">
                    {count} arquivo{count === 1 ? "" : "s"}
                  </span>
                </>
              )}
            </p>
          </div>
        </div>
        <ChevronDown
          className={clsx("w-4 h-4 text-muted-foreground transition-transform", open && "rotate-180")}
        />
      </button>

      {open && (
        <div className="px-4 pb-4">
          {loading && <p className="text-xs text-muted-foreground">Carregando…</p>}
          {!loading && docs && docs.length === 0 && (
            <p className="text-xs text-muted-foreground italic">
              Nenhum documento na Central de Documentos vinculado a este negócio ainda.
            </p>
          )}
          {!loading && docs && docs.length > 0 && (
            <ul className="space-y-2">
              {docs.map((d) => {
                const cat = getCategoryMeta(d.category);
                const status = getStatusMeta(normalizeBackendStatus(d.status));
                const StatusIcon = status.icon;
                // Surface the persistence race: an `assinado` document
                // whose bytes haven't landed in Supabase yet should not
                // pretend to be fully downloadable. Empty/legacy rows
                // (no persistence_status field) are treated as stored
                // since they predate the Fase 3/S1 tracking.
                const persistencePending = d.persistence_status === "pending";
                const persistenceFailed = d.persistence_status === "failed";
                const openable = !persistencePending && !persistenceFailed;
                return (
                  <li
                    key={d.id}
                    className="flex items-center justify-between gap-3 rounded-xl border border-border bg-muted/30 px-3 py-2.5"
                  >
                    <div className="min-w-0 flex items-center gap-2">
                      <FileText className="w-4 h-4 text-muted-foreground shrink-0" />
                      <div className="min-w-0">
                        <div className="text-sm font-semibold text-foreground truncate">{d.name}</div>
                        <div className="text-xs text-muted-foreground flex items-center gap-2 mt-0.5 flex-wrap">
                          <span
                            className={clsx(
                              "inline-flex items-center gap-1 py-0.5 px-1.5 rounded-full border text-micro font-bold",
                              cat.badgeClass,
                            )}
                          >
                            {cat.label}
                          </span>
                          <span
                            className={clsx(
                              "inline-flex items-center gap-1 py-0.5 px-1.5 rounded-full border text-micro font-bold",
                              status.badgeClass,
                            )}
                          >
                            <StatusIcon className="w-3 h-3" />
                            {status.label}
                          </span>
                          {persistencePending && (
                            <span
                              className="inline-flex items-center gap-1 py-0.5 px-1.5 rounded-full border text-micro font-bold bg-status-info-soft text-status-info border-status-info/30"
                              title="O arquivo está sendo baixado da Clicksign — atualize a página em alguns segundos."
                            >
                              <Clock className="w-3 h-3" />
                              Aguardando arquivo
                            </span>
                          )}
                          {persistenceFailed && (
                            <span
                              className="inline-flex items-center gap-1 py-0.5 px-1.5 rounded-full border text-micro font-bold bg-status-critical-soft text-status-critical border-status-critical/30"
                              title={d.last_persist_error ?? "Falha na persistência — será reprocessado pelo backfill."}
                            >
                              <XCircle className="w-3 h-3" />
                              Falha no download
                            </span>
                          )}
                        </div>
                      </div>
                    </div>
                    <Button
                      type="button"
                      variant="link"
                      size="sm"
                      onClick={() => openable && openDoc(d)}
                      disabled={!openable}
                      className="h-auto p-0 text-xs font-bold text-primary-600 hover:text-primary-700"
                      title={
                        openable
                          ? undefined
                          : persistenceFailed
                            ? "Persistência falhou — aguarde o backfill ou contate o suporte"
                            : "Aguardando download da Clicksign"
                      }
                    >
                      Abrir
                    </Button>
                  </li>
                );
              })}
            </ul>
          )}
        </div>
      )}

      {viewer && (
        <DocumentViewerDialog
          open={!!viewer}
          onOpenChange={(o) => !o && setViewer(null)}
          url={viewer.url}
          title={viewer.title}
          contentType={viewer.mime}
        />
      )}
    </div>
  );
}
