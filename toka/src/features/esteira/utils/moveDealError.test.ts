import { describe, it, expect } from "vitest";
import { moveDealErrorMessage } from "./moveDealError";
import type { DealDTO } from "../../../shared/services/deal.service";

const deal = {
  id: "deal-1",
  parties: [
    { id: "p-tenant", name: "João Silva", email: "joao@x.com", role: "tenant" },
    { id: "p-owner", name: "", email: "owner@x.com", role: "owner" },
  ],
} as unknown as DealDTO;

describe("moveDealErrorMessage", () => {
  it("parties pending missing document → names party, CPF/CNPJ and target stage", () => {
    const raw =
      "required parties have incomplete data (name, email or document missing): party_id=p-tenant role=tenant missing=document";
    const msg = moveDealErrorMessage(raw, deal, "CONTRACT");
    expect(msg).toContain("João Silva");
    expect(msg).toContain("inquilino");
    expect(msg).toContain("CPF/CNPJ");
    expect(msg).toContain("Contrato");
  });

  it("parties pending missing name/email → falls back to email when name empty", () => {
    const raw =
      "required parties have incomplete data (name, email or document missing): party_id=p-owner role=owner missing=name/email";
    const msg = moveDealErrorMessage(raw, deal, "ANALYSIS");
    expect(msg).toContain("nome ou e-mail");
    expect(msg).toContain("owner@x.com");
  });

  it("party not found in deal → degrades to role label without crashing", () => {
    const raw =
      "required parties have incomplete data (...): party_id=ghost role=guarantor missing=document";
    const msg = moveDealErrorMessage(raw, deal, "CONTRACT");
    expect(msg.toLowerCase()).toContain("fiador");
    expect(msg).toContain("uma parte obrigatória");
  });

  it("proposal not approved", () => {
    const msg = moveDealErrorMessage(
      "proposal must be accepted by all required parties before advancing",
      deal,
      "ANALYSIS",
    );
    expect(msg).toContain("proposta precisa ser aceita");
  });

  it("spouse missing resolves principal name", () => {
    const msg = moveDealErrorMessage(
      "spouse required before advancing: principal_party_id=p-tenant",
      deal,
      "ANALYSIS",
    );
    expect(msg).toContain("cônjuge");
    expect(msg).toContain("João Silva");
  });

  it("approval threshold", () => {
    const msg = moveDealErrorMessage(
      "deal value exceeds approval threshold — requires deals:approve permission: deal_value=600000.00 threshold=500000.00 target=CONTRACT",
      deal,
      "CONTRACT",
    );
    expect(msg).toContain("deals:approve");
  });

  it("checklist gate", () => {
    const msg = moveDealErrorMessage(
      "required checklist items must be approved before moving to CONTRACT (tenant gate enabled)",
      deal,
      "CONTRACT",
    );
    expect(msg).toContain("checklist");
  });

  it("unknown message passes through unchanged", () => {
    expect(moveDealErrorMessage("some random error", deal, "CONTRACT")).toBe("some random error");
  });

  it("empty → generic message", () => {
    expect(moveDealErrorMessage("", deal, "CONTRACT")).toBe(
      "Não foi possível mover o negócio. Tente novamente.",
    );
  });

  it("undefined deal → no crash, role label still used", () => {
    const raw =
      "required parties have incomplete data (...): party_id=p-tenant role=tenant missing=document";
    const msg = moveDealErrorMessage(raw, undefined, "CONTRACT");
    expect(msg).toContain("inquilino");
  });
});
