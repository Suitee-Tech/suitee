import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ContractDocument } from "../../types/contractDocument";

const { getPresignedDownloadUrlMock } = vi.hoisted(() => ({
  getPresignedDownloadUrlMock: vi.fn(),
}));

vi.mock("../../services/document.service", () => ({
  documentService: {
    getPresignedDownloadUrl: getPresignedDownloadUrlMock,
  },
}));

vi.mock("react-pdf", () => ({
  pdfjs: { GlobalWorkerOptions: {} },
  Document: ({ file, children }: { file: string; children: ReactNode }) => (
    <div data-testid="pdf-document" data-file={file}>
      {children}
    </div>
  ),
  Page: () => <div data-testid="pdf-page" />,
}));

import { DocumentGridCard } from "./DocumentGridCard";

class IntersectionObserverMock {
  static instances: IntersectionObserverMock[] = [];

  private readonly callback: IntersectionObserverCallback;
  private target: Element | null = null;

  constructor(callback: IntersectionObserverCallback) {
    this.callback = callback;
    IntersectionObserverMock.instances.push(this);
  }

  observe = vi.fn((target: Element) => {
    this.target = target;
  });

  unobserve = vi.fn();
  disconnect = vi.fn();
  takeRecords = vi.fn(() => []);
  root = null;
  rootMargin = "0px";
  thresholds = [0];

  trigger(isIntersecting: boolean) {
    if (!this.target) throw new Error("No observed target");
    this.callback(
      [{ isIntersecting, target: this.target } as IntersectionObserverEntry],
      this as unknown as IntersectionObserver,
    );
  }
}

function makeDocument(overrides: Partial<ContractDocument> = {}): ContractDocument {
  return {
    id: "document-1",
    contractId: "contract-1",
    name: "Documento de teste",
    category: "contrato",
    fileType: "image/png",
    fileSize: 2048,
    uploadDate: "2026-07-25T12:00:00Z",
    uploadedBy: "user-1",
    status: "assinado",
    signers: [],
    fileUrl:
      "http://host.docker.internal:54321/storage/v1/object/authenticated/toca-documents/document-1.png",
    ...overrides,
  };
}

beforeEach(() => {
  IntersectionObserverMock.instances = [];
  vi.stubGlobal("IntersectionObserver", IntersectionObserverMock);
  getPresignedDownloadUrlMock.mockReset();
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("DocumentGridCard previews", () => {
  it("loads an image URL only after the card becomes visible and never renders the private URL", async () => {
    let resolveUrl!: (url: string) => void;
    getPresignedDownloadUrlMock.mockReturnValue(
      new Promise<string>((resolve) => {
        resolveUrl = resolve;
      }),
    );
    const doc = makeDocument();
    const signedUrl =
      "https://project.supabase.co/storage/v1/object/sign/toca-documents/document-1.png?token=signed";
    const { container } = render(<DocumentGridCard doc={doc} onClick={vi.fn()} />);

    expect(getPresignedDownloadUrlMock).not.toHaveBeenCalled();
    expect(screen.queryByRole("img", { name: doc.name })).not.toBeInTheDocument();

    act(() => IntersectionObserverMock.instances[0].trigger(true));

    await waitFor(() => {
      expect(getPresignedDownloadUrlMock).toHaveBeenCalledWith(doc.id);
    });
    expect(screen.queryByRole("img", { name: doc.name })).not.toBeInTheDocument();
    expect(container.innerHTML).not.toContain(doc.fileUrl);

    await act(async () => {
      resolveUrl(signedUrl);
      await Promise.resolve();
    });

    expect(await screen.findByRole("img", { name: doc.name })).toHaveAttribute("src", signedUrl);
    expect(container.innerHTML).not.toContain(doc.fileUrl);
  });

  it("uses the signed URL for PDF thumbnails", async () => {
    const signedUrl =
      "https://project.supabase.co/storage/v1/object/sign/toca-documents/document-1.pdf?token=signed";
    getPresignedDownloadUrlMock.mockResolvedValue(signedUrl);
    const doc = makeDocument({
      fileType: "application/pdf",
      fileUrl:
        "http://host.docker.internal:54321/storage/v1/object/authenticated/toca-documents/document-1.pdf",
    });
    const { container } = render(<DocumentGridCard doc={doc} onClick={vi.fn()} />);

    act(() => IntersectionObserverMock.instances[0].trigger(true));

    expect(await screen.findByTestId("pdf-document")).toHaveAttribute("data-file", signedUrl);
    expect(container.innerHTML).not.toContain(doc.fileUrl);
  });

  it("keeps a safe placeholder when signing or loading the image fails", async () => {
    getPresignedDownloadUrlMock.mockRejectedValueOnce(new Error("signing failed"));
    const doc = makeDocument();
    const { rerender } = render(<DocumentGridCard doc={doc} onClick={vi.fn()} />);

    await act(async () => {
      IntersectionObserverMock.instances[0].trigger(true);
      await Promise.resolve();
    });

    expect(screen.queryByRole("img", { name: doc.name })).not.toBeInTheDocument();
    expect(screen.getByText("Imagem")).toBeInTheDocument();

    getPresignedDownloadUrlMock.mockResolvedValueOnce(
      "https://project.supabase.co/storage/v1/object/sign/toca-documents/document-2.png?token=signed",
    );
    const nextDoc = makeDocument({ id: "document-2", name: "Imagem inválida" });
    rerender(<DocumentGridCard doc={nextDoc} onClick={vi.fn()} />);
    const image = await screen.findByRole("img", { name: nextDoc.name });

    fireEvent.error(image);

    expect(screen.queryByRole("img", { name: nextDoc.name })).not.toBeInTheDocument();
    expect(screen.getByText("Imagem")).toBeInTheDocument();
  });

  it("ignores an obsolete signing response after the document changes", async () => {
    let resolveFirst!: (url: string) => void;
    let resolveSecond!: (url: string) => void;
    getPresignedDownloadUrlMock
      .mockReturnValueOnce(
        new Promise<string>((resolve) => {
          resolveFirst = resolve;
        }),
      )
      .mockReturnValueOnce(
        new Promise<string>((resolve) => {
          resolveSecond = resolve;
        }),
      );
    const firstDoc = makeDocument();
    const secondDoc = makeDocument({ id: "document-2", name: "Documento novo" });
    const { rerender } = render(<DocumentGridCard doc={firstDoc} onClick={vi.fn()} />);

    act(() => IntersectionObserverMock.instances[0].trigger(true));
    await waitFor(() => expect(getPresignedDownloadUrlMock).toHaveBeenCalledWith(firstDoc.id));

    rerender(<DocumentGridCard doc={secondDoc} onClick={vi.fn()} />);
    await waitFor(() => expect(getPresignedDownloadUrlMock).toHaveBeenCalledWith(secondDoc.id));

    await act(async () => {
      resolveFirst("https://project.supabase.co/obsolete.png?token=old");
      await Promise.resolve();
    });
    expect(screen.queryByRole("img")).not.toBeInTheDocument();

    const currentUrl = "https://project.supabase.co/current.png?token=new";
    await act(async () => {
      resolveSecond(currentUrl);
      await Promise.resolve();
    });
    expect(await screen.findByRole("img", { name: secondDoc.name })).toHaveAttribute("src", currentUrl);
  });
});
