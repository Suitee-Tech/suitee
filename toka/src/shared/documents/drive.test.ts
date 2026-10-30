import { describe, expect, it } from "vitest";
import type { ContractDocument } from "../types/contractDocument";
import {
  buildDocumentDrive,
  filterByFolder,
  groupDriveEntries,
  matchesCollection,
  matchesQuery,
  normalizeSearchText,
  sortDriveEntries,
  summarizeDriveEntries,
} from "./drive";
import { createDocumentReferenceResolver } from "./references";

const NOW = Date.parse("2026-09-18T12:00:00Z");

function makeDocument(overrides: Partial<ContractDocument> = {}): ContractDocument {
  return {
    id: "doc-1",
    contractId: "",
    name: "Contrato assinado — locação",
    category: "contrato",
    fileType: "application/pdf",
    fileSize: 524_000,
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
    { id: "ct-1", display_id: "CTR-2026-014", client: "Vanessa Rodrigues Melo", property_title: "Apartamento dos Ipê" },
  ],
  properties: [{ id: "pr-1", title: "Casa Rua Itapetinga", displayId: "IM-88" }],
  leads: [
    { id: "ld-1", name: "Lucas Mendes Barros", displayId: "LD-12" },
    { id: "ld-2", name: "Marina Alves", accountId: "acc-cliente-1" },
  ],
  deals: [
    {
      id: "dl-1",
      display_id: "NG-2026-07",
      property_id: "pr-1",
      customer_id: "ld-1",
      parties: [{ id: "party-1", name: "Lucas Mendes Barros" }],
    },
  ],
  members: [{ account_id: "9feafe7e-cbc4-94f4-9b8f-cc7a9c1c3bcc", name: "Bruno Exemplo", email: "bruno@imob.com" }],
});

describe("buildDocumentDrive — origem e referência legível", () => {
  it("resolve contrato pelo contract_id com cliente e imóvel no rótulo", () => {
    const drive = buildDocumentDrive([makeDocument({ contractId: "ct-1" })], resolver, NOW);
    const [entry] = drive.entries;

    expect(entry.origin).toBe("contract");
    expect(entry.reference.label).toBe("Vanessa Rodrigues Melo · Apartamento dos Ipê");
    expect(entry.reference.detail).toBe("Contrato CTR-2026-014");
    expect(entry.reference.href).toBe("/contratos/ct-1");
  });

  it("rotula evidência de análise pelo critério e pela parte, não por UUID", () => {
    const drive = buildDocumentDrive(
      [
        makeDocument({
          name: "Captura de Tela 2026-09-04 às 14.50.30.png",
          category: "fiscal",
          entityType: "analysis",
          entityId: "dl-1",
          analysisPartyId: "party-1",
          analysisCriterion: "cnd",
          uploadedBy: "9feafe7e-cbc4-94f4-9b8f-cc7a9c1c3bcc",
        }),
      ],
      resolver,
      NOW,
    );
    const [entry] = drive.entries;

    expect(entry.origin).toBe("analysis");
    expect(entry.reference.label).toBe("CND · Lucas Mendes Barros");
    expect(entry.reference.detail).toBe("Análise de crédito · CND");
    expect(entry.reference.href).toBe("/esteira/deals/dl-1");
    expect(entry.actor.label).toBe("Bruno Exemplo");
  });

  it("resolve imóvel e cliente pelos seus próprios vínculos", () => {
    const [propertyEntry] = buildDocumentDrive(
      [makeDocument({ entityType: "property", entityId: "pr-1", category: "foto" })],
      resolver,
      NOW,
    ).entries;
    const [clientEntry] = buildDocumentDrive(
      [makeDocument({ entityType: "lead", entityId: "ld-1", category: "identidade" })],
      resolver,
      NOW,
    ).entries;

    expect(propertyEntry.origin).toBe("property");
    expect(propertyEntry.reference.label).toBe("Casa Rua Itapetinga");
    expect(propertyEntry.reference.href).toBe("/imoveis/pr-1");

    expect(clientEntry.origin).toBe("client");
    expect(clientEntry.reference.label).toBe("Lucas Mendes Barros");
    expect(clientEntry.reference.href).toBe("/leads/ld-1");
  });

  it("agrupa vários critérios do mesmo negócio em um único nó com o rótulo do registro", () => {
    const drive = buildDocumentDrive(
      [
        makeDocument({
          id: "cnd",
          entityType: "analysis",
          entityId: "dl-1",
          analysisPartyId: "party-1",
          analysisCriterion: "cnd",
        }),
        makeDocument({
          id: "score",
          entityType: "analysis",
          entityId: "dl-1",
          analysisPartyId: "party-1",
          analysisCriterion: "score",
        }),
      ],
      resolver,
      NOW,
    );

    // Cada arquivo mantém seu próprio rótulo ("CND", "Score de crédito")...
    expect(drive.entries.map((entry) => entry.reference.label)).toEqual([
      "CND · Lucas Mendes Barros",
      "Score de crédito · Lucas Mendes Barros",
    ]);
    // ...mas a barra lateral mostra o negócio analisado, uma única vez.
    expect(drive.entitiesByOrigin.analysis).toHaveLength(1);
    expect(drive.entitiesByOrigin.analysis[0].label).toBe("Lucas Mendes Barros · Casa Rua Itapetinga");
    expect(drive.entitiesByOrigin.analysis[0].detail).toBe("Análise de crédito do negócio");
    expect(drive.entitiesByOrigin.analysis[0].count).toBe(2);
  });

  it("agrupa arquivos sem vínculo em um único nó honesto", () => {
    const drive = buildDocumentDrive(
      [
        makeDocument({ id: "a", source: "manual" }),
        makeDocument({ id: "b", entityType: "general", source: "manual" }),
      ],
      resolver,
      NOW,
    );

    expect(drive.counts.origins.unassigned).toBe(2);
    const nodes = drive.entitiesByOrigin.unassigned;
    expect(nodes).toHaveLength(1);
    expect(nodes[0].label).toBe("Sem vínculo operacional");
    expect(nodes[0].count).toBe(2);
  });

  it("nunca mostra UUID cru na referência nem no responsável", () => {
    const drive = buildDocumentDrive(
      [
        makeDocument({
          id: "sem-vinculo",
          uploadedBy: "9feafe7e-cbc4-94f4-9b8f-cc7a9c1c3bcc",
        }),
      ],
      resolver,
      NOW,
    );

    expect(drive.entries[0].reference.label).not.toMatch(/[0-9a-f]{8}-[0-9a-f]{4}/i);
    expect(drive.entries[0].actor.label).toBe("Bruno Exemplo");
  });

  it("resolve o responsável pelo e-mail e marca origens automáticas", () => {
    expect(resolver.resolveActor("bruno@imob.com").label).toBe("Bruno Exemplo");
    expect(resolver.resolveActor("Clicksign")).toEqual({
      label: "Clicksign",
      detail: "Assinatura digital",
      kind: "system",
    });
    expect(resolver.resolveActor("system:toca-ops").label).toBe("Sistema · Toca Ops");
    expect(resolver.resolveActor("").label).toBe("—");
  });

  it("cai para cliente do portal quando a conta pertence a um lead", () => {
    expect(resolver.resolveActor("acc-cliente-1").label).toBe("Marina Alves (cliente)");
  });
});

describe("buildDocumentDrive — contagens", () => {
  const documents = [
    makeDocument({ id: "1", contractId: "ct-1", status: "assinado", uploadDate: "2026-09-17T12:00:00Z" }),
    makeDocument({
      id: "2",
      entityType: "analysis",
      entityId: "dl-1",
      category: "fiscal",
      status: "pendente_assinatura",
      uploadDate: "2026-08-01T12:00:00Z",
      tags: ["due-diligence"],
    }),
    makeDocument({
      id: "3",
      entityType: "property",
      entityId: "pr-1",
      category: "foto",
      status: "rascunho",
      uploadDate: "2026-07-01T12:00:00Z",
    }),
    makeDocument({ id: "4", source: "manual", status: "expirado", uploadDate: "2026-06-01T12:00:00Z" }),
  ];

  const drive = buildDocumentDrive(documents, resolver, NOW);

  it("conta cada coleção pelo estado real do documento", () => {
    expect(drive.counts.total).toBe(4);
    expect(drive.counts.collections).toEqual({
      all: 4,
      recent: 1,
      // Doc 2 tem status "pendente_assinatura": entrou mesmo em fluxo.
      awaiting: 1,
      not_started: 0,
      signed: 1,
      not_required: 0,
      draft: 1,
      attention: 1,
    });
  });

  it("conta origens e tipos de documento sem depender da página carregada", () => {
    expect(drive.counts.origins).toMatchObject({ contract: 1, analysis: 1, property: 1, unassigned: 1 });
    // Documentos 1 e 4 não declaram tipo e herdam o padrão "contrato".
    expect(drive.counts.categories).toMatchObject({ contrato: 2, fiscal: 1, foto: 1 });
    expect(drive.counts.tags).toEqual({ "due-diligence": 1 });
  });

  it("recalcula facetas para o subconjunto filtrado pela busca", () => {
    const matches = drive.entries.filter((entry) => matchesQuery(entry, "casa itapetinga"));
    const facets = summarizeDriveEntries(matches, NOW);

    // O documento de análise é encontrado pelo imóvel do negócio vinculado.
    expect(matches.map((entry) => entry.doc.id).sort()).toEqual(["2", "3"]);
    expect(facets.counts.total).toBe(2);
    expect(facets.counts.origins).toEqual({ analysis: 1, property: 1 });
    expect(facets.counts.collections.all).toBe(2);
  });

  it("encontra documento pelo nome do cliente, do imóvel e do responsável", () => {
    const byClient = drive.entries.filter((entry) => matchesQuery(entry, "vanessa"));
    const byProperty = drive.entries.filter((entry) => matchesQuery(entry, "itapetinga"));
    const byActor = drive.entries.filter((entry) => matchesQuery(entry, "bruno Exemplo"));

    expect(byClient.map((e) => e.doc.id)).toEqual(["1"]);
    expect(byProperty.map((e) => e.doc.id).sort()).toEqual(["2", "3"]);
    expect(byActor.map((e) => e.doc.id)).toEqual(["1", "2", "3", "4"]);
  });

  it("ignora acentos e caixa na busca livre", () => {
    expect(normalizeSearchText("Imóvel  São João")).toBe("imovel sao joao");
    expect(drive.entries.filter((entry) => matchesQuery(entry, "IMOVEL")).length).toBeGreaterThan(0);
  });
});

describe("classificação de assinatura", () => {
  const states = (docs: ContractDocument[]) =>
    buildDocumentDrive(docs, resolver, NOW).entries.map((entry) => entry.signatureState);

  it("arquivo de apoio não vira pendência de assinatura", () => {
    expect(
      states([
        makeDocument({ id: "foto", entityType: "property", entityId: "pr-1", category: "foto", status: "pendente" }),
        makeDocument({ id: "matricula", entityType: "property", entityId: "pr-1", category: "juridico", status: "pendente" }),
        makeDocument({ id: "comprovante", category: "comprovante", status: "pendente" }),
        makeDocument({ id: "planilha", source: "manual", category: "outros", status: "pendente" }),
      ]),
    ).toEqual(["not_required", "not_required", "not_required", "not_required"]);
  });

  it("contrato exige assinatura mesmo sem envelope criado", () => {
    const drive = buildDocumentDrive(
      [makeDocument({ id: "contrato", contractId: "ct-1", category: "contrato", status: "pendente" })],
      resolver,
      NOW,
    );

    expect(drive.entries[0].signatureState).toBe("not_started");
    expect(drive.entries[0].signatureLabel).toBe("Assinatura não iniciada");
    expect(drive.counts.collections.not_started).toBe(1);
    expect(drive.counts.collections.not_required).toBe(0);
  });

  it("aditivo e procuração também exigem assinatura por natureza", () => {
    expect(
      states([
        makeDocument({ id: "aditivo", category: "aditivo", status: "pendente" }),
        makeDocument({ id: "procuracao", category: "procuracao", status: "pendente" }),
        makeDocument({ id: "notificacao", category: "notificacao", status: "pendente" }),
      ]),
    ).toEqual(["not_started", "not_started", "not_started"]);
  });

  it("signatários definem o estágio do fluxo", () => {
    expect(
      states([
        makeDocument({
          id: "aguardando",
          category: "contrato",
          status: "pendente",
          signers: [{ id: "s1", name: "A", email: "a@x.com", role: "locador", status: "pendente" }],
        }),
        makeDocument({
          id: "parcial",
          category: "contrato",
          status: "pendente",
          signers: [
            { id: "s1", name: "A", email: "a@x.com", role: "locador", status: "assinado" },
            { id: "s2", name: "B", email: "b@x.com", role: "locatario", status: "pendente" },
          ],
        }),
        makeDocument({
          id: "concluido",
          category: "contrato",
          status: "pendente",
          signers: [{ id: "s1", name: "A", email: "a@x.com", role: "locador", status: "assinado" }],
        }),
      ]),
    ).toEqual(["awaiting", "partially_signed", "signed"]);
  });

  it("um comprovante com signatários passa a exigir assinatura", () => {
    expect(
      states([
        makeDocument({
          id: "comprovante-assinado",
          category: "comprovante",
          status: "pendente",
          signers: [{ id: "s1", name: "A", email: "a@x.com", role: "testemunha", status: "pendente" }],
        }),
      ]),
    ).toEqual(["awaiting"]);
  });

  it("status explícito de assinatura vence a categoria", () => {
    expect(states([makeDocument({ id: "foto-assinada", category: "foto", status: "assinado" })])).toEqual(["signed"]);
    expect(states([makeDocument({ id: "foto-pendente", category: "foto", status: "pendente_assinatura" })])).toEqual([
      "awaiting",
    ]);
  });

  it("rascunho, expirado e cancelado têm precedência sobre a assinatura", () => {
    expect(
      states([
        makeDocument({ id: "rascunho", category: "contrato", status: "rascunho" }),
        makeDocument({ id: "expirado", category: "contrato", status: "expirado" }),
        makeDocument({ id: "cancelado", category: "contrato", status: "cancelado" }),
      ]),
    ).toEqual(["draft", "expired", "cancelled"]);
  });

  it("as coleções separam o que foi enviado do que nem foi enviado", () => {
    const drive = buildDocumentDrive(
      [
        makeDocument({ id: "foto", category: "foto", status: "pendente" }),
        makeDocument({ id: "enviado", category: "contrato", status: "pendente_assinatura" }),
        makeDocument({ id: "nao-enviado", category: "contrato", status: "pendente" }),
        makeDocument({ id: "assinado", category: "contrato", status: "assinado" }),
      ],
      resolver,
      NOW,
    );

    expect(drive.counts.collections.awaiting).toBe(1);
    expect(drive.counts.collections.not_started).toBe(1);
    expect(drive.counts.collections.signed).toBe(1);
    expect(drive.counts.collections.not_required).toBe(1);
  });
});

describe("filtro por pasta", () => {
  const drive = buildDocumentDrive(
    [
      makeDocument({ id: "contrato", contractId: "ct-1" }),
      makeDocument({ id: "imovel", entityType: "property", entityId: "pr-1", category: "foto" }),
      makeDocument({ id: "avulso", source: "manual", tags: ["urgente"] }),
    ],
    resolver,
    NOW,
  );

  it("seleciona a coleção, a origem e o registro específico", () => {
    expect(filterByFolder(drive.entries, { kind: "collection", id: "all" }, NOW)).toHaveLength(3);
    expect(filterByFolder(drive.entries, { kind: "origin", origin: "property" }, NOW).map((e) => e.doc.id)).toEqual([
      "imovel",
    ]);
    expect(
      filterByFolder(drive.entries, { kind: "origin-entity", origin: "property", entityId: "pr-1" }, NOW).map(
        (e) => e.doc.id,
      ),
    ).toEqual(["imovel"]);
    expect(
      filterByFolder(drive.entries, { kind: "origin-entity", origin: "property", entityId: "pr-9" }, NOW),
    ).toHaveLength(0);
  });

  it("filtra por tipo de documento e por tag", () => {
    expect(filterByFolder(drive.entries, { kind: "category", category: "foto" }, NOW).map((e) => e.doc.id)).toEqual([
      "imovel",
    ]);
    expect(filterByFolder(drive.entries, { kind: "tag", tag: "urgente" }, NOW).map((e) => e.doc.id)).toEqual([
      "avulso",
    ]);
  });

  it("a coleção de recentes respeita a janela de 7 dias", () => {
    const recentDrive = buildDocumentDrive(
      [
        makeDocument({ id: "antigo", uploadDate: "2026-01-01T12:00:00Z" }),
        makeDocument({ id: "novo", uploadDate: "2026-09-18T09:00:00Z" }),
      ],
      resolver,
      NOW,
    );
    const [antigo, novo] = recentDrive.entries;

    expect(matchesCollection(antigo, "recent", NOW)).toBe(false);
    expect(matchesCollection(novo, "recent", NOW)).toBe(true);
  });
});

describe("agrupamento e ordenação", () => {
  const drive = buildDocumentDrive(
    [
      makeDocument({ id: "1", contractId: "ct-1", name: "Zeta", uploadDate: "2026-09-01T12:00:00Z" }),
      makeDocument({ id: "2", contractId: "ct-1", name: "Alfa", uploadDate: "2026-09-10T12:00:00Z" }),
      makeDocument({ id: "3", entityType: "property", entityId: "pr-1", category: "foto", name: "Beta" }),
    ],
    resolver,
    NOW,
  );

  it("agrupa por origem com contagem e dica de contexto", () => {
    const sections = groupDriveEntries(drive.entries, "origin");

    expect(sections.map((s) => s.label)).toEqual(["Contratos", "Imóveis"]);
    expect(sections[0].count).toBe(2);
    expect(sections[0].hint).toBe("Minutas, contratos assinados e aditivos");
  });

  it("agrupa por tipo, status e responsável", () => {
    expect(groupDriveEntries(drive.entries, "category").map((s) => s.label).sort()).toEqual(["Contrato", "Foto"]);
    expect(groupDriveEntries(drive.entries, "status").map((s) => s.label)).toEqual(["Assinado"]);
    expect(groupDriveEntries(drive.entries, "actor").map((s) => s.label)).toEqual(["Bruno Exemplo"]);
    expect(groupDriveEntries(drive.entries, "none")).toEqual([]);
  });

  it("ordena por data, nome e tamanho", () => {
    // Datas: doc 3 (17/09), doc 2 (10/09) e doc 1 (01/09).
    expect(sortDriveEntries(drive.entries, "recent").map((e) => e.doc.id)).toEqual(["3", "2", "1"]);
    expect(sortDriveEntries(drive.entries, "oldest").map((e) => e.doc.id)).toEqual(["1", "2", "3"]);
    expect(sortDriveEntries(drive.entries, "name").map((e) => e.doc.name)).toEqual(["Alfa", "Beta", "Zeta"]);
    expect(sortDriveEntries(drive.entries, "size")[0].doc.fileSize).toBe(524_000);
  });
});
