import api from "./api";
import type { CreatePropertyDTO, PropertyDTO } from "./property.service";

export interface CaptacaoOwnerInput {
  owner_id: string;
  owner_type: "person" | "company";
  percentage?: number;
}

export interface CaptacaoCapturingBrokerInput {
  broker_id: string;
  creci?: string;
  role: "captador" | "co_captador" | "visitador" | "atendente" | "fechamento" | "imobiliaria";
  brokerage_type?: "captacao" | "atendimento" | "visita";
  commission_pct?: number;
}

export interface CaptacaoRequest {
  property: CreatePropertyDTO;
  owners: CaptacaoOwnerInput[];
  capturing_brokers: CaptacaoCapturingBrokerInput[];
}

class CaptacaoService {
  private readonly baseUrl = "/captacao";

  async criar(req: CaptacaoRequest, idempotencyKey?: string) {
    const headers: Record<string, string> = {};
    if (idempotencyKey) headers["X-Idempotency-Key"] = idempotencyKey;
    return api.post<{ data: PropertyDTO }>(this.baseUrl, req, { headers });
  }
}

export const captacaoService = new CaptacaoService();
