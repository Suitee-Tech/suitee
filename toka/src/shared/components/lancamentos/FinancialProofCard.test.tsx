import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { proofService } from "../../services/proof.service";
import { FinancialProofCard } from "./FinancialProofCard";

const { toastSuccess, toastError } = vi.hoisted(() => ({
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
}));

vi.mock("sonner", () => ({
  toast: {
    success: toastSuccess,
    error: toastError,
  },
}));

vi.mock("../DocumentViewerDialog", () => ({
  DocumentViewerDialog: ({ title }: { title?: string }) => <div role="dialog">{title}</div>,
}));

describe("FinancialProofCard", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("presents a signed proof as a file card instead of exposing the raw URL", () => {
    const url = "https://storage.example/storage/v1/object/sign/financial-proofs/tenant-a/commission/2026-08/550e8400-e29b-41d4-a716-446655440000.pdf?token=secret";

    render(<FinancialProofCard url={url} />);

    expect(screen.getByRole("group", { name: "Comprovante de pagamento" })).toBeInTheDocument();
    expect(screen.getByText("PDF")).toBeInTheDocument();
    expect(screen.getByText("Arquivo de comprovante")).toBeInTheDocument();
    expect(screen.getByText("Acesso autenticado pela sessão")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /abrir comprovante/i })).toBeInTheDocument();
    expect(screen.queryByText(url)).not.toBeInTheDocument();
  });

  it("recognizes the relative authenticated link returned by the API", () => {
    render(<FinancialProofCard url="/api/v1/proofs/content/opaque-token" />);

    expect(screen.getByText("Comprovante armazenado")).toBeInTheDocument();
    expect(screen.getByText("Acesso autenticado pela sessão")).toBeInTheDocument();
  });

  it("copies the complete URL without displaying it in the card", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const url = "https://files.example/commission-proof.png";
    const clipboardWriteText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText: clipboardWriteText },
    });

    render(<FinancialProofCard url={url} />);
    await user.click(screen.getByRole("button", { name: /copiar link/i }));

    expect(clipboardWriteText).toHaveBeenCalledWith(url);
    expect(toastSuccess).toHaveBeenCalledWith("Link externo do comprovante copiado");
    expect(toastError).not.toHaveBeenCalled();
  });

  it("prefers the original filename when the source provides one", () => {
    render(
      <FinancialProofCard
        url="https://files.example/generated-proof.pdf"
        filename="comprovante-pix-agosto.pdf"
      />,
    );

    expect(screen.getByText("comprovante-pix-agosto.pdf")).toBeInTheDocument();
    expect(screen.queryByText("generated-proof.pdf")).not.toBeInTheDocument();
  });

  it("downloads internal proofs with the session and opens the standard document viewer", async () => {
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const url = "https://storage.example/storage/v1/object/sign/financial-proofs/tenant-a/commission/2026-08/550e8400-e29b-41d4-a716-446655440000.pdf?token=legacy";
    vi.spyOn(proofService, "download").mockResolvedValue({
      blob: new Blob(["%PDF-1.4"], { type: "application/pdf" }),
      contentType: "application/pdf",
      filename: "comprovante-pix.pdf",
    });
    vi.spyOn(URL, "createObjectURL").mockReturnValue("blob:proof-preview");

    render(<FinancialProofCard url={url} />);
    await user.click(screen.getByRole("button", { name: /abrir comprovante/i }));

    await waitFor(() => {
      expect(proofService.download).toHaveBeenCalledWith(url);
      expect(screen.getByRole("dialog")).toHaveTextContent("comprovante-pix.pdf");
    });
  });
});
