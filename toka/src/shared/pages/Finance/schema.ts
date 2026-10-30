import * as z from "zod";

// Versão do envelope persistido em localStorage. Bump quando qualquer
// campo de Lancamento mudar de shape — drafts antigos serão descartados
// no restore em vez de aplicados parcialmente.
export const LANCAMENTO_DRAFT_VERSION = 3 as const;

export const STORAGE_KEY = "finance_lancamento_draft";

export const ParticipantSchema = z.object({
  id: z.number(),
  role: z.string(),
  name: z.string(),
  percent: z.number(),
  brokerId: z.string().optional(),
  amount: z.number().optional(),
  commissionType: z.string().optional(),
});

export type Participant = z.infer<typeof ParticipantSchema>;

export type CommissionDistributionData = {
  commissions: Array<{
    broker_id: string;
    role: string;
    percentage: number;
    amount_cents: number;
  }>;
  deal_type: string;
  total_commission_cents: number;
  contract_id?: string;
  deal_id: string;
};

export const LancamentoSchema = z.object({
  lancamentoTipo: z.enum(["recebimento", "comissao"]),
  lancamentoOrigem: z.enum(["venda", "locacao"]),
  selectedContractId: z.string(),
  /** ID do corretor/parceiro selecionado como beneficiário (comissão avulsa) */
  selectedBeneficiaryId: z.string().optional(),
  /** Nome do corretor/parceiro selecionado como beneficiário (comissão avulsa) */
  selectedBeneficiaryName: z.string().optional(),
  baseValue: z.number(),
  commissionPercent: z.number(),
  distributeCommission: z.boolean(),
  participants: z.array(ParticipantSchema),
  loadingCommission: z.boolean().optional(),
  commissionDataLoaded: z.boolean().optional(),
  commissionDataError: z.string().optional(),
  /** Resumo do plano de pagamentos programado na esteira (ex.: "Entrada + 11x"). */
  paymentPlanLabel: z.string().optional(),
  /** Primeiro vencimento programado na esteira (YYYY-MM-DD). */
  firstDueDate: z.string().optional(),
  /** Resumo dos descontos/abatimentos aplicáveis programados na esteira. */
  discountSummary: z.string().optional(),
});

export type LancamentoDraft = z.infer<typeof LancamentoSchema>;

export const LancamentoDraftEnvelopeSchema = z.object({
  _version: z.literal(LANCAMENTO_DRAFT_VERSION),
  _savedAt: z.number(),
  payload: LancamentoSchema,
});

export type LancamentoDraftEnvelope = z.infer<typeof LancamentoDraftEnvelopeSchema>;

// 24h — drafts mais velhos são considerados abandonados.
export const DRAFT_TTL_MS = 86_400_000;

export const initialLancamentoDraft = (defaultCommissionPercent: number): LancamentoDraft => ({
  lancamentoTipo: "recebimento",
  lancamentoOrigem: "venda",
  selectedContractId: "",
  selectedBeneficiaryId: "",
  selectedBeneficiaryName: "",
  baseValue: 0,
  commissionPercent: defaultCommissionPercent,
  distributeCommission: true,
  participants: [],
  loadingCommission: false,
  commissionDataLoaded: false,
  commissionDataError: undefined,
  paymentPlanLabel: undefined,
  firstDueDate: undefined,
  discountSummary: undefined,
});
