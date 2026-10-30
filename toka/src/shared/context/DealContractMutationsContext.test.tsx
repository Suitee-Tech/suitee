import { render } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ContractStatus, type Contract, type CreateContractRequest } from "../contracts/types";
import type { Property } from "../data";
import type { DealDTO } from "../services/deal.service";
import { DealContractMutationsProvider, useDealContractMutationsContext } from "./DealContractMutationsContext";

const mocks = vi.hoisted(() => ({
  createContract: vi.fn(),
  warning: vi.fn(),
}));

vi.mock("../services/contract.service", () => ({
  contractService: { create: mocks.createContract },
}));

vi.mock("sonner", () => ({
  toast: { warning: mocks.warning },
}));

const deal: DealDTO = {
  id: "deal-1",
  type: "SALE",
  status: "CONTRACT",
  property_id: "property-1",
  owner_id: "owner-1",
  customer_id: "customer-1",
  broker_id: "broker-1",
  value: 3950000,
  commission: 6,
  created_at: "2026-08-28T00:00:00Z",
  updated_at: "2026-08-28T00:00:00Z",
  tenant_id: "tenant-1",
  signers: [
    { id: "buyer-1", name: "Buyer", email: "buyer@example.com", role: "comprador", status: "PENDING" },
    { id: "spouse-1", name: "Spouse", email: "spouse@example.com", role: "conjuge", status: "PENDING" },
  ],
};

const request: CreateContractRequest = {
  deal_id: deal.id,
  content: "data:application/vnd.openxmlformats-officedocument.wordprocessingml.document;base64,AA==",
  signatories: deal.signers!.map(({ id, name, email, role }) => ({ id, name, email, role })),
  start_date: "2026-08-28T00:00:00Z",
  type: "venda",
};

let createFromDeal: ReturnType<typeof useDealContractMutationsContext>["createContractFromDeal"] | undefined;

function Probe() {
  createFromDeal = useDealContractMutationsContext().createContractFromDeal;
  return null;
}

type MoveDeal = (dealId: string, stage: any) => Promise<unknown>;
type UpdatePropertyStatus = (id: string, status: Property["status"]) => Promise<boolean>;
type AddContract = (contract: Contract) => void;

function renderProvider(overrides?: {
  moveDeal?: MoveDeal;
  updatePropertyStatus?: UpdatePropertyStatus;
  addContract?: AddContract;
}) {
  const moveDeal = overrides?.moveDeal ?? vi.fn<MoveDeal>().mockResolvedValue(undefined);
  const updatePropertyStatus = overrides?.updatePropertyStatus ?? vi.fn<UpdatePropertyStatus>().mockResolvedValue(true);
  const addContract = overrides?.addContract ?? vi.fn<AddContract>();

  render(
    <DealContractMutationsProvider
      fetchDealById={vi.fn().mockResolvedValue(deal)}
      updatePropertyStatus={updatePropertyStatus}
      moveDeal={moveDeal}
      addContract={addContract}
    >
      <Probe />
    </DealContractMutationsProvider>,
  );

  return { moveDeal, updatePropertyStatus, addContract };
}

describe("DealContractMutationsProvider", () => {
  beforeEach(() => {
    createFromDeal = undefined;
    mocks.createContract.mockReset();
    mocks.warning.mockReset();
  });

  it("propagates the contract API error to the screen", async () => {
    const apiError = new Error("invalid signatory role");
    mocks.createContract.mockRejectedValueOnce(apiError);
    const { moveDeal, updatePropertyStatus } = renderProvider();

    await expect(createFromDeal!(deal.id, request)).rejects.toBe(apiError);

    expect(moveDeal).not.toHaveBeenCalled();
    expect(updatePropertyStatus).not.toHaveBeenCalled();
  });

  it("does not report creation failure when a post-create projection fails", async () => {
    const created: Contract = {
      id: "contract-1",
      deal_id: deal.id,
      status: ContractStatus.DRAFT,
      content: request.content,
      signatories: request.signatories,
      start_date: request.start_date,
      type: request.type,
      created_at: "2026-08-28T00:00:00Z",
      updated_at: "2026-08-28T00:00:00Z",
      tenant_id: "tenant-1",
    };
    mocks.createContract.mockResolvedValueOnce(created);
    const moveDeal = vi.fn<MoveDeal>().mockRejectedValueOnce(new Error("pipeline unavailable"));
    const addContract = vi.fn<AddContract>();
    renderProvider({ moveDeal, addContract });

    await expect(createFromDeal!(deal.id, request)).resolves.toBe(created);

    expect(addContract).toHaveBeenCalledWith(created);
    expect(mocks.warning).toHaveBeenCalledWith(
      "Contrato criado, mas a etapa ou o imóvel não foi atualizado automaticamente.",
    );
  });
});
