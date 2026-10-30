export interface Transaction {
  id: string;
  contractId: string;
  type: "receita" | "despesa" | "repasse" | "comissao";
  description: string;
  value: number;
  date: string;
  status:
    | "PENDING"
    | "PAID"
    | "OVERDUE"
    | "REVERSED"
    | "CANCELLED"
    | "Pendente"
    | "Pago"
    | "Em Atraso"
    | "Estornado"
    | "Cancelado";
  dueDate?: string;
  paymentDate?: string | null;
  paymentMethod?: string | null;
  category?: string;
  month?: string;
  grossValue?: number;
  adminFee?: number;
  maintenanceDeductions?: number;
  utilityDeductions?: number;
  /** ID único para rastreabilidade e idempotência.
   *  Prefixo "contract.signed:" = gerado pelo pipeline de eventos
   *  UUID aleatório = gerado manualmente via Finance.tsx */
  sourceEventID?: string;
  /** ID comum às linhas criadas por uma mesma operação manual. */
  operationID?: string;
  /** ID persistido e estável desta linha dentro da operação. */
  postingID?: string;
  /** Origem do registro exibido. Campos de apresentação não são enviados em
   *  mutações: servem para reconciliar projeções sem inferir por descrição,
   *  valor ou competência. */
  recordSource?: "transaction" | "invoice";
  /** Cobrança canônica que originou uma projeção de fatura, quando informada
   *  explicitamente pela API. */
  linkedChargeID?: string;
}
