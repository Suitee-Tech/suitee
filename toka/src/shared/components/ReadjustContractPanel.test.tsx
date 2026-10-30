import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { contractService } from "../services/contract.service";
import { ContractStatus, type Contract } from "../contracts/types";

vi.mock("../services/contract.service", () => ({
  contractService: { readjust: vi.fn() },
}));

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

import { ReadjustContractPanel } from "./ReadjustContractPanel";

const contrato = (overrides: Partial<Contract> = {}) =>
  ({
    id: "c1",
    status: ContractStatus.SIGNED,
    value: 3430,
    adjust_index: "IGPM",
    ...overrides,
  }) as Contract;

const renderPanel = (contract = contrato(), onCompleted = vi.fn()) => {
  render(
    <ReadjustContractPanel open onClose={vi.fn()} contract={contract} onCompleted={onCompleted} />,
  );
  return onCompleted;
};

describe("ReadjustContractPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(contractService.readjust).mockResolvedValue(contrato({ value: 3584.35 }));
  });

  // O modal anterior aplicava o reajuste no clique do botão: uma alteração de
  // valor com efeito em cobranças já emitidas, sem nenhuma confirmação. Agora o
  // primeiro clique apenas abre a confirmação.
  it("não aplica o reajuste ao revisar: só abre a confirmação", async () => {
    const user = userEvent.setup();
    renderPanel();

    await user.type(screen.getByLabelText(/%\s*de reajuste/i), "4.5");
    await user.click(screen.getByRole("button", { name: /Revisar e aplicar/ }));

    await waitFor(() => expect(screen.getByText(/Confirmar reajuste do contrato/)).toBeInTheDocument());
    expect(contractService.readjust).not.toHaveBeenCalled();
  });

  it("aplica o reajuste somente na confirmação, com o índice informado", async () => {
    const user = userEvent.setup();
    const onCompleted = renderPanel();

    await user.type(screen.getByLabelText(/%\s*de reajuste/i), "4.5");
    await user.click(screen.getByRole("button", { name: /Revisar e aplicar/ }));
    await waitFor(() => expect(screen.getByText(/Confirmar reajuste do contrato/)).toBeInTheDocument());
    await user.click(screen.getByRole("button", { name: /^Aplicar reajuste$/ }));

    await waitFor(() => expect(contractService.readjust).toHaveBeenCalledTimes(1));
    const [id, req] = vi.mocked(contractService.readjust).mock.calls[0];
    expect(id).toBe("c1");
    expect(req.index_value).toBe(4.5);
    expect(req.index_name).toBe("IGPM");
    // `effective_from` é o que define QUAIS cobranças são recalculadas: precisa ir.
    expect(req.effective_from).toBeTruthy();
    await waitFor(() => expect(onCompleted).toHaveBeenCalled());
  });

  // O aviso dizia que cobranças já emitidas não são alteradas. O financeiro
  // recalcula as PENDING com vencimento a partir da vigência — o operador tem de
  // saber disso antes de confirmar.
  it("avisa que cobranças em aberto a partir da vigência serão recalculadas", async () => {
    const user = userEvent.setup();
    renderPanel();
    // Aparece na descrição do painel e no aviso: os dois precisam existir.
    expect(screen.getAllByText(/recalcula as cobranças em aberto/i).length).toBeGreaterThanOrEqual(2);

    await user.type(screen.getByLabelText(/%\s*de reajuste/i), "4.5");
    await user.click(screen.getByRole("button", { name: /Revisar e aplicar/ }));

    // O texto é quebrado por um <strong> no meio; compara pelo conteúdo
    // normalizado do parágrafo.
    await waitFor(() => {
      const paragrafos = Array.from(document.querySelectorAll("p"));
      const achou = paragrafos.some((p) =>
        /em aberto com vencimento a partir da vigência serão recalculadas/i.test(p.textContent || ""),
      );
      expect(achou).toBe(true);
    });
    expect(screen.queryByText(/já emitidas não são alteradas/i)).not.toBeInTheDocument();
  });

  it("mostra o valor atual e o novo valor com a variação", async () => {
    const user = userEvent.setup();
    renderPanel();
    await user.type(screen.getByLabelText(/%\s*de reajuste/i), "4.5");

    // 3430 * 1.045 = 3584.35
    await waitFor(() => expect(screen.getByText("R$ 3.584,35")).toBeInTheDocument());
  });

  it("não deixa aplicar sem índice informado", () => {
    renderPanel();
    expect(screen.getByRole("button", { name: /Revisar e aplicar/ })).toBeDisabled();
  });

  // A data já agendada no contrato é o padrão do reajuste: usar hoje mudaria
  // quais cobranças entram no recálculo.
  it("usa a data agendada do contrato como vigência", () => {
    renderPanel(contrato({ next_adjustment: "2027-09-14T00:00:00Z" }));
    expect(screen.getByLabelText(/Vigente a partir de/)).toHaveValue("2027-09-14");
  });
});
