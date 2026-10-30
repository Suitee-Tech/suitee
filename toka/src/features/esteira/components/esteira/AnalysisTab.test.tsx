import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { DealDTO } from "../../../../shared/services/deal.service";
import { AnalysisTab } from "./AnalysisTab";

const mocks = vi.hoisted(() => ({
  updateDueDiligence: vi.fn(),
  updatePartyDueDiligence: vi.fn(),
  notifyApiError: vi.fn(),
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
  getAnalysisDocuments: vi.fn(),
  uploadAnalysisDocument: vi.fn(),
  deleteAnalysisDocument: vi.fn(),
  getPresignedDownloadUrl: vi.fn(),
}));

vi.mock("../../../../shared/context/DealsContext", () => ({
  useDealsContext: () => ({
    updateDueDiligence: mocks.updateDueDiligence,
    updatePartyDueDiligence: mocks.updatePartyDueDiligence,
  }),
}));

vi.mock("../../../../shared/services/errorToast", () => ({
  notifyApiError: mocks.notifyApiError,
}));

vi.mock("../../../../shared/config/flags", () => ({
  flags: { multiPartyUI: true },
}));

vi.mock("../../../../shared/components/PartyDueDiligenceCard", () => ({
  ANALYSIS_CRITERIA: ["score", "cnd", "legal_opinion", "income"],
  PartyDueDiligenceCard: ({
    party,
    onUploadEvidence,
    evidenceByCriterion,
  }: {
    party: { name: string };
    onUploadEvidence?: (criterion: "cnd", file: File) => Promise<void>;
    evidenceByCriterion?: { score?: Array<{ id: string }> };
  }) => (
    <div>
      Análise individual: {party.name}
      <span>Evidências de Score: {evidenceByCriterion?.score?.length ?? 0}</span>
      {onUploadEvidence && (
        <button type="button" onClick={() => void onUploadEvidence("cnd", new File(["cnd"], "cnd.pdf", { type: "application/pdf" }))}>
          Anexar CND de {party.name}
        </button>
      )}
    </div>
  ),
}));

vi.mock("../../../../shared/services/document.service", () => ({
  documentService: {
    getByEntity: mocks.getAnalysisDocuments,
    upload: mocks.uploadAnalysisDocument,
    delete: mocks.deleteAnalysisDocument,
    getPresignedDownloadUrl: mocks.getPresignedDownloadUrl,
  },
}));

vi.mock("sonner", () => ({
  toast: {
    success: mocks.toastSuccess,
    error: mocks.toastError,
  },
}));

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((done, fail) => {
    resolve = done;
    reject = fail;
  });
  return { promise, resolve, reject };
}

function makeDeal(withParty = false): DealDTO {
  return {
    id: "deal-1",
    tenant_id: "tenant-1",
    type: "RENT",
    status: "ANALYSIS",
    property_id: "property-1",
    owner_id: "owner-1",
    customer_id: "customer-1",
    broker_id: "broker-1",
    value: 3_200,
    commission: 0,
    created_at: "2026-07-28T12:00:00.000Z",
    updated_at: "2026-07-28T12:00:00.000Z",
    due_diligence: {
      score_serasa: 700,
      cnd_status: "OK",
      legal_opinion: "Sem restrições",
      status: "PENDING",
      updated_at: "2026-07-28T12:00:00.000Z",
    },
    parties: withParty
      ? [
          {
            id: "party-1",
            name: "Fernando Pessoa",
            email: "fernando@example.com",
            role: "tenant",
            is_primary: true,
            created_at: "2026-07-28T12:00:00.000Z",
            updated_at: "2026-07-28T12:00:00.000Z",
          },
        ]
      : [],
    signers: [],
  };
}

describe("AnalysisTab async state", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.getAnalysisDocuments.mockResolvedValue({ data: [] });
    mocks.uploadAnalysisDocument.mockResolvedValue({ id: "document-1", name: "cnd.pdf" });
  });

  it("uses the canonical context mutation and keeps the aggregate editor pending", async () => {
    const user = userEvent.setup();
    const request = deferred<DealDTO | null>();
    mocks.updateDueDiligence.mockReturnValueOnce(request.promise);
    render(<AnalysisTab dealDetails={makeDeal()} />);

    await user.click(screen.getByRole("button", { name: "Editar" }));
    await user.click(screen.getByRole("button", { name: "Salvar" }));

    expect(mocks.updateDueDiligence).toHaveBeenCalledWith(
      "deal-1",
      expect.objectContaining({
        score_serasa: 700,
        cnd_status: "OK",
        legal_opinion: "Sem restrições",
        status: "PENDING",
      }),
    );
    expect(screen.getByRole("button", { name: "Salvando…" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Cancelar" })).toBeDisabled();

    await act(async () => {
      request.resolve(makeDeal());
      await request.promise;
    });

    await waitFor(() => {
      expect(mocks.toastSuccess).toHaveBeenCalledWith("Análise atualizada!");
    });
    expect(screen.queryByRole("button", { name: "Salvar" })).not.toBeInTheDocument();
  });

  it("keeps aggregate edits visible when persistence fails", async () => {
    const user = userEvent.setup();
    const error = new Error("update failed");
    mocks.updateDueDiligence.mockRejectedValueOnce(error);
    render(<AnalysisTab dealDetails={makeDeal()} />);

    await user.click(screen.getByRole("button", { name: "Editar" }));
    await user.clear(screen.getByPlaceholderText("Ex: 750"));
    await user.type(screen.getByPlaceholderText("Ex: 750"), "650");
    await user.click(screen.getByRole("button", { name: "Salvar" }));

    await waitFor(() => {
      expect(mocks.notifyApiError).toHaveBeenCalledWith(error, {
        context: "Atualizar análise",
      });
    });
    expect(screen.getByDisplayValue("650")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Salvar" })).toBeEnabled();
  });

  it("can switch from the aggregate view to party cards without changing hook order", () => {
    const view = render(<AnalysisTab dealDetails={makeDeal()} />);
    expect(screen.getByText("Consulta CPF/CNPJ")).toBeInTheDocument();

    view.rerender(<AnalysisTab dealDetails={makeDeal(true)} />);

    expect(screen.getByText("Análise individual: Fernando Pessoa")).toBeInTheDocument();
    expect(screen.getByText("Resumo da análise")).toBeInTheDocument();
  });

  it("links an uploaded criterion document to the deal, party and CND", async () => {
    const user = userEvent.setup();
    render(<AnalysisTab dealDetails={makeDeal(true)} />);

    await waitFor(() => {
      expect(mocks.getAnalysisDocuments).toHaveBeenCalledWith("deal-1", "analysis");
    });
    await user.click(screen.getByRole("button", { name: "Anexar CND de Fernando Pessoa" }));

    await waitFor(() => {
      expect(mocks.uploadAnalysisDocument).toHaveBeenCalledWith(
        expect.objectContaining({ name: "cnd.pdf" }),
        expect.objectContaining({
          category: "fiscal",
          entity_id: "deal-1",
          entity_type: "analysis",
          analysis_party_id: "party-1",
          analysis_criterion: "cnd",
        }),
      );
    });
    expect(mocks.toastSuccess).toHaveBeenCalledWith("Evidência de CND anexada.");
  });

  it("keeps general documents separate from party criterion evidence", async () => {
    mocks.getAnalysisDocuments.mockResolvedValueOnce({
      data: [
        {
          id: "score-document",
          name: "score-fernando.pdf",
          analysis_party_id: "party-1",
          analysis_criterion: "score",
        },
        {
          id: "general-document",
          name: "politica-geral.pdf",
          category: "comprovante",
          status: "pendente",
        },
      ],
    });

    render(<AnalysisTab dealDetails={makeDeal(true)} />);

    expect(await screen.findByText("Evidências de Score: 1")).toBeInTheDocument();
    expect(screen.getByText("politica-geral.pdf")).toBeInTheDocument();
    expect(screen.queryByText("score-fernando.pdf")).not.toBeInTheDocument();
    const documentsHeading = screen.getByRole("heading", { name: /Documentos de apoio/i });
    const partyHeading = screen.getByRole("heading", { name: /Inquilino/i });
    expect(
      partyHeading.compareDocumentPosition(documentsHeading) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });
});
