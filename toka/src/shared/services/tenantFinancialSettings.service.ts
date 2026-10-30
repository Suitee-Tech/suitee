import api from "./api";

export interface TenantFinancialSettings {
  tenant_id: string;
  default_late_fee_pct: number;
  default_monthly_interest_pct: number;
  default_due_day: number;
  default_commission_pct_rent: number;
  default_commission_pct_sale: number;
  default_currency: string;
  payout_proof_required: boolean;
  admin_fee_pct: number;
  /** Prazo padrão sugerido para contratos de locação (meses). */
  default_lease_term_months: number;
  /**
   * Alíquota padrão do INSS retido do corretor autônomo (0 a 100; 0 = não
   * retém). Tipicamente 20%.
   */
  default_inss_pct: number;
  /**
   * Alíquota padrão do ISS retido na fonte (0 a 100; 0 = não retém). Varia por
   * município — tipicamente entre 2% e 5%.
   */
  default_iss_pct: number;
  updated_by?: string;
  updated_at: string;
}

export type UpdateTenantFinancialSettingsInput = Partial<
  Omit<TenantFinancialSettings, "tenant_id" | "updated_by" | "updated_at">
>;

class TenantFinancialSettingsService {
  private readonly baseUrl = "/financial-settings";

  async get(): Promise<TenantFinancialSettings> {
    const res = await api.get<{ data: TenantFinancialSettings }>(this.baseUrl);
    return res.data?.data;
  }

  async update(input: UpdateTenantFinancialSettingsInput): Promise<TenantFinancialSettings> {
    const res = await api.put<{ data: TenantFinancialSettings }>(this.baseUrl, input);
    return res.data?.data;
  }
}

export const tenantFinancialSettingsService = new TenantFinancialSettingsService();
