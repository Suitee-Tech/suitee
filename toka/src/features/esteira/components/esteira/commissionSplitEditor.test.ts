import { describe, expect, it } from "vitest";

import type { BrokerParticipation } from "../../../../shared/services/deal.service";
import {
  buildCommissionSplitAutofillParticipants,
  buildCommissionSplitEditorRows,
  evaluateCommissionSplitRows,
  serializeCommissionSplitRows,
} from "./commissionSplitEditor";

const EXISTING_WITH_DUPLICATE_ROLE: BrokerParticipation[] = [
  { broker_id: "broker-captador-1", role: "captador", percentage: 20 },
  { broker_id: "broker-captador-2", role: "captador", percentage: 10 },
  { broker_id: "broker-visitador", role: "visitador", percentage: 20 },
  { broker_id: "broker-fechamento", role: "fechamento", percentage: 50 },
];

describe("commissionSplitEditor", () => {
  it("preserva todas as participações com o mesmo papel e cria defaults apenas para papéis ausentes", () => {
    const rows = buildCommissionSplitEditorRows(
      EXISTING_WITH_DUPLICATE_ROLE,
      { co_captador: 15, atendente: 10 },
    );

    expect(rows.filter((row) => row.role === "captador")).toEqual([
      expect.objectContaining({
        brokerId: "broker-captador-1",
        percentage: 20,
      }),
      expect.objectContaining({
        brokerId: "broker-captador-2",
        percentage: 10,
      }),
    ]);
    expect(rows.filter((row) => row.role === "co_captador")).toEqual([
      expect.objectContaining({ brokerId: "", percentage: 0 }),
    ]);
    expect(rows).toHaveLength(7);
    expect(serializeCommissionSplitRows(rows)).toEqual(EXISTING_WITH_DUPLICATE_ROLE);
  });

  it("compara cada beneficiário duplicado sem confundir participantes do mesmo papel", () => {
    const rows = buildCommissionSplitEditorRows(
      EXISTING_WITH_DUPLICATE_ROLE,
      {},
    );

    expect(
      evaluateCommissionSplitRows(rows, EXISTING_WITH_DUPLICATE_ROLE, {}),
    ).toMatchObject({
      sumOk: true,
      hasEmptyBroker: false,
      hasRowsChanged: false,
      canSaveWithoutCommissionChange: false,
    });

    const changedRows = rows.map((row) =>
      row.rowKey === "existing:captador:1"
        ? { ...row, brokerId: "broker-captador-3" }
        : row,
    );

    expect(
      evaluateCommissionSplitRows(
        changedRows,
        EXISTING_WITH_DUPLICATE_ROLE,
        {},
      ),
    ).toMatchObject({
      sumOk: true,
      hasEmptyBroker: false,
      hasRowsChanged: true,
      canSaveWithoutCommissionChange: true,
    });
  });

  it("habilita o save de um deal novo quando apenas os corretores dos defaults são escolhidos", () => {
    const initialRows = buildCommissionSplitEditorRows([], {});
    const rowsWithBrokers = initialRows.map((row) =>
      row.percentage > 0
        ? { ...row, brokerId: `broker-${row.role}` }
        : row,
    );

    expect(
      evaluateCommissionSplitRows(rowsWithBrokers, [], {}),
    ).toMatchObject({
      sum: 100,
      sumOk: true,
      hasEmptyBroker: false,
      hasRowsChanged: true,
      canSaveWithoutCommissionChange: true,
    });
    expect(serializeCommissionSplitRows(rowsWithBrokers)).toEqual([
      { broker_id: "broker-captador", role: "captador", percentage: 30 },
      { broker_id: "broker-visitador", role: "visitador", percentage: 20 },
      { broker_id: "broker-fechamento", role: "fechamento", percentage: 50 },
    ]);
  });

  it("serializa o remainder da imobiliária sem inventar um broker favorecido", () => {
    const initialRows = buildCommissionSplitEditorRows([], {
      captador: 30,
      fechamento: 50,
      imobiliaria: 20,
    });
    const rows = initialRows.map((row) =>
      row.role !== "imobiliaria" && row.percentage > 0
        ? { ...row, brokerId: `broker-${row.role}` }
        : row,
    );

    expect(evaluateCommissionSplitRows(rows, [], {})).toMatchObject({
      sum: 100,
      sumOk: true,
      hasEmptyBroker: false,
      hasDuplicateParticipant: false,
    });
    expect(serializeCommissionSplitRows(rows)).toContainEqual({
      broker_id: "",
      role: "imobiliaria",
      percentage: 20,
    });
  });

  it("bloqueia o mesmo corretor repetido na mesma função", () => {
    const rows = buildCommissionSplitEditorRows(EXISTING_WITH_DUPLICATE_ROLE, {}).map(
      (row) =>
        row.role === "captador"
          ? { ...row, brokerId: "broker-repetido" }
          : row,
    );

    expect(
      evaluateCommissionSplitRows(rows, EXISTING_WITH_DUPLICATE_ROLE, {}),
    ).toMatchObject({
      hasDuplicateParticipant: true,
      canSaveWithoutCommissionChange: false,
    });
  });

  it("carrega participações existentes de captador e atendente sem sobrescrever", () => {
    const existing: BrokerParticipation[] = [
      { broker_id: "broker-captador", role: "captador", percentage: 60 },
      { broker_id: "broker-atendente", role: "atendente", percentage: 40 },
    ];

    const rows = buildCommissionSplitEditorRows(existing, {});

    expect(rows.filter((row) => row.role === "captador")).toEqual([
      expect.objectContaining({ brokerId: "broker-captador", percentage: 60 }),
    ]);
    expect(rows.filter((row) => row.role === "atendente")).toEqual([
      expect.objectContaining({ brokerId: "broker-atendente", percentage: 40 }),
    ]);
    // Every other role is an empty row, untouched by any default.
    const others = rows.filter((row) => row.role !== "captador" && row.role !== "atendente");
    expect(others.every((row) => row.brokerId === "" && row.percentage === 0)).toBe(true);
  });

  it("não injeta defaults de papéis ausentes quando o deal já tem participações", () => {
    const existing: BrokerParticipation[] = [
      { broker_id: "broker-captador", role: "captador", percentage: 100 },
    ];
    // Config comum: captador + atendente configurados, mas o deal só tem captador.
    const rows = buildCommissionSplitEditorRows(existing, { captador: 30, atendente: 20 });

    const atendente = rows.find((row) => row.role === "atendente");
    expect(atendente?.percentage).toBe(0);
    expect(atendente?.brokerId).toBe("");
  });

  it("preenche captador pelo imóvel e fechamento pelo atendente do lead usando as regras da imobiliária", () => {
    const autofill = buildCommissionSplitAutofillParticipants(
      [{ id: "broker-property", role: "captador" }],
      "broker-lead",
    );
    const rules = {
      captador: 30,
      co_captador: 0,
      visitador: 0,
      atendente: 0,
      fechamento: 50,
      imobiliaria: 20,
    };
    const rows = buildCommissionSplitEditorRows([], rules, autofill);

    expect(rows.find((row) => row.role === "captador")).toMatchObject({
      brokerId: "broker-property",
      percentage: 30,
    });
    expect(rows.find((row) => row.role === "fechamento")).toMatchObject({
      brokerId: "broker-lead",
      percentage: 50,
    });
    expect(rows.find((row) => row.role === "atendente")).toMatchObject({
      brokerId: "",
      percentage: 0,
    });
    expect(rows.find((row) => row.role === "imobiliaria")).toMatchObject({
      brokerId: "",
      percentage: 20,
    });
    expect(evaluateCommissionSplitRows(rows, [], rules)).toMatchObject({
      sum: 100,
      sumOk: true,
      hasEmptyBroker: false,
      hasRowsChanged: true,
      canSaveWithoutCommissionChange: true,
    });
  });

  it("reconstrói o rateio legado vindo apenas da captação com os percentuais configurados", () => {
    const existing: BrokerParticipation[] = [
      { broker_id: "broker-property", role: "captador", percentage: 100 },
    ];
    const autofill = buildCommissionSplitAutofillParticipants(
      [{ id: "broker-property", role: "captador" }],
      "broker-lead",
    );
    const rules = { captador: 20, fechamento: 40, imobiliaria: 40 };
    const rows = buildCommissionSplitEditorRows(existing, rules, autofill);

    expect(serializeCommissionSplitRows(rows)).toEqual([
      { broker_id: "broker-property", role: "captador", percentage: 20 },
      { broker_id: "broker-lead", role: "fechamento", percentage: 40 },
      { broker_id: "", role: "imobiliaria", percentage: 40 },
    ]);
    expect(evaluateCommissionSplitRows(rows, existing, rules)).toMatchObject({
      sumOk: true,
      hasRowsChanged: true,
      canSaveWithoutCommissionChange: true,
    });
  });

  it("preserva um rateio completo já editado mesmo quando as fontes e regras mudam", () => {
    const existing: BrokerParticipation[] = [
      { broker_id: "broker-manual-cap", role: "captador", percentage: 35 },
      { broker_id: "broker-manual-close", role: "fechamento", percentage: 45 },
      { broker_id: "", role: "imobiliaria", percentage: 20 },
    ];
    const autofill = buildCommissionSplitAutofillParticipants(
      [{ id: "broker-property", role: "captador" }],
      "broker-lead",
    );
    const rows = buildCommissionSplitEditorRows(
      existing,
      { captador: 20, fechamento: 40, imobiliaria: 40 },
      autofill,
    );

    expect(serializeCommissionSplitRows(rows)).toEqual(existing);
    expect(
      evaluateCommissionSplitRows(
        rows,
        existing,
        { captador: 20, fechamento: 40, imobiliaria: 40 },
      ),
    ).toMatchObject({
      sumOk: true,
      hasRowsChanged: false,
      canSaveWithoutCommissionChange: false,
    });
  });

  it("divide o percentual configurado do papel entre múltiplos captadores do imóvel", () => {
    const autofill = buildCommissionSplitAutofillParticipants(
      [
        { id: "broker-property-1", role: "captador" },
        { id: "broker-property-2", role: "captador" },
      ],
      "broker-lead",
    );
    const rows = buildCommissionSplitEditorRows(
      [],
      { captador: 30, fechamento: 50, imobiliaria: 20 },
      autofill,
    );

    expect(rows.filter((row) => row.role === "captador")).toEqual([
      expect.objectContaining({ brokerId: "broker-property-1", percentage: 15 }),
      expect.objectContaining({ brokerId: "broker-property-2", percentage: 15 }),
    ]);
    expect(rows.reduce((sum, row) => sum + row.percentage, 0)).toBe(100);
  });

  it("ignora papéis do imóvel que não representam captação e deduplica corretor por papel", () => {
    expect(
      buildCommissionSplitAutofillParticipants(
        [
          { id: "broker-cap", role: "captador" },
          { id: "broker-cap", role: "captador" },
          { id: "broker-visitor", role: "visitador" },
        ],
        "broker-lead",
      ),
    ).toEqual([
      { brokerId: "broker-cap", role: "captador", source: "property" },
      { brokerId: "broker-lead", role: "fechamento", source: "lead" },
    ]);
  });
});
