export enum ContractStatus {
  DRAFT = "DRAFT",
  PENDING_SIGNATURE = "PENDING_SIGNATURE",
  // Estado intermediário: o contrato sai de PENDING_SIGNATURE no primeiro
  // signatário que assina (com grupos de ordem, o intervalo pode ser longo — o
  // Grupo 2 só é convidado depois do Grupo 1) e só vira SIGNED no último.
  // Faltava no enum, e por isso nenhuma tela o reconhecia.
  PARTIALLY_SIGNED = "PARTIALLY_SIGNED",
  SIGNED = "SIGNED",
  ACTIVE = "ACTIVE",
  EXPIRED = "EXPIRED",
  TERMINATED = "TERMINATED",
}

export type SignerAuthMethod = "email" | "whatsapp" | "sms" | "selfie" | "icp_brasil" | "selfie+document";

export interface Signatory {
  id: string;
  name: string;
  email: string;
  role: string;
  signed_at?: string;
  signature?: string;
  phone_number?: string;
  documentation?: string;
  birthday?: string;
  clicksign_signer_key?: string;
  auth_methods?: SignerAuthMethod[];
  evidence_url?: string;
  group?: number;
}

export interface ContractAdjustmentEntry {
  applied_at: string;
  previous_value: number;
  new_value: number;
  index_name?: string;
  index_value?: number;
  effective_from: string;
  triggered_by?: string;
  mode?: "manual" | "auto";
  note?: string;
}

export interface ContractTermination {
  terminated_at: string;
  reason?: string;
  early_termination_fee?: number;
  security_deposit_return?: number;
  security_deposit_deductions?: number;
  exit_inspection_id?: string;
  triggered_by?: string;
}

export interface ContractAmendmentEntry {
  new_contract_id: string;
  created_at: string;
  reason?: string;
  triggered_by?: string;
  superseded_at?: string;
}

export interface Contract {
  id: string;
  display_id?: string;
  deal_id?: string;
  status: ContractStatus;
  content: string;
  signatories: Signatory[];
  start_date: string;
  end_date?: string;
  type?: string;
  property_id?: string;
  property_title?: string;
  client?: string;
  value?: number;
  commission_pct?: number;
  customer_id?: string;
  index?: string;
  autoIndex?: boolean;
  nextAdjustment?: string;
  adjust_index?: string;
  auto_adjust?: boolean;
  next_adjustment?: string;
  late_fee_pct?: number;
  monthly_interest_pct?: number;
  security_deposit?: number;
  adjustments?: ContractAdjustmentEntry[];
  amendments?: ContractAmendmentEntry[];
  amends_contract_id?: string;
  termination?: ContractTermination;
  entry_inspection_id?: string;
  created_at: string;
  updated_at: string;
  tenant_id: string;
  external_envelope_id?: string;
  signature_provider?: string;
  sent_for_signature_at?: string;
  signature_locked_at?: string;
  signed_document_url?: string;
  draft_revisions?: {
    content: string;
    signatories: Signatory[];
    replaced_at: string;
    replaced_by: string;
  }[];
}

export type SignatureEnvelopeStatus =
  | "PENDING"
  | "DRAFT"
  | "RUNNING"
  | "PARTIALLY_SIGNED"
  | "FINISHED"
  | "REFUSED"
  | "CANCELED"
  | "EXPIRED"
  | "ERROR";

export type SignatureSignerStatus = "PENDING" | "VIEWED" | "SIGNED" | "REFUSED";

export interface SignatureEnvelopeSigner {
  signer_id: string;
  clicksign_signer_key?: string;
  name: string;
  email: string;
  phone_number?: string;
  documentation?: string;
  birthday?: string;
  role: "sign" | "witness";
  auth: SignerAuthMethod[];
  status: SignatureSignerStatus;
  signed_at?: string;
  request_signature_key?: string;
  group?: number;
}

export interface SignatureEnvelopeDocument {
  clicksign_document_key?: string;
  sha256?: string;
  size_bytes?: number;
  filename?: string;
}

export interface SignatureEnvelope {
  id: string;
  tenant_id: string;
  contract_id: string;
  deal_id?: string;
  external_envelope_id?: string;
  environment: "sandbox" | "production";
  status: SignatureEnvelopeStatus;
  signers: SignatureEnvelopeSigner[];
  documents?: SignatureEnvelopeDocument[];
  document?: SignatureEnvelopeDocument;
  signed_document_url?: string;
  last_error?: string;
  created_at: string;
  updated_at: string;
  finished_at?: string;
}

export interface SignatureInlineAttachment {
  filename: string;
  content_base64: string;
}

export interface SendForSignatureRequest {
  signer_auth: Record<string, SignerAuthMethod[]>;
  signer_groups?: Record<string, number>;
  attachment_document_ids?: string[];
  inline_attachments?: SignatureInlineAttachment[];
  locale?: string;
}

export interface CreateContractRequest {
  deal_id: string;
  content: string;
  signatories: Signatory[];
  start_date: string;
  end_date?: string;
  type?: string;
  property_id?: string;
  property_title?: string;
  client?: string;
  value?: number;
  commission_pct?: number;
}

export interface SignContractRequest {
  signatory_id: string;
  signature: string;
}
