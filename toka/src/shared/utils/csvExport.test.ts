import { afterEach, describe, expect, it, vi } from "vitest";
import { UTF8_BOM, downloadCsv, withUtf8Bom } from "./csvExport";

describe("csvExport", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("prefixa o BOM UTF-8 uma única vez", () => {
    expect(withUtf8Bom("coluna\nvalor")).toBe(UTF8_BOM + "coluna\nvalor");
    // Idempotente: conteúdo que já tem BOM não ganha outro.
    expect(withUtf8Bom(UTF8_BOM + "coluna")).toBe(UTF8_BOM + "coluna");
  });

  it("baixa o CSV como Blob com BOM (Excel lê os acentos corretamente)", async () => {
    const blobs: Blob[] = [];
    vi.stubGlobal("URL", {
      createObjectURL: vi.fn((blob: Blob) => {
        blobs.push(blob);
        return "blob:test";
      }),
      revokeObjectURL: vi.fn(),
    });
    const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});

    downloadCsv("relatorio.csv", "Descrição,Não\nAluguel,Comissão");

    expect(click).toHaveBeenCalledTimes(1);
    expect(blobs).toHaveLength(1);
    expect(blobs[0].type).toBe("text/csv;charset=utf-8");
    // Blob.text() remove o BOM na decodificação; os bytes provam o prefixo
    // EF BB BF que o Excel usa para detectar UTF-8.
    const bytes = new Uint8Array(await blobs[0].arrayBuffer());
    expect([bytes[0], bytes[1], bytes[2]]).toEqual([0xEF, 0xBB, 0xBF]);
    expect(await blobs[0].text()).toContain("Comissão");
  });
});
