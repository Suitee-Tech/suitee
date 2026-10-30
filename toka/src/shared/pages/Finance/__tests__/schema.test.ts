import { describe, it, expect } from "vitest";
import {
  LancamentoSchema,
  ParticipantSchema,
  LancamentoDraftEnvelopeSchema,
  LANCAMENTO_DRAFT_VERSION,
  initialLancamentoDraft,
} from "../schema";

describe("ParticipantSchema", () => {
  it("deve validar participante completo com todos os campos", () => {
    const result = ParticipantSchema.safeParse({
      id: 1,
      role: "captador",
      name: "João Silva",
      percent: 30,
      brokerId: "b1",
      amount: 15000,
      commissionType: "captador",
    });
    expect(result.success).toBe(true);
  });

  it("deve validar participante mínimo (apenas campos obrigatórios)", () => {
    const result = ParticipantSchema.safeParse({
      id: 1,
      role: "Participante",
      name: "",
      percent: 0,
    });
    expect(result.success).toBe(true);
  });

  it("deve rejeitar participante sem campos obrigatórios", () => {
    const result = ParticipantSchema.safeParse({});
    expect(result.success).toBe(false);
  });
});

describe("LancamentoSchema", () => {
  it("deve validar um draft completo válido", () => {
    const result = LancamentoSchema.safeParse({
      lancamentoTipo: "recebimento",
      lancamentoOrigem: "venda",
      selectedContractId: "c1",
      baseValue: 500000,
      commissionPercent: 6,
      distributeCommission: true,
      participants: [
        { id: 1, role: "captador", name: "João", percent: 50 },
        { id: 2, role: "fechamento", name: "Maria", percent: 50 },
      ],
      loadingCommission: false,
      commissionDataLoaded: true,
      commissionDataError: undefined,
    });
    expect(result.success).toBe(true);
  });

  it("deve permitir draft sem campos opcionais de commission", () => {
    const result = LancamentoSchema.safeParse(initialLancamentoDraft(6));
    expect(result.success).toBe(true);
  });

  it("deve rejeitar lancamentoTipo inválido", () => {
    const result = LancamentoSchema.safeParse({
      ...initialLancamentoDraft(6),
      lancamentoTipo: "invalido",
    });
    expect(result.success).toBe(false);
  });

  it("deve rejeitar lancamentoOrigem inválida", () => {
    const result = LancamentoSchema.safeParse({
      ...initialLancamentoDraft(6),
      lancamentoOrigem: "doacao",
    });
    expect(result.success).toBe(false);
  });
});

describe("LancamentoDraftEnvelopeSchema", () => {
  it("deve validar envelope correto", () => {
    const result = LancamentoDraftEnvelopeSchema.safeParse({
      _version: LANCAMENTO_DRAFT_VERSION,
      _savedAt: Date.now(),
      payload: initialLancamentoDraft(6),
    });
    expect(result.success).toBe(true);
  });

  it("deve rejeitar envelope com versão incorreta", () => {
    const result = LancamentoDraftEnvelopeSchema.safeParse({
      _version: 999,
      _savedAt: Date.now(),
      payload: initialLancamentoDraft(6),
    });
    expect(result.success).toBe(false);
  });

  it("deve rejeitar envelope sem _savedAt", () => {
    const result = LancamentoDraftEnvelopeSchema.safeParse({
      _version: LANCAMENTO_DRAFT_VERSION,
      payload: initialLancamentoDraft(6),
    });
    expect(result.success).toBe(false);
  });

  it("deve rejeitar envelope sem payload", () => {
    const result = LancamentoDraftEnvelopeSchema.safeParse({
      _version: LANCAMENTO_DRAFT_VERSION,
      _savedAt: Date.now(),
    });
    expect(result.success).toBe(false);
  });

  it("deve rejeitar envelope com versão antiga (v1) após bump para v2", () => {
    const result = LancamentoDraftEnvelopeSchema.safeParse({
      _version: 1,
      _savedAt: Date.now(),
      payload: {
        lancamentoTipo: "recebimento",
        lancamentoOrigem: "venda",
        selectedContractId: "c1",
        baseValue: 1000,
        commissionPercent: 6,
        distributeCommission: true,
        participants: [],
      },
    });
    expect(result.success).toBe(false);
  });
});

describe("initialLancamentoDraft", () => {
  it("deve criar draft com valores padrão corretos", () => {
    const draft = initialLancamentoDraft(6);
    expect(draft.lancamentoTipo).toBe("recebimento");
    expect(draft.lancamentoOrigem).toBe("venda");
    expect(draft.commissionPercent).toBe(6);
    expect(draft.distributeCommission).toBe(true);
    expect(draft.participants).toEqual([]);
    expect(draft.loadingCommission).toBe(false);
    expect(draft.commissionDataLoaded).toBe(false);
    expect(draft.commissionDataError).toBeUndefined();
  });

  it("deve aceitar diferentes valores de comissão padrão", () => {
    const draftVenda = initialLancamentoDraft(6);
    expect(draftVenda.commissionPercent).toBe(6);

    const draftLocacao = initialLancamentoDraft(100);
    expect(draftLocacao.commissionPercent).toBe(100);
  });
});
