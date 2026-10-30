import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ContractTemplates } from "./ContractTemplates";

// O editor de modelos é DOCX (SuperDoc) + detecção de variáveis: pesado e
// dependente de canvas. Aqui ele é reduzido ao mínimo para provar o CONTRATO do
// modo embutido — o painel da esteira monta esta tela para cadastrar o primeiro
// modelo, e nesse modo ela abre direto no cadastro, fecha pelo `onExit` e devolve
// o modelo salvo por `onSaved`.
vi.mock("superdoc", () => ({
  BlankDOCX: new Uint8Array(),
  getFileObject: async (_content: unknown, name: string) =>
    new File([new Uint8Array()], name, {
      type: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
    }),
}));

vi.mock("@superdoc-dev/react", () => ({
  SuperDocEditor: () => <div data-testid="superdoc-editor" />,
}));

vi.mock("../hooks/useVariableDetection", () => ({
  useVariableDetection: () => ({ variables: [], triggerScan: vi.fn() }),
}));

vi.mock("../hooks/useDomVariableHighlight", () => ({ useDomVariableHighlight: vi.fn() }));

vi.mock("../components/contract-editor/VariableSidebar", () => ({ VariableSidebar: () => null }));
vi.mock("../components/contract-editor/TextToVariablePopover", () => ({ TextToVariablePopover: () => null }));
vi.mock("../components/contract-editor/VariableMappingDialog", () => ({ VariableMappingDialog: () => null }));

const templateMocks = vi.hoisted(() => ({
  list: vi.fn(async () => []),
  create: vi.fn(),
  update: vi.fn(),
}));

vi.mock("../services/template.service", () => ({ templateService: templateMocks }));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() } }));

beforeEach(() => {
  vi.clearAllMocks();
  templateMocks.list.mockResolvedValue([]);
});

describe("ContractTemplates no modo embutido", () => {
  it("abre direto no cadastro e esconde a lista e a tela cheia", async () => {
    render(<ContractTemplates embedded onExit={vi.fn()} onSaved={vi.fn()} />);

    expect(await screen.findByPlaceholderText("Nome do modelo...")).toBeInTheDocument();
    expect(await screen.findByTestId("superdoc-editor")).toBeInTheDocument();
    // A LISTA da tela de modelos nunca aparece dentro do painel da esteira.
    expect(screen.queryByRole("heading", { name: "Modelos de Contrato" })).not.toBeInTheDocument();
    expect(screen.queryByTitle("Tela Cheia")).not.toBeInTheDocument();
  });

  it("fecha pelo onExit em vez de voltar para a lista", async () => {
    const onExit = vi.fn();
    const user = userEvent.setup();
    render(<ContractTemplates embedded onExit={onExit} onSaved={vi.fn()} />);

    await user.click(await screen.findByRole("button", { name: "Fechar cadastro do modelo" }));

    expect(onExit).toHaveBeenCalledTimes(1);
    // Sem `onExit` a tela cairia na lista de modelos; dentro do painel ela
    // apenas some — quem fecha o painel é a esteira.
    await waitFor(() =>
      expect(screen.queryByRole("heading", { name: "Modelos de Contrato" })).not.toBeInTheDocument(),
    );
  });

  it("na rota, segue abrindo pela lista (o cadastro não abre sozinho)", async () => {
    render(<ContractTemplates />);

    expect(await screen.findByRole("heading", { name: "Modelos de Contrato" })).toBeInTheDocument();
    expect(screen.queryByPlaceholderText("Nome do modelo...")).not.toBeInTheDocument();
  });
});
