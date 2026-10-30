import { contractService } from "../services/contract.service";
import { chargeService, type Charge } from "../services/charge.service";
import { transferService, type OwnerTransfer } from "../services/transfer.service";
import type { Contract } from "./types";

export const contractsKeys = {
  all: () => ["contracts"] as const,
  list: () => ["contracts", "list"] as const,
  detail: (id: string) => ["contracts", "detail", id] as const,
  byDeal: (dealId: string) => ["contracts", "by-deal", dealId] as const,
  charges: (contractId: string) => ["contracts", contractId, "charges"] as const,
  transfers: (contractId: string) => ["contracts", contractId, "transfers"] as const,
};

export const fetchContractsList = async (): Promise<Contract[]> => {
  return contractService.getAll();
};

export const fetchContractsByDeal = async (dealId: string): Promise<Contract[]> => {
  return (await contractService.listByDeal(dealId)) ?? [];
};

export const fetchContractCharges = async (contractId: string): Promise<Charge[]> => {
  const res = await chargeService.getAll({ contract_id: contractId });
  return res?.data ?? [];
};

export const fetchContractTransfers = async (contractId: string): Promise<OwnerTransfer[]> => {
  const res = await transferService.getAll({ contract_id: contractId });
  return res?.data ?? [];
};
