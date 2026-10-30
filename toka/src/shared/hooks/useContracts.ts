import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { notifyApiError } from "../services/errorToast";
import { contractService } from "../services/contract.service";
import {
  chargeService,
  type CreateChargeDTO,
  type MarkPaidDTO,
  type RepairRentalAllocationsResult,
  type RepairRentalAllocationsEligibility,
} from "../services/charge.service";
import { unwrapApiData } from "../services/api";
import { formatCurrencyBRL } from "../utils/format";
import { transferService, type CreateTransferDTO } from "../services/transfer.service";
import { rentalChargeConfigService } from "../services/rentalChargeConfig.service";
import {
  utilityAccountService,
  type UtilityAccount,
  type CreateUtilityAccountDTO,
  type UpdateUtilityAccountDTO,
} from "../services/utilityAccount.service";
import {
  reminderRuleService,
  type ReminderRule,
  type CreateReminderRuleDTO,
  type UpdateReminderRuleDTO,
} from "../services/reminderRule.service";
import {
  contractsKeys,
  fetchContractCharges,
  fetchContractTransfers,
  fetchContractsList,
} from "../contracts/queries";
import { lancamentosKeys } from "../queries/lancamentos";
import type { Contract } from "../contracts/types";
import type { CreateRentalChargeConfigDTO, UpdateRentalChargeConfigDTO } from "../rentals/types";

function replaceById<T extends { id: string }>(
  current: T[] | undefined,
  updated: T,
): T[] {
  if (!current) return [updated];
  const exists = current.some((item) => item.id === updated.id);
  return exists
    ? current.map((item) => (item.id === updated.id ? updated : item))
    : [updated, ...current];
}

export function useContractsListQuery() {
  return useQuery({
    queryKey: contractsKeys.list(),
    queryFn: fetchContractsList,
    staleTime: 30_000,
  });
}

export function useContractChargesQuery(contractId: string | null | undefined) {
  return useQuery({
    queryKey: contractsKeys.charges(contractId ?? ""),
    queryFn: () => fetchContractCharges(contractId!),
    enabled: Boolean(contractId),
    staleTime: 30_000,
  });
}

export function useContractTransfersQuery(contractId: string | null | undefined) {
  return useQuery({
    queryKey: contractsKeys.transfers(contractId ?? ""),
    queryFn: () => fetchContractTransfers(contractId!),
    enabled: Boolean(contractId),
    staleTime: 30_000,
  });
}

export function useNotifyChargeMutation() {
  return useMutation({
    mutationFn: (chargeId: string) => chargeService.notify(chargeId),
    onSuccess: (res) => {
      const channels = res?.data?.channels_sent ?? [];
      const skipped = res?.data?.skipped ?? [];
      if (channels.length > 0) {
        toast.success(`Lembrete enviado (${channels.join(", ")})`);
      }
      if (skipped.length > 0) {
        toast.message("Algumas vias não foram usadas", { description: skipped.join("; ") });
      }
    },
    onError: (error: any) => {
      notifyApiError(error, { context: "Enviar lembrete" });
    },
  });
}

export function useMarkChargePaidMutation(contractId: string | null | undefined) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ chargeId, data }: { chargeId: string; data: MarkPaidDTO }) => chargeService.markPaid(chargeId, data),
    onSuccess: () => {
      if (contractId) {
        qc.invalidateQueries({ queryKey: contractsKeys.charges(contractId) });
        qc.invalidateQueries({ queryKey: contractsKeys.transfers(contractId) });
      }
      qc.invalidateQueries({ queryKey: lancamentosKeys.charges() });
      qc.invalidateQueries({ queryKey: lancamentosKeys.transfers() });
      qc.invalidateQueries({ queryKey: lancamentosKeys.commissions() });
      toast.success("Pagamento registrado");
    },
    onError: (error: any) => {
      notifyApiError(error, { context: "Registrar pagamento" });
    },
  });
}

/**
 * Elegibilidade do reparo de alocação (read-only). Só dispara a consulta quando
 * o chamador já sabe que a cobrança é de locação quitada — o botão depende
 * dela para aparecer apenas quando há algo a reparar.
 */
export function useRepairRentalAllocationsEligibilityQuery(
  chargeId: string | null | undefined,
  enabled: boolean,
) {
  return useQuery({
    queryKey: ["charges", chargeId ?? "", "repair-rental-allocations", "eligibility"] as const,
    queryFn: async () =>
      unwrapApiData<RepairRentalAllocationsEligibility>(
        await chargeService.getRepairRentalAllocationsEligibility(chargeId as string),
      ),
    enabled: Boolean(chargeId) && enabled,
    staleTime: 30 * 1000,
  });
}

/**
 * Reparo de alocação da intermediação de uma locação quitada antes do plano
 * existir. Mexe em cobrança, ledger (estorno + relançamento), comissão, payout
 * e repasse — por isso invalida todas essas listas de uma vez.
 */
export function useRepairRentalAllocationsMutation(contractId: string | null | undefined) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ chargeId, reason }: { chargeId: string; reason: string }) =>
      chargeService.repairRentalAllocations(chargeId, reason),
    onSuccess: (response) => {
      if (contractId) {
        qc.invalidateQueries({ queryKey: contractsKeys.charges(contractId) });
        qc.invalidateQueries({ queryKey: contractsKeys.transfers(contractId) });
      }
      qc.invalidateQueries({ queryKey: lancamentosKeys.charges() });
      qc.invalidateQueries({ queryKey: lancamentosKeys.transfers() });
      qc.invalidateQueries({ queryKey: lancamentosKeys.commissions() });
      qc.invalidateQueries({ queryKey: lancamentosKeys.payouts() });

      const result = unwrapApiData<RepairRentalAllocationsResult>(response);
      if (result?.warnings?.length) {
        toast.warning("Reparo aplicado com avisos", { description: result.warnings.join(" ") });
      }
      if (result?.skipped_reason) {
        toast.info("Nada a reparar", { description: result.skipped_reason });
        return;
      }
      const funded = result?.intermediation_funded_cents ?? 0;
      const parts: string[] = [];
      if (funded > 0) parts.push(`Intermediação de ${formatCurrencyBRL(funded / 100)} financiada`);
      if (result?.journal_entries_reversed) parts.push(`${result.journal_entries_reversed} lançamento(s) estornado(s)`);
      if (result?.journal_entries_reposted) parts.push(`${result.journal_entries_reposted} regravado(s)`);
      if (result?.transfer_reconciled) parts.push("repasse atualizado");
      toast.success("Alocações reparadas", { description: parts.join(" · ") || undefined });
    },
    onError: (error: any) => {
      notifyApiError(error, { context: "Reparar alocações" });
    },
  });
}

export function useCreateChargeMutation() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: CreateChargeDTO) => chargeService.create(data),
    onSuccess: (_res, vars) => {
      qc.invalidateQueries({ queryKey: contractsKeys.charges(vars.contract_id) });
      toast.success("Cobrança criada com sucesso");
    },
    onError: (error: any) => {
      notifyApiError(error, { context: "Criar cobrança" });
    },
  });
}

export function useCreateTransferMutation() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: CreateTransferDTO) => transferService.create(data),
    onSuccess: (_res, vars) => {
      qc.invalidateQueries({ queryKey: contractsKeys.transfers(vars.contract_id) });
      qc.invalidateQueries({ queryKey: lancamentosKeys.transfers() });
      toast.success("Repasse gerado com sucesso");
    },
    onError: (error: any) => {
      notifyApiError(error, { context: "Gerar repasse" });
    },
  });
}

export function useApproveTransferMutation(contractId: string | null | undefined) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (transferId: string) => transferService.approve(transferId),
    onSuccess: () => {
      if (contractId) qc.invalidateQueries({ queryKey: contractsKeys.transfers(contractId) });
      qc.invalidateQueries({ queryKey: lancamentosKeys.transfers() });
      toast.success("Repasse aprovado");
    },
    onError: (error: any) => {
      notifyApiError(error, { context: "Aprovar repasse" });
    },
  });
}

export function useProcessTransferMutation(contractId: string | null | undefined) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (
      input:
        | string
        | {
            transferId: string;
            evidence?: { proof_url?: string; note?: string };
          },
    ) => {
      const transferId = typeof input === "string" ? input : input.transferId;
      const evidence = typeof input === "string" ? undefined : input.evidence;
      return transferService.process(transferId, evidence);
    },
    onSuccess: () => {
      if (contractId) qc.invalidateQueries({ queryKey: contractsKeys.transfers(contractId) });
      qc.invalidateQueries({ queryKey: lancamentosKeys.transfers() });
      toast.success("Repasse processado");
    },
    onError: (error: any) => {
      notifyApiError(error, { context: "Processar repasse" });
    },
  });
}

export const rentalChargeConfigKey = (contractId: string) => ["contracts", contractId, "rental-charge-config"] as const;

export function useRentalChargeConfigQuery(contractId: string | null | undefined) {
  return useQuery({
    queryKey: rentalChargeConfigKey(contractId ?? ""),
    queryFn: () => rentalChargeConfigService.getByContractId(contractId!),
    enabled: Boolean(contractId),
    staleTime: 30_000,
  });
}

export function useCreateRentalChargeConfigMutation() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: CreateRentalChargeConfigDTO) => rentalChargeConfigService.create(data),
    onSuccess: (res, vars) => {
      qc.setQueryData(rentalChargeConfigKey(vars.contract_id), res.config);
      qc.invalidateQueries({ queryKey: rentalChargeConfigKey(vars.contract_id) });
      qc.invalidateQueries({ queryKey: contractsKeys.charges(vars.contract_id) });
      toast.success("Plano-base salvo. Revise a composição e emita as cobranças quando estiver pronto.");
    },
    onError: (error: any) => {
      notifyApiError(error, { context: "Salvar configuração" });
    },
  });
}

export function useUpdateRentalChargeConfigMutation(contractId: string | null | undefined) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: UpdateRentalChargeConfigDTO }) => rentalChargeConfigService.update(id, data),
    onSuccess: (updated) => {
      if (contractId) {
        qc.setQueryData(rentalChargeConfigKey(contractId), updated);
        qc.invalidateQueries({ queryKey: rentalChargeConfigKey(contractId) });
        qc.invalidateQueries({ queryKey: contractsKeys.charges(contractId) });
      }
      toast.success("Configuração atualizada");
    },
    onError: (error: any) => {
      const msg = error?.response?.data?.message;
      if (msg === "first charge already paid") {
        toast.error("Não é possível editar: a primeira cobrança já foi paga.");
        return;
      }
      toast.error(msg ?? "Não foi possível atualizar a configuração");
    },
  });
}

export function useGenerateRentalChargesMutation(contractId: string | null | undefined) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (configId: string) => rentalChargeConfigService.generateCharges(configId),
    onSuccess: (res) => {
      if (contractId) qc.invalidateQueries({ queryKey: contractsKeys.charges(contractId) });
      if (res.charges_created > 0) {
        toast.success(`${res.charges_created} cobranças emitidas`);
      } else {
        toast.message("Calendário já estava atualizado", {
          description: "Nenhuma cobrança nova precisou ser criada.",
        });
      }
    },
    onError: (error: any) => {
      notifyApiError(error, { context: "Gerar cobranças" });
    },
  });
}

export const utilityAccountsKey = (contractId: string) => ["contracts", contractId, "utility-accounts"] as const;

export function useUtilityAccountsQuery(contractId: string | null | undefined) {
  return useQuery({
    queryKey: utilityAccountsKey(contractId ?? ""),
    queryFn: () => utilityAccountService.list(contractId!),
    enabled: Boolean(contractId),
    staleTime: 30_000,
  });
}

export function useCreateUtilityAccountMutation() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: CreateUtilityAccountDTO) => utilityAccountService.create(data),
    onSuccess: (_res, vars) => {
      qc.invalidateQueries({ queryKey: utilityAccountsKey(vars.contract_id) });
      toast.success("Conta vinculada com sucesso");
    },
    onError: (error: any) => {
      notifyApiError(error, { context: "Vincular conta" });
    },
  });
}

export function useUpdateUtilityAccountMutation(contractId: string | null | undefined) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: UpdateUtilityAccountDTO }) => utilityAccountService.update(id, data),
    onMutate: async ({ id, data }) => {
      if (!contractId) return { previous: undefined, queryKey: undefined };
      const queryKey = utilityAccountsKey(contractId);
      await qc.cancelQueries({ queryKey });
      const previous = qc.getQueryData<UtilityAccount[]>(queryKey);
      qc.setQueryData<UtilityAccount[]>(queryKey, (current = []) =>
        current.map((account) =>
          account.id === id ? { ...account, ...data } : account,
        ),
      );
      return { previous, queryKey };
    },
    onSuccess: (updated) => {
      if (contractId) {
        qc.setQueryData<UtilityAccount[]>(
          utilityAccountsKey(contractId),
          (current) => replaceById(current, updated),
        );
      }
      toast.success("Conta atualizada");
    },
    onError: (error: any, _variables, context) => {
      if (context?.queryKey && context.previous !== undefined) {
        qc.setQueryData(context.queryKey, context.previous);
      }
      notifyApiError(error, { context: "Atualizar conta" });
    },
    onSettled: () => {
      if (contractId) {
        qc.invalidateQueries({ queryKey: utilityAccountsKey(contractId) });
      }
    },
  });
}

export function useDeleteUtilityAccountMutation(contractId: string | null | undefined) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => utilityAccountService.remove(id),
    onSuccess: () => {
      if (contractId) qc.invalidateQueries({ queryKey: utilityAccountsKey(contractId) });
      toast.success("Conta removida");
    },
    onError: (error: any) => {
      notifyApiError(error, { context: "Remover conta" });
    },
  });
}

export const reminderRulesKey = () => ["reminder-rules"] as const;

export function useReminderRulesQuery() {
  return useQuery({
    queryKey: reminderRulesKey(),
    queryFn: () => reminderRuleService.list(),
    staleTime: 30_000,
  });
}

export function useCreateReminderRuleMutation() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: CreateReminderRuleDTO) => reminderRuleService.create(data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: reminderRulesKey() });
      toast.success("Regra criada");
    },
    onError: (error: any) => {
      notifyApiError(error, { context: "Criar regra" });
    },
  });
}

export function useUpdateReminderRuleMutation() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: UpdateReminderRuleDTO }) => reminderRuleService.update(id, data),
    onMutate: async ({ id, data }) => {
      const queryKey = reminderRulesKey();
      await qc.cancelQueries({ queryKey });
      const previous = qc.getQueryData<ReminderRule[]>(queryKey);
      qc.setQueryData<ReminderRule[]>(queryKey, (current = []) =>
        current.map((rule) => (rule.id === id ? { ...rule, ...data } : rule)),
      );
      return { previous, queryKey };
    },
    onSuccess: (updated) => {
      qc.setQueryData<ReminderRule[]>(
        reminderRulesKey(),
        (current) => replaceById(current, updated),
      );
    },
    onError: (error: any, _variables, context) => {
      if (context?.previous !== undefined) {
        qc.setQueryData(context.queryKey, context.previous);
      }
      notifyApiError(error, { context: "Atualizar regra" });
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: reminderRulesKey() });
    },
  });
}

export function useDeleteReminderRuleMutation() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => reminderRuleService.remove(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: reminderRulesKey() });
      toast.success("Regra removida");
    },
    onError: (error: any) => {
      notifyApiError(error, { context: "Remover regra" });
    },
  });
}

export function useRunReminderRulesNowMutation() {
  return useMutation({
    mutationFn: () => reminderRuleService.runNow(),
    onSuccess: (res) => {
      toast.success(`Régua avaliada: ${res.notifies_sent} envios, ${res.notify_skipped} pulos`);
      if (res.errors && res.errors.length > 0) {
        toast.message("Erros durante a execução", { description: res.errors.slice(0, 3).join("; ") });
      }
    },
    onError: (error: any) => {
      notifyApiError(error, { context: "Executar régua" });
    },
  });
}

export function useTerminateContractMutation() {
  const qc = useQueryClient();
  return useMutation<Contract, Error, { id: string; reason?: string }>({
    mutationFn: ({ id }) => contractService.update(id, { status: "TERMINATED" } as any),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: contractsKeys.list() });
      toast.success("Contrato encerrado");
    },
    onError: (error: any) => {
      notifyApiError(error, { context: "Encerrar contrato" });
    },
  });
}

export interface AdjustContractInput {
  value: number;
  adjust_index: string;
  next_adjustment: string;
}

interface AdjustContractMutationContext {
  previous: Contract[] | undefined;
  queryKey: ReturnType<typeof contractsKeys.list>;
}

export function useAdjustContractMutation() {
  const qc = useQueryClient();
  return useMutation<
    Contract,
    Error,
    { id: string; data: AdjustContractInput },
    AdjustContractMutationContext
  >({
    mutationFn: ({ id, data }) =>
      contractService.update(
        id,
        {
          value: data.value,
          adjust_index: data.adjust_index,
          next_adjustment: data.next_adjustment,
        } as any,
      ),
    onMutate: async ({ id, data }) => {
      const queryKey = contractsKeys.list();
      await qc.cancelQueries({ queryKey });
      const previous = qc.getQueryData<Contract[]>(queryKey);
      qc.setQueryData<Contract[]>(queryKey, (current = []) =>
        current.map((contract) =>
          contract.id === id ? { ...contract, ...data } : contract,
        ),
      );
      return { previous, queryKey };
    },
    onSuccess: (updated) => {
      qc.setQueryData<Contract[]>(
        contractsKeys.list(),
        (current) => replaceById(current, updated),
      );
      toast.success("Reajuste aplicado com sucesso");
    },
    onError: (error: any, _variables, context) => {
      if (context?.previous !== undefined) {
        qc.setQueryData(context.queryKey, context.previous);
      }
      notifyApiError(error, { context: "Aplicar reajuste" });
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: contractsKeys.list() });
    },
  });
}
