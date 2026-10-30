import { describe, expect, it } from "vitest";
import type { DealDTO } from "../../../shared/services/deal.service";
import type { Lead } from "../../../shared/data";
import { mapDealToEsteiraDeal } from "./mapDealToEsteiraDeal";
const deal: DealDTO = { id: "deal", type: "SALE", status: "CONTRACT", property_id: "property", owner_id: "owner", customer_id: "customer", broker_id: "broker", value: 100000, commission: 5, created_at: "2026-09-15T12:00:00Z", updated_at: "2026-09-16T12:00:00.123456789Z", tenant_id: "agency" };
const customer: Lead = { id: "customer", name: "Cliente", document: "synthetic-document", email: "client@example.invalid", phone: "", property: "", budget: 0, status: "Qualificado", date: deal.created_at };
describe("typed deal presentation mapping", () => {
  it("preserves exact temporal fields and the authorized customer document", () => {
    expect(mapDealToEsteiraDeal(deal, [], [customer])).toMatchObject({ id: "deal", client: "Cliente", client_doc: "synthetic-document", stage: "Contrato", created_at: deal.created_at, updated_at: deal.updated_at });
  });
  it("tolerates missing linked records without inventing client identity", () => {
    expect(mapDealToEsteiraDeal(deal, [], [])).toMatchObject({ client: "Cliente Desconhecido", client_doc: "", property: "Imóvel Desconhecido" });
  });
});
