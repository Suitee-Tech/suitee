import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { toast } from "sonner";
import { useAuth } from "./AuthContext";
import { useAuditLogContext } from "./AuditLogContext";
import { usePermissionsContext } from "./PermissionsContext";
import { useDataChangeRefresh } from "../hooks/useDataChangeRefresh";
import { useRequestScopeGuard } from "../hooks/useRequestScopeGuard";
import { normalizeBackendStatus } from "../lib/documents";
import { unwrapApiData } from "../services/api";
import { documentService, type CreateDocumentDTO, type DocumentDTO, type UpdateDocumentDTO } from "../services/document.service";
import type { ContractDocument } from "../types/contractDocument";

type DocumentsContextValue = {
  documents: ContractDocument[];
  addDocument: (dto: CreateDocumentDTO) => Promise<boolean>;
  updateDocument: (id: string, updates: UpdateDocumentDTO) => Promise<boolean>;
  deleteDocument: (id: string) => Promise<boolean>;
  fetchDocumentsByContract: (contractId: string) => Promise<ContractDocument[]>;
  fetchDocuments: () => Promise<void>;
};

const DocumentsContext = createContext<DocumentsContextValue | null>(null);

export function DocumentsProvider({ children }: { children: ReactNode }) {
  const { isAuthenticated, user } = useAuth();
  const { can, isLoading: permissionsLoading } = usePermissionsContext();
  const { appendAuditLogEntry } = useAuditLogContext();

  const [documents, setDocuments] = useState<ContractDocument[]>([]);
  const requestScope = `${isAuthenticated}:${user?.current_company_id ?? ""}`;
  const beginScopedRequest = useRequestScopeGuard(requestScope);
  const docsMutationRef = useRef<Promise<unknown> | null>(null);
  const docsVersionRef = useRef(0);
  const docsCooldownUntilRef = useRef<number>(0);

  const mapDocDTO = useCallback(
    (d: DocumentDTO): ContractDocument => ({
      id: d.id,
      contractId: d.contract_id || "",
      name: d.name,
      category: d.category,
      fileType: d.file_type,
      fileSize: d.file_size,
      uploadDate: d.upload_date,
      uploadedBy: d.uploaded_by,
      status: normalizeBackendStatus(d.status),
      signers: (d.signers || []).map((s) => ({ ...s, signedAt: s.signed_at })),
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
    }),
    [],
  );

  const fetchDocumentsByContract = useCallback(
    async (contractId: string): Promise<ContractDocument[]> => {
      const isCurrent = beginScopedRequest();
      try {
        const res = await documentService.getByContract(contractId);
        const raw = unwrapApiData<DocumentDTO[]>(res) ?? [];
        const mapped = raw.map(mapDocDTO);
        if (!isCurrent()) return [];

        setDocuments((prev) => {
          const otherDocs = prev.filter((d) => d.contractId !== contractId);
          return [...otherDocs, ...mapped];
        });

        return mapped;
      } catch (error) {
        if (!isCurrent()) return [];
        console.error("Erro ao buscar documentos:", error);
        return [];
      }
    },
    [beginScopedRequest, mapDocDTO],
  );

  const fetchDocuments = useCallback(async () => {
    const pendingMutation = docsMutationRef.current;
    if (pendingMutation) {
      try { await pendingMutation; } catch {}
    }
    const isCurrent = beginScopedRequest();
    const fetchVersion = docsVersionRef.current;
    try {
      const res = await documentService.getAll();
      const raw = unwrapApiData<DocumentDTO[]>(res);
      if (!isCurrent()) return;
      if (docsVersionRef.current !== fetchVersion) return;
      if (Date.now() < docsCooldownUntilRef.current) return;
      if (Array.isArray(raw)) {
        const mapped = raw.map(mapDocDTO);
        setDocuments(mapped);
      } else {
        setDocuments([]);
      }
    } catch (error) {
      if (!isCurrent()) return;
      console.error("Erro ao buscar documentos:", error);
    }
  }, [beginScopedRequest, mapDocDTO]);

  const addDocument = useCallback(
    async (dto: CreateDocumentDTO) => {
      let releasePlaceholder: () => void = () => {};
      const placeholder = new Promise<void>((r) => { releasePlaceholder = r; });
      docsMutationRef.current = placeholder;
      ++docsVersionRef.current;
      const isCurrent = beginScopedRequest();
      let mutationPromise: Promise<unknown> | undefined;
      try {
        const res = await (mutationPromise = documentService.create(dto));
        const created = unwrapApiData<DocumentDTO>(res);
        if (!isCurrent()) { releasePlaceholder(); return false; }
        if (!created?.id) throw new Error("A API não retornou o documento criado.");

        const doc = mapDocDTO(created);
        setDocuments((prev) => [...prev, doc]);
        docsCooldownUntilRef.current = Date.now() + 2000;
        appendAuditLogEntry({
          id: `AUD-${Date.now()}-d`,
          entityType: "document",
          entityId: doc.id,
          action: "criacao",
          changes: [
            { field: "Nome", oldValue: "", newValue: doc.name },
            { field: "Categoria", oldValue: "", newValue: doc.category },
          ],
          timestamp: new Date().toISOString(),
          user: "Usuario Atual",
        });
        toast.success("Documento adicionado com sucesso!");
        return true;
      } catch (error) {
        if (!isCurrent()) { releasePlaceholder(); return false; }
        ++docsVersionRef.current;
        console.error("Erro ao criar documento:", error);
        toast.error("Erro ao adicionar documento.");
        throw error;
      } finally {
        releasePlaceholder();
        if (mutationPromise && docsMutationRef.current === placeholder || docsMutationRef.current === mutationPromise) {
          docsMutationRef.current = null;
        }
      }
    },
    [appendAuditLogEntry, beginScopedRequest, mapDocDTO],
  );

  const updateDocument = useCallback(
    async (id: string, updates: UpdateDocumentDTO) => {
      let releasePlaceholder: () => void = () => {};
      const placeholder = new Promise<void>((r) => { releasePlaceholder = r; });
      docsMutationRef.current = placeholder;
      ++docsVersionRef.current;
      const isCurrent = beginScopedRequest();
      const oldDoc = documents.find((d) => d.id === id);
      let mutationPromise: Promise<unknown> | undefined;

      try {
        const res = await (mutationPromise = documentService.update(id, updates));
        const updated = unwrapApiData<DocumentDTO>(res);
        if (!isCurrent()) { releasePlaceholder(); return false; }
        if (!updated?.id) throw new Error("A API não retornou o documento atualizado.");

        const mapped = mapDocDTO(updated);
        setDocuments((prev) => prev.map((d) => (d.id === id ? mapped : d)));
        docsCooldownUntilRef.current = Date.now() + 2000;

        if (oldDoc) {
          const changes: { field: string; oldValue: string; newValue: string }[] = [];
          if (updates.name && updates.name !== oldDoc.name) changes.push({ field: "Nome", oldValue: oldDoc.name, newValue: updates.name });
          if (updates.status && updates.status !== oldDoc.status) changes.push({ field: "Status", oldValue: oldDoc.status, newValue: updates.status });
          if (updates.category && updates.category !== oldDoc.category) {
            changes.push({ field: "Categoria", oldValue: oldDoc.category, newValue: updates.category });
          }

          if (changes.length > 0) {
            appendAuditLogEntry({
              id: `AUD-${Date.now()}-${Math.random().toString(36).substring(2, 6)}`,
              entityType: "document",
              entityId: id,
              action: updates.status ? "status_change" : "edicao",
              changes,
              timestamp: new Date().toISOString(),
              user: "Usuario Atual",
            });
          }
        }
        return true;
      } catch (error) {
        if (!isCurrent()) { releasePlaceholder(); return false; }
        ++docsVersionRef.current;
        console.error("Erro ao atualizar documento:", error);
        toast.error("Erro ao atualizar documento.");
        throw error;
      } finally {
        releasePlaceholder();
        if (mutationPromise && docsMutationRef.current === placeholder || docsMutationRef.current === mutationPromise) {
          docsMutationRef.current = null;
        }
      }
    },
    [appendAuditLogEntry, beginScopedRequest, documents, mapDocDTO],
  );

  const deleteDocument = useCallback(
    async (id: string) => {
      let releasePlaceholder: () => void = () => {};
      const placeholder = new Promise<void>((r) => { releasePlaceholder = r; });
      docsMutationRef.current = placeholder;
      ++docsVersionRef.current;
      const isCurrent = beginScopedRequest();
      const doc = documents.find((d) => d.id === id);
      let mutationPromise: Promise<unknown> | undefined;
      try {
        await (mutationPromise = documentService.delete(id));
        if (!isCurrent()) { releasePlaceholder(); return false; }
        setDocuments((prev) => prev.filter((d) => d.id !== id));
        docsCooldownUntilRef.current = Date.now() + 2000;
        if (doc) {
          appendAuditLogEntry({
            id: `AUD-${Date.now()}-dd`,
            entityType: "document",
            entityId: id,
            action: "exclusao",
            changes: [
              { field: "Nome", oldValue: doc.name, newValue: "" },
              { field: "Categoria", oldValue: doc.category, newValue: "" },
            ],
            timestamp: new Date().toISOString(),
            user: "Usuario Atual",
          });
          toast.success("Documento removido.");
        }
        return true;
      } catch (error) {
        if (!isCurrent()) { releasePlaceholder(); return false; }
        ++docsVersionRef.current;
        console.error("Erro ao excluir documento:", error);
        toast.error("Erro ao excluir documento.");
        throw error;
      } finally {
        releasePlaceholder();
        if (mutationPromise && docsMutationRef.current === placeholder || docsMutationRef.current === mutationPromise) {
          docsMutationRef.current = null;
        }
      }
    },
    [appendAuditLogEntry, beginScopedRequest, documents],
  );

  useDataChangeRefresh(["documents", "signatures", "contracts", "checklists"], fetchDocuments, {
    enabled:
      isAuthenticated &&
      Boolean(user?.current_company_id) &&
      !permissionsLoading &&
      can("documents:read"),
  });

  useEffect(() => {
    if (!isAuthenticated || !user?.current_company_id || permissionsLoading) {
      setDocuments([]);
      return;
    }

    setDocuments([]);

    if (!can("documents:read")) return;

    fetchDocuments().catch((error) => {
      console.error("Erro ao carregar documentos:", error);
    });
  }, [can, fetchDocuments, isAuthenticated, permissionsLoading, user?.current_company_id]);

  const value = useMemo<DocumentsContextValue>(
    () => ({
      documents,
      addDocument,
      updateDocument,
      deleteDocument,
      fetchDocumentsByContract,
      fetchDocuments,
    }),
    [addDocument, deleteDocument, documents, fetchDocuments, fetchDocumentsByContract, updateDocument],
  );

  return <DocumentsContext.Provider value={value}>{children}</DocumentsContext.Provider>;
}

export function useDocumentsContext() {
  const ctx = useContext(DocumentsContext);
  if (!ctx) throw new Error("useDocumentsContext must be used within DocumentsProvider");
  return ctx;
}
