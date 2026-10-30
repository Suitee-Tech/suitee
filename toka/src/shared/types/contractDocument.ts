import type {
  AnalysisCriterion,
  DocumentCategory,
  DocumentStatus,
  DocumentSource,
  DocumentPersistenceStatus,
  EntityType,
  SignerStatus,
} from "../documents/types";

export type {
  AnalysisCriterion,
  DocumentCategory,
  DocumentStatus,
  DocumentSource,
  DocumentPersistenceStatus,
  EntityType,
  SignerStatus,
};

export interface DocumentSigner {
  id: string;
  name: string;
  email: string;
  role: string;
  status: SignerStatus;
  signedAt?: string;
}

export interface ContractDocument {
  id: string;
  contractId: string;
  name: string;
  category: DocumentCategory;
  fileType: string;
  fileSize: number;
  uploadDate: string;
  uploadedBy: string;
  status: DocumentStatus;
  signers: DocumentSigner[];
  expiresAt?: string;
  notes?: string;
  tags?: string[];
  fileUrl?: string;
  createdAt?: string;
  updatedAt?: string;
  tenantId?: string;
  entityId?: string;
  entityType?: EntityType;
  /** Escopo da análise de crédito/due diligence: parte e critério. */
  analysisPartyId?: string;
  analysisCriterion?: AnalysisCriterion;
  storageKey?: string;
  fileHash?: string;
  source?: DocumentSource;
  version?: number;
  parentId?: string;
  persistenceStatus?: DocumentPersistenceStatus;
  lastPersistError?: string;
  persistAttemptedAt?: string;
}

