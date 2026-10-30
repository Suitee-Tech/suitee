import { describe, expect, it } from "vitest";
import type { DealDTO } from "../../../shared/services/deal.service";
import type { PortalOverviewLead } from "../services/portal.service";
import {
  dealForLead,
  effectiveLeadStatus,
  leadJourney,
  leadPropertyLabel,
  leadStatusLabel,
  nextLeadAction,
} from "./portalRelationship";

function lead(overrides: Partial<PortalOverviewLead> = {}): PortalOverviewLead {
  return {
    id: "lead-1",
    name: "Cliente",
    has_preferences: false,
    matched_properties: [],
    ...overrides,
  };
}

function deal(overrides: Partial<DealDTO> = {}): DealDTO {
  return {
    id: "deal-1",
    type: "SALE",
    status: "NEGOTIATION",
    property_id: "property-1",
    owner_id: "owner-1",
    customer_id: "lead-1",
    broker_id: "broker-1",
    value: 100,
    commission: 1,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    tenant_id: "tenant-1",
    ...overrides,
  };
}

describe("portalRelationship", () => {
  it("traduz o status e marca a etapa atual da jornada", () => {
    expect(leadStatusLabel("CONTACTED")).toBe("Em atendimento");
    expect(leadStatusLabel("UNKNOWN")).toBe("unknown");
    expect(leadJourney("VISIT_SCHEDULED").map((step) => step.state)).toEqual([
      "done",
      "done",
      "current",
      "todo",
      "todo",
    ]);
  });

  it("usa a visita confirmada como etapa efetiva do atendimento", () => {
    const current = lead({
      status: "CONTACTED",
      visit_request: { date: "2026-01-02", time: "10:00", status: "Confirmado" },
    });
    expect(effectiveLeadStatus(current)).toBe("VISIT_SCHEDULED");
    expect(leadJourney(effectiveLeadStatus(current))[2].state).toBe("current");
  });

  it("propõe a próxima ação conforme visita/etapa", () => {
    expect(nextLeadAction(lead({ status: "NEW" })).title).toMatch(/contato/i);
    expect(nextLeadAction(lead({ status: "PROPOSAL" })).title).toMatch(/proposta/i);
    expect(
      nextLeadAction(
        lead({
          status: "VISIT_SCHEDULED",
          visit_request: { date: "2026-01-02", time: "10:00", status: "Confirmado" },
        }),
      ).title,
    ).toMatch(/visita/i);
  });

  it("usa o imóvel recomendado como identificador e cruza lead↔negócio", () => {
    const current = lead({
      recommended_property: {
        id: "property-1",
        title: "Casa no Centro",
        matched_properties: [],
      } as any,
    });
    expect(leadPropertyLabel(current)).toBe("Casa no Centro");
    expect(dealForLead(current, [deal()])?.id).toBe("deal-1");
    expect(dealForLead(lead({ id: "lead-2" }), [deal()])).toBeNull();
  });
});
