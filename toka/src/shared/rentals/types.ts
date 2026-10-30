export type FirstChargeType = "same_month_pro_rata" | "next_month_accumulated";

export type ProRataPolicy = "legacy_30_360" | "actual_actual";

export interface BundledItem {
  key: string;
  description: string;
  amount_cents: number;
  enabled: boolean;
}

export interface RentAdjustment {
  event_id: string;
  effective_from: string;
  monthly_amount: number;
  occurred_at: string;
}

export interface RentalChargeConfig {
  id: string;
  contract_id: string;
  tenant_id: string;
  property_id: string;
  tenant_party_id: string;
  owner_id: string;
  start_date: string;
  end_date: string;
  first_charge_date: string;
  first_charge_type: FirstChargeType;
  pro_rata_policy?: ProRataPolicy;
  default_due_day: number;
  // monthly_amount is the recurring rent in integer cents (mirrors the Go
  // int64 field). Divide by 100 to render reais.
  monthly_amount: number;
  base_monthly_amount?: number;
  adjustments?: RentAdjustment[];
  first_charge_paid: boolean;
  monthly_product_revision: number;
  bundled_items?: BundledItem[];
  created_at: string;
  updated_at: string;
}

export interface CreateRentalChargeConfigDTO {
  contract_id: string;
  tenant_party_id: string;
  owner_id: string;
  property_id: string;
  start_date: string;
  end_date: string;
  first_charge_date: string;
  first_charge_type: FirstChargeType;
  default_due_day: number;
  monthly_amount: number;
}

export interface UpdateRentalChargeConfigDTO {
  first_charge_date: string;
  default_due_day: number;
}
