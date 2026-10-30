import { useCallback, useDeferredValue, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useNavigate } from "react-router";
import {
  BadgeCheck,
  CheckCircle2,
  ChevronRight,
  Clock,
  Download,
  ExternalLink,
  Eye,
  FileCheck,
  FileText,
  FolderOpen,
  History,
  Hourglass,
  LayoutGrid,
  List as ListIcon,
  Search,
  SlidersHorizontal,
  Trash2,
  X,
  XCircle,
} from "lucide-react";
import { clsx } from "clsx";
import { motion, AnimatePresence } from "motion/react";
import { toast } from "sonner";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "../ui/button";
import { PageState } from "../ui/page-state";
import { PageHeader } from "../ui/page-header";
import type { ContractDocument, DocumentCategory, DocumentStatus } from "../types/contractDocument";
import { CATEGORY_OPTIONS, getCategoryMeta, getSafeDocumentFallbackUrl, normalizeBackendStatus } from "../lib/documents";
import { useContractsContext } from "../context/ContractsContext";
import { useDocumentsContext } from "../context/DocumentsContext";
import { usePropertiesContext } from "../context/PropertiesContext";
import { useLeadsContext } from "../context/LeadsContext";
import { useDealsContext } from "../context/DealsContext";
import { useInspectionsContext } from "../context/InspectionsContext";
import { useMaintenanceContext } from "../context/MaintenanceContext";
import { useAgentsContext } from "../context/AgentsContext";
import { useAuth } from "../context/AuthContext";
import { AuditTrailPanel } from "../components/AuditTrailPanel";
import { ExpandableSidePanel } from "../components/ExpandableSidePanel";
import { useFocusMode } from "../context/FocusModeContext";
import { documentService, type DocumentDTO } from "../services/document.service";
import { collectPages } from "../services/collectPages";
import { settingsService } from "../services/settings.service";
import { useViewPreferences } from "../context/ViewPreferencesContext";
import { MultiSelectTagPicker } from "../components/MultiSelectTagPicker";
import { DocumentViewerDialog } from "../components/DocumentViewerDialog";
import { useConfirmAction } from "../hooks/useConfirmAction";
import { DocumentGridCard } from "../components/documents/DocumentGridCard";
import { DocumentFolderTree } from "../components/documents/DocumentFolderTree";
import { useDocumentAutoSave, type AutoSaveStatus } from "../hooks/useDocumentAutoSave";
import {
  buildDocumentDrive,
  documentFolderKeyToString,
  filterByFolder,
  getDocumentCollection,
  groupDriveEntries,
  matchesQuery,
  normalizeFolderKeyString,
  sortDriveEntries,
  summarizeDriveEntries,
  DOCUMENT_GROUPS,
  DOCUMENT_SORTS,
  type DocumentFolderKey,
  type DocumentGroupId,
  type DocumentSortId,
  type DriveEntry,
} from "../documents/drive";
import { getDocumentOrigin, type DocumentOriginId } from "../documents/origins";
import { describeSignatureProgress, getSignatureStateMeta, resolveSignatureState } from "../documents/signature";
import { createDocumentReferenceResolver } from "../documents/references";
import { toGatewayUrl } from "../services/api";

/**
 * A Central de Documentos carrega o acervo do tenant de uma vez e resolve
 * origem, cliente, imóvel e responsável no cliente. É isso que permite
 * contar pastas, buscar por nome de cliente/imóvel e agrupar por origem sem
 * uma chamada por filtro — o comportamento de "drive" que a operação espera.
 */

const ORIGIN_TONES: Record<DocumentOriginId, string> = {
  contract: "bg-primary-50 text-primary-700 border-primary-200",
  deal: "bg-status-info-soft text-status-info border-status-info/30",
  property: "bg-status-info-soft text-status-info border-status-info/30",
  client: "bg-status-ok-soft text-status-ok border-status-ok/30",
  inspection: "bg-status-warning-soft text-status-warning border-status-warning/30",
  checklist: "bg-status-ok-soft text-status-ok border-status-ok/30",
  analysis: "bg-status-info-soft text-status-info border-status-info/30",
  fiscal: "bg-status-warning-soft text-status-warning border-status-warning/30",
  financial: "bg-status-info-soft text-status-info border-status-info/30",
  maintenance: "bg-status-warning-soft text-status-warning border-status-warning/30",
  unassigned: "bg-muted text-muted-foreground border-border",
};

function formatFileSize(bytes: number, status?: DocumentStatus): string {
  if (!bytes || bytes <= 0) {
    return status === "rascunho" ? "—" : "0 B";
  }
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1048576) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1048576).toFixed(1)} MB`;
}

function formatUploadDate(iso: string): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime()) || d.getFullYear() < 1970) return "—";
  return d.toLocaleString("pt-BR", {
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function statusLabel(status: AutoSaveStatus, savedAt: number | null, error: string | null): string {
  if (status === "saving") return "Salvando…";
  if (status === "error") return `Erro: ${error ?? "tente novamente"}`;
  if (status === "saved" && savedAt) {
    const d = new Date(savedAt);
    return `Salvo às ${d.toLocaleTimeString("pt-BR", { hour: "2-digit", minute: "2-digit" })}`;
  }
  return "Edições são salvas automaticamente";
}

function serializeDocumentFields(fields: { name: string; notes: string; tags: string[]; category: DocumentCategory }): string {
  return JSON.stringify({
    name: fields.name,
    notes: fields.notes,
    tags: fields.tags,
    category: fields.category,
  });
}

interface DocumentsViewPreferences extends Record<string, unknown> {
  viewMode: "list" | "grid";
  pinnedFolders: string[];
  groupBy: DocumentGroupId;
  sort: DocumentSortId;
  showEmptyFolders: boolean;
}

const DEFAULT_VIEW_PREFS: DocumentsViewPreferences = {
  viewMode: "list",
  pinnedFolders: [],
  groupBy: "origin",
  sort: "recent",
  showEmptyFolders: false,
};

const DEFAULT_FOLDER: DocumentFolderKey = { kind: "collection", id: "all" };

const SIGNER_STATUS_LABELS: Record<string, string> = {
  pendente: "aguardando",
  assinado: "assinou",
  recusado: "recusou",
};

export function Documents() {
  const { confirmAction, confirmationDialog } = useConfirmAction();
  const { deleteDocument: deleteDocFromCrm } = useDocumentsContext();
  const { contracts } = useContractsContext();
  const { properties } = usePropertiesContext();
  const { leads } = useLeadsContext();
  const { dealDTOs } = useDealsContext();
  const { inspections } = useInspectionsContext();
  const { tickets } = useMaintenanceContext();
  const { agents } = useAgentsContext();
  const { user } = useAuth();
  const tenantId = user?.current_company_id;
  const { focusMode } = useFocusMode();
  const queryClient = useQueryClient();

  const [viewPrefs, setViewPrefs] = useViewPreferences<DocumentsViewPreferences>("documents", DEFAULT_VIEW_PREFS);

  const [activeFolder, setActiveFolder] = useState<DocumentFolderKey>(DEFAULT_FOLDER);
  const [searchTerm, setSearchTerm] = useState("");
  const deferredSearch = useDeferredValue(searchTerm);

  const [selectedDocumentId, setSelectedDocumentId] = useState<string | null>(null);
  const [deletingDocumentId, setDeletingDocumentId] = useState<string | null>(null);
  const [auditDocId, setAuditDocId] = useState<string | null>(null);
  const [auditDocLabel, setAuditDocLabel] = useState("");

  const membersQuery = useQuery({
    queryKey: ["company-members", tenantId ?? "no-tenant"],
    queryFn: async () => {
      if (!tenantId) return [];
      try {
        return await settingsService.listCompanyMembers(tenantId);
      } catch (error) {
        // Sem permissão de listar membros o drive continua funcionando: os
        // nomes caem para os corretores já carregados e, por fim, para o id.
        console.warn("[documents] membros da empresa indisponíveis", error);
        return [];
      }
    },
    enabled: Boolean(tenantId),
    staleTime: 5 * 60_000,
  });

  const documentsQuery = useQuery({
    queryKey: ["documents", tenantId ?? "no-tenant", "drive"],
    queryFn: async ({ signal }) => {
      const raw = await collectPages<DocumentDTO>(paging => documentService.getAll(paging), { signal });
      return { documents: raw.map(mapDtoToDocument) };
    },
    enabled: Boolean(tenantId),
    staleTime: 30_000,
  });

  const knownTagsQuery = useQuery({
    queryKey: ["documents", tenantId ?? "no-tenant", "known-tags"],
    queryFn: () => documentService.listKnownTags(),
    enabled: Boolean(tenantId),
    staleTime: 60_000,
  });

  const resolver = useMemo(
    () =>
      createDocumentReferenceResolver({
        contracts: contracts.map((c) => ({
          id: c.id,
          display_id: c.display_id,
          client: c.client,
          property_title: c.property_title,
          property_id: c.property_id,
        })),
        properties: properties.map((p) => ({ id: p.id, title: p.title, displayId: p.displayId })),
        leads: leads.map((l) => ({ id: l.id, name: l.name, accountId: l.accountId, displayId: l.displayId })),
        deals: dealDTOs.map((d) => ({
          id: d.id,
          display_id: d.display_id,
          property_id: d.property_id,
          customer_id: d.customer_id,
          lead_id: d.lead_id,
          parties: (d.parties ?? []).map((p) => ({ id: p.id, name: p.name })),
        })),
        inspections: inspections.map((i) => ({
          id: i.id,
          property_id: i.property_id,
          contract_id: i.contract_id,
          type: i.type,
          status: i.status,
          scheduled_date: i.scheduled_date,
        })),
        tickets: tickets.map((t) => ({
          id: t.id,
          property_id: t.property_id,
          property_title: t.property_title,
          type: t.type,
          category: t.category,
          status: t.status,
        })),
        members: membersQuery.data ?? [],
        agents: agents.map((a) => ({ id: a.id, name: a.name })),
      }),
    [contracts, properties, leads, dealDTOs, inspections, tickets, membersQuery.data, agents],
  );

  const drive = useMemo(
    () => buildDocumentDrive(documentsQuery.data?.documents ?? [], resolver),
    [documentsQuery.data, resolver],
  );

  // Contagens refletem a busca livre (faceta), não a pasta selecionada: assim
  // a barra lateral nunca promete um número que a lista não entrega.
  const searchMatches = useMemo(
    () => drive.entries.filter((entry) => matchesQuery(entry, deferredSearch)),
    [drive.entries, deferredSearch],
  );

  const facets = useMemo(() => summarizeDriveEntries(searchMatches), [searchMatches]);

  const visibleEntries = useMemo(
    () => sortDriveEntries(filterByFolder(searchMatches, activeFolder), viewPrefs.sort),
    [searchMatches, activeFolder, viewPrefs.sort],
  );

  const sections = useMemo(
    () => groupDriveEntries(visibleEntries, viewPrefs.groupBy),
    [visibleEntries, viewPrefs.groupBy],
  );

  const knownTags = knownTagsQuery.data ?? [];
  const loadedDocuments = documentsQuery.data?.documents ?? [];
  const selectedDocumentFromList = useMemo(
    () => loadedDocuments.find((doc) => doc.id === selectedDocumentId) ?? null,
    [loadedDocuments, selectedDocumentId],
  );
  const selectedDocumentQuery = useQuery({
    queryKey: ["documents", tenantId ?? "no-tenant", "detail", selectedDocumentId],
    queryFn: async () => {
      if (!selectedDocumentId) return null;
      const response = await documentService.getById(selectedDocumentId);
      const dto = response?.data as DocumentDTO | undefined;
      return dto ? mapDtoToDocument(dto) : null;
    },
    enabled: Boolean(selectedDocumentId && tenantId),
    placeholderData: selectedDocumentFromList,
    staleTime: 0,
  });
  const selectedDocument = selectedDocumentQuery.data ?? selectedDocumentFromList;
  const selectedEntry = useMemo(
    () => (selectedDocument ? drive.entries.find((entry) => entry.doc.id === selectedDocument.id) ?? null : null),
    [drive.entries, selectedDocument],
  );

  const stats = useMemo(
    () => ({
      total: facets.counts.total,
      signed: facets.counts.collections.signed,
      // "Enviados e não concluídos" + "exige assinatura e nem foi enviado".
      signaturePending: facets.counts.collections.awaiting + facets.counts.collections.not_started,
      notRequired: facets.counts.collections.not_required,
    }),
    [facets.counts],
  );

  const togglePinFolder = useCallback(
    (key: DocumentFolderKey) => {
      const keyStr = documentFolderKeyToString(key);
      const isPinned = viewPrefs.pinnedFolders.some((k) => normalizeFolderKeyString(k) === keyStr);
      const next = isPinned
        ? viewPrefs.pinnedFolders.filter((k) => normalizeFolderKeyString(k) !== keyStr)
        : [...viewPrefs.pinnedFolders, keyStr];
      setViewPrefs({ pinnedFolders: next });
    },
    [viewPrefs.pinnedFolders, setViewPrefs],
  );

  const invalidateDocuments = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: ["documents", tenantId ?? "no-tenant"] });
  }, [queryClient, tenantId]);

  const handleDelete = useCallback(
    async (id: string) => {
      if (deletingDocumentId) return;
      setDeletingDocumentId(id);
      try {
        const applied = await deleteDocFromCrm(id);
        if (!applied) return;
        invalidateDocuments();
        if (selectedDocumentId === id) setSelectedDocumentId(null);
      } catch (err) {
        console.warn("Failed to delete document", err);
        throw err;
      } finally {
        setDeletingDocumentId((current) => (current === id ? null : current));
      }
    },
    [deleteDocFromCrm, deletingDocumentId, invalidateDocuments, selectedDocumentId],
  );

  const requestDelete = useCallback(
    (doc: { id: string; name: string }) => {
      confirmAction(
        `Excluir o documento "${doc.name}"? Esta ação não pode ser desfeita.`,
        () => handleDelete(doc.id),
        { title: "Excluir documento", confirmLabel: "Excluir", variant: "danger" },
      );
    },
    [confirmAction, handleDelete],
  );

  const handleDownload = useCallback(async (id: string) => {
    try {
      await documentService.download(id);
    } catch (err) {
      console.warn("download failed", err);
      toast.error("Não foi possível baixar este documento.");
    }
  }, []);

  const activeFilterLabel = useMemo(() => {
    switch (activeFolder.kind) {
      case "collection":
        return getDocumentCollection(activeFolder.id).label;
      case "category":
        return getCategoryMeta(activeFolder.category).label;
      case "origin":
        return getDocumentOrigin(activeFolder.origin).label;
      case "origin-entity": {
        // Busca o rótulo no acervo completo: o filtro ativo não deve depender
        // do termo digitado no momento.
        const node = (drive.entitiesByOrigin[activeFolder.origin] ?? []).find(
          (n) => n.entityId === activeFolder.entityId,
        );
        return `${getDocumentOrigin(activeFolder.origin).label} · ${node?.label ?? "registro"}`;
      }
      case "tag":
        return `Tag: ${activeFolder.tag}`;
      default:
        return "";
    }
  }, [activeFolder, drive.entitiesByOrigin]);

  const isDefaultFolder = activeFolder.kind === "collection" && activeFolder.id === "all";
  const hasActiveFilters = !isDefaultFolder || deferredSearch.trim().length > 0;

  const clearFilters = useCallback(() => {
    setActiveFolder(DEFAULT_FOLDER);
    setSearchTerm("");
  }, []);

  const renderEntry = (entry: DriveEntry) => (
    <DocumentListItem
      key={entry.doc.id}
      entry={entry}
      onOpen={() => setSelectedDocumentId(entry.doc.id)}
      onDownload={() => void handleDownload(entry.doc.id)}
      onDelete={() => requestDelete(entry.doc)}
      deleting={deletingDocumentId === entry.doc.id}
      deleteDisabled={deletingDocumentId !== null}
    />
  );

  const renderResults = () => {
    if (documentsQuery.isLoading) {
      return <PageState loading skeleton={<ListSkeleton />}>{null}</PageState>;
    }
    if (documentsQuery.isError) {
      return (
        <PageState
          error="Não foi possível carregar o acervo completo. Tente novamente para consultar os documentos."
          onRetry={() => void documentsQuery.refetch()}
          retrying={documentsQuery.isFetching}
        >{null}</PageState>
      );
    }

    if (visibleEntries.length === 0) {
      return (
        <div className="bg-card border border-dashed border-border rounded-xl p-8 text-center space-y-3">
          <FolderOpen className="w-8 h-8 text-muted-foreground mx-auto" />
          <p className="text-sm font-bold text-foreground">
            {hasActiveFilters ? "Nenhum documento neste recorte" : "Nenhum documento na central"}
          </p>
          <p className="text-xs text-muted-foreground">
            {hasActiveFilters
              ? "Tente outra origem, outro tipo de documento ou limpe os filtros."
              : "Os arquivos enviados pela esteira, contratos, vistorias e portal aparecem aqui automaticamente."}
          </p>
          {hasActiveFilters && (
            <Button variant="outline" size="sm" onClick={clearFilters}>
              <X className="w-4 h-4" /> Limpar filtros
            </Button>
          )}
        </div>
      );
    }

    if (viewPrefs.groupBy === "none" || sections.length === 0) {
      return viewPrefs.viewMode === "grid" ? (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
          {visibleEntries.map((entry) => (
            <DocumentGridCard
              key={entry.doc.id}
              doc={entry.doc}
              reference={entry.reference.label}
              originLabel={getDocumentOrigin(entry.origin).label}
              onClick={() => setSelectedDocumentId(entry.doc.id)}
            />
          ))}
        </div>
      ) : (
        <div className="space-y-3">{visibleEntries.map(renderEntry)}</div>
      );
    }

    return (
      <div className="space-y-6">
        {sections.map((section) => (
          <section key={section.key} className="space-y-3">
            <div className="flex items-center justify-between gap-3 px-1">
              <div className="min-w-0">
                <h3 className="text-sm font-bold text-foreground truncate">{section.label}</h3>
                {section.hint && <p className="text-caption text-muted-foreground truncate">{section.hint}</p>}
              </div>
              <span className="text-xs font-bold text-muted-foreground tabular-nums shrink-0">
                {section.count} {section.count === 1 ? "documento" : "documentos"}
              </span>
            </div>
            {viewPrefs.viewMode === "grid" ? (
              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
                {section.entries.map((entry) => (
                  <DocumentGridCard
                    key={entry.doc.id}
                    doc={entry.doc}
                    reference={entry.reference.label}
                    originLabel={getDocumentOrigin(entry.origin).label}
                    onClick={() => setSelectedDocumentId(entry.doc.id)}
                  />
                ))}
              </div>
            ) : (
              <div className="space-y-3">{section.entries.map(renderEntry)}</div>
            )}
          </section>
        ))}
      </div>
    );
  };

  return (
    <div className="flex-1 overflow-hidden bg-muted font-sans">
      {selectedEntry && (
        <DocumentDetailDrawer
          entry={selectedEntry}
          knownTags={knownTags}
          onClose={() => setSelectedDocumentId(null)}
          onSaved={invalidateDocuments}
          onDelete={() => requestDelete(selectedEntry.doc)}
          deletePending={deletingDocumentId === selectedEntry.doc.id}
          onAudit={() => {
            setAuditDocId(selectedEntry.doc.id);
            setAuditDocLabel(selectedEntry.doc.name);
          }}
        />
      )}

      {confirmationDialog}

      <div className="h-full flex flex-col overflow-hidden">
        <div className="max-w-7xl w-full mx-auto p-6 lg:p-8 space-y-6 flex-1 overflow-y-auto">
          <PageHeader
            title="Central de Documentos"
            description="Drive da imobiliária: tudo organizado por origem — contrato, negócio, imóvel e cliente"
          >
            <div className="flex items-center gap-2">
              <ViewModeToggle value={viewPrefs.viewMode} onChange={(mode) => setViewPrefs({ viewMode: mode })} />
            </div>
          </PageHeader>

          <AnimatePresence>
            {!focusMode && (
              <motion.div
                initial={{ opacity: 0, height: 0 }}
                animate={{ opacity: 1, height: "auto" }}
                exit={{ opacity: 0, height: 0 }}
                transition={{ duration: 0.25 }}
              >
                <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
                  <StatCard
                    icon={<FolderOpen className="w-4 h-4 text-primary-600" />}
                    label="Documentos"
                    value={stats.total}
                    hint="Tudo que está na central, com ou sem assinatura"
                  />
                  <StatCard
                    icon={<BadgeCheck className="w-4 h-4 text-status-ok" />}
                    label="Assinados"
                    value={stats.signed}
                    hint="Assinatura concluída por todos os signatários"
                  />
                  <StatCard
                    icon={<Hourglass className="w-4 h-4 text-status-warning" />}
                    label="Com assinatura pendente"
                    value={stats.signaturePending}
                    hint="Enviados aguardando assinatura, parcialmente assinados e contratos não enviados"
                  />
                  <StatCard
                    icon={<FileText className="w-4 h-4 text-muted-foreground" />}
                    label="Não exige assinatura"
                    value={stats.notRequired}
                    hint="Fotos, comprovantes, matrículas e arquivos de apoio"
                  />
                </div>
              </motion.div>
            )}
          </AnimatePresence>

          <div className="bg-card p-4 rounded-xl border border-border shadow-sm space-y-3">
            <div className="relative">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
              <input
                type="text"
                value={searchTerm}
                onChange={(e) => setSearchTerm(e.target.value)}
                placeholder="Buscar por documento, cliente, imóvel, contrato, responsável ou tag..."
                aria-label="Buscar documentos"
                className="w-full pl-9 pr-4 py-2 bg-muted border border-border rounded-lg text-sm focus:ring-2 focus:ring-primary-500 outline-none"
              />
            </div>

            <div className="flex items-center gap-2 flex-wrap">
              <span className="inline-flex items-center gap-1 text-caption font-bold text-muted-foreground">
                <SlidersHorizontal className="w-3.5 h-3.5" /> Agrupar por
              </span>
              <SegmentedControl
                ariaLabel="Agrupar documentos por"
                value={viewPrefs.groupBy}
                options={DOCUMENT_GROUPS.map((g) => ({ value: g.id, label: g.label }))}
                onChange={(value) => setViewPrefs({ groupBy: value as DocumentGroupId })}
              />
              <label className="ml-auto inline-flex items-center gap-1.5 text-caption font-bold text-muted-foreground">
                Ordenar
                <select
                  aria-label="Ordenar documentos"
                  value={viewPrefs.sort}
                  onChange={(e) => setViewPrefs({ sort: e.target.value as DocumentSortId })}
                  className="bg-muted border border-border rounded-lg px-2 py-1 text-caption font-bold text-foreground outline-none focus:ring-2 focus:ring-primary-500"
                >
                  {DOCUMENT_SORTS.map((option) => (
                    <option key={option.id} value={option.id}>
                      {option.label}
                    </option>
                  ))}
                </select>
              </label>
              {hasActiveFilters && (
                <button
                  type="button"
                  onClick={clearFilters}
                  className="inline-flex items-center gap-1.5 text-caption font-bold text-muted-foreground hover:text-foreground"
                >
                  <X className="w-3.5 h-3.5" /> Limpar filtros
                </button>
              )}
            </div>

            <div className="flex items-center gap-2 flex-wrap text-caption text-muted-foreground">
              <span className="inline-flex items-center gap-1 font-bold text-foreground">
                <ChevronRight className="w-3.5 h-3.5 text-muted-foreground" />
                {activeFilterLabel}
              </span>
              <span>·</span>
              <span>
                {visibleEntries.length} de {facets.counts.total}{" "}
                {facets.counts.total === 1 ? "documento" : "documentos"}
              </span>

            </div>
          </div>

          <div className="grid grid-cols-1 lg:grid-cols-[280px_minmax(0,1fr)] gap-4">
            <aside className="bg-card rounded-xl border border-border shadow-sm h-fit lg:sticky lg:top-0">
              <DocumentFolderTree
                counts={facets.counts}
                entitiesByOrigin={facets.entitiesByOrigin}
                activeKey={activeFolder}
                onSelect={setActiveFolder}
                pinnedKeys={viewPrefs.pinnedFolders}
                onTogglePin={togglePinFolder}
                showEmpty={viewPrefs.showEmptyFolders}
                onToggleShowEmpty={() => setViewPrefs({ showEmptyFolders: !viewPrefs.showEmptyFolders })}
              />
            </aside>

            <section className="space-y-3 min-w-0">{renderResults()}</section>
          </div>
        </div>
      </div>

      {auditDocId && (
        <AuditTrailPanel
          entityId={auditDocId}
          entityLabel={auditDocLabel}
          onClose={() => {
            setAuditDocId(null);
            setAuditDocLabel("");
          }}
        />
      )}
    </div>
  );
}

function OriginBadge({ entry, className }: { entry: DriveEntry; className?: string }) {
  const origin = getDocumentOrigin(entry.origin);
  const Icon = origin.icon;
  return (
    <span
      title={origin.description}
      className={clsx(
        "inline-flex items-center gap-1 py-0.5 px-2 rounded-full text-caption font-bold border shrink-0",
        ORIGIN_TONES[entry.origin],
        className,
      )}
    >
      <Icon className="w-3 h-3" />
      {origin.label}
    </span>
  );
}

function DocumentListItem({
  entry,
  onOpen,
  onDownload,
  onDelete,
  deleting,
  deleteDisabled,
}: {
  entry: DriveEntry;
  onOpen: () => void;
  onDownload: () => void;
  onDelete: () => void;
  deleting: boolean;
  deleteDisabled: boolean;
}) {
  const { doc, actor, reference, signatureState } = entry;
  const categoryMeta = getCategoryMeta(doc.category);
  const signatureMeta = getSignatureStateMeta(signatureState);
  const SignatureIcon = signatureMeta.icon;
  const progress = describeSignatureProgress(doc);
  const isSigned = signatureState === "signed";

  return (
    <motion.div
      layout
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      onClick={onOpen}
      className="bg-card rounded-xl border border-border shadow-sm hover:border-primary-300 transition-all p-4 flex items-center gap-4 group cursor-pointer"
    >
      <div
        className={clsx(
          "w-12 h-12 rounded-lg flex items-center justify-center border-2 shrink-0",
          isSigned ? "bg-status-ok-soft border-status-ok/20" : "bg-muted border-border",
        )}
      >
        <FileText className={clsx("w-6 h-6", isSigned ? "text-status-ok" : "text-muted-foreground")} />
      </div>

      <div className="flex-1 min-w-0">
        <div className="flex justify-between items-start gap-2">
          <div className="min-w-0 flex-1">
            <h4 className="font-bold text-foreground truncate">{doc.name}</h4>
            <div className="mt-1 flex items-center gap-2 min-w-0">
              <OriginBadge entry={entry} />
              <span className="text-xs text-muted-foreground truncate" title={reference.detail}>
                {reference.label}
              </span>
            </div>
          </div>
          <div className="flex items-center gap-2 shrink-0">
            <span className={clsx("text-micro font-bold px-2 py-1 rounded-full border", categoryMeta.badgeClass)}>
              {categoryMeta.label}
            </span>
            <span
              title={signatureMeta.description}
              className={clsx(
                "inline-flex items-center gap-1 text-micro font-bold px-2 py-1 rounded-full border",
                signatureMeta.badgeClass,
              )}
            >
              <SignatureIcon className="w-3 h-3" />
              {signatureMeta.label}
            </span>
          </div>
        </div>

        <div className="flex items-center justify-between mt-2 gap-3 flex-wrap">
          <div className="text-xs text-muted-foreground flex items-center gap-1.5 flex-wrap">
            <span>{formatUploadDate(doc.uploadDate)}</span>
            <span>•</span>
            <span>{formatFileSize(doc.fileSize, doc.status)}</span>
            <span>•</span>
            <span className="truncate max-w-[220px]" title={actor.detail ?? actor.label}>
              Enviado por {actor.label}
            </span>
            {progress && !isSigned && (
              <>
                <span>•</span>
                <span className="truncate" title={progress.label}>
                  {progress.label}
                </span>
              </>
            )}
            {(doc.tags ?? []).slice(0, 3).map((tag) => (
              <span
                key={tag}
                className="inline-flex items-center py-0.5 px-2 rounded-full text-caption font-medium bg-muted border border-border"
              >
                {tag}
              </span>
            ))}
            {(doc.tags ?? []).length > 3 && <span className="text-caption">+{(doc.tags ?? []).length - 3}</span>}
          </div>
          <div className="flex gap-2 opacity-0 group-hover:opacity-100 transition-opacity" onClick={(e) => e.stopPropagation()}>
            <Button
              variant="ghost"
              size="icon"
              onClick={onOpen}
              className="size-8 bg-muted text-muted-foreground hover:text-primary-600 hover:bg-primary-50"
              title="Visualizar"
            >
              <Eye className="w-4 h-4" />
            </Button>
            {doc.fileUrl && (
              <Button
                variant="ghost"
                size="icon"
                onClick={onDownload}
                className="size-8 bg-muted text-muted-foreground hover:text-primary-600 hover:bg-primary-50"
                title="Baixar"
              >
                <Download className="w-4 h-4" />
              </Button>
            )}
            <Button
              variant="ghost"
              size="icon"
              onClick={onDelete}
              disabled={deleteDisabled}
              className="size-8 bg-muted text-muted-foreground hover:text-status-critical hover:bg-status-critical-soft"
              title="Excluir"
            >
              {deleting ? <Clock className="w-4 h-4 animate-spin" /> : <Trash2 className="w-4 h-4" />}
            </Button>
          </div>
        </div>
      </div>
    </motion.div>
  );
}

function ViewModeToggle({ value, onChange }: { value: "list" | "grid"; onChange: (v: "list" | "grid") => void }) {
  return (
    <div className="inline-flex rounded-lg border border-border bg-card p-0.5">
      <button
        type="button"
        onClick={() => onChange("list")}
        className={clsx(
          "p-1.5 rounded-md transition-colors flex items-center gap-1",
          value === "list" ? "bg-primary-50 text-primary-700" : "text-muted-foreground hover:bg-muted",
        )}
        title="Modo lista"
      >
        <ListIcon className="w-4 h-4" />
      </button>
      <button
        type="button"
        onClick={() => onChange("grid")}
        className={clsx(
          "p-1.5 rounded-md transition-colors flex items-center gap-1",
          value === "grid" ? "bg-primary-50 text-primary-700" : "text-muted-foreground hover:bg-muted",
        )}
        title="Modo grade"
      >
        <LayoutGrid className="w-4 h-4" />
      </button>
    </div>
  );
}

function SegmentedControl({
  value,
  options,
  onChange,
  ariaLabel,
}: {
  value: string;
  options: { value: string; label: string }[];
  onChange: (value: string) => void;
  ariaLabel: string;
}) {
  return (
    <div role="group" aria-label={ariaLabel} className="inline-flex flex-wrap rounded-lg border border-border bg-muted p-0.5">
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          onClick={() => onChange(option.value)}
          aria-pressed={value === option.value}
          className={clsx(
            "px-2.5 py-1 rounded-md text-caption font-bold transition-colors",
            value === option.value ? "bg-card text-primary-700 shadow-sm" : "text-muted-foreground hover:text-foreground",
          )}
        >
          {option.label}
        </button>
      ))}
    </div>
  );
}

function StatCard({ icon, label, value, hint }: { icon: ReactNode; label: string; value: number; hint?: string }) {
  return (
    <div className="bg-card p-4 rounded-xl border border-border shadow-sm" title={hint}>
      <div className="flex items-center gap-2 mb-2">
        {icon}
        <p className="text-xs font-bold text-muted-foreground uppercase">{label}</p>
      </div>
      <p className="text-2xl font-bold text-foreground">{value}</p>
    </div>
  );
}

function ListSkeleton() {
  return (
    <div className="space-y-3">
      {Array.from({ length: 4 }).map((_, i) => (
        <div key={i} className="bg-card rounded-xl border border-border shadow-sm p-4 animate-pulse">
          <div className="flex items-center gap-4">
            <div className="w-12 h-12 rounded-lg bg-muted" />
            <div className="flex-1 space-y-2">
              <div className="h-4 w-1/3 bg-muted rounded" />
              <div className="h-3 w-2/3 bg-muted rounded" />
            </div>
          </div>
        </div>
      ))}
    </div>
  );
}

function DocumentDetailDrawer({
  entry,
  knownTags,
  onClose,
  onSaved,
  onDelete,
  deletePending,
  onAudit,
}: {
  entry: DriveEntry;
  knownTags: string[];
  onClose: () => void;
  onSaved: () => void;
  onDelete: () => void;
  deletePending: boolean;
  onAudit: () => void;
}) {
  const doc = entry.doc;
  const { reference, actor } = entry;
  const origin = getDocumentOrigin(entry.origin);
  const OriginIcon = origin.icon;

  const [name, setName] = useState(doc.name);
  const [notes, setNotes] = useState(doc.notes ?? "");
  const [tags, setTags] = useState<string[]>(doc.tags ?? []);
  const [category, setCategory] = useState<DocumentCategory>(doc.category);
  const [presignedUrl, setPresignedUrl] = useState<string | null>(null);
  const [viewerOpen, setViewerOpen] = useState(false);
  const fallbackUrl = getSafeDocumentFallbackUrl(doc.fileUrl);
  const acceptedServerRef = useRef({
    docId: doc.id,
    snapshot: serializeDocumentFields({
      name: doc.name,
      notes: doc.notes ?? "",
      tags: doc.tags ?? [],
      category: doc.category,
    }),
  });

  useEffect(() => {
    let cancelled = false;
    setPresignedUrl(null);
    if (doc.status === "rascunho" && !doc.fileUrl) return;
    (async () => {
      try {
        const url = await documentService.getPresignedDownloadUrl(doc.id);
        const safeUrl = getSafeDocumentFallbackUrl(toGatewayUrl(url));
        if (!cancelled) setPresignedUrl(safeUrl || fallbackUrl);
      } catch {
        if (!cancelled) setPresignedUrl(fallbackUrl);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [doc.id, doc.fileUrl, doc.status, fallbackUrl]);

  const previewUrl = presignedUrl ?? fallbackUrl ?? "";
  const canPreview = !!previewUrl;

  const baseline = useMemo(
    () => ({
      name: doc.name,
      notes: doc.notes ?? "",
      tags: doc.tags ?? [],
      category: doc.category,
    }),
    [doc.id, doc.name, doc.notes, doc.tags, doc.category],
  );

  const draft = useMemo(
    () => ({
      name: name.trim() || doc.name,
      notes,
      tags,
      category,
    }),
    [name, notes, tags, category, doc.name],
  );

  const navigate = useNavigate();

  const { status, savedAt, error } = useDocumentAutoSave({
    docId: doc.id,
    baseline,
    draft,
    onSaved,
  });

  const serverSnapshot = serializeDocumentFields(baseline);
  const localSnapshot = serializeDocumentFields({
    name: draft.name ?? doc.name,
    notes: draft.notes ?? "",
    tags: draft.tags ?? [],
    category: draft.category ?? doc.category,
  });
  useEffect(() => {
    const documentChanged = acceptedServerRef.current.docId !== doc.id;
    const localMatchesAcceptedServer = localSnapshot === acceptedServerRef.current.snapshot;
    const serverAcknowledgedLocalDraft = serverSnapshot === localSnapshot;

    if (documentChanged || localMatchesAcceptedServer || serverAcknowledgedLocalDraft) {
      setName(doc.name);
      setNotes(doc.notes ?? "");
      setTags(doc.tags ?? []);
      setCategory(doc.category);
      if (documentChanged) setViewerOpen(false);
      acceptedServerRef.current = { docId: doc.id, snapshot: serverSnapshot };
    }
  }, [doc.id, doc.name, doc.notes, doc.tags, doc.category, localSnapshot, serverSnapshot]);

  // A classificação usa a categoria do rascunho: ao trocar o tipo do
  // documento, o rótulo de assinatura acompanha antes mesmo de salvar.
  const signatureState = resolveSignatureState({ ...doc, category });
  const signatureMeta = getSignatureStateMeta(signatureState);
  const SignatureIcon = signatureMeta.icon;
  const signatureProgress = describeSignatureProgress(doc);

  return (
    <ExpandableSidePanel
      open={true}
      onClose={onClose}
      defaultSize="2xl"
      expandedSize="full"
      renderHeader={({ expandButton, closeButton }) => (
        <div className="px-6 py-4 border-b border-border flex items-center justify-between shrink-0">
          <div className="flex items-center gap-3 min-w-0">
            <div className="p-2.5 bg-muted rounded-lg shrink-0">
              <FileText className="w-5 h-5 text-muted-foreground" />
            </div>
            <div className="min-w-0">
              <h3 className="text-base font-bold text-foreground">Detalhes do Documento</h3>
              <p className="text-xs text-muted-foreground truncate">{statusLabel(status, savedAt, error)}</p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            {expandButton}
            {closeButton}
          </div>
        </div>
      )}
    >
      <div className="flex-1 overflow-y-auto p-6 space-y-6 flex flex-col">
        <div className="space-y-3">
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="w-full text-lg font-bold text-foreground bg-transparent border-b border-transparent hover:border-border focus:border-primary-500 outline-none transition-colors py-1"
            placeholder="Nome do documento"
          />
          <div className="flex items-center gap-2 flex-wrap">
            <span
              title={signatureMeta.description}
              className={clsx(
                "inline-flex items-center gap-1 py-1 px-2.5 rounded-full text-caption font-bold border",
                signatureMeta.badgeClass,
              )}
            >
              <SignatureIcon className="w-3 h-3" />
              {signatureMeta.label}
            </span>
            <OriginBadge entry={entry} />
          </div>
        </div>

        <div className="bg-muted p-4 rounded-xl space-y-2">
          <div className="flex items-start gap-3">
            <div className="p-2 bg-card rounded-lg border border-border shrink-0">
              <SignatureIcon className="w-4 h-4 text-muted-foreground" />
            </div>
            <div className="min-w-0 flex-1">
              <p className="text-micro font-bold text-muted-foreground uppercase tracking-wider">Assinatura</p>
              <p className="text-sm font-bold text-foreground">{signatureMeta.label}</p>
              <p className="text-caption text-muted-foreground">
                {signatureMeta.description}
                {signatureProgress ? ` · ${signatureProgress.label}` : ""}
              </p>
            </div>
          </div>
          {doc.signers.length > 0 && (
            <ul className="space-y-1 pt-1">
              {doc.signers.map((signer) => (
                <li key={signer.id} className="flex items-center gap-2 text-caption">
                  {signer.status === "assinado" ? (
                    <CheckCircle2 className="w-3.5 h-3.5 text-status-ok shrink-0" />
                  ) : signer.status === "recusado" ? (
                    <XCircle className="w-3.5 h-3.5 text-status-critical shrink-0" />
                  ) : (
                    <Clock className="w-3.5 h-3.5 text-status-warning shrink-0" />
                  )}
                  <span className="text-foreground truncate">{signer.name || signer.email || "Signatário"}</span>
                  <span className="text-muted-foreground">{SIGNER_STATUS_LABELS[signer.status] ?? signer.status}</span>
                </li>
              ))}
            </ul>
          )}
        </div>

        <div className="bg-muted p-4 rounded-xl space-y-3">
          <div className="flex items-start gap-3">
            <div className="p-2 bg-card rounded-lg border border-border shrink-0">
              <OriginIcon className="w-4 h-4 text-muted-foreground" />
            </div>
            <div className="min-w-0 flex-1">
              <p className="text-micro font-bold text-muted-foreground uppercase tracking-wider">Origem</p>
              <p className="text-sm font-bold text-foreground break-words">{reference.label}</p>
              <p className="text-caption text-muted-foreground break-words">{reference.detail}</p>
            </div>
          </div>
          {reference.href && (
            <Button variant="outline" size="sm" onClick={() => navigate(reference.href as string)}>
              <ExternalLink className="w-3.5 h-3.5" /> Abrir {origin.label.replace(/s$/, "").toLowerCase()}
            </Button>
          )}
        </div>

        <div>
          <label
            htmlFor="document-category"
            className="block text-xs font-bold text-muted-foreground uppercase tracking-wider mb-2"
          >
            Tipo de documento
          </label>
          <select
            id="document-category"
            value={category}
            onChange={(e) => setCategory(e.target.value as DocumentCategory)}
            className="w-full bg-card border border-border rounded-lg px-3 py-2 text-sm focus:ring-2 focus:ring-primary-500 outline-none"
          >
            {CATEGORY_OPTIONS.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
          <p className="text-caption text-muted-foreground mt-1">
            Corrija o tipo quando o arquivo não corresponder ao rótulo sugerido na importação.
          </p>
        </div>

        <button
          type="button"
          onClick={() => canPreview && setViewerOpen(true)}
          disabled={!canPreview}
          className={clsx(
            "w-full border border-border rounded-xl p-6 flex flex-col items-center justify-center gap-3 transition-colors",
            canPreview ? "bg-muted hover:bg-card cursor-pointer" : "bg-muted opacity-60 cursor-not-allowed",
          )}
        >
          <FileText className="w-10 h-10 text-muted-foreground" />
          <div className="text-center">
            <p className="text-sm font-bold text-foreground">
              {canPreview ? "Visualizar arquivo" : "Arquivo ainda não disponível"}
            </p>
            <p className="text-xs text-muted-foreground mt-0.5">
              {doc.fileType || "—"} · {formatFileSize(doc.fileSize, doc.status)}
            </p>
          </div>
        </button>

        <div className="grid grid-cols-2 gap-4 bg-muted p-4 rounded-xl">
          <Field label="Formato" value={doc.fileType || "—"} />
          <Field label="Tamanho" value={formatFileSize(doc.fileSize, doc.status)} />
          <Field label="Upload em" value={formatUploadDate(doc.uploadDate)} />
          <Field label="Enviado por" value={actor.label} />
        </div>

        <div>
          <p className="text-xs font-bold text-muted-foreground uppercase tracking-wider mb-2">Tags</p>
          <MultiSelectTagPicker
            value={tags}
            options={knownTags}
            allowCreate
            onChange={setTags}
            placeholder="Adicione tags para organizar o documento"
          />
        </div>

        <div>
          <p className="text-xs font-bold text-muted-foreground uppercase tracking-wider mb-2">Notas</p>
          <textarea
            value={notes}
            onChange={(e) => setNotes(e.target.value)}
            placeholder="Anotações internas sobre este documento"
            className="w-full min-h-[80px] bg-card border border-border rounded-lg px-3 py-2 text-sm focus:ring-2 focus:ring-primary-500 outline-none"
          />
        </div>
      </div>

      <div className="bg-card px-6 py-4 border-t border-border flex items-center gap-3 flex-wrap shrink-0">
        <Button variant="outline" onClick={onAudit}>
          <History className="w-4 h-4" /> Histórico
        </Button>
        {doc.contractId && (
          <Button variant="outline" onClick={() => navigate(`/contratos/${doc.contractId}`)}>
            <FileCheck className="w-4 h-4" /> Ver contrato
          </Button>
        )}
        <Button
          variant="outline"
          onClick={async () => {
            try {
              await documentService.download(doc.id);
            } catch (err) {
              console.warn("download failed", err);
              toast.error("Não foi possível baixar este documento.");
            }
          }}
          disabled={!doc.fileUrl}
        >
          <Download className="w-4 h-4" /> Download
        </Button>
        <Button
          variant="outline"
          size="icon"
          onClick={onDelete}
          disabled={deletePending}
          className="ml-auto border-status-critical/30 text-status-critical hover:bg-status-critical-soft hover:text-status-critical"
        >
          {deletePending ? <Clock className="w-4 h-4 animate-spin" /> : <Trash2 className="w-4 h-4" />}
        </Button>
      </div>

      <DocumentViewerDialog
        open={viewerOpen}
        onOpenChange={setViewerOpen}
        url={previewUrl}
        title={doc.name}
        contentType={doc.fileType}
      />
    </ExpandableSidePanel>
  );
}

function Field({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <p className="text-micro font-medium text-muted-foreground uppercase tracking-wider mb-1">{label}</p>
      <p className="text-sm font-bold text-foreground break-words">{value}</p>
    </div>
  );
}

function mapDtoToDocument(d: DocumentDTO): ContractDocument {
  return {
    id: d.id,
    contractId: d.contract_id || "",
    name: d.name,
    category: d.category as DocumentCategory,
    fileType: d.file_type,
    fileSize: d.file_size,
    uploadDate: d.upload_date,
    uploadedBy: d.uploaded_by,
    status: normalizeBackendStatus(d.status),
    signers: (d.signers ?? []).map((s) => ({
      id: s.id,
      name: s.name,
      email: s.email,
      role: s.role,
      status: s.status,
      signedAt: s.signed_at,
    })),
    expiresAt: d.expires_at,
    notes: d.notes,
    tags: d.tags ?? [],
    fileUrl: d.file_url,
    createdAt: d.created_at,
    updatedAt: d.updated_at,
    tenantId: d.tenant_id,
    entityId: d.entity_id,
    entityType: d.entity_type,
    analysisPartyId: d.analysis_party_id,
    analysisCriterion: d.analysis_criterion,
    storageKey: d.storage_key,
    fileHash: d.file_hash,
    source: d.source,
    version: d.version,
    parentId: d.parent_id,
    persistenceStatus: d.persistence_status,
    lastPersistError: d.last_persist_error,
    persistAttemptedAt: d.persist_attempted_at,
  };
}
