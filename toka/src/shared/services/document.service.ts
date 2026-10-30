import api from "./api";
import type {
  AnalysisCriterion,
  DocumentCategory,
  DocumentSource,
  DocumentStatus,
  EntityType,
  SignerStatus,
} from "../documents/types";

export type { DocumentCategory, DocumentSource, DocumentStatus, EntityType, SignerStatus } from "../documents/types";
export type { AnalysisCriterion } from "../documents/types";

export interface DocumentSignerDTO {
  id: string;
  name: string;
  email: string;
  role: string;
  status: SignerStatus;
  signed_at?: string;
}

export interface DocumentDTO {
  id: string;
  tenant_id: string;
  name: string;
  file_url?: string;
  file_type: string;
  file_size: number;
  storage_key?: string;
  file_hash?: string;
  persistence_status?: "pending" | "stored" | "failed";
  last_persist_error?: string;
  persist_attempted_at?: string;
  category: DocumentCategory;
  status: DocumentStatus;
  entity_id?: string;
  entity_type?: EntityType;
  analysis_party_id?: string;
  analysis_criterion?: AnalysisCriterion;
  contract_id?: string;
  source: DocumentSource;
  uploaded_by: string;
  upload_date: string;
  signers: DocumentSignerDTO[];
  expires_at?: string;
  notes?: string;
  tags?: string[];
  version: number;
  parent_id?: string;
  // OCR / AI extraction (preenchido assincronamente após o upload).
  extraction_status?: ExtractionStatus;
  extraction?: ExtractedDocumentDTO | null;
  extraction_error?: string;
  created_at: string;
  updated_at: string;
}

export type ExtractionStatus = "pending" | "processing" | "completed" | "failed" | "empty";
export type ConfidenceTier = "high" | "medium" | "low";
export type FieldValidationStatus = "valid" | "invalid" | "unchecked" | "empty";

export interface ExtractedFieldDTO {
  name: string;
  label?: string;
  target?: string;
  value: string;
  raw_value?: string;
  evidence?: string;
  confidence: number;
  tier: ConfidenceTier;
  source: string;
  validation_status: FieldValidationStatus;
  validation_message?: string;
  applied?: boolean;
}

export interface ExtractedDocumentDTO {
  status: string;
  error?: string;
  fields: ExtractedFieldDTO[];
  overall_confidence: number;
  model?: string;
  ocr_provider?: string;
  ocr_confidence?: number | null;
  applied_party_id?: string;
  duration_ms?: number;
  extracted_at: string;
}

export interface DocumentExtractionResult {
  document_id: string;
  extraction_status?: ExtractionStatus;
  extraction?: ExtractedDocumentDTO | null;
  extraction_error?: string;
}

export interface CreateDocumentDTO {
  name: string;
  category: DocumentCategory;
  file_type: string;
  file_size?: number;
  contract_id?: string;
  entity_id?: string;
  entity_type?: EntityType;
  analysis_party_id?: string;
  analysis_criterion?: AnalysisCriterion;
  source?: DocumentSource;
  expires_at?: string;
  notes?: string;
  tags?: string[];
  signers?: Omit<DocumentSignerDTO, "id" | "signed_at">[];
}

export interface UpdateDocumentDTO {
  name?: string;
  category?: DocumentCategory;
  status?: DocumentStatus;
  entity_id?: string;
  entity_type?: EntityType;
  analysis_party_id?: string;
  analysis_criterion?: AnalysisCriterion;
  expires_at?: string;
  notes?: string;
  tags?: string[];
  signers?: DocumentSignerDTO[];
}

class DocumentService {
  private readonly baseUrl = "/documents";

  async getAll(params?: {
    contract_id?: string;
    entity_id?: string;
    entity_type?: EntityType;
    category?: DocumentCategory;
    status?: DocumentStatus;
    source?: DocumentSource;
    q?: string;
    tags?: string[];
    limit?: number;
    offset?: number;
  }) {
    const { tags, ...rest } = params ?? {};
    const merged: Record<string, unknown> = { ...rest };
    if (tags && tags.length > 0) {
      merged.tags = tags.join(",");
    }
    const response = await api.get<{ data: DocumentDTO[]; meta?: any }>(this.baseUrl, { params: merged });
    return response.data;
  }

  async listKnownTags(): Promise<string[]> {
    const response = await api.get<{ data: string[] }>(`${this.baseUrl}/tags`);
    return response.data?.data ?? [];
  }

  async getByContract(contractId: string) {
    const response = await api.get<{ data: DocumentDTO[] }>(this.baseUrl, {
      params: { contract_id: contractId },
    });
    return response.data;
  }

  async getByEntity(entityId: string, entityType: EntityType) {
    const response = await api.get<{ data: DocumentDTO[] }>(`${this.baseUrl}/entity/${entityType}/${entityId}`);
    return response.data;
  }

  async getById(id: string) {
    const response = await api.get<{ data: DocumentDTO }>(`${this.baseUrl}/${id}`);
    return response.data;
  }

  async create(data: CreateDocumentDTO) {
    const response = await api.post<{ data: DocumentDTO }>(this.baseUrl, data);
    return response.data;
  }

  async update(id: string, data: UpdateDocumentDTO) {
    const response = await api.put<{ data: DocumentDTO }>(`${this.baseUrl}/${id}`, data);
    return response.data;
  }

  async delete(id: string) {
    const response = await api.delete(`${this.baseUrl}/${id}`);
    return response.data;
  }

  async upload(
    file: File,
    metadata?: {
      name?: string;
      category?: DocumentCategory;
      contract_id?: string;
      entity_id?: string;
      entity_type?: EntityType;
      analysis_party_id?: string;
      analysis_criterion?: AnalysisCriterion;
      source?: DocumentSource;
      notes?: string;
    },
  ): Promise<DocumentDTO> {
    const formData = new FormData();
    formData.append("file", file);
    if (metadata?.name) formData.append("name", metadata.name);
    if (metadata?.category) formData.append("category", metadata.category);
    if (metadata?.contract_id) formData.append("contract_id", metadata.contract_id);
    if (metadata?.entity_id) formData.append("entity_id", metadata.entity_id);
    if (metadata?.entity_type) formData.append("entity_type", metadata.entity_type);
    if (metadata?.analysis_party_id) formData.append("analysis_party_id", metadata.analysis_party_id);
    if (metadata?.analysis_criterion) formData.append("analysis_criterion", metadata.analysis_criterion);
    if (metadata?.source) formData.append("source", metadata.source);
    if (metadata?.notes) formData.append("notes", metadata.notes);

    const response = await api.post<{ data: DocumentDTO }>(`${this.baseUrl}/upload`, formData);
    return response.data?.data;
  }

  async uploadToExisting(id: string, file: File): Promise<DocumentDTO> {
    const formData = new FormData();
    formData.append("file", file);

    const response = await api.post<{ data: DocumentDTO }>(`${this.baseUrl}/${id}/upload`, formData);
    return response.data?.data;
  }

  getDownloadUrl(id: string): string {
    return `${this.baseUrl}/${id}/download`;
  }

  async getPresignedDownloadUrl(id: string): Promise<string> {
    const response = await api.get<{ url: string }>(`${this.baseUrl}/${id}/download-url`);
    return response.data?.url;
  }

  async download(id: string): Promise<void> {
    try {
      const url = await this.getPresignedDownloadUrl(id);
      if (url) {
        window.open(url, "_blank");
        return;
      }
    } catch (e) {
      console.warn("Failed to get presigned URL, falling back to blob download", e);
    }
    const response = await api.get<Blob>(`${this.baseUrl}/${id}/download`, {
      responseType: "blob",
    });
    const blobUrl = window.URL.createObjectURL(response.data);
    window.open(blobUrl, "_blank");
  }

  async createVersion(id: string, file: File): Promise<DocumentDTO> {
    const formData = new FormData();
    formData.append("file", file);

    const response = await api.post<{ data: DocumentDTO }>(`${this.baseUrl}/${id}/versions`, formData);
    return response.data?.data;
  }

  async search(query: string, limit = 50, offset = 0) {
    const response = await api.get<{ data: DocumentDTO[]; meta?: any }>(`${this.baseUrl}/search`, {
      params: { q: query, limit, offset },
    });
    return response.data;
  }

  /** Resultado da extração OCR+IA de um documento (polling pós-upload). */
  async getExtraction(id: string): Promise<DocumentExtractionResult> {
    const response = await api.get<{ data: DocumentExtractionResult }>(`${this.baseUrl}/${id}/extraction`);
    return response.data?.data;
  }

  /** Reenfileira o documento para nova extração OCR+IA. */
  async reExtract(id: string): Promise<DocumentExtractionResult> {
    const response = await api.post<{ data: DocumentExtractionResult }>(`${this.baseUrl}/${id}/extract`);
    return response.data?.data;
  }
}

export const documentService = new DocumentService();
