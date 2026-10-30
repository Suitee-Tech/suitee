import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { PartyDTO } from "../services/deal.service";
import { PartyDueDiligenceCard } from "./PartyDueDiligenceCard";

const mocks = vi.hoisted(() => ({
  notifyApiError: vi.fn(),
}));

vi.mock("../services/errorToast", () => ({
  notifyApiError: mocks.notifyApiError,
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

function makeParty(id = "party-1"): PartyDTO {
  return {
    id,
    name: "Fernando Pessoa",
    email: "fernando@example.com",
    role: "tenant",
    is_primary: true,
    due_diligence: {
      score_serasa: 700,
      cnd_status: "OK",
      legal_opinion: "Sem restrições",
      monthly_income: 5000,
      status: "PENDING",
      updated_at: "2026-07-28T12:00:00.000Z",
    },
    created_at: "2026-07-28T12:00:00.000Z",
    updated_at: "2026-07-28T12:00:00.000Z",
  };
}

describe("PartyDueDiligenceCard async state", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("keeps the editor pending until the mutation succeeds and blocks duplicate saves", async () => {
    const user = userEvent.setup();
    const request = deferred<unknown>();
    const update = vi.fn().mockReturnValueOnce(request.promise);
    render(
      <PartyDueDiligenceCard
        dealId="deal-1"
        party={makeParty()}
        onUpdatePartyDueDiligence={update}
      />,
    );

    await user.click(screen.getByRole("button", { name: /Editar/i }));
    await user.click(screen.getByRole("button", { name: "Salvar análise" }));

    expect(update).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("button", { name: "Salvando…" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Cancelar" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "Salvando…" }));
    expect(update).toHaveBeenCalledTimes(1);

    await act(async () => {
      request.resolve({});
      await request.promise;
    });

    await waitFor(() => {
      expect(screen.queryByRole("button", { name: "Salvar análise" })).not.toBeInTheDocument();
    });
  });

  it("keeps the editor open and surfaces a failed mutation", async () => {
    const user = userEvent.setup();
    const error = new Error("update failed");
    const update = vi.fn().mockRejectedValueOnce(error);
    render(
      <PartyDueDiligenceCard
        dealId="deal-1"
        party={makeParty()}
        onUpdatePartyDueDiligence={update}
      />,
    );

    await user.click(screen.getByRole("button", { name: /Editar/i }));
    await user.click(screen.getByRole("button", { name: "Salvar análise" }));

    await waitFor(() => {
      expect(mocks.notifyApiError).toHaveBeenCalledWith(error, {
        context: "Atualizar análise da parte",
      });
    });
    expect(screen.getByRole("button", { name: "Salvar análise" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Cancelar" })).toBeEnabled();
  });

  it("does not let an old deal request close the editor for the next deal", async () => {
    const user = userEvent.setup();
    const request = deferred<unknown>();
    const update = vi.fn().mockReturnValueOnce(request.promise);
    const view = render(
      <PartyDueDiligenceCard
        dealId="deal-1"
        party={makeParty()}
        onUpdatePartyDueDiligence={update}
      />,
    );

    await user.click(screen.getByRole("button", { name: /Editar/i }));
    await user.click(screen.getByRole("button", { name: "Salvar análise" }));

    view.rerender(
      <PartyDueDiligenceCard
        dealId="deal-2"
        party={makeParty()}
        onUpdatePartyDueDiligence={update}
      />,
    );
    await user.click(screen.getByRole("button", { name: /Editar/i }));

    await act(async () => {
      request.resolve({});
      await request.promise;
    });

    expect(screen.getByRole("button", { name: "Salvar análise" })).toBeInTheDocument();
    expect(mocks.notifyApiError).not.toHaveBeenCalled();
  });

  it("shows independent evidence areas and uploads to the selected criterion", async () => {
    const user = userEvent.setup();
    const uploadEvidence = vi.fn().mockResolvedValue(undefined);
    const { container } = render(
      <PartyDueDiligenceCard
        dealId="deal-1"
        party={makeParty()}
        onUpdatePartyDueDiligence={vi.fn()}
        evidenceByCriterion={{
          score: [
            {
              id: "document-score",
              tenant_id: "tenant-1",
              name: "consulta-score.pdf",
              file_type: "application/pdf",
              file_size: 100,
              category: "comprovante",
              status: "pendente",
              source: "toca-deal",
              uploaded_by: "broker-1",
              upload_date: "2026-07-28T12:00:00.000Z",
              signers: [],
              version: 1,
              created_at: "2026-07-28T12:00:00.000Z",
              updated_at: "2026-07-28T12:00:00.000Z",
            },
          ],
        }}
        onUploadEvidence={uploadEvidence}
      />,
    );

    expect(screen.getByRole("region", { name: "Critério Score" })).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Critério CND" })).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Critério Parecer" })).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Critério Renda" })).toBeInTheDocument();
    expect(screen.getByText("consulta-score.pdf")).toBeInTheDocument();
    expect(screen.getByText("R$ 5.000,00")).toBeInTheDocument();

    const inputs = container.querySelectorAll<HTMLInputElement>('input[type="file"]');
    expect(inputs).toHaveLength(4);
    const cndFile = new File(["cnd"], "cnd.pdf", { type: "application/pdf" });
    await user.upload(inputs[1], cndFile);

    expect(uploadEvidence).toHaveBeenCalledWith("cnd", cndFile);

    const incomeFile = new File(["renda"], "comprovante.pdf", { type: "application/pdf" });
    await user.upload(inputs[3], incomeFile);
    expect(uploadEvidence).toHaveBeenCalledWith("income", incomeFile);
  });

  it("salva a renda mensal no formulário de edição", async () => {
    const user = userEvent.setup();
    const update = vi.fn().mockResolvedValue(undefined);
    render(
      <PartyDueDiligenceCard
        dealId="deal-1"
        party={makeParty()}
        onUpdatePartyDueDiligence={update}
      />,
    );

    await user.click(screen.getByRole("button", { name: /Editar/i }));
    const income = screen.getByLabelText("Renda mensal (R$)");
    await user.clear(income);
    await user.type(income, "6000");
    await user.click(screen.getByRole("button", { name: "Salvar análise" }));

    expect(update).toHaveBeenCalledWith(
      "deal-1",
      "party-1",
      expect.objectContaining({ monthly_income: 6000 }),
    );
  });

  it("atualiza o status de um critério", async () => {
    const user = userEvent.setup();
    const updateCriterion = vi.fn().mockResolvedValue(undefined);
    render(
      <PartyDueDiligenceCard
        dealId="deal-1"
        party={makeParty()}
        onUpdatePartyDueDiligence={vi.fn()}
        onUpdateCriterion={updateCriterion}
      />,
    );

    await user.selectOptions(screen.getByLabelText("Status do critério Score"), "approved");
    expect(updateCriterion).toHaveBeenCalledWith("score", "approved");
  });
});
