import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ContractSignaturePanel } from "./ContractSignaturePanel";
import { ContractStatus, type Contract, type SignatureEnvelope } from "../../contracts/types";

// Defeito relatado: "Baixar PDF assinado" na aba Assinatura abria o XML do
// storage — `<Code>AccessDenied</Code><Message>Request has expired</Message>` —
// porque o link era o `signed_document_url` guardado no contrato/envelope, um
// link pré-assinado com validade de ~5 minutos. A URL precisa ser pedida NOVA a
// cada clique.
const STALE_URL =
  "https://toca-docs.s3.sa-east-1.amazonaws.com/signed.pdf?X-Amz-Expires=299&X-Amz-Signature=velha";
const FRESH_URL =
  "https://minio-beta.example.com/toca-documents/signed.pdf?X-Amz-Expires=900&X-Amz-Signature=nova";

const signatureMocks = vi.hoisted(() => ({
  envelope: null as unknown,
  envelopeLoading: false,
  envelopeLoadFailed: false,
}));

vi.mock("../../../features/esteira/components/esteira/useContractSignatureEnvelope", () => ({
  useContractSignatureEnvelope: () => ({
    envelope: signatureMocks.envelope,
    envelopeLoading: signatureMocks.envelopeLoading,
    envelopeLoadFailed: signatureMocks.envelopeLoadFailed,
    reconcileBusy: false,
    resendAllBusy: false,
    cancelBusy: false,
    handleReconcile: vi.fn(),
    handleResendAll: vi.fn(),
    handleCancel: vi.fn(),
    handleAddSignerToEnvelope: vi.fn(),
    handleEditSignerSubmit: vi.fn(),
  }),
}));

const getSignedFileUrl = vi.hoisted(() => vi.fn());
vi.mock("../../services/contract.service", () => ({
  contractService: {
    getSignedFileUrl: (...args: unknown[]) => getSignedFileUrl(...args),
  },
}));

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const contract = {
  id: "contract-1",
  display_id: "69c4362b",
  status: ContractStatus.SIGNED,
  signatories: [{ id: "signer-1", name: "Penisvaldo", email: "p@example.com", role: "LOCATARIO", signed_at: "2026-10-06T11:09:27Z" }],
  signed_document_url: STALE_URL,
} as unknown as Contract;

const envelope = {
  id: "env-1",
  contract_id: "contract-1",
  status: "FINISHED",
  external_envelope_id: "clicksign-1",
  signed_document_url: STALE_URL,
  signers: [],
} as unknown as SignatureEnvelope;

const clickSpy = vi.fn();

beforeEach(() => {
  vi.clearAllMocks();
  signatureMocks.envelope = envelope;
  signatureMocks.envelopeLoading = false;
  signatureMocks.envelopeLoadFailed = false;
  getSignedFileUrl.mockResolvedValue({ url: FRESH_URL, source: "minio", status: "SIGNED" });
  vi.stubGlobal("URL", { ...URL, createObjectURL: vi.fn(() => "blob:contrato"), revokeObjectURL: vi.fn() });
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => ({ ok: true, blob: async () => new Blob(["pdf"]) })),
  );
  clickSpy.mockClear();
  // O clique do `<a download>` não navega no jsdom: só registramos.
  vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(clickSpy);
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function mount() {
  return render(
    <ContractSignaturePanel
      contract={contract}
      envelope={envelope}
      onEnvelopeChange={vi.fn()}
      onContractChange={vi.fn()}
      onRefresh={vi.fn()}
    />,
  );
}

describe("ContractSignaturePanel — PDF assinado", () => {
  it("não expõe o link guardado no contrato (que já expirou)", () => {
    mount();

    expect(screen.getByText("Documento assinado disponível.")).toBeInTheDocument();
    const linkAntigo = screen.queryByRole("link", { name: "Baixar PDF assinado" });
    expect(linkAntigo).toBeNull();
    expect(document.body.innerHTML).not.toContain("X-Amz-Signature=velha");
  });

  it("pede uma URL nova e baixa o PDF no clique", async () => {
    mount();

    fireEvent.click(screen.getByRole("button", { name: "Baixar PDF assinado" }));

    await waitFor(() => expect(getSignedFileUrl).toHaveBeenCalledWith("contract-1"));
    // O download usa a URL RECÉM-assinada, não a guardada.
    await waitFor(() => expect(fetch).toHaveBeenCalledWith(FRESH_URL, { credentials: "omit" }));
    expect(clickSpy).toHaveBeenCalled();
  });

  it("avisa quando o PDF ainda não existe em vez de abrir uma URL vazia", async () => {
    getSignedFileUrl.mockResolvedValue(null);
    mount();

    fireEvent.click(screen.getByRole("button", { name: "Baixar PDF assinado" }));

    await waitFor(() => expect(fetch).not.toHaveBeenCalled());
  });

  it("não oferece o download enquanto a assinatura não terminou", () => {
    signatureMocks.envelope = { ...envelope, status: "RUNNING" } as unknown as SignatureEnvelope;
    render(
      <ContractSignaturePanel
        contract={{ ...contract, status: ContractStatus.PENDING_SIGNATURE } as unknown as Contract}
        envelope={{ ...envelope, status: "RUNNING" } as unknown as SignatureEnvelope}
        onEnvelopeChange={vi.fn()}
        onContractChange={vi.fn()}
        onRefresh={vi.fn()}
      />,
    );

    expect(screen.queryByRole("button", { name: "Baixar PDF assinado" })).toBeNull();
  });
});
