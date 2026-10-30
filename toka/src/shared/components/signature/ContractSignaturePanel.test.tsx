import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";

import { ContractStatus, type Contract, type SignatureEnvelope } from "../../contracts/types";
import type { ContractSignatureEnvelopeController } from "../../../features/esteira/components/esteira/useContractSignatureEnvelope";

// O ciclo de vida do envelope (carregar, reenviar, cancelar, sincronizar) é
// responsabilidade do hook da esteira, que tem teste próprio de integração.
// Aqui a fronteira é a APRESENTAÇÃO: o painel precisa mostrar o estado certo de
// cada signatário e oferecer as ações cabíveis em cada situação.
const controller: Partial<ContractSignatureEnvelopeController> = {};

vi.mock("../../../features/esteira/components/esteira/useContractSignatureEnvelope", () => ({
  useContractSignatureEnvelope: () => ({
    envelope: null,
    envelopeLoading: false,
    envelopeLoadFailed: false,
    reconcileBusy: false,
    cancelBusy: false,
    cancelConfirmOpen: false,
    resendAllBusy: false,
    resendingSignerId: null,
    signerEditing: null,
    envelopeAddOpen: false,
    handleReconcile: vi.fn(),
    handleCancel: vi.fn(),
    handleResendAll: vi.fn(),
    handleResendSigner: vi.fn(),
    handleEditSignerSubmit: vi.fn(),
    handleAddSignerToEnvelope: vi.fn(),
    setCancelConfirmOpen: vi.fn(),
    setSignerEditing: vi.fn(),
    setEnvelopeAddOpen: vi.fn(),
    ...controller,
  }),
}));

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

import { ContractSignaturePanel } from "./ContractSignaturePanel";

const baseContract = (overrides: Partial<Contract> = {}): Contract =>
  ({
    id: "c1",
    status: ContractStatus.PENDING_SIGNATURE,
    signatories: [
      { id: "s1", name: "Lana del Rey", email: "lana@example.com", role: "locatario", signed_at: "2026-09-10T10:00:00Z" },
      { id: "s2", name: "Maria Locador", email: "maria@example.com", role: "locador" },
    ],
    ...overrides,
  }) as Contract;

const envelopeWith = (signers: SignatureEnvelope["signers"], overrides: Partial<SignatureEnvelope> = {}): SignatureEnvelope =>
  ({
    id: "e1",
    tenant_id: "t1",
    contract_id: "c1",
    environment: "sandbox",
    // O painel só libera as ações de envelope quando existe envelope EXTERNO
    // (enviado de fato ao provedor). Sem isto, o envelope é um rascunho local.
    external_envelope_id: "ext-1",
    status: "RUNNING",
    signers,
    created_at: "2026-09-10T09:00:00Z",
    updated_at: "2026-09-10T09:00:00Z",
    ...overrides,
  }) as SignatureEnvelope;

const renderPanel = (contract: Contract, envelope: SignatureEnvelope | null) =>
  render(
    <ContractSignaturePanel
      contract={contract}
      envelope={envelope}
      onEnvelopeChange={vi.fn()}
      onContractChange={vi.fn()}
      onRefresh={vi.fn()}
    />,
  );

describe("ContractSignaturePanel", () => {
  beforeEach(() => {
    for (const key of Object.keys(controller)) delete (controller as Record<string, unknown>)[key];
  });

  it("mostra o estado de cada signatário vindo do envelope", () => {
    const envelope = envelopeWith([
      { signer_id: "s1", name: "Lana del Rey", email: "lana@example.com", role: "sign", auth: [], status: "SIGNED", signed_at: "2026-09-10T10:00:00Z" },
      { signer_id: "s2", name: "Maria Locador", email: "maria@example.com", role: "sign", auth: [], status: "PENDING" },
    ]);
    renderPanel(baseContract(), envelope);

    expect(screen.getByText("Lana del Rey")).toBeInTheDocument();
    expect(screen.getByText("Maria Locador")).toBeInTheDocument();
    expect(screen.getByText(/Assinou/)).toBeInTheDocument();
    // "Aguardando assinatura" também é o rótulo do envelope em andamento, então
    // o texto aparece duas vezes — uma no selo, outra no signatário.
    expect(screen.getAllByText(/Aguardando assinatura/).length).toBe(2);
  });

  // O contrato de aditivo nasce sem envelope. O painel precisa dizer isso e não
  // oferecer ações que dependem de um envelope externo inexistente.
  it("sem envelope, avisa que nada foi enviado e desabilita as ações de envelope", () => {
    renderPanel(baseContract({ status: ContractStatus.DRAFT }), null);
    expect(screen.getByText(/Nenhum envelope de assinatura/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Sincronizar/ })).toBeDisabled();
    expect(screen.getByRole("button", { name: /Reenviar/ })).toBeDisabled();
    expect(screen.queryByRole("button", { name: /Cancelar/ })).not.toBeInTheDocument();
  });

  // Envelope DRAFT ainda é um rascunho local: não foi enviado ao provedor e não
  // há o que sincronizar, reenviar ou cancelar do lado de fora.
  it("envelope em rascunho (sem id externo) não libera as ações", () => {
    renderPanel(baseContract(), envelopeWith([], { status: "DRAFT", external_envelope_id: undefined }));
    expect(screen.getByRole("button", { name: /Sincronizar/ })).toBeDisabled();
    expect(screen.getByRole("button", { name: /Reenviar/ })).toBeDisabled();
    expect(screen.queryByRole("button", { name: /Cancelar/ })).not.toBeInTheDocument();
  });

  it("com envelope em andamento, libera sincronizar, reenviar e cancelar", () => {
    renderPanel(baseContract(), envelopeWith([]));
    expect(screen.getByRole("button", { name: /Sincronizar/ })).toBeEnabled();
    expect(screen.getByRole("button", { name: /Reenviar/ })).toBeEnabled();
    expect(screen.getByRole("button", { name: /Cancelar/ })).toBeInTheDocument();
  });

  // Envelope concluído não deve oferecer reenvio nem cancelamento: não há mais
  // o que assinar.
  it("assinatura concluída não oferece reenviar nem cancelar", () => {
    renderPanel(
      baseContract({ status: ContractStatus.SIGNED }),
      envelopeWith([], { status: "FINISHED", signed_document_url: "https://exemplo/assinado.pdf" }),
    );
    expect(screen.getByRole("button", { name: /Reenviar/ })).toBeDisabled();
    expect(screen.queryByRole("button", { name: /Cancelar/ })).not.toBeInTheDocument();
    expect(screen.getByText(/Documento assinado disponível/)).toBeInTheDocument();
  });

  // Um contrato assinado sem registro de envelope acessível não pode dizer
  // "nenhum envelope criado": o documento assinado está logo acima. A mensagem
  // contradizia a própria tela.
  it("não anuncia 'nenhum envelope' quando o contrato já tem documento assinado", () => {
    renderPanel(baseContract({ status: ContractStatus.SIGNED, signed_document_url: "https://exemplo/assinado.pdf" }), null);
    expect(screen.queryByText(/Nenhum envelope de assinatura/)).not.toBeInTheDocument();
    expect(screen.getByText(/Documento assinado disponível/)).toBeInTheDocument();
  });

  it("expõe o erro do envelope em vez de esconder a falha", () => {
    renderPanel(baseContract(), envelopeWith([], { status: "ERROR", last_error: "Credencial Clicksign inválida" }));
    expect(screen.getByText("Credencial Clicksign inválida")).toBeInTheDocument();
  });

  it("mostra aviso quando a carga do envelope falha", () => {
    controller.envelopeLoadFailed = true;
    renderPanel(baseContract(), null);
    expect(screen.getByText(/Não foi possível carregar o envelope/)).toBeInTheDocument();
  });

  // A URL do contrato pode estar preenchida por estado anterior (regeneração,
  // tentativa encerrada). Num envelope que ainda aguarda assinatura, anunciar
  // "Documento assinado disponível" contradiz o próprio selo de status ao lado.
  it("não anuncia documento assinado com envelope ainda em andamento", () => {
    renderPanel(
      baseContract({ status: ContractStatus.PENDING_SIGNATURE, signed_document_url: "https://exemplo/antigo.pdf" }),
      envelopeWith([], { status: "RUNNING", signed_document_url: undefined }),
    );
    expect(screen.queryByText(/Documento assinado disponível/)).not.toBeInTheDocument();
  });

  it("anuncia o documento assinado quando o envelope terminou", () => {
    renderPanel(
      baseContract({ status: ContractStatus.SIGNED }),
      envelopeWith([], { status: "FINISHED", signed_document_url: "https://exemplo/assinado.pdf" }),
    );
    expect(screen.getByText(/Documento assinado disponível/)).toBeInTheDocument();
  });
  // O estado que faltava: o contrato está em PARTIALLY_SIGNED (o primeiro
  // signatário já assinou) e o restante ainda não. A tela precisa dizer QUEM
  // assinou, quantos faltam e — com ordem de assinatura — quem ainda nem foi
  // convidado, porque o provedor só convida o grupo da vez.
  it("mostra o progresso e quem ainda espera a vez no estado parcial", () => {
    const envelope = envelopeWith(
      [
        { signer_id: "s1", name: "Lana del Rey", email: "lana@example.com", role: "sign", auth: [], status: "SIGNED", group: 1 },
        { signer_id: "s3", name: "Ana Compradora", email: "ana@example.com", role: "sign", auth: [], status: "PENDING", group: 1 },
        { signer_id: "s2", name: "Maria Locador", email: "maria@example.com", role: "sign", auth: [], status: "PENDING", group: 2 },
      ],
      { status: "PARTIALLY_SIGNED" },
    );
    const contract = baseContract({
      status: ContractStatus.PARTIALLY_SIGNED,
      signatories: [
        { id: "s1", name: "Lana del Rey", email: "lana@example.com", role: "locatario", group: 1, signed_at: "2026-10-05T13:10:00Z" },
        { id: "s3", name: "Ana Compradora", email: "ana@example.com", role: "locatario", group: 1 },
        { id: "s2", name: "Maria Locador", email: "maria@example.com", role: "locador", group: 2 },
      ],
    });
    renderPanel(contract, envelope);

    // Selo do envelope com o status parcial e o progresso ao lado (o mesmo
    // número aparece no selo e na linha de progresso — os dois são a leitura que
    // o operador procura).
    expect(screen.getByText(/Parcialmente assinado/)).toBeInTheDocument();
    expect(screen.getAllByText(/1 de 3 assinou/).length).toBeGreaterThan(0);
    // Grupo da vez destacado.
    expect(screen.getAllByText(/Grupo 1/).length).toBeGreaterThan(0);
    expect(screen.getByText(/em andamento/)).toBeInTheDocument();
    // Quem espera a vez é identificado como tal, e não como "aguardando
    // assinatura" (que sugere pedido já enviado).
    expect(screen.getAllByText(/Aguardando o Grupo 1/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/aguardando/).length).toBeGreaterThan(0);
    expect(screen.getByText(/Só é notificado quando o Grupo 1 concluir/)).toBeInTheDocument();
  });

  // Sem ordem de assinatura não existe "esperando a vez": todos foram
  // convidados juntos e o rótulo genérico é o correto.
  it("sem grupos, o parcial não inventa espera pela vez", () => {
    const envelope = envelopeWith(
      [
        { signer_id: "s1", name: "Lana del Rey", email: "lana@example.com", role: "sign", auth: [], status: "SIGNED" },
        { signer_id: "s2", name: "Maria Locador", email: "maria@example.com", role: "sign", auth: [], status: "PENDING" },
      ],
      { status: "PARTIALLY_SIGNED" },
    );
    renderPanel(baseContract({ status: ContractStatus.PARTIALLY_SIGNED }), envelope);

    expect(screen.getAllByText(/1 de 2 assinou/).length).toBeGreaterThan(0);
    expect(screen.queryByText(/Aguardando o Grupo/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Assinatura sequencial/)).not.toBeInTheDocument();
  });
});
