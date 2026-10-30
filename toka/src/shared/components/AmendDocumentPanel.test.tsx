import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { contractService } from "../services/contract.service";
import { templateService } from "../services/template.service";
import { ContractStatus, type Contract } from "../contracts/types";

vi.mock("../services/contract.service", () => ({
  contractService: { amend: vi.fn(), update: vi.fn() },
}));

vi.mock("../services/template.service", () => ({
  templateService: { list: vi.fn() },
}));

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

vi.mock("react-router", () => ({ useNavigate: () => vi.fn() }));

// O editor é um componente de terceiro pesado; aqui interessa o contrato do
// painel com o serviço, não o SuperDoc em si.
const { exportDocument } = vi.hoisted(() => ({ exportDocument: vi.fn() }));
vi.mock("@superdoc-dev/react", async () => {
  const { forwardRef, useImperativeHandle } = await import("react");
  return { SuperDocEditor: forwardRef((_props, ref) => {
    useImperativeHandle(ref, () => ({ getInstance: () => ({ exportEditorsToDOCX: exportDocument }) }));
    return <div data-testid="editor" />;
  }) };
});

// O contexto de variáveis vem do contrato (deal, imóvel, partes). Aqui só
// interessa o que o painel faz com ele.
vi.mock("../hooks/useContractVariableData", () => ({
  useContractVariableData: () => ({ data: { buyer_name: "Lana del Rey", property_address: "Rua A, 10" }, loading: false }),
}));

vi.mock("../utils/docx", async () => {
  const actual = await vi.importActual<typeof import("../utils/docx")>("../utils/docx");
  return {
    ...actual,
    extractDocxTemplateVariables: vi.fn(() => ["buyer_name"]),
    fillDocxTemplate: vi.fn(() => "cHJlZW5jaGlkby1jb20tdmFyaWF2ZWlz"),
  };
});
vi.mock("@superdoc-dev/react/style.css", () => ({}));

import { AmendDocumentPanel } from "./AmendDocumentPanel";

const contract = { id: "c1", status: "DRAFT", content: "Y29udHJhdG8tb3JpZ2luYWw=" } as Contract;

const renderPanel = () =>
  render(<AmendDocumentPanel open onClose={vi.fn()} contract={contract} />);

describe("AmendDocumentPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    exportDocument.mockResolvedValue([new Blob(["revised-document"])]);
    vi.mocked(templateService.list).mockResolvedValue([]);
    vi.mocked(contractService.amend).mockResolvedValue({ id: "novo" } as Contract);
  });


  it("salva a revisão no mesmo contrato antes de abrir a assinatura", async () => {
    const user = userEvent.setup();
    const completed = vi.fn();
    vi.mocked(contractService.update).mockResolvedValue(contract);
    render(<AmendDocumentPanel open mode="edit" onClose={vi.fn()} contract={contract} onCompleted={completed} />);
    await user.click(screen.getByRole("button", { name: "Salvar e revisar assinatura" }));
    await waitFor(() => expect(completed).toHaveBeenCalledWith(contract, true));
    // O conteúdo é gravado como data URL: o prefixo `data:...;base64,` é o que
    // diz ao renderizador do backend que aquilo é um DOCX. Sem ele o contrato
    // chegava ao signatário como uma página de base64 cru.
    const conteudo = vi.mocked(contractService.update).mock.calls[0][1] as { content: string };
    expect(conteudo.content.startsWith("data:")).toBe(true);
    expect(conteudo.content).toContain(btoa("revised-document"));
    expect(contractService.amend).not.toHaveBeenCalled();
  });

  it("mantém o editor aberto e não salva conteúdo antigo quando a exportação falha", async () => {
    exportDocument.mockResolvedValue([]);
    const completed = vi.fn();
    render(<AmendDocumentPanel open mode="edit" onClose={vi.fn()} contract={contract} onCompleted={completed} />);
    await userEvent.click(screen.getByRole("button", { name: "Salvar alterações" }));
    const { toast } = await import("sonner");
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("Não foi possível obter o documento do aditivo."));
    expect(contractService.update).not.toHaveBeenCalled();
    expect(completed).not.toHaveBeenCalled();
  });

  // O fluxo antigo criava o aditivo sem documento: o que ia para assinatura era
  // o conteúdo do contrato original. Este é o comportamento que não pode voltar.
  it("envia o documento carregado como conteúdo do aditivo", async () => {
    const user = userEvent.setup();
    renderPanel();

    await user.click(screen.getByText("Carregar documento"));
    const file = new File(["docx-aditivo"], "aditivo.docx", {
      type: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
    });
    const input = document.querySelector('input[type="file"]') as HTMLInputElement;
    await user.upload(input, file);

    await user.type(screen.getByPlaceholderText(/prorrogação de prazo/), "Prorrogação");
    await user.click(screen.getByRole("button", { name: /Criar aditivo/ }));

    await waitFor(() => expect(contractService.amend).toHaveBeenCalledTimes(1));
    const [id, payload] = vi.mocked(contractService.amend).mock.calls[0];
    expect(id).toBe("c1");
    expect(payload.reason).toBe("Prorrogação");
    // Conteúdo do documento carregado, e não o `contract.content` herdado — que
    // era o defeito do fluxo antigo de aditivo. E no formato que o renderizador
    // do backend reconhece: data URL, não base64 puro.
    const conteudo = payload.content ?? "";
    expect(conteudo.startsWith("data:")).toBe(true);
    expect(conteudo).toContain(btoa("revised-document"));
    expect(conteudo).not.toBe(contract.content);
  });

  it("recusa arquivo que não é .docx", async () => {
    const user = userEvent.setup();
    renderPanel();
    await user.click(screen.getByText("Carregar documento"));
    const input = document.querySelector('input[type="file"]') as HTMLInputElement;
    await user.upload(input, new File(["x"], "aditivo.pdf", { type: "application/pdf" }));

    const { toast } = await import("sonner");
    expect(toast.error).toHaveBeenCalledWith("Envie um arquivo .docx.");
    expect(contractService.amend).not.toHaveBeenCalled();
  });

  // Sem motivo o backend rejeita ("amendment reason is required"): a interface
  // não deve deixar a chamada acontecer.
  it("não cria o aditivo sem motivo", async () => {
    const user = userEvent.setup();
    renderPanel();
    await user.click(screen.getByText("Carregar documento"));
    const input = document.querySelector('input[type="file"]') as HTMLInputElement;
    await user.upload(input, new File(["x"], "aditivo.docx"));

    const submit = screen.getByRole("button", { name: /Criar aditivo/ });
    expect(submit).toBeDisabled();
    await user.click(submit);
    expect(contractService.amend).not.toHaveBeenCalled();
  });

  it("oferece os dois caminhos de documento", () => {
    renderPanel();
    expect(screen.getByText("Carregar documento")).toBeInTheDocument();
    expect(screen.getByText("Criar a partir de modelo")).toBeInTheDocument();
  });

  it("lista os modelos disponíveis no caminho do editor", async () => {
    vi.mocked(templateService.list).mockResolvedValue([
      { id: "t1", name: "Aditivo de prazo", description: "", content: "bW9kZWxv", variables: [], created_at: "", updated_at: "" },
    ]);
    const user = userEvent.setup();
    renderPanel();
    await user.click(screen.getByText("Criar a partir de modelo"));

    await waitFor(() => expect(screen.getByRole("option", { name: "Aditivo de prazo" })).toBeInTheDocument());
  });

  // Paridade com o editor de contrato da esteira: as variáveis do modelo são
  // extraídas, o operador confirma e o documento é preenchido com elas.
  it("extrai, preenche e aplica as variáveis do modelo", async () => {
    const { extractDocxTemplateVariables, fillDocxTemplate } = await import("../utils/docx");
    vi.mocked(templateService.list).mockResolvedValue([
      { id: "t1", name: "Aditivo de prazo", description: "", content: "bW9kZWxv", variables: [], created_at: "", updated_at: "" },
    ]);
    const user = userEvent.setup();
    renderPanel();
    await user.click(screen.getByText("Criar a partir de modelo"));
    await waitFor(() => expect(screen.getByRole("option", { name: "Aditivo de prazo" })).toBeInTheDocument());

    await user.selectOptions(
      screen.getByRole("combobox"),
      screen.getByRole("option", { name: "Aditivo de prazo" }),
    );

    // A extração usa o conteúdo do modelo e o contexto do contrato.
    await waitFor(() => expect(extractDocxTemplateVariables).toHaveBeenCalled());

    // O formulário de variáveis abre com o que o contrato já sabe.
    await waitFor(() => expect(screen.getByText(/Preencher Variáveis do Contrato/)).toBeInTheDocument());
    expect(screen.getByDisplayValue("Lana del Rey")).toBeInTheDocument();

    const confirm = screen.getByRole("button", { name: /Confirmar|Aplicar|Gerar/i });
    await user.click(confirm);

    await waitFor(() => expect(fillDocxTemplate).toHaveBeenCalled());
    const [, data] = vi.mocked(fillDocxTemplate).mock.calls[0];
    expect(data).toMatchObject({ buyer_name: "Lana del Rey" });
  });

  // Variável sem valor não pode virar a linha de preenchimento (`______`): o
  // aditivo vai para assinatura eletrônica e esse texto entraria no contrato.
  it("variável sem valor vai vazia, não com a linha de preenchimento", async () => {
    const { fillDocxTemplate } = await import("../utils/docx");
    vi.mocked(templateService.list).mockResolvedValue([
      { id: "t1", name: "Aditivo de prazo", description: "", content: "bW9kZWxv", variables: [], created_at: "", updated_at: "" },
    ]);
    const user = userEvent.setup();
    renderPanel();
    await user.click(screen.getByText("Criar a partir de modelo"));
    await waitFor(() => expect(screen.getByRole("option", { name: "Aditivo de prazo" })).toBeInTheDocument());
    await user.selectOptions(screen.getByRole("combobox"), screen.getByRole("option", { name: "Aditivo de prazo" }));
    await waitFor(() => expect(screen.getByText(/Preencher Variáveis do Contrato/)).toBeInTheDocument());

    await user.click(screen.getByRole("button", { name: /Confirmar|Aplicar|Gerar/i }));

    await waitFor(() => expect(fillDocxTemplate).toHaveBeenCalled());
    const [, data] = vi.mocked(fillDocxTemplate).mock.calls[0];
    expect(Object.values(data as Record<string, unknown>)).not.toContain("______________________");
  });

  // O envio exige rascunho sem envio anterior: o backend recusa com 409 quando
  // já existe `signature_locked_at`. Num contrato já enviado o botão de revisar
  // não pode aparecer — antes aparecia e o operador tomava um 409 sem explicação.
  it("não oferece 'salvar e revisar' quando o contrato já foi enviado", () => {
    render(
      <AmendDocumentPanel
        open
        mode="edit"
        onClose={vi.fn()}
        contract={{
          id: "c1",
          content: "Y29udHJhdG8=",
          status: ContractStatus.PENDING_SIGNATURE,
          signature_locked_at: "2026-09-13T03:57:56Z",
        } as unknown as Contract}
      />,
    );
    expect(screen.queryByRole("button", { name: /Salvar e revisar assinatura/ })).not.toBeInTheDocument();
    // E explica por que, em vez de sumir sem contexto.
    expect(screen.getByText(/não está em rascunho/)).toBeInTheDocument();
  });

  it("oferece 'salvar e revisar' em contrato em rascunho", () => {
    render(
      <AmendDocumentPanel
        open
        mode="edit"
        onClose={vi.fn()}
        contract={{ id: "c1", content: "Y29udHJhdG8=", status: ContractStatus.DRAFT } as unknown as Contract}
      />,
    );
    expect(screen.getByRole("button", { name: /Salvar e revisar assinatura/ })).toBeInTheDocument();
  });
});
