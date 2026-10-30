import { describe, expect, it } from "vitest";

import { buildContractVariableData } from "./contractVariableData";

/**
 * Dia de vencimento vem das configurações da imobiliária. Quando a configuração
 * não existe, o chamador precisa poder dizer "não há dia" — e o builder não pode
 * transformar isso numa data plausível e errada.
 */
const rentalArgs = {
  dealDetails: { type: "RENT", parties: [], value: 3430 } as never,
  property: null,
  signers: [],
  buyerLead: null,
  sellerLead: null,
  brokersById: {},
  ownerLeadsById: {},
  rentalStartDate: "2026-09-14",
  rentalEndDate: "2029-03-14",
  computedEndDate: "2029-03-14",
  firstChargeDate: "2026-09-14",
  firstChargeType: "same_month_pro_rata" as const,
};

const build = (defaultDueDay: number) =>
  buildContractVariableData({
    ...rentalArgs,
    contract: { type: "locacao" } as never,
    defaultDueDay,
  });

describe("buildContractVariableData · datas ausentes", () => {
  // Contrato de locação sem data de primeira cobrança (ou sem término) montava
  // `new Date("T00:00:00Z")`, que é inválida: `.toISOString()` lançava
  // `RangeError: Invalid time value` e derrubava a página inteira do contrato.
  it("não quebra e deixa as variáveis vazias quando faltam datas", () => {
    const data = buildContractVariableData({
      ...rentalArgs,
      rentalStartDate: "",
      rentalEndDate: "",
      computedEndDate: "",
      firstChargeDate: "",
      contract: { type: "locacao" } as never,
      defaultDueDay: 10,
    });
    expect(data.contract_start_date_iso).toBe("");
    expect(data.rent_standard_due_date).toBe("");
  });

  it("degrada para valores válidos em vez de lançar", () => {
    expect(() =>
      buildContractVariableData({
        ...rentalArgs,
        rentalStartDate: "data-invalida",
        contract: { type: "locacao" } as never,
        defaultDueDay: 10,
      }),
    ).not.toThrow();
  });
});

describe("buildContractVariableData · vencimento padrão", () => {
  it("usa o dia configurado para a variável e para a data de vencimento", () => {
    const data = build(10);
    expect(data.rent_default_due_day).toBe("10");
    expect(data.rent_standard_due_date).toBe("10/10/2026");
  });

  it("respeita um dia diferente do padrão do sistema", () => {
    const data = build(25);
    expect(data.rent_default_due_day).toBe("25");
    expect(data.rent_standard_due_date).toBe("25/09/2026");
  });

  // `setUTCDate(0)` transborda para o último dia do mês anterior e valores
  // negativos andam para trás: davam uma data de vencimento plausível e errada
  // (0 → 01/10, -1 → 30/09) em vez de ficar em branco.
  it("não inventa data quando não há dia configurado", () => {
    for (const invalido of [0, -1, Number.NaN]) {
      const data = build(invalido);
      expect(data.rent_standard_due_date).toBe("");
    }
  });
});

describe("buildContractVariableData · vocabulário e qualificação", () => {
  it("uses actual qualification and explicit spouse/representative links", () => {
    const data = buildContractVariableData({
      ...rentalArgs, defaultDueDay: 10, contract: null,
      dealDetails: { id: "d1", type: "RENT", value: 3430, parties: [
        { id: "t1", role: "tenant", name: "Maria", document: "123", email: "maria@example.com", is_primary: true,
          birth_date: "1990-04-30", representation: { type: "attorney", representative_party_id: "a1", requires_principal_signature: true, allow_representative_proposal: false },
          custom_fields: { rg: "RG123", nacionalidade: "Brasileira", estado_civil: "Casada", profissao: "Arquiteta", logradouro: "Rua A", numero: "45", bairro: "Centro", cidade: "São Paulo", uf: "SP", cep: "01000-000" } },
        { id: "s1", role: "spouse", spouse_of_party_id: "t1", name: "José", document: "456", email: "jose@example.com" },
        { id: "a1", role: "attorney", name: "Ana", document: "789", email: "ana@example.com" },
        { id: "o1", role: "owner", name: "Empresa A", custom_fields: { razao_social: "Empresa A Ltda", inscricao_estadual: "IE123" }, is_primary: true },
      ] } as never,
    });
    expect(data.tenant_nationality).toBe("Brasileira");
    expect(data.buyer_name).toBe("Maria");
    expect(data.tenant_marital_status).toBe("Casada");
    expect(data.tenant_identity_document).toBe("RG123");
    expect(data.tenant_occupation).toBe("Arquiteta");
    expect(data.tenant_address).toBe("Rua A, 45, Centro, São Paulo - SP, CEP 01000-000");
    expect(data.tenant_birth_date_iso).toBe("1990-04-30");
    expect(data.tenants[0].birth_date_iso).toBe("1990-04-30");
    expect(data.tenant_spouse_name).toBe("José");
    expect(data.tenant_representative_name).toBe("Ana");
    expect(data.attorneys[0].represented_party_names_list).toBe("Maria");
    expect(data.landlord_company_legal_name).toBe("Empresa A Ltda");
    expect(data.landlord_company_state_registration).toBe("IE123");
    expect(data.parties[0].address).toBe(data.tenant_address);
  });
  it("keeps optional missing qualification empty and preserves legacy placeholders", () => {
    const data = build(10);
    expect(data.tenant_nationality).toBe("");
    expect(data.tenant_address).toBe("");
    expect(data.tenant_spouse_name).toBe("");
    expect(data.tenant_representative_name).toBe("");
    expect(data.property_registration).toBe("______________________");
    expect(data.tenant_2_name).toBe("______________________");
  });
  it("maps property/proposal fields, saved terms and English broker aliases", () => {
    const data = buildContractVariableData({
      ...rentalArgs, defaultDueDay: 10,
      dealDetails: { id: "d1", type: "RENT", value: 3430, broker_id: "b1", parties: [],
        broker_participations: [{ broker_id: "b1", role: "captador" }, { broker_id: "b2", role: "visitador" }],
        proposals: [{ id: "p1", value: 3430, counter_value: 3200, status: "ACCEPTED", payment_method: "PIX", created_at: "2026-09-12", notes: "Nota" }],
      } as never,
      brokersById: { b1: { id: "b1", name: "Ana", email: "ana@example.com" }, b2: { id: "b2", name: "Carlos" } } as never,
      property: { id: "p1", areas: { total_area_m2: 150, usable_area_m2: 120, land_area_m2: 200, built_area_m2: 140 },
        features: { bedrooms: 3, bathrooms: 2, suites: 1, parking_spaces: 0 }, capturing_brokers: [{ id: "b1", creci: "12345" }],
        address: { street: "Rua A", number: "10", neighborhood: "Centro", city: "SP", state: "SP", postal_code: "01000" },
        amenities: ["Piscina"], pricing: {}, owner: { id: "o1", percentage: 100 },
      } as never,
      contract: { id: "c1", signatories: [], late_fee_pct: 2, monthly_interest_pct: 1, security_deposit: 6000, adjust_index: "IPCA", auto_adjust: true, next_adjustment: "2027-09-14" } as never,
    });
    expect(data.property_usable_area).toBe("120");
    expect(data.property_parking_spaces).toBe("0");
    expect(data.property_owners_count).toBe("1");
    expect(data.proposal_id).toBe("p1");
    expect(data.proposal_notes).toBe("Nota");
    expect(data.broker_listing_name).toBe(data.broker_captador_name);
    expect(data.broker_visiting_name).toBe(data.broker_visitador_name);
    expect(data.broker_primary_registration).toBe("12345");
    expect(data.contract_late_fee_pct).toBe("2");
    expect(data.contract_adjust_index).toBe("IPCA");
    expect(data.contract_auto_adjust).toBe("Sim");
    expect(data.contract_next_adjustment_iso).toBe("2027-09-14");
    expect(data.contract_next_adjustment_written).toBe("14 de setembro de 2027");
  });
});

describe("buildContractVariableData · datas e prazo efetivos", () => {
  it("uses actual months and clamps due days to the last day of the month", () => {
    const data = buildContractVariableData({ ...rentalArgs, defaultDueDay: 31, contract: null,
      rentalStartDate: "2026-01-31", rentalEndDate: "2027-01-31", firstChargeDate: "2026-01-31" });
    expect(data.contract_term_months).toBe("12");
    expect(data.rent_standard_due_date).toBe("28/02/2026");
    expect(data.rent_standard_due_date_iso).toBe("2026-02-28");
  });
  it("uses persisted dates when requested and editable dates during regeneration", () => {
    const args = { ...rentalArgs, defaultDueDay: 10, contract: { start_date: "2025-08-10T00:00:00Z", end_date: "2027-08-10T00:00:00Z" } as never };
    const saved = buildContractVariableData({ ...args, preferContractDates: true });
    expect(saved.contract_start_date_iso).toBe("2025-08-10");
    expect(saved.contract_end_date_iso).toBe("2027-08-10");
    expect(saved.contract_term_months).toBe("24");
    const edited = buildContractVariableData({ ...args, rentalStartDate: "2026-08-10", rentalEndDate: "2027-08-10" });
    expect(edited.contract_start_date_iso).toBe("2026-08-10");
    expect(edited.contract_term_months).toBe("12");
    const cleared = buildContractVariableData({ ...args, rentalStartDate: "", rentalEndDate: "", computedEndDate: "" });
    expect(cleared.contract_start_date_iso).toBe("");
    expect(cleared.contract_end_date_iso).toBe("");
  });
  it("uses saved sale dates and rejects rollover calendar dates", () => {
    const args = { ...rentalArgs, defaultDueDay: 10, dealDetails: { id: "d1", type: "SALE", value: 1000, parties: [] } as never,
      contract: { start_date: "2025-10-10", end_date: "2026-10-10" } as never };
    expect(buildContractVariableData(args).contract_start_date_iso).toBe("2025-10-10");
    const invalid = buildContractVariableData({ ...rentalArgs, defaultDueDay: 10, contract: null, rentalStartDate: "2026-02-30" });
    expect(invalid.contract_start_date_iso).toBe("");
    expect(invalid.contract_term_months).toBe("");
  });
});

describe("buildContractVariableData · compatibilidade dos fallbacks", () => {
  it("retains signer/lead fallbacks when a registered principal has blank contacts", () => {
    const data = buildContractVariableData({ ...rentalArgs, defaultDueDay: 10, contract: null,
      dealDetails: { id: "d1", type: "RENT", value: 3430, parties: [{ id: "t1", role: "tenant", is_primary: true, name: "", email: "", phone: "", document: "" }] } as never,
      buyerLead: { id: "l1", name: "Maria Cliente", email: "lead@example.com", phone: "119999999", document: "123", qualification: { profissao: "Arquiteta" } } as never,
      signers: [{ id: "s1", role: "inquilino", name: "Maria Signatária", email: "signer@example.com", status: "PENDING" }],
    });
    expect(data.tenant_name).toBe("Maria Signatária");
    expect(data.buyer_name).toBe("Maria Signatária");
    expect(data.tenant_email).toBe("signer@example.com");
    expect(data.tenant_phone).toBe("119999999");
    expect(data.tenant_document).toBe("123");
    expect(data.tenant_occupation).toBe("Arquiteta");
  });
});

it("does not borrow qualification from a lead linked to a different principal", () => {
  const data = buildContractVariableData({ ...rentalArgs, defaultDueDay: 10, contract: null,
    dealDetails: { id: "d1", type: "RENT", value: 3430, parties: [{ id: "t1", lead_id: "other", role: "tenant", name: "Ana", is_primary: true }] } as never,
    buyerLead: { id: "l1", name: "Maria", qualification: { nacionalidade: "Brasileira" } } as never,
  });
  expect(data.tenant_nationality).toBe("");
});

describe("buildContractVariableData · empresa", () => {
  it("expõe os dados reais da imobiliária no contrato", () => {
    const data = buildContractVariableData({
      ...rentalArgs,
      company: {
        id: "company-1",
        name: "Imobiliária Real",
        legal_name: "Imobiliária Real Ltda.",
        tax_id: "12.345.678/0001-90",
        email: "contato@real.test",
        phone: "(11) 99999-0000",
        address: {
          street: "Rua A", number: "10", district: "Centro", city: "São Paulo", state: "SP", zip_code: "01000-000",
        },
        type: "imobiliaria",
        is_active: true,
      },
      contract: null,
      defaultDueDay: 10,
    });

    expect(data.company_name).toBe("Imobiliária Real");
    expect(data.company_legal_name).toBe("Imobiliária Real Ltda.");
    expect(data.company_document).toBe("12.345.678/0001-90");
    expect(data.company_address).toContain("São Paulo");
  });
});
