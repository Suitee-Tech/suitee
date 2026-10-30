import { describe, expect, it } from "vitest";
import type { ChecklistItemDTO, Proposal } from "../../../shared/services/deal.service";
import type { PortalAppointment } from "../services/portal.service";
import { portalCopy, resolvePortalPersona } from "../lib/portalPersona";
import {
  formatAppointmentDate,
  getAppointmentStatusMeta,
  getChecklistProgress,
  getPortalDealNextAction,
  sortAppointments,
  sortProposalsNewestFirst,
} from "./PortalDealDetail.presentation";

function checklistItem(overrides: Partial<ChecklistItemDTO>): ChecklistItemDTO {
  return {
    id: "item-1",
    storage_key: "",
    name: "Documento",
    category: "pessoal",
    status: "PENDING",
    required: true,
    solicited: true,
    ...overrides,
  };
}

function proposal(overrides: Partial<Proposal>): Proposal {
  return {
    id: "proposal-1",
    value: 500_000,
    payment_method: "Financiamento",
    status: "SENT",
    created_at: "2026-09-05T12:00:00.000Z",
    created_by: "broker-1",
    ...overrides,
  };
}

function appointment(overrides: Partial<PortalAppointment>): PortalAppointment {
  return {
    id: "appointment-1",
    title: "Visita",
    type: "Visita",
    date: "2026-09-10",
    time: "14:00",
    end_time: "15:00",
    status: "Pendente",
    created_at: "2026-09-01T12:00:00.000Z",
    ...overrides,
  };
}

describe("PortalDealDetail presentation", () => {
  it("prioritizes rejected and pending required documents during analysis", () => {
    const checklist = [
      checklistItem({ id: "approved", status: "APPROVED" }),
      checklistItem({ id: "rejected", status: "REJECTED" }),
      checklistItem({ id: "optional", required: false, status: "PENDING" }),
    ];

    expect(getChecklistProgress(checklist)).toEqual({
      total: 2,
      approved: 1,
      awaitingReview: 0,
      needsAction: 1,
      percent: 50,
    });
    expect(getPortalDealNextAction({ status: "ANALYSIS", checklist })).toMatchObject({
      title: "Documentos aguardando você",
      label: "Resolver pendência",
      stage: "ANALYSIS",
      action: "stage",
    });
  });

  it("distinguishes a signature action from waiting for the other signers", () => {
    expect(
      getPortalDealNextAction({
        status: "CONTRACT",
        signature: {
          has_envelope: true,
          my_signer: { signer_id: "me", name: "Ana", role: "Compradora", status: "PENDING" },
          other_signers: [],
          signed_count: 0,
          total_count: 1,
        },
      }),
    ).toMatchObject({ label: "Assinar contrato", tone: "warning" });

    expect(
      getPortalDealNextAction({
        status: "CONTRACT",
        signature: {
          has_envelope: true,
          my_signer: { signer_id: "me", name: "Ana", role: "Compradora", status: "SIGNED" },
          other_signers: [{ signer_id: "other", name: "João", role: "Vendedor", status: "PENDING" }],
          signed_count: 1,
          total_count: 2,
        },
      }),
    ).toMatchObject({ title: "Aguardando as outras assinaturas", tone: "success" });
  });

  it("uses the newest proposal even when the API list is not ordered", () => {
    const older = proposal({ id: "older", created_at: "2026-09-01T12:00:00.000Z" });
    const newer = proposal({ id: "newer", created_at: "2026-09-06T12:00:00.000Z" });

    expect(sortProposalsNewestFirst([older, newer]).map((item) => item.id)).toEqual(["newer", "older"]);
  });

  it("sorts visits chronologically and presents status and time in pt-BR", () => {
    const later = appointment({ id: "later", date: "2026-09-12", status: "Confirmado" });
    const sooner = appointment({ id: "sooner", date: "2026-09-08", time: "09:30", end_time: "10:15" });

    expect(sortAppointments([later, sooner]).map((item) => item.id)).toEqual(["sooner", "later"]);
    expect(formatAppointmentDate(sooner.date, sooner.time, sooner.end_time)).toContain("09:30–10:15");
    expect(getAppointmentStatusMeta("Confirmado")).toEqual({ label: "Confirmada", tone: "success" });
  });

  it("não manda quem anuncia agendar visita ao próprio imóvel", () => {
    const ownerVisits = portalCopy(resolvePortalPersona({ role: "owner", dealType: "SALE" })).visits;

    expect(getPortalDealNextAction({ status: "NEGOTIATION", visits: ownerVisits })).toMatchObject({
      title: "Visitas ao seu imóvel",
      label: "Acompanhar visitas",
      action: "stage",
    });
    expect(
      getPortalDealNextAction({ status: "NEGOTIATION", visits: ownerVisits, appointments: [appointment({})] }),
    ).toMatchObject({ title: "Visitas ao seu imóvel", action: "stage" });
  });

  it("quem procura continua com a próxima ação de agendar visita", () => {
    const seekerVisits = portalCopy(resolvePortalPersona({ role: "buyer", dealType: "SALE" })).visits;

    expect(getPortalDealNextAction({ status: "NEGOTIATION", visits: seekerVisits })).toMatchObject({
      title: "Conheça o imóvel de perto",
      label: "Agendar visita",
      action: "schedule",
    });
  });
});
