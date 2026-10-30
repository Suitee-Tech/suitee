import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { FileDropzone } from "./FileDropzone";

const file = (name: string, bytes: number, type = "application/pdf") =>
  new File([new Uint8Array(bytes)], name, { type });

// O container não tem role: o alvo do drop é o ancestral do input de arquivo.
const dropzone = () => screen.getByText("Clique ou arraste um arquivo").closest("div.relative") as HTMLElement;

const drop = (dropped: File) =>
  fireEvent.drop(dropzone(), { dataTransfer: { files: [dropped], types: ["Files"] } });

describe("FileDropzone com limite de tamanho", () => {
  it("recusa arquivo acima do teto sem chamar o upload", async () => {
    const onUpload = vi.fn();
    render(<FileDropzone maxSize={25 * 1024 * 1024} onUpload={onUpload} />);

    drop(file("contrato-grande.pdf", 26 * 1024 * 1024));

    expect(await screen.findByText(/excede o limite de 25MB/i)).toBeInTheDocument();
    expect(onUpload).not.toHaveBeenCalled();
  });

  it("aceita arquivo dentro do teto", async () => {
    const onUpload = vi.fn().mockResolvedValue(undefined);
    render(<FileDropzone maxSize={25 * 1024 * 1024} onUpload={onUpload} />);

    drop(file("documento.pdf", 1024));

    expect(onUpload).toHaveBeenCalledTimes(1);
    expect(screen.queryByText(/excede o limite/i)).not.toBeInTheDocument();
  });

  it("mostra o teto no rótulo do dropzone", () => {
    render(<FileDropzone maxSize={25 * 1024 * 1024} onUpload={vi.fn()} />);
    expect(screen.getByText(/até 25MB/i)).toBeInTheDocument();
  });
});
