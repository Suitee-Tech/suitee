import api from "./api";

// NFS-e não é emitida nesta versão: não há integração com emissor municipal.
// O cliente de emissão (listNfse/issueNfse) e seus tipos foram removidos de
// propósito — reativar a área exige implementar e validar a integração antes,
// em vez de reexpor um registro interno que não é nota autorizada.
export type DimobStatus = "PENDING" | "GENERATING" | "READY" | "SUBMITTED" | "ERROR";
export type InformeStatus = "PENDING" | "GENERATING" | "READY" | "ERROR";
export type FiscalReportType = "DIMOB" | "NFSE" | "INFORME";

export interface DimobReportDTO {
  id: string;
  year: number;
  status: DimobStatus;
  total_rentals: number;
  total_sales: number;
  total_amount: number;
  /**
   * Dados da declarante (registro R01) capturados na geração. Sem o CNPJ o
   * download do arquivo é recusado pela validação do leiaute.
   */
  declarante_cnpj?: string;
  declarante_nome?: string;
  file_url?: string;
  generated_at?: string;
  submitted_at?: string;
  created_at: string;
  tenant_id: string;
}

export interface InformeDTO {
  id: string;
  year: number;
  owner_id: string;
  owner_name: string;
  total_gross: number;
  total_admin_fee: number;
  // T-05: intermediação imobiliária retida dos repasses do ano. 0 em
  // informes gerados antes de a linha existir.
  total_intermediation: number;
  total_net: number;
  /**
   * Separação por fluxo (T-06): locação é renda tributável do proprietário;
   * venda é repasse de capital, e a comissão de intermediação de venda é
   * receita da imobiliária retida do repasse.
   */
  rental_gross?: number;
  rental_admin_fee?: number;
  rental_intermediation?: number;
  rental_net?: number;
  rental_count?: number;
  sale_gross?: number;
  sale_intermediation?: number;
  sale_net?: number;
  sale_count?: number;
  status: InformeStatus;
  pdf_url?: string;
  generated_at?: string;
  created_at: string;
  tenant_id: string;
}

export interface FiscalDeadlineDTO {
  id: string;
  type: FiscalReportType;
  title: string;
  description: string;
  due_date: string;
  status: "UPCOMING" | "DUE_SOON" | "OVERDUE" | "COMPLETED";
}

export interface FiscalReportHistoryDTO {
  id: string;
  type: FiscalReportType;
  name: string;
  file_url?: string;
  generated_at: string;
  status: "READY" | "ERROR";
}

class FiscalService {
  private readonly baseUrl = "/fiscal";

  async getCalendar(year?: number) {
    const params: Record<string, string> = {};
    if (year) params.year = String(year);
    const response = await api.get<{ data: FiscalDeadlineDTO[] }>(
      `${this.baseUrl}/calendar`,
      { params },
    );
    return response.data;
  }

  async getDimobStatus(year: number) {
    const response = await api.get<{ data: DimobReportDTO }>(
      `${this.baseUrl}/dimob/${year}`,
    );
    return response.data;
  }

  /**
   * Gera a DIMOB do ano. O CNPJ e a razão social identificam a declarante
   * no registro R01 — sem eles a geração funciona, mas o download do TXT
   * responde 400 ("gere a declaração novamente informando CNPJ").
   */
  async generateDimob(year: number, companyCnpj?: string, companyName?: string) {
    const response = await api.post<{ data: DimobReportDTO }>(
      `${this.baseUrl}/dimob/generate`,
      { year, company_cnpj: companyCnpj ?? "", company_name: companyName ?? "" },
    );
    return response.data;
  }

  async listInformes(year?: number) {
    const params: Record<string, string> = {};
    if (year) params.year = String(year);
    const response = await api.get<{ data: InformeDTO[] }>(
      `${this.baseUrl}/informes`,
      { params },
    );
    return response.data;
  }

  async generateInformes(year: number) {
    const response = await api.post<{ data: InformeDTO[] }>(
      `${this.baseUrl}/informes/generate`,
      { year },
    );
    return response.data;
  }

  async getReportHistory(params?: { limit?: number; offset?: number; type?: string; year?: number }) {
    const query: Record<string, string> = {};
    if (params?.limit) query.limit = String(params.limit);
    if (params?.offset) query.offset = String(params.offset);
    if (params?.type) query.type = params.type;
    if (params?.year) query.year = String(params.year);
    const response = await api.get<{ data: FiscalReportHistoryDTO[]; meta: any }>(
      `${this.baseUrl}/reports`,
      { params: query },
    );
    return response.data;
  }

  /** Arquiva um relatório do histórico (soft delete — a linha fica para auditoria). */
  async archiveReport(id: string) {
    const response = await api.delete<{ success: boolean }>(`${this.baseUrl}/reports/${id}`);
    return response.data;
  }

  /**
   * Baixa o PDF do informe de rendimentos de um proprietário. O PDF é
   * renderizado sob demanda no backend e exige o Bearer (window.open não
   * envia o header).
   */
  async downloadInformePDF(id: string) {
    const response = await api.get<Blob>(`${this.baseUrl}/informes/${id}/pdf`, {
      responseType: "blob",
      transformResponse: [(data) => data],
    });
    return response;
  }

  async downloadReport(id: string) {
    const response = await api.get<Blob>(`${this.baseUrl}/reports/${id}/download`, {
      responseType: "blob",
    });
    return response.data;
  }

  /**
   * Baixa um arquivo fiscal pelo path relativo à API (/fiscal/...).
   * A requisição passa pelo cliente autenticado (Bearer) — um window.open
   * não envia o header e o gateway respondia 401.
   */
  async downloadFile(path: string) {
    const response = await api.get<Blob>(path, {
      responseType: "blob",
      transformResponse: [(data) => data],
    });
    return response;
  }
}

export const fiscalService = new FiscalService();
