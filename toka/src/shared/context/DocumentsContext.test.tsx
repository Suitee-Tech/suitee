import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { CreateDocumentDTO, DocumentDTO } from "../services/document.service";
import { DocumentsProvider, useDocumentsContext } from "./DocumentsContext";

const mocks = vi.hoisted(() => ({
  auth: {
    isAuthenticated: true,
    user: { current_company_id: "tenant-a" } as { current_company_id: string },
  },
  appendAuditLogEntry: vi.fn(),
  can: vi.fn(() => true),
  getAll: vi.fn(),
  getByContract: vi.fn(),
  create: vi.fn(),
  update: vi.fn(),
  delete: vi.fn(),
  toastError: vi.fn(),
}));

vi.mock("./AuthContext", () => ({
  useAuth: () => mocks.auth,
}));

vi.mock("./AuditLogContext", () => ({
  useAuditLogContext: () => ({
    appendAuditLogEntry: mocks.appendAuditLogEntry,
  }),
}));

vi.mock("./PermissionsContext", () => ({
  usePermissionsContext: () => ({
    can: mocks.can,
    isLoading: false,
  }),
}));

vi.mock("../hooks/useDataChangeRefresh", () => ({
  useDataChangeRefresh: () => undefined,
}));

vi.mock("../services/document.service", () => ({
  documentService: {
    getAll: mocks.getAll,
    getByContract: mocks.getByContract,
    create: mocks.create,
    update: mocks.update,
    delete: mocks.delete,
  },
}));

vi.mock("sonner", () => ({
  toast: {
    success: vi.fn(),
    error: mocks.toastError,
  },
}));

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

const createDto: CreateDocumentDTO = {
  name: "Contrato",
  category: "contrato",
  file_type: "PDF",
};

function documentDto(id: string): DocumentDTO {
  return {
    id,
    tenant_id: "tenant-a",
    name: "Contrato",
    file_type: "PDF",
    file_size: 10,
    category: "contrato",
    status: "rascunho",
    source: "manual",
    uploaded_by: "user-1",
    upload_date: "2026-07-28T00:00:00.000Z",
    signers: [],
    version: 1,
    created_at: "2026-07-28T00:00:00.000Z",
    updated_at: "2026-07-28T00:00:00.000Z",
  };
}

describe("DocumentsProvider mutations", () => {
  beforeEach(() => {
    mocks.auth.isAuthenticated = true;
    mocks.auth.user = { current_company_id: "tenant-a" };
    mocks.getAll.mockReset().mockResolvedValue({ data: [] });
    mocks.getByContract.mockReset().mockResolvedValue({ data: [] });
    mocks.create.mockReset();
    mocks.update.mockReset();
    mocks.delete.mockReset();
    mocks.appendAuditLogEntry.mockReset();
    mocks.can.mockClear();
    mocks.toastError.mockReset();
  });

  it("returns applied=false and ignores a create response from the previous tenant", async () => {
    const pendingCreate = deferred<{ data: DocumentDTO }>();
    mocks.create.mockReturnValueOnce(pendingCreate.promise);

    const { result, rerender } = renderHook(() => useDocumentsContext(), {
      wrapper: ({ children }) => <DocumentsProvider>{children}</DocumentsProvider>,
    });
    await waitFor(() => expect(mocks.getAll).toHaveBeenCalled());

    let createPromise!: Promise<boolean>;
    act(() => {
      createPromise = result.current.addDocument(createDto);
    });

    mocks.auth.user = { current_company_id: "tenant-b" };
    rerender();

    await act(async () => {
      pendingCreate.resolve({ data: documentDto("doc-a") });
    });

    await expect(createPromise).resolves.toBe(false);
    expect(result.current.documents).toEqual([]);
    expect(mocks.appendAuditLogEntry).not.toHaveBeenCalled();
  });

  it("rejects a malformed success response instead of reporting a false success", async () => {
    mocks.create.mockResolvedValueOnce({ data: undefined });

    const { result } = renderHook(() => useDocumentsContext(), {
      wrapper: ({ children }) => <DocumentsProvider>{children}</DocumentsProvider>,
    });
    await waitFor(() => expect(mocks.getAll).toHaveBeenCalled());

    await expect(result.current.addDocument(createDto)).rejects.toThrow(
      "A API não retornou o documento criado.",
    );
    expect(mocks.toastError).toHaveBeenCalledWith("Erro ao adicionar documento.");
  });
});
