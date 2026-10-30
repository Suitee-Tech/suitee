import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { DocumentFolderTree } from "./DocumentFolderTree";
import { buildDocumentDrive, type DocumentFolderKey } from "../../documents/drive";
import { createDocumentReferenceResolver } from "../../documents/references";
import type { ContractDocument } from "../../types/contractDocument";

const NOW = Date.parse("2026-09-18T12:00:00Z");

function makeDocument(overrides: Partial<ContractDocument> = {}): ContractDocument {
  return {
    id: "doc-1",
    contractId: "",
    name: "Documento",
    category: "contrato",
    fileType: "application/pdf",
    fileSize: 1024,
    uploadDate: "2026-09-17T12:00:00Z",
    uploadedBy: "Bruno Exemplo",
    status: "assinado",
    signers: [],
    tags: [],
    ...overrides,
  };
}

const resolver = createDocumentReferenceResolver({
  contracts: [
    { id: "ct-1", display_id: "CTR-1", client: "Vanessa Rodrigues", property_title: "Apartamento dos Ipê" },
    { id: "ct-2", display_id: "CTR-2", client: "Lucas Mendes", property_title: "Casa Itapetinga" },
  ],
  properties: [{ id: "pr-1", title: "Casa Rua Itapetinga" }],
});

const drive = buildDocumentDrive(
  [
    makeDocument({ id: "1", contractId: "ct-1", name: "Contrato assinado — locação", status: "assinado" }),
    makeDocument({ id: "2", contractId: "ct-2", name: "Contrato assinado — venda", status: "pendente_assinatura" }),
    makeDocument({
      id: "3",
      entityType: "property",
      entityId: "pr-1",
      category: "foto",
      name: "Fachada.jpg",
      status: "rascunho",
      tags: ["fachada"],
    }),
    makeDocument({ id: "4", source: "manual", name: "Extrato.pdf", category: "comprovante" }),
  ],
  resolver,
  NOW,
);

function renderTree(overrides: Partial<Parameters<typeof DocumentFolderTree>[0]> = {}) {
  const onSelect = vi.fn();
  const onTogglePin = vi.fn();
  const onToggleShowEmpty = vi.fn();

  render(
    <DocumentFolderTree
      counts={drive.counts}
      entitiesByOrigin={drive.entitiesByOrigin}
      activeKey={{ kind: "collection", id: "all" }}
      onSelect={onSelect}
      pinnedKeys={[]}
      onTogglePin={onTogglePin}
      onToggleShowEmpty={onToggleShowEmpty}
      {...overrides}
    />,
  );

  return { onSelect, onTogglePin, onToggleShowEmpty };
}

afterEach(cleanup);

describe("DocumentFolderTree", () => {
  it("organiza a navegação em coleções, origem, tipo e tags", () => {
    renderTree();

    expect(screen.getByText("Coleções")).toBeInTheDocument();
    expect(screen.getByText("Origem")).toBeInTheDocument();
    expect(screen.getByText("Tipo de documento")).toBeInTheDocument();
    expect(screen.getByText("Tags")).toBeInTheDocument();
    expect(screen.getByText("Todos os documentos")).toBeInTheDocument();
  });

  it("mostra a contagem real de cada pasta", () => {
    renderTree();

    const allRow = screen.getByText("Todos os documentos").closest("button");
    expect(allRow).not.toBeNull();
    expect(within(allRow as HTMLElement).getByText("4")).toBeInTheDocument();

    const contractsRow = screen.getByText("Contratos").closest("button");
    expect(within(contractsRow as HTMLElement).getByText("2")).toBeInTheDocument();
  });

  it("esconde coleções e origens sem documentos por padrão", () => {
    renderTree();

    expect(screen.queryByText("Expirados ou cancelados")).not.toBeInTheDocument();
    expect(screen.queryByText("Manutenção")).not.toBeInTheDocument();
    expect(screen.queryByText("Vistorias")).not.toBeInTheDocument();
  });

  it("expõe as pastas vazias quando solicitado", () => {
    renderTree({ showEmpty: true });

    expect(screen.getByText("Expirados ou cancelados")).toBeInTheDocument();
    expect(screen.getByText("Vistorias")).toBeInTheDocument();
    expect(screen.getByText("Aditivo")).toBeInTheDocument();
  });

  it("permite alternar a exibição de pastas vazias", () => {
    const { onToggleShowEmpty } = renderTree();

    fireEvent.click(screen.getByText("Mostrar pastas vazias"));
    expect(onToggleShowEmpty).toHaveBeenCalledTimes(1);
  });

  it("seleciona a coleção clicada", () => {
    const { onSelect } = renderTree();

    fireEvent.click(screen.getByText("Aguardando assinatura"));
    expect(onSelect).toHaveBeenCalledWith({ kind: "collection", id: "awaiting" });
  });

  it("expande a origem e permite escolher o registro específico", () => {
    const { onSelect } = renderTree();

    fireEvent.click(screen.getByLabelText("Expandir Contratos"));
    expect(screen.getByText("Vanessa Rodrigues · Apartamento dos Ipê")).toBeInTheDocument();
    expect(screen.getByText("Lucas Mendes · Casa Itapetinga")).toBeInTheDocument();

    fireEvent.click(screen.getByText("Vanessa Rodrigues · Apartamento dos Ipê"));
    expect(onSelect).toHaveBeenCalledWith({ kind: "origin-entity", origin: "contract", entityId: "ct-1" });
  });

  it("seleciona a origem inteira pelo rótulo da pasta", () => {
    const { onSelect } = renderTree();

    fireEvent.click(screen.getByText("Imóveis"));
    expect(onSelect).toHaveBeenCalledWith({ kind: "origin", origin: "property" });
  });

  it("mostra o nó de arquivos sem vínculo em vez de um UUID", () => {
    const { onSelect } = renderTree();

    fireEvent.click(screen.getByLabelText("Expandir Sem vínculo"));
    fireEvent.click(screen.getByText("Sem vínculo operacional"));
    expect(onSelect).toHaveBeenCalledWith({ kind: "origin-entity", origin: "unassigned", entityId: "" });
  });

  it("destaca a pasta ativa e mantém a origem do filtro aberta", () => {
    const activeKey: DocumentFolderKey = { kind: "origin", origin: "contract" };
    renderTree({ activeKey });

    expect(screen.getByText("Vanessa Rodrigues · Apartamento dos Ipê")).toBeInTheDocument();
    expect(screen.getByText("Contratos")).toHaveClass("font-bold");
  });

  it("fixa e desafixa pastas", () => {
    const { onTogglePin } = renderTree();

    fireEvent.click(screen.getAllByTitle("Fixar pasta")[0]);
    expect(onTogglePin).toHaveBeenCalled();
  });

  it("mantém pastas fixadas gravadas antes do redesenho (chaves legadas)", () => {
    renderTree({ pinnedKeys: ["all", "cat:contrato"] });

    expect(screen.getByText("Pastas fixadas")).toBeInTheDocument();
    // A pasta fixada aparece duas vezes (seção "Pastas fixadas" + seção
    // original) e as duas cópias ficam marcadas como fixadas.
    expect(screen.getAllByText("Todos os documentos")).toHaveLength(2);
    expect(screen.getAllByText("Contrato").length).toBeGreaterThanOrEqual(2);
    expect(screen.getAllByTitle("Desafixar pasta")).toHaveLength(4);
  });

  it("lista tags com contagem", () => {
    renderTree();

    const tagRow = screen.getByText("fachada").closest("button");
    expect(within(tagRow as HTMLElement).getByText("1")).toBeInTheDocument();
  });
});
